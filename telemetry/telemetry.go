// Package telemetry is ukubi's traces (mountain plan/15 §7): pushed over OTLP/HTTP when configured, nothing at all
// otherwise. A Tracing with no endpoint wraps nothing, so an app without one serves exactly what it served before.
// It came from ukubi-game's internal/telemetry by way of mountain/pkg; the one change since is the error handler below.
//
// Everything that decides a span is passed explicitly (the provider, the propagator, a no-op meter for otelhttp's own
// metrics, so no series is added anywhere), so a test can trace one router without touching another. The one OTel
// global set is the error handler, and only by New with an endpoint (standard §7): from then on every error the
// process reports through otel.Handle (a failed trace export, but also a metrics exporter's or reader's errors) is one
// slog line at WARN, msg "traces", with the error as err. A later SetErrorHandler replaces it for otel.Handle; a
// handler fetched before the first set keeps delegating to this first one.
// Without an endpoint nothing is set, and OTel's default prints those errors through the log package. Globals the
// libraries still read: otelhttp's defaults before its options override them, and OTel's internal logger, which
// prints its own config and limit warnings (a bad OTEL_* value, a span over its limits) as plain text on stderr.
//
// The SDK's own OTEL_* variables: overridden in code are the service name (OTEL_SERVICE_NAME), the sampler, the
// propagators and the endpoint (OTEL_EXPORTER_OTLP_ENDPOINT, _INSECURE); still honoured are OTEL_RESOURCE_ATTRIBUTES
// (merged into the resource), the exporter's headers, timeout, compression and certificates, OTEL_BSP_*, the span
// limits and OTEL_GO_X_*: the libraries' own reads, which each app's configuration docs name (the game's README,
// atlas's main.go doc comment).
//
// Spans: one server span per route, named by its pattern; a request no route takes has none. Every server is treated
// as public, so a traceparent from outside starts a new trace linked to it, never a child (a public endpoint): the
// choice is made before any gate runs, so it can only see what a caller can forge. Outbound requests carry no context
// yet; when propagation lands, a trust option comes with it, decided after the gate (a machine principal's call).
package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

// Tracing is an app's tracer, or none (the zero value: Handler wraps nothing).
type Tracing struct {
	tp       trace.TracerProvider
	flush    func(context.Context) error
	shutdown func(context.Context) error
}

// New is the tracing for endpoint, the OTLP/HTTP traces URL in full (its /v1/traces path included; "" = none).
func New(endpoint, service string) (*Tracing, error) {
	if endpoint == "" {
		return &Tracing{}, nil
	}
	exp, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithProxy(func(*http.Request) (*url.URL, error) { return nil, nil })) // in-cluster: no proxy
	if err != nil {
		return nil, err
	}
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { slog.Warn("traces", "err", err) }))
	return WithExporter(exp, service), nil
}

// WithExporter is a tracing over exp: New's, or a test's in-memory one.
func WithExporter(exp sdktrace.SpanExporter, service string) *Tracing {
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithResource(resource.NewSchemaless(semconv.ServiceName(service))))
	return &Tracing{tp: tp, flush: tp.ForceFlush, shutdown: tp.Shutdown}
}

// Handler is h with a server span per request named pattern, or h itself without tracing.
func (t *Tracing) Handler(pattern string, h http.Handler) http.Handler {
	if t == nil || t.tp == nil {
		return h
	}
	return otelhttp.NewHandler(h, pattern,
		otelhttp.WithTracerProvider(t.tp),
		otelhttp.WithPropagators(propagation.TraceContext{}),
		otelhttp.WithMeterProvider(noopmetric.NewMeterProvider()),
		otelhttp.WithPublicEndpointFn(func(*http.Request) bool { return true }),
		otelhttp.WithSpanNameFormatter(func(pattern string, _ *http.Request) string { return pattern }))
}

// Flush exports the spans buffered so far, or does nothing without tracing.
func (t *Tracing) Flush() error {
	if t == nil || t.flush == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return t.flush(ctx)
}

// Shutdown flushes the spans still buffered (a few seconds at most), or does nothing without tracing.
func (t *Tracing) Shutdown() error {
	if t == nil || t.shutdown == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return t.shutdown(ctx)
}

package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// TestServerOption: a gRPC server given ServerOption has one server span per call, named by its method, and a
// caller's traceparent is a link, never the parent. Without tracing it adds nothing and still builds a server.
func TestServerOption(t *testing.T) {
	_ = grpc.NewServer((&Tracing{}).ServerOption())
	exp := tracetest.NewInMemoryExporter()
	tr := WithExporter(exp, "core")
	defer func() { _ = tr.Shutdown() }()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(tr.ServerOption())
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	ctx := metadata.AppendToOutgoingContext(context.Background(), "traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	if _, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Flush(); err != nil {
		t.Fatal(err)
	}
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("%d spans", len(spans))
	}
	s := spans[0]
	if s.Name != "grpc.health.v1.Health/Check" || s.SpanKind != trace.SpanKindServer {
		t.Errorf("span %q kind %v", s.Name, s.SpanKind)
	}
	if s.Parent.IsValid() || s.SpanContext.TraceID().String() == "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("a public call became a child of the caller's trace: parent %v", s.Parent)
	}
	if len(s.Links) != 1 || s.Links[0].SpanContext.TraceID().String() != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("links: %+v", s.Links)
	}
}

// TestIDsOnly: WithExporter(nil) still wraps, so a request's context holds a valid trace id, and it has no span
// processor, so nothing is exported.
func TestIDsOnly(t *testing.T) {
	tr := WithExporter(nil, "core")
	defer func() { _ = tr.Shutdown() }()
	var sc trace.SpanContext
	h := tr.Handler("/x", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		sc = trace.SpanContextFromContext(r.Context())
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	if !sc.IsValid() {
		t.Error("no valid span context in the handler")
	}
	if n := len(tr.Interceptors()); n != 1 {
		t.Errorf("%d interceptors, want otelconnect's", n)
	}
	if err := tr.Flush(); err != nil {
		t.Errorf("flush: %v", err)
	}
}

// TestLoggerAt: a minimum level below INFO keeps DEBUG lines; nil is INFO.
func TestLoggerAt(t *testing.T) {
	var buf bytes.Buffer
	NewLoggerAt(&buf, slog.LevelDebug).Debug("kept")
	NewLoggerAt(&buf, nil).Debug("dropped")
	if out := buf.String(); !strings.Contains(out, `"msg":"kept"`) || strings.Contains(out, "dropped") {
		t.Errorf("lines: %s", out)
	}
}

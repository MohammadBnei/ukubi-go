package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/emptypb"
)

type plain struct{}

func (plain) ServeHTTP(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }

// TestNone: no endpoint, no tracing: Handler is the handler itself, untouched (nothing wraps it).
func TestNone(t *testing.T) {
	none, err := New("", "x")
	if err != nil {
		t.Fatal(err)
	}
	var nilT *Tracing
	for name, tr := range map[string]*Tracing{"New(\"\")": none, "nil": nilT, "zero": {}} {
		if h := tr.Handler("GET /x", plain{}); h != (plain{}) {
			t.Errorf("%s: the handler is wrapped (%T)", name, h)
		}
		if ics := tr.Interceptors(); ics != nil {
			t.Errorf("%s: %d interceptors", name, len(ics))
		}
		if err := tr.Shutdown(); err != nil {
			t.Errorf("%s: shutdown: %v", name, err)
		}
	}
	tr, err := New("http://127.0.0.1:1/v1/traces", "x")
	if err != nil || tr.tp == nil || len(tr.Interceptors()) != 1 {
		t.Errorf("an endpoint: %v", err) // the positive control: an endpoint makes a tracer (export happens later)
	}
	_ = tr.Shutdown()
	for _, bad := range []string{"http://[bad", "otel-collector:4318", "collector.svc/v1/traces"} {
		if _, err := New(bad, "x"); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

// TestSpans: with tracing, a request gets one server span named by its route's pattern, and the answer is the
// handler's; a traceparent from outside starts a new trace linked to it, not a child of it (the game is public).
func TestSpans(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tr := WithExporter(exp, "ukubi-game")
	h := tr.Handler("GET /api/x/{id}", plain{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/x/7", nil)
	r.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTeapot {
		t.Errorf("the answer: %d", w.Code)
	}
	if err := tr.Flush(); err != nil { // not Shutdown: it empties an in-memory exporter
		t.Fatal(err)
	}
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("%d spans", len(spans))
	}
	s := spans[0]
	if s.Name != "GET /api/x/{id}" || s.SpanKind != trace.SpanKindServer {
		t.Errorf("span %q kind %v", s.Name, s.SpanKind)
	}
	if s.Parent.IsValid() || s.SpanContext.TraceID().String() == "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("a public request became a child of the caller's trace: parent %v", s.Parent)
	}
	if len(s.Links) != 1 || s.Links[0].SpanContext.TraceID().String() != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("links: %+v", s.Links)
	}
	if v, ok := s.Resource.Set().Value("service.name"); !ok || v.AsString() != "ukubi-game" {
		t.Errorf("service.name: %v", v)
	}
}

// TestAPISpans: a Connect call through a handler given Interceptors gets one RPC server span named by its procedure,
// with the error's status; a caller's traceparent is linked, not trusted.
func TestAPISpans(t *testing.T) {
	const procedure = "/test.v1.TestService/Call"
	exp := tracetest.NewInMemoryExporter()
	tr := WithExporter(exp, "ukubi-bridge")
	h := connect.NewUnaryHandler(procedure, func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("no"))
	}, connect.WithInterceptors(tr.Interceptors()...))
	srv := httptest.NewServer(h)
	defer srv.Close()
	req := connect.NewRequest(&emptypb.Empty{})
	req.Header().Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	_, err := connect.NewClient[emptypb.Empty, emptypb.Empty](srv.Client(), srv.URL+procedure).CallUnary(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("the answer: %v", err)
	}
	if err := tr.Flush(); err != nil {
		t.Fatal(err)
	}
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("%d spans", len(spans))
	}
	s := spans[0]
	if s.Name != "test.v1.TestService/Call" || s.SpanKind != trace.SpanKindServer || s.Status.Code.String() != "Error" {
		t.Errorf("span %q kind %v status %v", s.Name, s.SpanKind, s.Status)
	}
	if s.Parent.IsValid() || s.SpanContext.TraceID().String() == "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("a public call became a child of the caller's trace: parent %v", s.Parent)
	}
	if len(s.Links) != 1 || s.Links[0].SpanContext.TraceID().String() != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("links: %+v", s.Links)
	}
	if v, ok := s.Resource.Set().Value("service.name"); !ok || v.AsString() != "ukubi-bridge" {
		t.Errorf("service.name: %v", v)
	}
}

// TestErrorHandler: with an endpoint, New sends every OTel error to slog at WARN, msg "traces"; without
// one it sets nothing, and OTel's default prints the error through the log package, which slog.SetDefault turns into
// an INFO line. Each case runs in its own process: the none case needs a process where no handler was ever set, and
// TestNone sets one already.
func TestErrorHandler(t *testing.T) {
	want := map[string]string{
		"endpoint": `level=WARN msg=traces err="export refused"`,
		"none":     `level=INFO msg="export refused"`,
	}
	if mode := os.Getenv("TELEMETRY_ERROR_HANDLER"); mode != "" { // the child
		if want[mode] == "" {
			t.Fatalf("unknown mode %q", mode)
		}
		var buf bytes.Buffer
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
		endpoint := ""
		if mode == "endpoint" {
			endpoint = "http://127.0.0.1:1/v1/traces" // never dialled: no span is started
		}
		tr, err := New(endpoint, "x")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tr.Shutdown() }()
		otel.Handle(errors.New("export refused"))
		if !strings.Contains(buf.String(), want[mode]) {
			t.Fatalf("%s: logged %q, want %s", mode, buf.String(), want[mode])
		}
		return
	}
	for mode := range want {
		cmd := exec.Command(os.Args[0], "-test.run=^TestErrorHandler$", "-test.count=1")
		cmd.Env = append(os.Environ(), "TELEMETRY_ERROR_HANDLER="+mode)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", mode, err, out)
		}
	}
}

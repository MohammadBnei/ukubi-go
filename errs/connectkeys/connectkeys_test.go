package connectkeys

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/MohammadBnei/ukubi-go/errs"
)

const keyChanged errs.Key = "agent.definition_changed"

type fd = descriptorpb.FieldDescriptorProto

func str(name string, num int32) *fd {
	return &fd{Name: proto.String(name), Number: proto.Int32(num), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
		Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String(name)}
}

// msg is a template detail full name pkg.ErrorKey with fields, built at run time: no generated code, no registry.
func msg(t testing.TB, pkg string, fields ...*fd) proto.Message {
	t.Helper()
	f, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: proto.String(strings.ReplaceAll(pkg, ".", "/") + "/errors.proto"), Package: proto.String(pkg), Syntax: proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("ErrorKey"), Field: fields}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return dynamicpb.NewMessage(f.Messages().Get(0))
}

// oneof is {string key = 1; oneof o { string source = 2; }}, or with synthetic, proto3 `optional string source = 2`.
func oneof(t testing.TB, synthetic bool) proto.Message {
	t.Helper()
	src := str("source", 2)
	src.OneofIndex, src.Proto3Optional = proto.Int32(0), proto.Bool(synthetic)
	name := "o"
	if synthetic {
		name = "_source"
	}
	f, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: proto.String("o/v1/errors.proto"), Package: proto.String("o.v1"), Syntax: proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("ErrorKey"), Field: []*fd{str("key", 1), src},
			OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: proto.String(name)}}}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return dynamicpb.NewMessage(f.Messages().Get(0))
}

// fleetLike is {string key = 1; string source = 2}: agentfleet.v1.ErrorKey's and bridge.v1.ErrorKey's shape.
func fleetLike(t testing.TB, pkg string) proto.Message {
	return msg(t, pkg, str("key", 1), str("source", 2))
}

func keys(t testing.TB) *Keys { return New(fleetLike(t, "test.v1")) }

// TestConstructors: the code and words are the caller's; the key rides as the detail, and Of reads it back.
func TestConstructors(t *testing.T) {
	k := keys(t)
	ce := k.Connect(connect.CodeFailedPrecondition, keyChanged, errors.New("DEFINITION_CHANGED: start a new thread"))
	if ce.Code() != connect.CodeFailedPrecondition || ce.Message() != "DEFINITION_CHANGED: start a new thread" || k.Of(ce) != keyChanged {
		t.Errorf("connect: %v %q", ce, k.Of(ce))
	}
	if len(ce.Details()) != 1 || ce.Details()[0].Type() != "test.v1.ErrorKey" {
		t.Errorf("connect details: %v", ce.Details())
	}
	ge := k.Status(codes.PermissionDenied, KeyInternal, "nope")
	if s, _ := status.FromError(ge); s.Code() != codes.PermissionDenied || s.Message() != "nope" || k.Of(ge) != KeyInternal {
		t.Errorf("grpc: %v %q", ge, k.Of(ge))
	}
	if k.Of(errors.New("plain")) != "" || k.Of(nil) != "" || k.Of(connect.NewError(connect.CodeInternal, errors.New("x"))) != "" ||
		k.Of(status.Error(codes.Internal, "x")) != "" {
		t.Error("an unkeyed error has a key") // the positive control for Of
	}
	// A foreign detail with the same bytes under another name is not a key: Of matches the template's full name.
	other := keys(t).Connect(connect.CodeAborted, keyChanged, errors.New("x"))
	foreign := New(fleetLike(t, "other.v1"))
	if foreign.Of(other) != "" || foreign.Of(k.Status(codes.Aborted, keyChanged, "x")) != "" {
		t.Error("a foreign detail read as a key")
	}
	sv := connect.NewError(connect.CodeAborted, errors.New("x"))
	d, _ := connect.NewErrorDetail(wrapperspb.String(string(keyChanged))) // field 1 is the same string
	sv.AddDetail(d)
	if k.Of(sv) != "" {
		t.Error("a StringValue detail read as a key")
	}
	defer func() {
		if recover() == nil {
			t.Error("Status with OK did not panic")
		}
	}()
	_ = k.Status(codes.OK, KeyInternal, "")
}

// sameFields: a and b hold the same field records, in any order. The test templates are dynamicpb, which marshals
// fields in map order; generated ErrorKeys marshal in field order (the proof's golden pins those bytes).
func sameFields(a, b []byte) bool {
	split := func(b []byte) []string {
		var out []string
		for len(b) > 0 {
			_, _, n := protowire.ConsumeField(b)
			if n < 0 {
				return nil
			}
			out, b = append(out, string(b[:n])), b[n:]
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(split(a), split(b))
}

// TestBytes: the detail is exactly field 1 = key, field 2 = source, the bytes fleet's and the bridge's ErrorKey send
// (and islands/src/errors.test.ts decodes by hand); with no source, field 1 alone.
func TestBytes(t *testing.T) {
	k := keys(t)
	enc := func(fields ...string) []byte {
		var b []byte
		for i, s := range fields {
			b = append(b, byte((i+1)<<3|2), byte(len(s)))
			b = append(b, s...)
		}
		return b
	}
	if got := k.Sourced(connect.CodeUnavailable, "store.unavailable", "store", nil).Details()[0].Bytes(); !sameFields(got, enc("store.unavailable", "store")) {
		t.Errorf("sourced: %x", got)
	}
	if got := k.Connect(connect.CodeUnknown, KeyInternal, nil).Details()[0].Bytes(); !sameFields(got, enc("server.internal")) {
		t.Errorf("connect: %x", got)
	}
	s, _ := status.FromError(k.Status(codes.Unknown, KeyInternal, "x"))
	if a := s.Proto().GetDetails(); len(a) != 1 || a[0].GetTypeUrl() != "type.googleapis.com/test.v1.ErrorKey" || !sameFields(a[0].GetValue(), enc("server.internal")) {
		t.Errorf("grpc: %v", a)
	}
}

// TestSourced: the source rides beside the key.
func TestSourced(t *testing.T) {
	k := keys(t)
	ce := k.Sourced(connect.CodeUnavailable, "store.unavailable", "store", errors.New("down"))
	m := fleetLike(t, "test.v1")
	if err := proto.Unmarshal(ce.Details()[0].Bytes(), m); err != nil {
		t.Fatal(err)
	}
	src := m.ProtoReflect().Get(m.ProtoReflect().Descriptor().Fields().ByName("source")).String()
	if k.Of(ce) != "store.unavailable" || src != "store" || ce.Code() != connect.CodeUnavailable {
		t.Errorf("sourced: %v key %q source %q", ce, k.Of(ce), src)
	}
}

func panics(f func()) (p bool) {
	defer func() { p = recover() != nil }()
	f()
	return false
}

// TestNew: a template needs `string key = 1`, and `string source = 2` if it has a source; other fields are allowed.
// Sourced refuses to drop a source a template cannot carry.
func TestNew(t *testing.T) {
	for _, c := range []struct {
		name  string
		tmpl  func() []proto.Message
		panic bool
	}{
		{"none", func() []proto.Message { return nil }, true},
		{"no key", func() []proto.Message { return []proto.Message{msg(t, "a.v1", str("code", 1))} }, true},
		{"key = 2", func() []proto.Message { return []proto.Message{msg(t, "a.v1", str("key", 2))} }, true},
		{"int key", func() []proto.Message {
			f := str("key", 1)
			f.Type = descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum()
			return []proto.Message{msg(t, "a.v1", f)}
		}, true},
		{"repeated key", func() []proto.Message {
			f := str("key", 1)
			f.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
			return []proto.Message{msg(t, "a.v1", f)}
		}, true},
		{"source = 3", func() []proto.Message { return []proto.Message{msg(t, "a.v1", str("key", 1), str("source", 3))} }, true},
		{"source in a oneof", func() []proto.Message { return []proto.Message{oneof(t, false)} }, true},
		{"optional source", func() []proto.Message { return []proto.Message{oneof(t, true)} }, false},
		{"second bad", func() []proto.Message { return []proto.Message{fleetLike(t, "a.v1"), msg(t, "b.v1", str("k", 1))} }, true},
		{"key only", func() []proto.Message { return []proto.Message{msg(t, "a.v1", str("key", 1))} }, false},
		{"extra field", func() []proto.Message {
			return []proto.Message{msg(t, "a.v1", str("key", 1), str("source", 2), str("hint", 3))}
		}, false},
		{"well-formed", func() []proto.Message { return []proto.Message{fleetLike(t, "a.v1")} }, false},
	} {
		if got := panics(func() { New(c.tmpl()...) }); got != c.panic {
			t.Errorf("%s: panic %v, want %v", c.name, got, c.panic)
		}
	}
	keyOnly := New(msg(t, "a.v1", str("key", 1)))
	if !panics(func() { keyOnly.Sourced(connect.CodeUnavailable, "store.unavailable", "store", nil) }) {
		t.Error("Sourced dropped a source")
	}
	if panics(func() { keyOnly.Connect(connect.CodeUnavailable, "store.unavailable", nil) }) {
		t.Error("Connect on a key-only template panicked")
	}
}

// TestTwoDetails: with two templates every error carries both details, in order, and either template alone reads it.
func TestTwoDetails(t *testing.T) {
	old, shared := fleetLike(t, "old.v1"), fleetLike(t, "ukubi.errors.v1")
	both := New(old, shared)
	for name, err := range map[string]error{
		"connect":  both.Connect(connect.CodeAborted, keyChanged, nil),
		"fallback": both.withConnectKey(errors.New("raw")),
		"grpc":     both.Status(codes.Aborted, keyChanged, "x"),
		"gfall":    both.withStatusKey(errors.New("raw")),
		// An error keyed by the old template alone, or the new one's detail with an empty key, gains (or has replaced)
		// the new one, keeping the key and source.
		"old only": both.withConnectKey(New(old).Sourced(connect.CodeAborted, keyChanged, "store", nil)),
		"g old":    both.withStatusKey(New(old).Status(codes.Aborted, keyChanged, "x")),
		"empty":    both.withConnectKey(both.withConnectKey(New(shared, old).Connect(connect.CodeAborted, "", nil))),
	} {
		var names []string
		var ce *connect.Error
		if errors.As(err, &ce) {
			for _, d := range ce.Details() {
				names = append(names, d.Type())
			}
		} else {
			s, _ := status.FromError(err)
			for _, a := range s.Proto().GetDetails() {
				names = append(names, a.GetTypeUrl()[len("type.googleapis.com/"):])
			}
		}
		if key, src, _ := New(shared).keyed(err); name == "old only" && (key != keyChanged || src != "store") {
			t.Errorf("old only: key %q source %q", key, src)
		}
		if strings.Join(names, ",") != "old.v1.ErrorKey,ukubi.errors.v1.ErrorKey" {
			t.Errorf("%s: details %v", name, names)
		}
		if New(old).Of(err) == "" || New(shared).Of(err) == "" || New(old).Of(err) != New(shared).Of(err) {
			t.Errorf("%s: old %q shared %q", name, New(old).Of(err), New(shared).Of(err))
		}
	}
}

// TestConnectFallback: a raw error and an unkeyed Connect error leave keyed internal with their code and words; a
// keyed one keeps its key (one detail); nil stays nil.
func TestConnectFallback(t *testing.T) {
	k := keys(t)
	raw := k.withConnectKey(errors.New("send: broken pipe"))
	if k.Of(raw) != KeyInternal || connect.CodeOf(raw) != connect.CodeUnknown || !strings.Contains(raw.Error(), "send: broken pipe") {
		t.Errorf("raw: %v", raw)
	}
	bare := k.withConnectKey(connect.NewError(connect.CodeNotFound, errors.New("gone")))
	if k.Of(bare) != KeyInternal || connect.CodeOf(bare) != connect.CodeNotFound {
		t.Errorf("bare: %v", bare)
	}
	own := k.withConnectKey(k.Connect(connect.CodeAborted, keyChanged, errors.New("x")))
	var ce *connect.Error
	if k.Of(own) != keyChanged || !errors.As(own, &ce) || len(ce.Details()) != 1 {
		t.Errorf("own: %v", own)
	}
	if k.withConnectKey(nil) != nil {
		t.Error("nil became an error")
	}
	unary := keyAll{k}.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) { return nil, errors.New("u") })
	if _, err := unary(context.Background(), nil); k.Of(err) != KeyInternal {
		t.Errorf("unary: %v", err)
	}
	stream := keyAll{k}.WrapStreamingHandler(func(context.Context, connect.StreamingHandlerConn) error { return errors.New("s") })
	if err := stream(context.Background(), nil); k.Of(err) != KeyInternal {
		t.Errorf("stream: %v", err)
	}
}

// TestGRPCFallback: the same over gRPC, unary and streaming.
func TestGRPCFallback(t *testing.T) {
	k := keys(t)
	unary := k.UnaryServerInterceptor()
	if _, err := unary(context.Background(), nil, nil, func(context.Context, any) (any, error) { return nil, status.Error(codes.NotFound, "gone") }); k.Of(err) != KeyInternal || status.Code(err) != codes.NotFound {
		t.Errorf("unary: %v", err)
	}
	stream := k.StreamServerInterceptor()
	if err := stream(nil, nil, nil, func(any, grpc.ServerStream) error { return errors.New("raw") }); k.Of(err) != KeyInternal || status.Code(err) != codes.Unknown {
		t.Errorf("stream: %v", err)
	}
	if _, err := unary(context.Background(), nil, nil, func(context.Context, any) (any, error) { return nil, k.Status(codes.Aborted, keyChanged, "x") }); k.Of(err) != keyChanged {
		t.Errorf("own: %v", err)
	}
	if _, err := unary(context.Background(), nil, nil, func(context.Context, any) (any, error) { return 1, nil }); err != nil {
		t.Errorf("nil: %v", err)
	}
}

// serve mounts a unary and a server-streaming procedure that both answer err, through opt.
func serve(t *testing.T, opt connect.HandlerOption, err error) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("/test.v1.S/Unary", connect.NewUnaryHandler("/test.v1.S/Unary",
		func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
			return nil, err
		}, opt))
	mux.Handle("/test.v1.S/Stream", connect.NewServerStreamHandler("/test.v1.S/Stream",
		func(context.Context, *connect.Request[emptypb.Empty], *connect.ServerStream[emptypb.Empty]) error {
			return err
		}, opt))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// call is a real Connect client's error from both procedures.
func call(t *testing.T, srv *httptest.Server) (unary, stream error) {
	_, unary = connect.NewClient[emptypb.Empty, emptypb.Empty](srv.Client(), srv.URL+"/test.v1.S/Unary").
		CallUnary(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	s, err := connect.NewClient[emptypb.Empty, emptypb.Empty](srv.Client(), srv.URL+"/test.v1.S/Stream").
		CallServerStream(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	if err != nil {
		t.Fatal(err)
	}
	for s.Receive() {
	}
	return unary, s.Err()
}

// TestWire: over a real Connect client and a real gRPC client, a handler's unkeyed error arrives keyed internal, with
// its code.
func TestWire(t *testing.T) {
	k := keys(t)
	u, s := call(t, serve(t, k.WithInterceptors(), connect.NewError(connect.CodeUnimplemented, errors.New("not here"))))
	for name, err := range map[string]error{"unary": u, "stream": s} {
		if connect.CodeOf(err) != connect.CodeUnimplemented || k.Of(err) != KeyInternal {
			t.Errorf("connect %s: %v key %q", name, err, k.Of(err))
		}
	}
	u, _ = call(t, serve(t, k.WithInterceptors(), k.Connect(connect.CodeAborted, keyChanged, errors.New("x"))))
	var ce *connect.Error
	if k.Of(u) != keyChanged || !errors.As(u, &ce) || len(ce.Details()) != 1 {
		t.Errorf("connect keyed: %v", u)
	}

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer(grpc.ChainUnaryInterceptor(k.UnaryServerInterceptor()), grpc.ChainStreamInterceptor(k.StreamServerInterceptor()))
	grpc_health_v1.RegisterHealthServer(gs, grpc_health_v1.UnimplementedHealthServer{})
	go func() { _ = gs.Serve(lis) }()
	defer gs.Stop()
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	hc := grpc_health_v1.NewHealthClient(conn)
	_, err = hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if status.Code(err) != codes.Unimplemented || k.Of(err) != KeyInternal {
		t.Errorf("grpc unary: %v key %q", err, k.Of(err))
	}
	ws, err := hc.Watch(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err == nil {
		_, err = ws.Recv()
	}
	if status.Code(err) != codes.Unimplemented || k.Of(err) != KeyInternal {
		t.Errorf("grpc stream: %v key %q", err, k.Of(err))
	}
}

// TestContextCodes: a plain context error keeps the code each protocol sends for it unkeyed (Connect's toWire, gRPC's
// FromContextError), wrapped or not; any other plain error is Unknown.
func TestContextCodes(t *testing.T) {
	k := keys(t)
	for _, c := range []struct {
		err  error
		cc   connect.Code
		gc   codes.Code
		name string
	}{
		{context.Canceled, connect.CodeCanceled, codes.Canceled, "canceled"},
		{context.DeadlineExceeded, connect.CodeDeadlineExceeded, codes.DeadlineExceeded, "deadline"},
		{fmt.Errorf("query: %w", context.DeadlineExceeded), connect.CodeDeadlineExceeded, codes.DeadlineExceeded, "wrapped deadline"},
		{os.ErrDeadlineExceeded, connect.CodeDeadlineExceeded, codes.Unknown, "os deadline"},
		{errors.New("broken pipe"), connect.CodeUnknown, codes.Unknown, "plain"},
	} {
		if got := k.withConnectKey(c.err); connect.CodeOf(got) != c.cc || k.Of(got) != KeyInternal || !strings.Contains(got.Error(), c.err.Error()) {
			t.Errorf("%s: Connect %v", c.name, got)
		}
		if got := k.withStatusKey(c.err); status.Code(got) != c.gc || k.Of(got) != KeyInternal {
			t.Errorf("%s: gRPC %v", c.name, got)
		}
	}
}

// TestSentinelUnchanged: keying never changes the handler's error, which may be a shared sentinel: concurrent requests
// do not race on it (run with -race), and it stays unkeyed with what it had, while the copy carries that, then the key.
// Both forms run: one with meta and a detail, and one with neither, whose Meta() would create the map on first read.
func TestSentinelUnchanged(t *testing.T) {
	k := keys(t)
	why, err := connect.NewErrorDetail(wrapperspb.String("gone"))
	if err != nil {
		t.Fatal(err)
	}
	full := connect.NewError(connect.CodeNotFound, errors.New("no such thread"))
	full.Meta().Set("X-Why", "gone")
	full.AddDetail(why)
	for _, c := range []struct {
		name     string
		sentinel *connect.Error
		details  int // the sentinel's own
		meta     string
	}{
		{"with meta and detail", full, 1, "gone"},
		{"bare", connect.NewError(connect.CodeNotFound, errors.New("no such thread")), 0, ""},
	} {
		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got := k.withConnectKey(c.sentinel)
				var ce *connect.Error
				if k.Of(got) != KeyInternal || !errors.As(got, &ce) || ce.Code() != connect.CodeNotFound || ce.Message() != "no such thread" ||
					ce.Meta().Get("X-Why") != c.meta || len(ce.Details()) != c.details+1 ||
					(c.details > 0 && ce.Details()[0].Type() != why.Type()) {
					t.Errorf("%s: keyed: %v", c.name, got)
				}
			}()
		}
		wg.Wait()
		if k.Of(c.sentinel) != "" || len(c.sentinel.Details()) != c.details || c.sentinel.Meta().Get("X-Why") != c.meta {
			t.Errorf("%s: the sentinel was changed: %d details", c.name, len(c.sentinel.Details()))
		}
	}
}

// refuse is an interceptor that answers every call with a plain error, the way a gate interceptor (CSRF, auth) does.
type refuse struct{}

func (refuse) WrapUnary(connect.UnaryFunc) connect.UnaryFunc {
	return func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, errors.New("refused")
	}
}
func (refuse) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}
func (refuse) WrapStreamingHandler(connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(context.Context, connect.StreamingHandlerConn) error { return errors.New("refused") }
}

// TestOutermost: WithInterceptors puts the fallback outside the handler's own interceptors, so an error one of them
// returns, before the handler runs, leaves keyed too.
func TestOutermost(t *testing.T) {
	k := keys(t)
	u, s := call(t, serve(t, k.WithInterceptors(refuse{}), nil))
	for name, err := range map[string]error{"unary": u, "stream": s} {
		if k.Of(err) != KeyInternal || connect.CodeOf(err) != connect.CodeUnknown || !strings.Contains(err.Error(), "refused") {
			t.Errorf("%s: an interceptor's refusal: %v key %q", name, err, k.Of(err))
		}
	}
}

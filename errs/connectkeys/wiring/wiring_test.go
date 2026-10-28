package wiring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const wrapper = "example.com/app/internal/errkeys"

const goodWrapper = `package errkeys

import (
	"connectrpc.com/connect"
	"google.golang.org/grpc"

	appv1 "example.com/app/proto/gen/go/app/v1"
	"github.com/MohammadBnei/ukubi-go/errs/connectkeys"
)

var keys = connectkeys.New(&appv1.ErrorKey{})

func WithInterceptors(ics ...connect.Interceptor) connect.HandlerOption { return keys.WithInterceptors(ics...) }

func UnaryServerInterceptor() grpc.UnaryServerInterceptor { return keys.UnaryServerInterceptor() }

func StreamServerInterceptor() grpc.StreamServerInterceptor { return keys.StreamServerInterceptor() }
`

const (
	goodHandler = `appv1connect.NewAppServiceHandler(nil, errkeys.WithInterceptors(), connect.WithReadMaxBytes(1<<20))`
	goodServer  = `grpc.NewServer(grpc.ChainUnaryInterceptor(errkeys.UnaryServerInterceptor()), grpc.ChainStreamInterceptor(errkeys.StreamServerInterceptor()))`
)

func server(handler, srv string, extra ...string) string {
	return `package main

import (
	"connectrpc.com/connect"
	"google.golang.org/grpc"

	"example.com/app/internal/errkeys"
	"example.com/app/proto/gen/go/app/v1/appv1connect"
)

var _ = connect.WithReadMaxBytes

func main() {
	_, _ = ` + handler + `
	_ = ` + srv + `
	` + strings.Join(extra, "\n\t") + `
}
`
}

// tree writes the good app, then files over it.
func tree(t *testing.T, files map[string]string) string {
	root := t.TempDir()
	all := map[string]string{
		"go.mod":                      "module example.com/app\n\ngo 1.27\n",
		"internal/errkeys/errkeys.go": goodWrapper,
		"cmd/app/main.go":             server(goodHandler, goodServer),
	}
	for p, s := range files {
		all[p] = s
	}
	for p, s := range all {
		if s == "" {
			continue // "" deletes
		}
		p = filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestCheck: the good tree passes and the check sees its handler and server (the positive control); each banned form
// yields exactly its one finding.
func TestCheck(t *testing.T) {
	main := func(h, s string, extra ...string) map[string]string {
		return map[string]string{"cmd/app/main.go": server(h, s, extra...)}
	}
	bad := server("appv1connect.NewAppServiceHandler(nil)", "grpc.NewServer()", "_ = grpc.UnaryInterceptor(nil)")
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string // the one finding; "" for none
	}{
		{"good", nil, ""},
		{"connect.WithInterceptors", main(`appv1connect.NewAppServiceHandler(nil, errkeys.WithInterceptors(), connect.WithInterceptors())`, goodServer), "connect.WithInterceptors skips the wrapper's fallback"},
		{"WithHandlerOptions", main(`appv1connect.NewAppServiceHandler(nil, errkeys.WithInterceptors(), connect.WithHandlerOptions())`, goodServer), "connect.WithHandlerOptions skips"},
		{"WithOptions", main(`appv1connect.NewAppServiceHandler(nil, errkeys.WithInterceptors(), connect.WithOptions())`, goodServer), "connect.WithOptions skips"},
		{"WithRequestGate", main(`appv1connect.NewAppServiceHandler(nil, errkeys.WithInterceptors(), connect.WithRequestGate(nil))`, goodServer), "connect.WithRequestGate skips"},
		{"spread", main(`appv1connect.NewAppServiceHandler(nil, opts...)`, goodServer), "options spread"},
		{"no wrapper", main(`appv1connect.NewAppServiceHandler(nil, connect.WithReadMaxBytes(1))`, goodServer), "with 0 wrapper WithInterceptors"},
		{"two wrappers", main(`appv1connect.NewAppServiceHandler(nil, errkeys.WithInterceptors(), errkeys.WithInterceptors())`, goodServer), "with 2 wrapper WithInterceptors"},
		{"wrapper not first", main(`appv1connect.NewAppServiceHandler(nil, connect.WithRecover(nil), errkeys.WithInterceptors())`, goodServer), "not the first option"},
		{"opaque option", main(`appv1connect.NewAppServiceHandler(nil, errkeys.WithInterceptors(), someOpt)`, goodServer), "option 2 is not a direct call"},
		{"other package option", main(`appv1connect.NewAppServiceHandler(nil, errkeys.WithInterceptors(), otel.Option())`, goodServer), "option 2 is not a direct call"},
		{"server without chains", main(goodHandler, `grpc.NewServer()`), "grpc.NewServer does not open"},
		{"empty chain", main(goodHandler, `grpc.NewServer(grpc.ChainUnaryInterceptor(), grpc.ChainStreamInterceptor(errkeys.StreamServerInterceptor()))`), "grpc.NewServer does not open"},
		{"chains swapped", main(goodHandler, `grpc.NewServer(grpc.ChainUnaryInterceptor(errkeys.StreamServerInterceptor()), grpc.ChainStreamInterceptor(errkeys.StreamServerInterceptor()))`), "grpc.NewServer does not open"},
		{"spread server", main(goodHandler, `grpc.NewServer(opts...)`), "grpc.NewServer does not open"},
		{"client interceptor", main(goodHandler, goodServer, `_ = grpc.WithChainUnaryInterceptor(nil)`, `_ = grpc.WithUnaryInterceptor(nil)`, `_ = grpc.WithStreamInterceptor(nil)`), ""},
		{"stray chain", main(goodHandler, goodServer, `_ = grpc.ChainStreamInterceptor(nil)`), "grpc.ChainStreamInterceptor outside the server's opening chains"},
		{"stray interceptor", main(goodHandler, goodServer, `_ = grpc.UnaryInterceptor(nil)`), "grpc.UnaryInterceptor outside the server's opening chains"},
		{"wrapper forwards to connect", map[string]string{"internal/errkeys/errkeys.go": strings.Replace(goodWrapper,
			"return keys.WithInterceptors(ics...)", "return connect.WithInterceptors(ics...)", 1)}, "wrapper WithInterceptors does not return"},
		{"wrapper drops ics", map[string]string{"internal/errkeys/errkeys.go": strings.Replace(goodWrapper,
			"return keys.WithInterceptors(ics...)", "return keys.WithInterceptors()", 1)}, "wrapper WithInterceptors does not return"},
		{"wrapper unnamed ics", map[string]string{"internal/errkeys/errkeys.go": strings.Replace(goodWrapper,
			"(ics ...connect.Interceptor) connect.HandlerOption { return keys.WithInterceptors(ics...)", "(...connect.Interceptor) connect.HandlerOption { return keys.WithInterceptors()", 1)}, "wrapper WithInterceptors does not return"},
		{"generated hand-built handler", map[string]string{"proto/gen/go/app/v1/appv1connect/app.connect.go": "// Code generated by protoc-gen-connect-go. DO NOT EDIT.\n\npackage appv1connect\n\nimport \"connectrpc.com/connect\"\n\nfunc NewAppServiceHandler(svc any, opts ...connect.HandlerOption) any {\n\treturn connect.NewUnaryHandler(\"/app.v1.S/Do\", nil, connect.WithHandlerOptions(opts...))\n}\n"}, ""},
		{"nested module", map[string]string{"tools/go.mod": "module example.com/tools\n", "tools/main.go": bad}, ""},
		{"wrapper's own package", map[string]string{"cmd/app/main.go": "", "internal/errkeys/serve.go": strings.NewReplacer("package main", "package errkeys",
			"\t\"example.com/app/internal/errkeys\"\n", "", "errkeys.", "").Replace(server(goodHandler, goodServer))}, ""},
		{"wrapper without connectkeys", map[string]string{"internal/errkeys/errkeys.go": strings.Replace(goodWrapper,
			"\t\"github.com/MohammadBnei/ukubi-go/errs/connectkeys\"\n", "", 1)}, "wrapper WithInterceptors does not return"}, // first of three
		{"wrapper missing a function", map[string]string{"internal/errkeys/errkeys.go": goodWrapper[:strings.Index(goodWrapper, "\nfunc StreamServerInterceptor")]}, "wrapper has no StreamServerInterceptor"},
		{"skipped dirs and tests", map[string]string{"testdata/x/bad.go": bad, "vendor/x/bad.go": bad, "_old/bad.go": bad, ".git/bad.go": bad, "cmd/app/main_test.go": bad}, ""},
	} {
		findings, handlers, servers, err := Check(tree(t, c.files), wrapper)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		switch {
		case c.name == "wrapper without connectkeys":
			if len(findings) != 3 || !strings.Contains(findings[0], c.want) {
				t.Errorf("%s: %q", c.name, findings)
			}
		case c.want == "" && len(findings) != 0, c.want != "" && (len(findings) != 1 || !strings.Contains(findings[0], c.want)):
			t.Errorf("%s: %q, want one finding %q", c.name, findings, c.want)
		}
		if handlers != 1 || servers != 1 {
			t.Errorf("%s: %d handlers, %d servers: the check no longer sees them", c.name, handlers, servers)
		}
	}
	// Hand-built handlers are checked too, generic or not.
	findings, handlers, _, err := Check(tree(t, map[string]string{"cmd/app/main.go": server(goodHandler, goodServer,
		`_ = connect.NewUnaryHandler("/s/Do", nil)`, `_ = connect.NewBidiStreamHandler[int, int]("/s/Do", nil, errkeys.WithInterceptors())`)}), wrapper)
	if err != nil || handlers != 3 || len(findings) != 1 || !strings.Contains(findings[0], "NewUnaryHandler with 0 wrapper") {
		t.Errorf("hand-built: %v %d %q", err, handlers, findings)
	}
	// An unaliased import is named by the wrapper's package clause, not its directory.
	renamed := tree(t, map[string]string{"internal/errkeys/errkeys.go": "", "internal/err-keys/errkeys.go": goodWrapper,
		"cmd/app/main.go": strings.Replace(server(goodHandler, goodServer), "/internal/errkeys", "/internal/err-keys", 1)})
	if findings, h, s, err := Check(renamed, "example.com/app/internal/err-keys"); err != nil || len(findings) != 0 || h != 1 || s != 1 {
		t.Errorf("renamed wrapper: %v %q %d %d", err, findings, h, s)
	}
	for p, want := range map[string]string{"connectrpc.com/connect/v2": "connect", "gopkg.in/yaml.v3": "yaml", "github.com/x/go-redis": "redis", "v2": "v2"} {
		if got := assumedName(p); got != want {
			t.Errorf("assumedName(%s) = %s, want %s", p, got, want)
		}
	}
	// Outside the module, though trimming the module path would name internal/errkeys.
	if _, _, _, err := Check(tree(t, nil), "example.com/appinternal/errkeys"); err == nil {
		t.Error("a wrapper outside the module was accepted")
	}
}

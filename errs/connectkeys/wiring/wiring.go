// Package wiring checks that an app installs the connectkeys fallback outermost on every Connect handler and gRPC
// server it builds (standard §6). An app calls Check from a test; servers never import this package, so they link
// nothing from go/*.
//
// It is syntactic, over the module's non-test files: it trusts names, not types (a typed check belongs in
// ukubicheck). It skips testdata, vendor, directories starting with "." or "_" and nested modules, as the go tool
// does. An unaliased import's name is the wrapper's own package name, else guessed from its path as goimports does.
//
// It finds generated Connect handlers by layout: a package whose import path contains /proto/gen/ and ends in
// "connect", as buf generates them. An app that generates elsewhere sees no handlers, so the check would pass
// vacuously: a caller asserts handlers and servers are above zero.
package wiring

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

const (
	connectPkg = "connectrpc.com/connect"
	grpcPkg    = "google.golang.org/grpc"
	keysPkg    = "github.com/MohammadBnei/ukubi-go/errs/connectkeys"
)

// banned are connect's options that carry interceptors of their own, options the check cannot see into, and
// WithRequestGate, whose gates run before the interceptors, so their errors skip the fallback.
//
//ukubicheck:table connect's options the wrapper check refuses
var banned = map[string]bool{"WithInterceptors": true, "WithHandlerOptions": true, "WithConditionalHandlerOptions": true, "WithOptions": true, "WithRequestGate": true}

// serverInterceptors are grpc's server options that install interceptors. grpc.With…Interceptor are dial options,
// for clients, and are not checked.
//
//ukubicheck:table grpc's server options that install interceptors
var serverInterceptors = map[string]bool{"UnaryInterceptor": true, "StreamInterceptor": true, "ChainUnaryInterceptor": true, "ChainStreamInterceptor": true}

// Check walks the Go module at root. wrapper is the import path of the app's package that holds its *connectkeys.Keys
// and exports WithInterceptors, UnaryServerInterceptor and StreamServerInterceptor, each returning that Keys' own.
//
// Every generated Connect handler (a New…Handler from a /proto/gen/ …connect package), and every one built by hand
// outside generated files (connect.New…Handler), takes wrapper.WithInterceptors as its first option, exactly once, and otherwise only direct calls to connect's own With* options, none of which
// carries interceptors (no connect.WithInterceptors, WithHandlerOptions, WithOptions, WithRequestGate, no spread). Every grpc.NewServer
// opens with grpc.ChainUnaryInterceptor(wrapper.UnaryServerInterceptor(), …) and
// grpc.ChainStreamInterceptor(wrapper.StreamServerInterceptor(), …), and no other grpc server interceptor option
// (UnaryInterceptor, StreamInterceptor, Chain…) appears; client dial options (grpc.With…Interceptor) are allowed.
// handlers and servers count what it saw: a caller asserts they are above zero.
func Check(root, wrapper string) (findings []string, handlers, servers int, err error) {
	mod, err := module(root)
	if err != nil {
		return nil, 0, 0, err
	}
	if wrapper != mod && !strings.HasPrefix(wrapper, mod+"/") {
		return nil, 0, 0, fmt.Errorf("wiring: wrapper %s is not in module %s", wrapper, mod)
	}
	add := func(p, f string, a ...any) {
		rel, _ := filepath.Rel(root, p)
		findings = append(findings, rel+": "+fmt.Sprintf(f, a...))
	}
	wdir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(wrapper, mod), "/")))
	wname, err := checkWrapper(wdir, add)
	if err != nil {
		return nil, 0, 0, err
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); p != root && (n == "testdata" || n == "vendor" || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_")) {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "go.mod")); p != root && err == nil {
				return filepath.SkipDir // another module
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.SkipObjectResolution|parser.ParseComments) // comments: ast.IsGenerated reads them
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		pkg := path.Join(mod, filepath.ToSlash(rel))
		h, s := checkFile(file, pkg, wrapper, wname, func(f string, a ...any) { add(p, f, a...) })
		handlers, servers = handlers+h, servers+s
		return nil
	})
	return findings, handlers, servers, err
}

func module(root string) (string, error) {
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(m), `"`), nil
		}
	}
	return "", fmt.Errorf("wiring: no module line in %s/go.mod", root)
}

// imports maps each file-level name to its import path: wrapper's is wname.
func imports(file *ast.File, wrapper, wname string) map[string]string {
	imp := map[string]string{}
	for _, im := range file.Imports {
		v, _ := strconv.Unquote(im.Path.Value)
		n := assumedName(v)
		switch {
		case im.Name != nil:
			n = im.Name.Name
		case v == wrapper && wname != "":
			n = wname
		}
		imp[n] = v
	}
	return imp
}

// assumedName is goimports' guess at an import's package name: the last element, the one before a /vN, without a
// go- prefix, cut at the first character an identifier cannot hold.
func assumedName(p string) string {
	base := path.Base(p)
	if v, ok := strings.CutPrefix(base, "v"); ok && path.Dir(p) != "." {
		if _, err := strconv.Atoi(v); err == nil {
			base = path.Base(path.Dir(p))
		}
	}
	base = strings.TrimPrefix(base, "go-")
	if i := strings.IndexFunc(base, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' }); i >= 0 {
		base = base[:i]
	}
	return base
}

// checkWrapper: the wrapper's three functions each return the same-named method of a package-level value (its Keys),
// passing their own parameters on, in a file that imports connectkeys. A wrapper that forwards to
// connect.WithInterceptors installs no fallback; one that drops ics drops the app's interceptors. name is the
// wrapper's package name.
func checkWrapper(dir string, add func(p, f string, a ...any)) (name string, _ error) {
	want := map[string]bool{"WithInterceptors": false, "UnaryServerInterceptor": false, "StreamServerInterceptor": false}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("wiring: wrapper: %w", err)
	}
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		p := filepath.Join(dir, n)
		file, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.SkipObjectResolution)
		if err != nil {
			return "", err
		}
		name = file.Name.Name
		imp := imports(file, "", "")
		hasKeys := false
		for _, v := range imp {
			hasKeys = hasKeys || v == keysPkg
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil {
				continue
			}
			if _, ok := want[fn.Name.Name]; !ok {
				continue
			}
			ok = hasKeys && returnsSame(fn, imp)
			if ok {
				want[fn.Name.Name] = true
			} else {
				add(p, "wrapper %s does not return its connectkeys.Keys' %s", fn.Name.Name, fn.Name.Name)
				delete(want, fn.Name.Name) // reported once
			}
		}
	}
	for _, n := range []string{"WithInterceptors", "UnaryServerInterceptor", "StreamServerInterceptor"} {
		if ok, seen := want[n]; seen && !ok {
			add(dir, "wrapper has no %s", n)
		}
	}
	return name, nil
}

// returnsSame: fn's body is `return x.<fn's name>(<fn's parameters>)`, x a file-level identifier that is not an
// import, a variadic parameter passed on with "...".
func returnsSame(fn *ast.FuncDecl, imp map[string]string) bool {
	if len(fn.Body.List) != 1 {
		return false
	}
	r, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(r.Results) != 1 {
		return false
	}
	c, ok := r.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	s, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := s.X.(*ast.Ident)
	if _, isImport := imp[x.String()]; !ok || isImport || s.Sel.Name != fn.Name.Name {
		return false
	}
	var params []string
	variadic := false
	for _, f := range fn.Type.Params.List {
		_, variadic = f.Type.(*ast.Ellipsis)
		if len(f.Names) == 0 {
			return false // an unnamed parameter cannot be passed on
		}
		for _, n := range f.Names {
			params = append(params, n.Name)
		}
	}
	if len(c.Args) != len(params) || c.Ellipsis.IsValid() != variadic {
		return false
	}
	for i, a := range c.Args {
		if id, ok := a.(*ast.Ident); !ok || id.Name != params[i] || id.Name == "_" {
			return false
		}
	}
	return true
}

// checkFile checks file, of package pkg.
func checkFile(file *ast.File, pkg, wrapper, wname string, add func(f string, a ...any)) (handlers, servers int) {
	imp := imports(file, wrapper, wname)
	generated := ast.IsGenerated(file)
	sel := func(e ast.Expr) (string, string) { // the import path and name e selects (pkg's own when unqualified), or ""
		switch g := e.(type) { // an instantiation, connect.NewUnaryHandler[Req, Res]
		case *ast.IndexExpr:
			e = g.X
		case *ast.IndexListExpr:
			e = g.X
		}
		if id, ok := e.(*ast.Ident); ok {
			return pkg, id.Name
		}
		s, ok := e.(*ast.SelectorExpr)
		if !ok {
			return "", ""
		}
		x, ok := s.X.(*ast.Ident)
		if !ok {
			return "", ""
		}
		return imp[x.Name], s.Sel.Name
	}
	call := func(e ast.Expr) (*ast.CallExpr, string, string) {
		c, ok := e.(*ast.CallExpr)
		if !ok {
			return nil, "", ""
		}
		p, n := sel(c.Fun)
		return c, p, n
	}
	opens := func(e ast.Expr, chain, first string) bool { // grpc.<chain>(wrapper.<first>(), …)
		c, p, n := call(e)
		if c == nil || p != grpcPkg || n != chain || len(c.Args) == 0 || c.Ellipsis.IsValid() {
			return false
		}
		_, p, n = call(c.Args[0])
		return p == wrapper && n == first
	}
	chained := map[ast.Expr]bool{}
	ast.Inspect(file, func(nd ast.Node) bool {
		c, ok := nd.(*ast.CallExpr)
		if !ok {
			return true
		}
		path, name := sel(c.Fun)
		skip := 0 // the handler's arguments before its options
		switch {
		case strings.HasSuffix(path, "connect") && strings.Contains(path, "/proto/gen/") && strings.HasPrefix(name, "New") && strings.HasSuffix(name, "Handler"):
			skip = 1 // the service
		case path == connectPkg && strings.HasPrefix(name, "New") && strings.Contains(name, "Handler") && !generated:
			skip = 2 // the procedure and the function
		}
		switch {
		case skip > 0:
			handlers++
			if c.Ellipsis.IsValid() {
				add("%s: options spread, the check cannot see them", name)
				return true
			}
			n, first := 0, false
			for i, a := range c.Args[min(skip, len(c.Args)):] {
				_, p, o := call(a)
				switch {
				case p == wrapper && o == "WithInterceptors":
					n++
					first = first || i == 0
				case p == connectPkg && strings.HasPrefix(o, "With") && !banned[o]:
				case p == connectPkg && banned[o]:
					add("%s: connect.%s skips the wrapper's fallback", name, o)
				default:
					add("%s: option %d is not a direct call to connect's or the wrapper's option", name, i+1)
				}
			}
			switch {
			case n != 1:
				add("%s with %d wrapper WithInterceptors, want 1", name, n)
			case !first:
				add("%s: the wrapper's WithInterceptors is not the first option", name)
			}
		case path == grpcPkg && name == "NewServer":
			servers++
			if len(c.Args) < 2 || c.Ellipsis.IsValid() || !opens(c.Args[0], "ChainUnaryInterceptor", "UnaryServerInterceptor") ||
				!opens(c.Args[1], "ChainStreamInterceptor", "StreamServerInterceptor") {
				add("grpc.NewServer does not open with the wrapper's chains")
			}
			if len(c.Args) >= 2 { // reported above if wrong: not again as strays
				chained[c.Args[0]], chained[c.Args[1]] = true, true
			}
		}
		return true
	})
	ast.Inspect(file, func(nd ast.Node) bool {
		c, ok := nd.(*ast.CallExpr)
		if !ok || chained[c] {
			return true
		}
		if path, name := sel(c.Fun); path == grpcPkg && serverInterceptors[name] {
			add("grpc.%s outside the server's opening chains", name)
		}
		return true
	})
	return handlers, servers
}

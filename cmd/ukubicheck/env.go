package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// readers read or change the process's environment, or hand it on (a nil Env inherits it): the standard's §5 list,
// the fleet core's env_test.go list plus unix.Exec and ioutil's temp files (TMPDIR).
//
//ukubicheck:table the env readers envban refuses
var readers = map[string]bool{
	"os.Getenv":                      true,
	"os.LookupEnv":                   true,
	"os.Environ":                     true,
	"os.ExpandEnv":                   true,
	"os.Setenv":                      true,
	"os.Unsetenv":                    true,
	"os.Clearenv":                    true,
	"os.UserHomeDir":                 true,
	"os.UserCacheDir":                true,
	"os.UserConfigDir":               true,
	"os.TempDir":                     true,
	"os.MkdirTemp":                   true,
	"os.CreateTemp":                  true,
	"os.Getwd":                       true,
	"os.StartProcess":                true,
	"io/ioutil.TempDir":              true,
	"io/ioutil.TempFile":             true,
	"syscall.Getenv":                 true,
	"syscall.Environ":                true,
	"syscall.Setenv":                 true,
	"syscall.Unsetenv":               true,
	"syscall.Clearenv":               true,
	"syscall.ForkExec":               true,
	"syscall.Exec":                   true,
	"syscall.StartProcess":           true,
	"golang.org/x/sys/unix.Getenv":   true,
	"golang.org/x/sys/unix.Environ":  true,
	"golang.org/x/sys/unix.Setenv":   true,
	"golang.org/x/sys/unix.Unsetenv": true,
	"golang.org/x/sys/unix.Clearenv": true,
	"golang.org/x/sys/unix.Exec":     true,
	"net/http.ProxyFromEnvironment":  true,
}

// whole are the packages refused whole: os/exec (and its drop-in execabs) reads PATH and hands the environment on,
// os/user reads HOME and USER.
//
//ukubicheck:table the packages envban refuses whole
var whole = map[string]bool{"os/exec": true, "os/user": true, "golang.org/x/sys/execabs": true}

// envBan: no non-test file but the -envban.allow ones reads the environment. Type-checked: a call, a value, an alias
// import and a dot import's use all name the same function. Dot imports of a reader's package, cgo and go:linkname
// are refused too. Files the build excludes (tags, GOOS, cgo off) are parsed and checked by import name.
func envBan() *analysis.Analyzer {
	var allow string
	a := &analysis.Analyzer{
		Name: "envban",
		Doc:  "only the config file (-envban.allow) reads the environment",
	}
	a.Flags.StringVar(&allow, "allow", "", "comma-separated module-relative files that may read the environment, one config file per binary; empty bans everywhere")
	a.Run = func(pass *analysis.Pass) (any, error) {
		allowed := list(allow)
		present, reads := map[string]token.Pos{}, map[string]int{}
		check := func(rel string) func(token.Pos, string) {
			return func(pos token.Pos, what string) {
				if allowed[rel] {
					reads[rel]++
					return
				}
				pass.Reportf(pos, "%s: only the config file (-envban.allow) reads the environment", what)
			}
		}
		for _, s := range sources(pass) {
			if s.test {
				continue
			}
			present[s.rel] = s.Package
			report := check(s.rel)
			if s.cgo {
				report(s.Package, "cgo")
			}
			envSyntax(s.File, report, false)
			ast.Inspect(s.File, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					if fn, ok := pass.TypesInfo.Uses[id].(*types.Func); ok && fn.Pkg() != nil && readers[fullName(fn)] {
						report(id.Pos(), fullName(fn))
					}
				}
				return true
			})
		}
		for _, name := range pass.IgnoredFiles { // excluded by the build: go/types never saw them
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			src, err := pass.ReadFile(name)
			if err != nil {
				return nil, err
			}
			f, err := parser.ParseFile(pass.Fset, name, src, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			rel := relFile(pass, name)
			present[rel] = f.Package
			envSyntax(f, check(rel), true)
		}
		control(pass, allowed, present, reads, "-envban.allow", "reads no environment")
		return nil, nil
	}
	return a
}

// envSyntax reports f's imports and directives that envban refuses. With uses (a file go/types did not check), it
// also reports each use of a reader as a selector on its import's name.
func envSyntax(f *ast.File, report func(token.Pos, string), uses bool) {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "//go:linkname") {
				report(c.Pos(), "go:linkname")
			}
		}
	}
	local := map[string]string{}
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		name := path.Base(p)
		if im.Name != nil {
			name = im.Name.Name
		}
		switch {
		case p == "C":
			report(f.Package, "cgo")
		case whole[p]:
			report(im.Pos(), "import "+strconv.Quote(p))
		case name == "." && readerPkg(p):
			report(im.Pos(), "import . "+strconv.Quote(p))
		default:
			local[name] = p
		}
	}
	if !uses {
		return
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := s.X.(*ast.Ident); ok && local[id.Name] != "" && readers[local[id.Name]+"."+s.Sel.Name] {
				report(s.Pos(), local[id.Name]+"."+s.Sel.Name)
			}
		}
		return true
	})
}

// readerPkg: p holds one of readers.
func readerPkg(p string) bool {
	for r := range readers {
		if strings.HasPrefix(r, p+".") && !strings.Contains(r[len(p)+1:], "/") {
			return true
		}
	}
	return false
}

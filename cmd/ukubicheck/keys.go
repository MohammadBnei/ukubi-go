package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// keyType is the error key's type, by default: errs.Key.
const keyType = "github.com/MohammadBnei/ukubi-go/errs.Key"

// errKeys: outside the registry file, no non-test file makes a key. An expression of the key type is made by a
// literal, an untyped constant, a conversion (constant or run time) or an operator (A+B); a key constant, a variable,
// a parameter, a field or a call's result passes, its maker checked where it was made. So a key constant declared
// outside the registry is caught at its literal. A dependency's run-time keys (errs.From's "source.code",
// connectkeys' Of) pass: dependencies are not checked against the app's registry.
func errKeys() *analysis.Analyzer {
	var typ, registry string
	a := &analysis.Analyzer{
		Name: "errkeys",
		Doc:  "a key is one of the registry's constants (-errkeys.registry)",
	}
	a.Flags.StringVar(&typ, "type", keyType, "the key type, as importpath.Name")
	a.Flags.StringVar(&registry, "registry", "", "the module-relative file that declares the key constants; empty: no file may make a key")
	a.Run = func(pass *analysis.Pass) (any, error) {
		dot := strings.LastIndex(typ, ".")
		if dot <= 0 {
			return nil, fmt.Errorf("-errkeys.type %q is not importpath.Name", typ)
		}
		isKey := func(t types.Type) bool {
			n, ok := types.Unalias(t).(*types.Named)
			return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == typ[:dot] && n.Obj().Name() == typ[dot+1:]
		}
		reg := map[string]bool{}
		if registry != "" {
			reg[path.Clean(registry)] = true
		}
		present, consts := map[string]token.Pos{}, map[string]int{}
		for _, s := range sources(pass) {
			if s.test {
				continue
			}
			present[s.rel] = s.Package
			if reg[s.rel] {
				for _, d := range s.Decls {
					if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.CONST {
						for _, sp := range g.Specs {
							for _, id := range sp.(*ast.ValueSpec).Names {
								if c, ok := pass.TypesInfo.Defs[id].(*types.Const); ok && isKey(c.Type()) {
									consts[s.rel]++
								}
							}
						}
					}
				}
				continue
			}
			ast.Inspect(s.File, func(n ast.Node) bool {
				e, ok := n.(ast.Expr)
				if !ok {
					return true
				}
				tv, ok := pass.TypesInfo.Types[e]
				if !ok || tv.IsType() || !isKey(tv.Type) {
					return true
				}
				if how := keyMade(pass.TypesInfo, e, isKey); how != "" {
					pass.Reportf(e.Pos(), "%s makes a key: a key is one of the registry's constants (-errkeys.registry)", how)
					return false // what it is made of is reported with it
				}
				return true
			})
		}
		control(pass, reg, present, consts, "-errkeys.registry", "declares no key constant")
		return nil, nil
	}
	return a
}

// keyMade is how e, an expression of the key type, makes a key, or "" when it passes one on.
func keyMade(info *types.Info, e ast.Expr, isKey func(types.Type) bool) string {
	switch x := e.(type) {
	case *ast.BasicLit:
		return "a literal"
	case *ast.Ident, *ast.SelectorExpr:
		id, _ := x.(*ast.Ident)
		if s, ok := x.(*ast.SelectorExpr); ok {
			id = s.Sel
		}
		if c, ok := info.Uses[id].(*types.Const); ok && !isKey(c.Type()) {
			return "the untyped constant " + c.Name()
		}
	case *ast.CallExpr:
		if info.Types[x.Fun].IsType() {
			return "a conversion"
		}
	case *ast.BinaryExpr:
		return "an operator"
	}
	return ""
}

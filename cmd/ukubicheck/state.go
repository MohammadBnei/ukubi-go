package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// tableDirective marks a package-level variable as a table: data Go cannot declare const, which code only reads. It
// takes a reason: //ukubicheck:table <what the data is>.
const tableDirective = "//ukubicheck:table"

// made are the calls a package-level variable may be made by: each returns a value nothing changes afterwards
// (connectkeys.New's Keys never changes after New, and connectkeys/wiring wants it in a package var).
//
//ukubicheck:table the makers of values nothing changes
var made = map[string]bool{
	"regexp.MustCompile": true,
	"errors.New":         true,
	"github.com/MohammadBnei/ukubi-go/errs/connectkeys.New": true,
}

// readOnly are the functions a table may be given to: each reads its argument and keeps no reference to it.
//
//ukubicheck:table the functions a table may be given to
var readOnly = map[string]bool{"slices.Contains": true, "slices.Index": true, "slices.Clone": true, "maps.Clone": true}

// clones are the readOnly functions that copy one level only: a table may be given to them only when its elements
// (and keys) hold no slice, map or pointer, since the clone would share those.
//
//ukubicheck:table the shallow copiers among readOnly
var clones = map[string]bool{"slices.Clone": true, "maps.Clone": true}

// tableFact marks a table, so the packages that import it check their reads too.
type tableFact struct{}

func (*tableFact) AFact()         {}
func (*tableFact) String() string { return "table" }

// pkgState: the module holds no mutable package-level state (standard §4, §11; ukubi-game TestPackageState). In
// non-test, non-generated files:
//   - every package-level variable is the blank _ (its value calling no function: a conversion or a builtin only), an embed (into a string or an embed.FS, never a []byte), a value made
//     by one of made, or a table (//ukubicheck:table <reason>);
//   - no func init();
//   - nothing writes a package-level variable of the module: an assignment, a range's key or value, ++ or --, &, or
//     delete, clear or copy into it, rooted at it through fields, indexes, slices and pointers;
//   - a table is only read: indexed, ranged over, compared, given to len or cap or to one of readOnly. The same holds
//     for what indexing or a field of it yields, when that is a slice, a map or a pointer.
func pkgState() *analysis.Analyzer {
	a := &analysis.Analyzer{
		Name:      "pkgstate",
		Doc:       "no mutable package-level state: main builds every dependency and passes it in",
		FactTypes: []analysis.Fact{new(tableFact)},
	}
	a.Run = func(pass *analysis.Pass) (any, error) {
		if pass.Module == nil || pass.Module.Path == "" {
			return nil, nil // the standard library, or no module: nothing to scope writes to
		}
		var srcs []source
		for _, s := range sources(pass) {
			if !s.test && !s.gen {
				srcs = append(srcs, s)
			}
		}
		for _, s := range srcs {
			declarations(pass, s.File)
		}
		if pass.Module.Version != "" {
			return nil, nil // a dependency module, visited for its table facts only: its writes are not this module's
		}
		mod := pass.Module.Path
		for _, s := range srcs {
			writes(pass, s.File, func(p *types.Package) bool { return p.Path() == mod || strings.HasPrefix(p.Path(), mod+"/") })
			tableUses(pass, s.File)
		}
		return nil, nil
	}
	return a
}

func declarations(pass *analysis.Pass, f *ast.File) {
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "init" {
			pass.Reportf(fd.Pos(), "func init() wires package-level state: main builds every dependency and passes it in")
		}
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.VAR {
			continue
		}
		for _, sp := range g.Specs {
			vs := sp.(*ast.ValueSpec)
			for i, id := range vs.Names {
				if id.Name == "_" {
					if i < len(vs.Values) {
						if call := calls(pass.TypesInfo, vs.Values[i]); call != nil {
							pass.Reportf(call.Pos(), "var _ = a call runs at package init like func init(): main builds every dependency and passes it in")
						}
					}
					continue
				}
				obj := pass.TypesInfo.Defs[id]
				if obj == nil {
					continue
				}
				if reason, ok := directive(tableDirective, g.Doc, vs.Doc); ok {
					if reason == "" {
						pass.Reportf(id.Pos(), "%s: %s takes a reason: what the data is", id.Name, tableDirective)
					}
					pass.ExportObjectFact(obj, new(tableFact))
					continue
				}
				if _, ok := directive("//go:embed", g.Doc, vs.Doc); ok && embeddable(obj.Type()) {
					continue
				}
				if i < len(vs.Values) {
					if fn := funcOf(pass.TypesInfo, vs.Values[i]); fn != nil && made[fullName(fn)] {
						continue
					}
				}
				pass.Reportf(id.Pos(), "%s is package-level state: it belongs to a value main builds (or, data nothing writes, mark it %s <reason>)", id.Name, tableDirective)
			}
		}
	}
}

// calls is the first call in e that is not a conversion or a builtin's, or nil.
func calls(info *types.Info, e ast.Expr) (call *ast.CallExpr) {
	ast.Inspect(e, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || call != nil {
			return call == nil
		}
		if tv := info.Types[c.Fun]; !tv.IsType() && !tv.IsBuiltin() {
			call = c
		}
		return call == nil
	})
	return call
}

// directive is the text after a //name directive in docs, and whether there is one.
func directive(name string, docs ...*ast.CommentGroup) (string, bool) {
	for _, d := range docs {
		if d == nil {
			continue
		}
		for _, c := range d.List {
			if rest, ok := strings.CutPrefix(c.Text, name); ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t') {
				return strings.TrimSpace(rest), true
			}
		}
	}
	return "", false
}

// embeddable: a //go:embed into t holds data nothing writes (a []byte could be written).
func embeddable(t types.Type) bool {
	if b, ok := types.Unalias(t).(*types.Basic); ok {
		return b.Kind() == types.String
	}
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && fullName(n.Obj()) == "embed.FS"
}

// writes reports each write in f into a package-level variable of a package inMod.
func writes(pass *analysis.Pass, f *ast.File, inMod func(*types.Package) bool) {
	wrote := func(e ast.Expr, how string) {
		v := root(pass.TypesInfo, e)
		if v == nil || v.Pkg() == nil || v.Parent() != v.Pkg().Scope() || !inMod(v.Pkg()) {
			return
		}
		pass.Reportf(e.Pos(), "%s %s.%s: package-level state is never written", how, v.Pkg().Name(), v.Name())
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok != token.DEFINE {
				for _, l := range x.Lhs {
					wrote(l, "assigns")
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.ASSIGN {
				for _, e := range []ast.Expr{x.Key, x.Value} {
					if e != nil {
						wrote(e, "assigns (a range's)")
					}
				}
			}
		case *ast.IncDecStmt:
			wrote(x.X, "steps")
		case *ast.UnaryExpr:
			if x.Op == token.AND {
				wrote(x.X, "takes the address of")
			}
		case *ast.CallExpr:
			if id, ok := ast.Unparen(x.Fun).(*ast.Ident); ok && len(x.Args) > 0 {
				if _, builtin := pass.TypesInfo.Uses[id].(*types.Builtin); builtin && (id.Name == "delete" || id.Name == "clear" || id.Name == "copy") {
					wrote(x.Args[0], id.Name+"s into")
				}
			}
		}
		return true
	})
}

// root is the variable an expression's storage belongs to: v, v.f, v[i], v[i:j], *v, &v, (v), pkg.V and their nestings.
func root(info *types.Info, e ast.Expr) *types.Var {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			v, _ := info.Uses[x].(*types.Var)
			return v
		case *ast.SelectorExpr:
			sel, ok := info.Selections[x]
			if !ok { // pkg.V
				v, _ := info.Uses[x.Sel].(*types.Var)
				return v
			}
			if sel.Kind() != types.FieldVal {
				return nil
			}
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.SliceExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		case *ast.UnaryExpr: // *&v
			if x.Op != token.AND {
				return nil
			}
			e = x.X
		default:
			return nil
		}
	}
}

// tableUses reports each use of a table in f that is not a read: an alias, a call outside readOnly, a return, a
// method value or a field all hand it to code that could write it. Indexing it, or a field of what that yields, is a
// read of a copy, unless the result is a slice, a map or a pointer: then the same rule holds for the result.
func tableUses(pass *analysis.Pass, f *ast.File) {
	info := pass.TypesInfo
	var stack []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		v, ok := info.Uses[id].(*types.Var)
		if !ok || v.Pkg() == nil || v.Parent() != v.Pkg().Scope() || !pass.ImportObjectFact(v, new(tableFact)) {
			return true
		}
		var use ast.Expr = id
		i := len(stack) - 2                                           // the parent
		if s, ok := stack[i].(*ast.SelectorExpr); ok && s.Sel == id { // pkg.V: its use is the selector's
			use, i = s, i-1
		}
		climbed := false
	climb:
		for ; i >= 0; i-- {
			switch p := stack[i].(type) {
			case *ast.ParenExpr:
			case *ast.IndexExpr:
				if p.X != use {
					break climb
				}
				climbed = true
			case *ast.StarExpr:
				climbed = true
			case *ast.SelectorExpr:
				if s, ok := info.Selections[p]; !ok || s.Kind() != types.FieldVal || p.X != use {
					break climb
				}
				climbed = true
			default:
				break climb
			}
			use = stack[i].(ast.Expr)
		}
		t := info.TypeOf(use)
		if tu, ok := t.(*types.Tuple); ok { // v, ok := tbl[k]: the value is the tuple's first
			t = tu.At(0).Type()
		}
		if climbed && !reference(t) {
			return true // a copy
		}
		switch p := stack[i].(type) {
		case *ast.RangeStmt:
			if p.X == use {
				return true
			}
		case *ast.BinaryExpr:
			return true
		case *ast.CallExpr:
			if fid, ok := ast.Unparen(p.Fun).(*ast.Ident); ok {
				if _, builtin := info.Uses[fid].(*types.Builtin); builtin && (fid.Name == "len" || fid.Name == "cap") {
					return true
				}
			}
			if fn := funcOf(info, p); fn != nil && readOnly[fullName(fn)] && p.Fun != use && (!clones[fullName(fn)] || flatElems(t)) {
				return true
			}
		}
		pass.Reportf(use.Pos(), "%s is a table: it is only read (indexed, ranged over, compared, len, cap, slices.Contains, slices.Index, slices.Clone, maps.Clone)", v.Name())
		return true
	})
}

// flatElems: t's elements (and a map's keys) hold no slice, map or pointer, so a one-level copy of t shares nothing.
func flatElems(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Slice:
		return flat(u.Elem())
	case *types.Map:
		return flat(u.Key()) && flat(u.Elem())
	}
	return false
}

// flat: a value of t holds no slice, map, pointer, interface, chan or func.
func flat(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Kind() != types.UnsafePointer
	case *types.Array:
		return flat(u.Elem())
	case *types.Struct:
		for i := range u.NumFields() {
			if !flat(u.Field(i).Type()) {
				return false
			}
		}
		return true
	}
	return false
}

// reference: a value of t can write what it was read from.
func reference(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Slice, *types.Map, *types.Pointer:
		return true
	}
	return false
}

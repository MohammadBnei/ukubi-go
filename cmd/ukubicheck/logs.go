package main

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// logLevel: no non-test file outside the -loglevel.wrappers packages uses the log package, whose lines are INFO under
// the JSON logger whatever they say (standard §7: every site has its level). Its Logger type, given to net/http as
// ErrorLog, is the one name allowed. Type-checked: an alias, a dot import or a method value is the same object
// (ukubi-game TestLogLevels). A wrapper package that uses no log fails: the check no longer sees it.
func logLevel() *analysis.Analyzer {
	var wrappers string
	a := &analysis.Analyzer{
		Name: "loglevel",
		Doc:  "lines get their level from slog: the log package only in the wrappers (-loglevel.wrappers)",
	}
	a.Flags.StringVar(&wrappers, "wrappers", "", "comma-separated module-relative package directories that may use the log package; empty bans everywhere")
	a.Run = func(pass *analysis.Pass) (any, error) {
		wrapper := list(wrappers)[relDir(pass)]
		uses := 0
		var first *ast.File
		for _, s := range sources(pass) {
			if s.test {
				continue
			}
			if first == nil {
				first = s.File
			}
			ast.Inspect(s.File, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				obj := pass.TypesInfo.Uses[id]
				if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "log" {
					return true
				}
				if _, isType := obj.(*types.TypeName); isType {
					return true
				}
				if wrapper {
					uses++
					return true
				}
				pass.Reportf(id.Pos(), "log.%s: give the line its level with slog (the log package is for -loglevel.wrappers)", obj.Name())
				return true
			})
		}
		if wrapper && first != nil && uses == 0 {
			pass.Reportf(first.Package, "-loglevel.wrappers %s uses no log: the check no longer sees it", relDir(pass))
		}
		return nil, nil
	}
	return a
}

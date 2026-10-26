// Package textlen refuses a byte count used as a limit on text.
//
// `len(s)` of a Go string counts bytes. The spec's `maxLength` counts
// characters, and so does Postgres's `VARCHAR(n)`, so a check written with
// len() refuses what both of them accept as soon as the text is not ASCII: a
// 101-letter Polish name is 202 bytes, and the check refuses it with a
// message about a limit the user did not exceed.
//
// In a module's api.go and handler.go - where input is checked - comparing
// len() of a string with a constant is refused. Count characters with
// utf8.RuneCountInString; a limit that really is in bytes says so with
// len([]byte(s)), which this rule accepts.
package textlen

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/analysis"

	"github.com/open-enu/kernel/analysis/modpath"
)

var Analyzer = &analysis.Analyzer{
	Name: "textlen",
	Doc:  "in a module's api.go and handler.go, a string's length is compared with a limit in characters (utf8.RuneCountInString), not bytes (len)",
	Run:  run,
}

var checked = map[string]bool{"api.go": true, "handler.go": true}

var comparisons = map[token.Token]bool{token.LSS: true, token.LEQ: true, token.GTR: true, token.GEQ: true, token.EQL: true, token.NEQ: true}

func run(pass *analysis.Pass) (any, error) {
	if _, _, _, ok := modpath.Module(pass.Pkg.Path()); !ok {
		return nil, nil
	}
	for _, f := range pass.Files {
		if !checked[filepath.Base(pass.Fset.File(f.Pos()).Name())] {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || !comparisons[bin.Op] {
				return true
			}
			for _, pair := range [2][2]ast.Expr{{bin.X, bin.Y}, {bin.Y, bin.X}} {
				if stringLen(pass, pair[0]) && limit(pass, pair[1]) {
					pass.Reportf(pair[0].Pos(), "len() of a string counts bytes, and a text limit counts characters - use utf8.RuneCountInString (or len([]byte(s)) for a limit that really is in bytes)")
				}
			}
			return true
		})
	}
	return nil, nil
}

// stringLen reports whether e is the builtin len applied to a string.
func stringLen(pass *analysis.Pass, e ast.Expr) bool {
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	id, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	if _, builtin := pass.TypesInfo.Uses[id].(*types.Builtin); !builtin || id.Name != "len" {
		return false
	}
	t := pass.TypesInfo.TypeOf(call.Args[0])
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}

// limit reports whether e is a constant other than zero: against zero, len()
// asks "is it empty?", and the answer is the same in bytes and in characters.
func limit(pass *analysis.Pass, e ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[e]
	return ok && tv.Value != nil && tv.Value.ExactString() != "0"
}

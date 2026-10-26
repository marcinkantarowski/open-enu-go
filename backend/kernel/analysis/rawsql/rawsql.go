// Package rawsql confines SQL to repository.go and makes every cross-tenant
// read say why.
//
// Two rules:
//
//   - pgx's Exec, Query and QueryRow are called only from a file named
//     repository.go. Anywhere else a query is a query nobody will find when
//     the schema changes.
//   - scope.Unscoped(ctx, reason, fn) takes a non-empty string literal as its
//     reason and is called only from repository.go. The literal is what makes
//     `grep Unscoped` a complete review of every place tenant isolation is
//     lifted (ADR-0009).
package rawsql

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/open-enu/kernel/analysis/modpath"
)

var Analyzer = &analysis.Analyzer{
	Name: "rawsql",
	Doc:  "pgx Exec/Query/QueryRow only in repository.go; scope.Unscoped needs a literal reason and lives in repository.go",
	Run:  run,
}

const pgxPrefix = "github.com/jackc/pgx/"

var sqlMethods = map[string]bool{"Exec": true, "Query": true, "QueryRow": true, "CopyFrom": true, "SendBatch": true}

func run(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		file := filepath.Base(pass.Fset.File(f.Pos()).Name())
		// A test file may run SQL: the RLS proofs read the table directly
		// to show what the policy did, and a mock of that would prove
		// nothing. The literal-reason rule for Unscoped still applies.
		inRepo := file == "repository.go" || strings.HasSuffix(file, "_test.go")
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			obj := pass.TypesInfo.Uses[sel.Sel]
			if obj == nil || obj.Pkg() == nil {
				return true
			}
			pkg := obj.Pkg().Path()

			if strings.HasPrefix(pkg, pgxPrefix) && sqlMethods[sel.Sel.Name] && !inRepo && !modpath.IsKernel(pass.Pkg.Path()) {
				pass.Reportf(call.Pos(), "%s.%s called in %s - SQL lives in repository.go, where the schema change will find it", recvName(obj), sel.Sel.Name, file)
			}

			if pkg == modpath.KernelPrefix+"/scope" && sel.Sel.Name == "Unscoped" {
				checkUnscoped(pass, call, file, inRepo)
			}
			return true
		})
	}
	return nil, nil
}

func checkUnscoped(pass *analysis.Pass, call *ast.CallExpr, file string, inRepo bool) {
	if !inRepo && !modpath.IsKernel(pass.Pkg.Path()) {
		pass.Reportf(call.Pos(), "scope.Unscoped called in %s - a cross-tenant read belongs in repository.go, where every one of them is in one place", file)
	}
	if len(call.Args) < 2 {
		return
	}
	lit, ok := call.Args[1].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		pass.Reportf(call.Args[1].Pos(), "scope.Unscoped reason must be a string literal - it is what `grep Unscoped` reviews; a variable hides the why")
		return
	}
	if strings.Trim(lit.Value, "`\"") == "" {
		pass.Reportf(lit.Pos(), "scope.Unscoped reason is empty - say why this read may cross tenants")
	}
}

func recvName(obj types.Object) string {
	if fn, ok := obj.(*types.Func); ok {
		if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
			t := sig.Recv().Type()
			if p, ok := t.(*types.Pointer); ok {
				t = p.Elem()
			}
			if named, ok := t.(*types.Named); ok {
				return named.Obj().Name()
			}
		}
	}
	return "pgx"
}

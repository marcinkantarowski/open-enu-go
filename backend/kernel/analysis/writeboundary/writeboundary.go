// Package writeboundary keeps writes inside handlers.
//
// Only a handler receives a *scope.Tx from the bus; a repository takes it
// as a parameter to run the SQL; a job (a River worker) writes through a
// command or holds one for its own work. Everywhere else in a module - the
// controller above all - a *scope.Tx means a write that bypassed the bus
// and left no audit entry. Opening a transaction (scope.Pool.Begin / Run)
// is the bus's job and, for reads, the repository's; nobody else's.
//
// The compiler already makes *scope.Tx unreachable without being handed
// one; this analyzer makes being handed one in the wrong file an error.
package writeboundary

import (
	"go/ast"
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/analysis"

	"github.com/open-enu/kernel/analysis/modpath"
)

var Analyzer = &analysis.Analyzer{
	Name: "writeboundary",
	Doc:  "in a module, *scope.Tx appears only in handler.go, repository.go and job.go; scope.Pool is opened only in repository.go",
	Run:  run,
}

const scopePkg = modpath.KernelPrefix + "/scope"

var txFiles = map[string]bool{"handler.go": true, "repository.go": true, "job.go": true}
var poolFiles = map[string]bool{"repository.go": true}

func run(pass *analysis.Pass) (any, error) {
	if _, _, _, ok := modpath.Module(pass.Pkg.Path()); !ok {
		return nil, nil
	}
	for _, f := range pass.Files {
		name := filepath.Base(pass.Fset.Position(f.Pos()).Filename)
		if name == "" || isTest(name) {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			// Values only - a field, a parameter, a variable. The type name
			// `scope.Tx` in a declaration is reported through the thing it
			// declares, not twice.
			var typ types.Type
			switch obj := pass.TypesInfo.ObjectOf(id).(type) {
			case *types.TypeName, *types.PkgName, nil:
				return true
			default:
				typ = obj.Type()
			}
			switch {
			case isScope(typ, "Tx") && !txFiles[name]:
				pass.Reportf(id.Pos(), "*scope.Tx in %s - a write happens in a handler the bus called; this file dispatches a command instead", name)
			case isScope(typ, "Pool") && !poolFiles[name]:
				pass.Reportf(id.Pos(), "scope.Pool in %s - only the bus opens a write transaction, and only repository.go opens one to read", name)
			}
			return true
		})
	}
	return nil, nil
}

func isTest(name string) bool { return len(name) > 8 && name[len(name)-8:] == "_test.go" }

// isScope reports whether t is scope.<name> or a pointer to it.
func isScope(t types.Type, name string) bool {
	if t == nil {
		return false
	}
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == scopePkg && named.Obj().Name() == name
}

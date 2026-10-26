// Package flagdeclared insists that every flag a package reads is one it
// declares.
//
// An undeclared flag resolves to the call's default forever: it never
// appears in the operator console, has no per-tenant override to attach to,
// and cannot be turned off during an incident - which is the one job a
// kill switch has. The declaration is a flags.Definition literal in the
// same package (flags.go by convention), listed in the module's
// Module.Flags; this analyzer checks the first half, the registry the
// second.
package flagdeclared

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/open-enu/kernel/analysis/modpath"
)

var Analyzer = &analysis.Analyzer{
	Name: "flagdeclared",
	Doc:  "a flag key read through flags.Flags must be declared as a flags.Definition in the same package",
	Run:  run,
}

const flagsPkg = modpath.KernelPrefix + "/flags"

var readers = map[string]bool{"Enabled": true, "String": true, "Int": true, "JSON": true}

func run(pass *analysis.Pass) (any, error) {
	if modpath.IsKernel(pass.Pkg.Path()) {
		return nil, nil
	}
	declared := map[string]bool{}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			if cl, ok := n.(*ast.CompositeLit); ok && isFlagsType(pass.TypesInfo.TypeOf(cl), "Definition") {
				if id, ok := identifierOf(pass, cl); ok {
					declared[id] = true
				}
			}
			return true
		})
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !readers[sel.Sel.Name] || len(call.Args) < 2 {
				return true
			}
			if !isFlagsType(pass.TypesInfo.TypeOf(sel.X), "") {
				return true
			}
			key, isConst := constString(pass, call.Args[1])
			switch {
			case !isConst:
				pass.Reportf(call.Args[1].Pos(), "flag key must be a string literal, so its declaration can be checked")
			case !declared[key]:
				pass.Reportf(call.Args[1].Pos(), "flag %q is read but not declared in this package - add flags.Definition{Identifier: %q, ...} to flags.go and list it in Module.Flags, or an operator can never switch it", key, key)
			}
			return true
		})
	}
	return nil, nil
}

// isFlagsType reports whether t is a named type from kernel/flags (or a
// pointer to one); with name set, that one type only.
func isFlagsType(t types.Type, name string) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != flagsPkg {
		return false
	}
	return name == "" || named.Obj().Name() == name
}

// identifierOf reads the Identifier field of a Definition literal, keyed or
// positional.
func identifierOf(pass *analysis.Pass, cl *ast.CompositeLit) (string, bool) {
	for i, e := range cl.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Identifier" {
				return constString(pass, kv.Value)
			}
			continue
		}
		if i == 0 {
			return constString(pass, e)
		}
	}
	return "", false
}

func constString(pass *analysis.Pass, e ast.Expr) (string, bool) {
	tv, ok := pass.TypesInfo.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

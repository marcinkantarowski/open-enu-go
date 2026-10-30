// Package impersonationguard insists that a controller which dispatches a
// command refuses an impersonated session first (ADR-0020).
//
// Support may look; support may not act in someone else's name. The guard
// is auth.Guard.DenyImpersonated, and it has to run before the command: a
// check after the dispatch is a check after the write. A route an
// impersonated session is meant to use - or one that has no principal at
// all, like a public sign-in - says so with a comment on the method:
//
//	//impersonation:allowed <why>
//
// The reason is not optional: an exemption nobody can read is an omission.
// A package's own helper that calls DenyImpersonated (`a.mayWrite(ctx,
// action)`) counts as the guard when the controller calls it first.
package impersonationguard

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/open-enu/kernel/analysis/modpath"
)

var Analyzer = &analysis.Analyzer{
	Name: "impersonationguard",
	Doc:  "a controller method in a module's api.go that dispatches a command must call DenyImpersonated before it, or carry //impersonation:allowed <reason>",
	Run:  run,
}

const (
	commandPkg = modpath.KernelPrefix + "/command"
	authPkg    = modpath.KernelPrefix + "/auth"
	directive  = "//impersonation:allowed"
)

func run(pass *analysis.Pass) (any, error) {
	if _, _, rest, ok := modpath.Module(pass.Pkg.Path()); !ok || rest != "internal" {
		return nil, nil
	}
	controller := serverMethods(pass.Pkg)
	if len(controller) == 0 {
		return nil, nil
	}
	guards := guardHelpers(pass)
	for _, f := range pass.Files {
		if filepath.Base(pass.Fset.Position(f.Pos()).Filename) != "api.go" {
			continue
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil || !controller[fn.Name.Name] {
				continue
			}
			check(pass, f, fn, guards)
		}
	}
	return nil, nil
}

// serverMethods names the methods of the generated StrictServerInterface:
// the controller methods, one per operation in the spec.
func serverMethods(pkg *types.Package) map[string]bool {
	obj := pkg.Scope().Lookup("StrictServerInterface")
	if obj == nil {
		return nil
	}
	iface, ok := obj.Type().Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	out := map[string]bool{}
	for i := 0; i < iface.NumMethods(); i++ {
		out[iface.Method(i).Name()] = true
	}
	return out
}

// guardHelpers are the package's own functions and methods that call
// DenyImpersonated themselves - a module's `mayWrite(ctx, action)` that
// requires the permission and refuses impersonation in one place. Calling one
// is the guard. One level deep, on purpose: a helper of a helper is a guard
// nobody can see from the controller.
func guardHelpers(pass *analysis.Pass) map[*types.Func]bool {
	out := map[*types.Func]bool{}
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if !ok {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok && isDeny(pass, call) {
					out[obj] = true
					return false
				}
				return true
			})
		}
	}
	return out
}

func isDeny(pass *analysis.Pass, call *ast.CallExpr) bool {
	callee, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	return ok && callee.Pkg() != nil && callee.Pkg().Path() == authPkg && callee.Name() == "DenyImpersonated"
}

func check(pass *analysis.Pass, f *ast.File, fn *ast.FuncDecl, guards map[*types.Func]bool) {
	dispatch, guard := token.NoPos, token.NoPos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
		if !ok || callee.Pkg() == nil {
			return true
		}
		switch {
		case callee.Pkg().Path() == commandPkg && callee.Name() == "Dispatch":
			if !dispatch.IsValid() || call.Pos() < dispatch {
				dispatch = call.Pos()
			}
		case (callee.Pkg().Path() == authPkg && callee.Name() == "DenyImpersonated") || guards[callee.Origin()]:
			if !guard.IsValid() || call.Pos() < guard {
				guard = call.Pos()
			}
		}
		return true
	})
	if !dispatch.IsValid() || (guard.IsValid() && guard < dispatch) {
		return
	}
	if pos, reason, found := allowance(f, fn); found {
		if reason == "" {
			pass.Reportf(pos, "%s needs a reason: say why an impersonated session may use %s", directive, fn.Name.Name)
		}
		return
	}
	pass.Reportf(dispatch, "%s dispatches a command without refusing an impersonated session first - call a.guard.DenyImpersonated(ctx, \"<action>\") before the dispatch, or mark the method %s <reason> (ADR-0020)", fn.Name.Name, directive)
}

// allowance finds the directive on the method: in its doc comment or
// anywhere inside it.
func allowance(f *ast.File, fn *ast.FuncDecl) (token.Pos, string, bool) {
	for _, cg := range f.Comments {
		if cg.End() < fn.Pos() && (fn.Doc == nil || cg != fn.Doc) {
			continue
		}
		if cg.Pos() > fn.End() {
			break
		}
		for _, c := range cg.List {
			rest, ok := strings.CutPrefix(c.Text, directive)
			if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
				continue
			}
			// A trailing comment on the same line is not a reason.
			rest, _, _ = strings.Cut(rest, "//")
			return c.Pos(), strings.TrimSpace(rest), true
		}
	}
	return token.NoPos, "", false
}

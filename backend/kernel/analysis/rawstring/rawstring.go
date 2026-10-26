// Package rawstring keeps sentences out of the code.
//
// Every user-facing string is an i18n.Key resolved from a catalogue. The
// type makes that mostly structural - a function wanting a Key cannot take a
// plain string variable - but an untyped string constant converts to Key
// silently, so `httpx.NotFound("Project not found.")` compiles. This analyzer
// looks at every constant that ends up as a Key and insists it has the
// dotted-lowercase shape of a catalogue key.
package rawstring

import (
	"go/ast"
	"go/constant"
	"go/types"
	"regexp"

	"golang.org/x/tools/go/analysis"

	"github.com/open-enu/kernel/analysis/modpath"
)

var Analyzer = &analysis.Analyzer{
	Name: "rawstring",
	Doc:  "a constant used as an i18n.Key must be a catalogue key (dotted lowercase), not a sentence",
	Run:  run,
}

var keyShape = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

func run(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				checkCall(pass, x)
			case *ast.ValueSpec:
				// const K i18n.Key = "..." and var k i18n.Key = "..."
				if x.Type != nil && isKey(pass.TypesInfo.TypeOf(x.Type)) {
					for _, v := range x.Values {
						checkConst(pass, v)
					}
				}
			}
			return true
		})
	}
	return nil, nil
}

// checkCall covers both i18n.Key("...") conversions and arguments passed to
// a parameter of type i18n.Key.
func checkCall(pass *analysis.Pass, call *ast.CallExpr) {
	if tv, ok := pass.TypesInfo.Types[call.Fun]; ok && tv.IsType() && isKey(tv.Type) && len(call.Args) == 1 {
		checkConst(pass, call.Args[0])
		return
	}
	sig, ok := pass.TypesInfo.TypeOf(call.Fun).(*types.Signature)
	if !ok {
		return
	}
	params := sig.Params()
	for i, arg := range call.Args {
		var pt types.Type
		switch {
		case i < params.Len():
			pt = params.At(i).Type()
		case sig.Variadic() && params.Len() > 0:
			pt = params.At(params.Len() - 1).Type()
		}
		if sl, ok := pt.(*types.Slice); ok && sig.Variadic() && i >= params.Len()-1 {
			pt = sl.Elem()
		}
		if isKey(pt) {
			checkConst(pass, arg)
		}
	}
}

func checkConst(pass *analysis.Pass, e ast.Expr) {
	tv, ok := pass.TypesInfo.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return // not a constant: a variable or a call, which the type system already vetted
	}
	s := constant.StringVal(tv.Value)
	if !keyShape.MatchString(s) {
		pass.Reportf(e.Pos(), "%q is not a translation key - a user-facing string is a dotted lowercase key in the module's i18n catalogue (e.g. example.project.not_found)", s)
	}
}

func isKey(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "Key" && obj.Pkg() != nil && obj.Pkg().Path() == modpath.KernelPrefix+"/i18n"
}

// Package thincontroller keeps a module's api.go thin.
//
// api.go turns a request into a command and a result into a response, and
// nothing else. Two things make it fat: importing infrastructure directly
// (a database driver, a Redis client, another module's internals), and
// reaching sideways in its own package for what the handler and the
// repository own. The first is an import check; the second is a check on
// which file declared each identifier api.go uses.
package thincontroller

import (
	"go/ast"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/open-enu/kernel/analysis/modpath"
)

var Analyzer = &analysis.Analyzer{
	Name: "thincontroller",
	Doc:  "a module's api.go imports only the kernel's HTTP surface and its own contract, and uses nothing declared in handler.go or job.go (it may read through repository.go)",
	Run:  run,
}

// Kernel packages api.go may import: the HTTP surface, translation keys,
// the command bus, the platform value Mount receives, flags (a route behind
// a switched-off flag is 404, and that is the controller's call) and auth
// (who is asking is the controller's question; what to do with it is the
// handler's). Not scope, not cache, not storage.
var allowedKernel = map[string]bool{
	modpath.KernelPrefix + "/httpx":    true,
	modpath.KernelPrefix + "/i18n":     true,
	modpath.KernelPrefix + "/command":  true,
	modpath.KernelPrefix + "/platform": true,
	modpath.KernelPrefix + "/flags":    true,
	modpath.KernelPrefix + "/auth":     true,
}

// Files in the same package whose declarations api.go may not touch. Not
// repository.go: a controller reads through it for a GET, and writeboundary
// + rawsql already keep the transaction and the SQL out of api.go.
var forbiddenFiles = map[string]string{
	"handler.go": "a controller dispatches a command through the bus; it never calls a handler",
	"job.go":     "a controller enqueues a job through the bus; it never runs one",
}

func run(pass *analysis.Pass) (any, error) {
	root, _, rest, ok := modpath.Module(pass.Pkg.Path())
	if !ok || rest != "internal" {
		return nil, nil
	}
	for _, f := range pass.Files {
		if filepath.Base(pass.Fset.File(f.Pos()).Name()) != "api.go" {
			continue
		}
		checkImports(pass, f, root)
		checkUses(pass, f)
	}
	return nil, nil
}

func checkImports(pass *analysis.Pass, f *ast.File, root string) {
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		switch {
		case !strings.Contains(strings.SplitN(path, "/", 2)[0], "."): // standard library
			if path == "database/sql" {
				pass.Reportf(imp.Pos(), "api.go imports database/sql - a controller does not touch the database; dispatch a command")
			}
		case allowedKernel[path]:
		case path == root+"/contract":
		case modpath.IsKernel(path):
			pass.Reportf(imp.Pos(), "api.go imports %s - a controller may import only httpx, i18n, command, platform, flags and auth from the kernel; %s belongs in the handler", path, filepath.Base(path))
		default:
			pass.Reportf(imp.Pos(), "api.go imports %s - a controller imports the kernel's HTTP surface and its own contract/, nothing else; move this into the handler or a service", path)
		}
	}
}

func checkUses(pass *analysis.Pass, f *ast.File) {
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pass.TypesInfo.Uses[id]
		if obj == nil || obj.Pkg() != pass.Pkg || !obj.Pos().IsValid() {
			return true
		}
		declFile := filepath.Base(pass.Fset.File(obj.Pos()).Name())
		if why, bad := forbiddenFiles[declFile]; bad {
			pass.Reportf(id.Pos(), "api.go uses %s, declared in %s - %s", id.Name, declFile, why)
		}
		return true
	})
}

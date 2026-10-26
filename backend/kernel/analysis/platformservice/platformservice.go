// Package platformservice forces modules through the kernel's contracts for
// files and caching.
//
// The kernel owns storage (tenant-scoped, with the real constraints) and the
// cache (tenant-prefixed keys, tag invalidation). A module that writes a file
// with os.WriteFile or opens its own Redis client has stepped around both,
// and the data it wrote is invisible to tenant deletion, GDPR erasure and the
// backup. Re-implementing a platform service locally is the most expensive
// mistake available here (AGENTS.md), so it is a build failure.
package platformservice

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/open-enu/kernel/analysis/modpath"
)

var Analyzer = &analysis.Analyzer{
	Name: "platformservice",
	Doc:  "outside the kernel: no direct file writes (os.WriteFile/Create/OpenFile/Mkdir*) and no raw Redis client",
	Run:  run,
}

var fileWrites = map[string]bool{
	"WriteFile": true, "Create": true, "OpenFile": true, "CreateTemp": true,
	"Mkdir": true, "MkdirAll": true, "MkdirTemp": true, "Remove": true, "RemoveAll": true, "Rename": true,
}

var redisClients = []string{"github.com/redis/rueidis", "github.com/redis/go-redis", "github.com/gomodule/redigo"}

func run(pass *analysis.Pass) (any, error) {
	if modpath.IsKernel(pass.Pkg.Path()) || strings.Contains(pass.Pkg.Path(), "/tools/") || strings.HasSuffix(pass.Pkg.Path(), "/tools") {
		return nil, nil // the kernel implements the contracts; the generators write source files
	}
	for _, f := range pass.Files {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, c := range redisClients {
				if path == c || strings.HasPrefix(path, c+"/") {
					pass.Reportf(imp.Pos(), "%s imported outside the kernel - cache through kernel/cache, which prefixes keys with the tenant and invalidates by tag", path)
				}
			}
		}
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
			if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "os" || !fileWrites[sel.Sel.Name] {
				return true
			}
			pass.Reportf(call.Pos(), "os.%s outside the kernel - files go through kernel/storage, which scopes them to the tenant and includes them in erasure and backup", sel.Sel.Name)
			return true
		})
	}
	return nil, nil
}

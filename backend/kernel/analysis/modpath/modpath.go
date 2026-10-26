// Package modpath answers the two questions every guardrail asks about a
// package path: is this the kernel, and which module is this in.
package modpath

import "strings"

// KernelPrefix is the kernel's module path. Nothing under it is a module.
const KernelPrefix = "github.com/open-enu/kernel"

// IsKernel reports whether pkg is the kernel or one of its packages.
func IsKernel(pkg string) bool {
	return pkg == KernelPrefix || strings.HasPrefix(pkg, KernelPrefix+"/")
}

// Module splits a package path at /internal/module/<name>. It returns the
// module's root import path (…/internal/module/<name>), the module name and
// the remainder ("" for module.go's package, "internal", "contract", …).
func Module(pkg string) (root, name, rest string, ok bool) {
	const marker = "/internal/module/"
	i := strings.Index(pkg, marker)
	if i < 0 {
		return "", "", "", false
	}
	after := pkg[i+len(marker):]
	name, rest, _ = strings.Cut(after, "/")
	if name == "" {
		return "", "", "", false
	}
	return pkg[:i+len(marker)] + name, name, rest, true
}

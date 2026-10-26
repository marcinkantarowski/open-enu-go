// Package kernel is the root of github.com/open-enu/kernel - the framework the
// application modules are built on.
//
// It is a separate Go module with no dependency on github.com/open-enu/app, and
// that absence is a guardrail, not an accident: the kernel knows nothing about
// the application, so nothing in it can reach for an application type, and
// `go build` itself refuses the import. `make selftest` breaks it on purpose to
// prove that still holds.
//
// Each platform concern is one sub-package (see README.md); this file only
// names the package and carries its version.
package kernel

// Version is the kernel's own version, bumped on every change a project built
// from this platform may want to pull. It is what `make platform-update` will
// compare against.
const Version = "0.0.0"

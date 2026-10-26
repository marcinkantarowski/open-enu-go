// Command genspec merges every module's openapi.json with the kernel's shared
// components into backend/openapi.json - the one file the frontends, the MCP
// server and `make types` read.
//
// Run by `go generate ./cmd/openenu` (cwd cmd/openenu); `make arch` runs it
// with -check so a module whose fragment changed without the merged file
// being regenerated fails the build.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/open-enu/app/tools/internal/spec"
	"github.com/open-enu/kernel"
	"github.com/open-enu/kernel/httpx"
)

func main() {
	check := flag.Bool("check", false, "exit 1 if backend/openapi.json is stale instead of rewriting it")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, "genspec:", err)
		os.Exit(1)
	}
}

func run(check bool) error {
	backend, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return err
	}
	shared, err := spec.ParseComponents(httpx.SharedComponents)
	if err != nil {
		return err
	}
	fragments := map[string]*spec.Document{}
	matches, _ := filepath.Glob(filepath.Join(backend, "internal", "module", "*", "openapi.json"))
	for _, path := range matches {
		name := filepath.Base(filepath.Dir(path))
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		d, err := spec.ParseFragment(name+"/openapi.json", data)
		if err != nil {
			return err
		}
		fragments[name] = d
	}
	merged, err := spec.Merge(kernel.Version, shared, fragments)
	if err != nil {
		return err
	}
	out, err := spec.Render(merged)
	if err != nil {
		return err
	}
	target := filepath.Join(backend, "openapi.json")
	if check {
		cur, err := os.ReadFile(target)
		if err != nil || !bytes.Equal(cur, out) {
			return fmt.Errorf("backend/openapi.json is stale - run `go generate ./cmd/openenu` and commit the result")
		}
		return nil
	}
	return os.WriteFile(target, out, 0o644)
}

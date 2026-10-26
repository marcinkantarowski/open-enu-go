package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/open-enu/kernel/inventory"
)

// `openenu inventory` writes .ai/inventory.json from the registry and the
// module directories (kernel/inventory). -check compares instead of writing,
// which is how `make arch` keeps the committed copy true. Hints about
// near-duplicate names go to stderr and never fail anything.
func init() {
	subcommands["inventory"] = command{summary: "write .ai/inventory.json - what already exists (-check: fail if stale)", run: runInventory}
}

func runInventory(args []string) int {
	fs := flag.NewFlagSet("inventory", flag.ContinueOnError)
	check := fs.Bool("check", false, "exit 1 if the committed inventory is stale instead of rewriting it")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu inventory:", err)
		return 1
	}
	reg, err := registry()
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu inventory:", err)
		return 1
	}
	doc, err := inventory.Build(filepath.Join(root, "backend", "internal", "module"), reg.All())
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu inventory:", err)
		return 1
	}
	target := filepath.Join(root, ".ai", "inventory.json")
	stale, err := doc.Sync(target, *check)
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu inventory:", err)
		return 1
	}
	switch {
	case *check && stale:
		fmt.Fprintln(os.Stderr, "openenu inventory: .ai/inventory.json is stale - run `make inventory` and commit the result")
		return 1
	case *check:
		fmt.Printf("%d modules, matching the code\n", len(doc.Modules))
	case stale:
		fmt.Printf("wrote .ai/inventory.json (%d modules)\n", len(doc.Modules))
	default:
		fmt.Printf(".ai/inventory.json already current (%d modules)\n", len(doc.Modules))
	}
	// Hints only when writing: `make inventory` is when somebody is adding a
	// name; `-check` in arch is not the moment, and a hint printed on every
	// run is a hint nobody reads.
	if !*check {
		for _, h := range doc.Hints() {
			fmt.Fprintln(os.Stderr, "  hint: similar names:", h)
		}
	}
	return 0
}

// repoRoot walks up from the working directory to the directory holding
// .project.json - the binary is run from backend/ by make and from anywhere
// by a person.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".project.json")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .project.json above %s - run from inside the repository", dir)
		}
		dir = parent
	}
}

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/open-enu/kernel/spec"
)

// `openenu route-coverage TRACE` subtracts what the tests reached from what
// the spec publishes. The trace is the file httpx writes when ROUTE_TRACE is
// set (`make test` sets it); an operation absent from it fails the build,
// unless its fragment carries `x-no-test-required` with a written reason -
// reported as exempt, never counted as covered.
//
// A subcommand rather than a test because it has to run AFTER every suite
// has finished writing, and a test that depends on another suite having run
// passes or fails by ordering.
func init() {
	subcommands["route-coverage"] = command{summary: "fail if an operation in the spec was never reached by a test (-list prints them all)", run: runRouteCoverage}
}

func runRouteCoverage(args []string) int {
	fs := flag.NewFlagSet("route-coverage", flag.ContinueOnError)
	list := fs.Bool("list", false, "print every operation's pattern and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	reg, err := registry()
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu route-coverage:", err)
		return 1
	}
	var ops []spec.Operation
	for _, m := range reg.All() {
		if len(m.OpenAPI) == 0 {
			continue
		}
		found, err := spec.Operations(m.OpenAPI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "openenu route-coverage: %s: %v\n", m.Manifest.Name, err)
			return 1
		}
		ops = append(ops, found...)
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].Pattern() < ops[j].Pattern() })
	if *list {
		for _, op := range ops {
			fmt.Println(op.Pattern())
		}
		return 0
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: openenu route-coverage TRACE   (the file ROUTE_TRACE named while the tests ran)")
		return 2
	}
	hit, err := readTrace(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "openenu route-coverage: %v - run `make test` first\n", err)
		return 1
	}
	var untested []spec.Operation
	exempt := 0
	for _, op := range ops {
		if hit[op.Pattern()] {
			continue
		}
		if op.NoTestReason != "" {
			fmt.Printf("  . %-48s exempt - %s\n", op.Pattern(), op.NoTestReason)
			exempt++
			continue
		}
		untested = append(untested, op)
	}
	if len(untested) == 0 {
		fmt.Printf("%d operation(s), %d reached by a test, %d exempt\n", len(ops), len(ops)-exempt, exempt)
		return 0
	}
	fmt.Fprintf(os.Stderr, "%d operation(s) were never reached by a test:\n", len(untested))
	for _, op := range untested {
		fmt.Fprintf(os.Stderr, "  x %s (%s)\n", op.Pattern(), op.ID)
	}
	fmt.Fprintln(os.Stderr, "\n  Write a test that calls it, or - if it genuinely cannot have one - add")
	fmt.Fprintln(os.Stderr, "  \"x-no-test-required\": \"why\" to the operation in the module's openapi.json.")
	return 1
}

func readTrace(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	hit := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			hit[line] = true
		}
	}
	return hit, sc.Err()
}

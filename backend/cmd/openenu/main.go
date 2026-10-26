// Command openenu is the one binary: api, worker, scheduler and mcp are the
// same executable started with a different first argument, which is what lets
// the production image carry one static file and nothing else.
//
// The console verbs (user, tenant, apikey, manager, attachment) live beside
// the servers; an unknown subcommand is refused with the usage text, never
// silently accepted.
package main

//go:generate go run ../../tools/genregistry
//go:generate go run ../../tools/genspec
//go:generate go run ../../tools/genacl

import (
	"fmt"
	"os"
	"sort"

	"github.com/open-enu/kernel"
	"github.com/open-enu/kernel/module"
)

type command struct {
	summary string
	run     func(args []string) int
}

// subcommands is filled by each file's init(); the generated registry is
// what every one of them reads.
var subcommands = map[string]command{}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Println("openenu kernel", kernel.Version)
		return 0
	case "help", "-h", "--help":
		usage()
		return 0
	}
	cmd, ok := subcommands[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "openenu: unknown command %q\n\n", args[0])
		usage()
		return 2
	}
	return cmd.run(args[1:])
}

// registry validates the generated list. Every subcommand goes through it,
// so a broken declaration is reported the same way whether you asked for the
// module table or started the server.
func registry() (*module.Registry, error) {
	return module.NewRegistry(discoveredModules...)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: openenu <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  version    print the kernel version")
	names := make([]string, 0, len(subcommands))
	for n := range subcommands {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(os.Stderr, "  %-10s %s\n", n, subcommands[n].summary)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "user, tenant, apikey, manager and attachment take a verb; run one without arguments for its flags.")
}

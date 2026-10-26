package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/open-enu/kernel"
	kmcp "github.com/open-enu/kernel/mcp"
	"github.com/open-enu/kernel/module"
)

// `openenu mcp` serves the API to an MCP client over stdio, as a client of
// the API with one scoped key. The credential is an argument, never a
// default: a server that fell back to some ambient key would give an agent
// whatever access the operator happened to have, which is the opposite of
// the point. Create a scoped key first (`openenu apikey create`),
// then point a client at:
//
//	{"command": "docker", "args": ["compose", "exec", "-T", "api",
//	 "/openenu", "mcp", "-key", "sk_..."]}
func init() {
	subcommands["mcp"] = command{summary: "serve the API to an MCP client over stdio (-key, -url, -list)", run: runMCP}
}

func runMCP(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	key := fs.String("key", os.Getenv("OPEN_ENU_MCP_KEY"), "API key (or OPEN_ENU_MCP_KEY)")
	url := fs.String("url", envOr("API_URL", "http://localhost:8080"), "API base URL; defaults to this installation's")
	list := fs.Bool("list", false, "print the tools a client would be offered and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	reg, err := registry()
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu mcp:", err)
		return 1
	}
	tools, err := catalogue(reg).Tools()
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu mcp:", err)
		return 1
	}
	if *list {
		for _, t := range tools {
			fmt.Printf("%-32s %s\n", t.Name, t.Description)
		}
		return 0
	}
	if *key == "" {
		// Refused rather than started unauthenticated: a server with no
		// credential is one every call fails through, and the client reports
		// that as "the API is broken".
		fmt.Fprintln(os.Stderr, "openenu mcp: an API key is required: -key sk_... or OPEN_ENU_MCP_KEY")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv := kmcp.Server{Tools: tools, BaseURL: *url, Key: *key, Version: kernel.Version}
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "openenu mcp:", err)
		return 1
	}
	return 0
}

// catalogue is the three sources in one place: the modules' fragments, the
// generated ACL (acl_gen.go) and the registry's permission ownership.
func catalogue(reg *module.Registry) kmcp.Catalogue {
	var fragments [][]byte
	for _, m := range reg.All() {
		if len(m.OpenAPI) > 0 {
			fragments = append(fragments, m.OpenAPI)
		}
	}
	return kmcp.Catalogue{
		Fragments:   fragments,
		Permissions: operationPermissions,
		Declared:    func(p string) bool { _, ok := reg.OwnerOf(p); return ok },
	}
}

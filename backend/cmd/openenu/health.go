package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// `openenu health` is the production container's healthcheck. The prod image
// is distroless - no shell, no curl, no wget - so the probe is the binary
// asking itself. It reads the body, not only the status: a crashed kernel
// still answers something, and a 200 has hidden a fatal error here once.
func init() {
	subcommands["health"] = command{summary: "probe a running api (-url; default from API_ADDR); exit 1 unless the body says ok", run: runHealth}
}

func runHealth(args []string) int {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	url := fs.String("url", healthURL(envOr("API_ADDR", ":8080")), "the health URL to probe")
	want := fs.String("want", `"status":"ok"`, "a fragment the body must contain")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(*url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openenu health: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), *want) {
		fmt.Fprintf(os.Stderr, "openenu health: %s answered %d without %s: %s\n", *url, resp.StatusCode, *want, strings.TrimSpace(string(body)))
		return 1
	}
	fmt.Println(strings.TrimSpace(string(body)))
	return 0
}

// healthURL turns a listen address (":8080", "0.0.0.0:8080") into the
// loopback URL of its shallow health probe.
func healthURL(addr string) string {
	host, port, ok := strings.Cut(addr, ":")
	if !ok {
		port = addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + host + ":" + port + "/health"
}

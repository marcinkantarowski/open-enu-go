package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/open-enu/kernel/heartbeat"
)

// `openenu health` is the production container's healthcheck. The prod image
// is distroless - no shell, no curl, no wget - so the probe is the binary
// asking itself. It reads the body, not only the status: a crashed kernel
// still answers something, and a 200 has hidden a fatal error here once.
//
// The worker and the scheduler serve no HTTP; they touch a heartbeat file
// (kernel/heartbeat, HEARTBEAT_FILE) while healthy, and `-heartbeat <path>`
// probes that instead: exit 0 when the file is younger than -max-age.
func init() {
	subcommands["health"] = command{summary: "probe a running api (-url; default from API_ADDR), or a worker/scheduler (-heartbeat <file> [-max-age 2m]); exit 1 unless healthy", run: runHealth}
}

func runHealth(args []string) int {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	url := fs.String("url", healthURL(envOr("API_ADDR", ":8080")), "the health URL to probe")
	want := fs.String("want", `"status":"ok"`, "a fragment the body must contain")
	beat := fs.String("heartbeat", "", "probe this heartbeat file instead of the URL (worker, scheduler)")
	maxAge := fs.Duration("max-age", heartbeat.DefaultMaxAge, "with -heartbeat: the oldest beat that still counts as alive")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *beat != "" {
		return checkHeartbeat(*beat, *maxAge, time.Now())
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

// checkHeartbeat prints one line and answers the exit code.
func checkHeartbeat(path string, maxAge time.Duration, now time.Time) int {
	age, err := heartbeat.Check(path, maxAge, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openenu health: %v\n", err)
		return 1
	}
	fmt.Printf("ok: %s beat %s ago\n", path, age.Round(time.Second))
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

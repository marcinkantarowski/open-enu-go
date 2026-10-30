// Package heartbeat is the liveness signal of a process that serves no HTTP:
// the worker and the scheduler touch a file while they are healthy, and
// `openenu health -heartbeat <path>` - the container's healthcheck, since
// the production image has no shell - reports how old it is.
//
// A file and not a port: the probe needs no listener in a process that has
// no other reason to open one, and a stuck loop stops touching it, which is
// exactly the failure a healthcheck exists to see.
package heartbeat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

const (
	// Env names the file; DefaultPath is used when it is unset.
	Env = "HEARTBEAT_FILE"
	// DefaultPath is writable by the distroless nonroot user.
	DefaultPath = "/tmp/openenu-heartbeat"
	// Interval is how often a healthy process touches the file.
	Interval = 30 * time.Second
	// DefaultMaxAge is how old the file may be before the process counts as
	// dead: four missed beats.
	DefaultMaxAge = 2 * time.Minute
)

// Path is HEARTBEAT_FILE, or DefaultPath.
func Path() string {
	if p := os.Getenv(Env); p != "" {
		return p
	}
	return DefaultPath
}

// Touch creates the file if it is missing and sets its modification time
// to now.
func Touch(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	return nil
}

// Run touches path now and then every `every` while healthy answers nil,
// until ctx ends. An unhealthy beat is skipped and logged, so the file goes
// stale and the healthcheck says so; healthy nil means "alive is healthy".
func Run(ctx context.Context, path string, every time.Duration, healthy func(context.Context) error, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	beat := func() {
		if healthy != nil {
			if err := healthy(ctx); err != nil {
				log.WarnContext(ctx, "heartbeat skipped - not healthy", "error", err.Error(), "file", path)
				return
			}
		}
		if err := Touch(path); err != nil {
			log.ErrorContext(ctx, "heartbeat not written", "error", err.Error())
		}
	}
	beat()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			beat()
		}
	}
}

// ErrStale is returned by Check for a file older than the allowed age.
var ErrStale = errors.New("heartbeat: stale")

// Check reports the file's age at now, and an error when it is missing or
// older than maxAge.
func Check(path string, maxAge time.Duration, now time.Time) (time.Duration, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("heartbeat: %w", err)
	}
	age := now.Sub(st.ModTime())
	if age > maxAge {
		return age, fmt.Errorf("%w: %s last beat %s ago, more than %s", ErrStale, path, age.Round(time.Second), maxAge)
	}
	return age, nil
}

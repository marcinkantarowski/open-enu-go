package heartbeat

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckFreshStaleAndMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "beat")
	if _, err := Check(path, DefaultMaxAge, time.Now()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file must fail as missing: %v", err)
	}
	if err := Touch(path); err != nil {
		t.Fatal(err)
	}
	if age, err := Check(path, DefaultMaxAge, time.Now()); err != nil || age > time.Minute {
		t.Fatalf("fresh: %s %v", age, err)
	}
	old := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(path, DefaultMaxAge, time.Now()); !errors.Is(err, ErrStale) || !strings.Contains(err.Error(), path) {
		t.Fatalf("stale must fail naming the file: %v", err)
	}
	if err := Touch(path); err != nil { // an existing file is touched, not refused
		t.Fatal(err)
	}
	if _, err := Check(path, DefaultMaxAge, time.Now()); err != nil {
		t.Fatalf("touched again: %v", err)
	}
}

// Run beats once before it waits for the first tick, so a cancelled
// context shows exactly one beat - or none, when the process is not healthy.
func TestRunBeatsOnlyWhileHealthy(t *testing.T) {
	dir := t.TempDir()
	sick := filepath.Join(dir, "sick")
	Run(cancelled(), sick, time.Hour, func(context.Context) error { return errors.New("db down") }, nil)
	if _, err := os.Stat(sick); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an unhealthy process must not beat: %v", err)
	}
	well := filepath.Join(dir, "well")
	Run(cancelled(), well, time.Hour, func(context.Context) error { return nil }, nil)
	if _, err := Check(well, DefaultMaxAge, time.Now()); err != nil {
		t.Fatalf("a healthy process beats at once: %v", err)
	}
}

func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

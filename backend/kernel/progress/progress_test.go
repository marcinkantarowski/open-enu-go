package progress

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestRecorderFollowsAJob(t *testing.T) {
	ctx := context.Background()
	var r Recorder
	id, _ := r.Start(ctx, "example.archive", 3, "")
	if len(id) != 26 {
		t.Fatalf("id is 26 chars of base32, got %q", id)
	}
	_ = r.Advance(ctx, id, 2, "halfway")
	j, ok := r.Get(id)
	if !ok || j.Done != 2 || j.Label != "halfway" || j.Status != StatusRunning || j.Total != 3 {
		t.Fatalf("got %+v", j)
	}
	_ = r.Finish(ctx, id, map[string]any{"archived": 2})
	if j, _ := r.Get(id); j.Status != StatusDone || j.Result["archived"] != 2 {
		t.Fatalf("got %+v", j)
	}
	id2, _ := r.Start(ctx, "x.y", 1, "")
	_ = r.Fail(ctx, id2, "disk full")
	if j, _ := r.Get(id2); j.Status != StatusFailed || j.Reason != "disk full" {
		t.Fatalf("got %+v", j)
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("unknown id")
	}
}

func TestLoggingWritesTheCalls(t *testing.T) {
	var buf bytes.Buffer
	l := Logging{Log: slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	ctx := context.Background()
	id, err := l.Start(ctx, "k", 2, "l")
	if err != nil || id == "" {
		t.Fatal(err)
	}
	_ = l.Advance(ctx, id, 1, "")
	_ = l.Finish(ctx, id, nil)
	_ = l.Fail(ctx, id, "r")
	for _, want := range []string{"progress.start", "progress.advance", "progress.finish", "progress.fail", `"job":"` + id + `"`, `"total":2`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %s in\n%s", want, buf.String())
		}
	}
}

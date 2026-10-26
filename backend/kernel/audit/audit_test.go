package audit

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestSlogWritesEveryField(t *testing.T) {
	var buf bytes.Buffer
	l := Slog{Log: slog.New(slog.NewJSONHandler(&buf, nil))}
	l.Record(context.Background(), Entry{
		Action: "demo.thing.rename", SubjectID: "s1", ActorID: "u1", OnBehalfOf: "op1", TenantID: "t1",
		RequestID: "r1", Succeeded: true, Before: map[string]any{"name": "a"}, After: map[string]any{"name": "b"},
		Context: map[string]any{"duration_ms": 3},
	})
	out := buf.String()
	for _, want := range []string{`"msg":"audit"`, `"action":"demo.thing.rename"`, `"on_behalf_of":"op1"`, `"before":{"name":"a"}`, `"after":{"name":"b"}`, `"duration_ms":3`, `"request_id":"r1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log line lacks %s:\n%s", want, out)
		}
	}
}

// panicHandler is a slog.Handler that panics, standing in for a sink that
// blows up: the entry is lost, the caller is not.
type panicHandler struct{ slog.Handler }

func (panicHandler) Handle(context.Context, slog.Record) error { panic("sink down") }

func TestSlogNeverPanics(t *testing.T) {
	l := Slog{Log: slog.New(panicHandler{slog.NewTextHandler(&bytes.Buffer{}, nil)})}
	l.Record(context.Background(), Entry{Action: "x.y"}) // must return
}

func TestFailedAndRecorder(t *testing.T) {
	e := Entry{Action: "a.b", Succeeded: true}
	f := e.Failed("boom")
	if f.Succeeded || f.FailureReason != "boom" || !e.Succeeded {
		t.Fatalf("Failed must copy: %+v / %+v", e, f)
	}
	var r Recorder
	r.Record(context.Background(), e)
	r.Record(context.Background(), f)
	if got := r.Entries(); len(got) != 2 || got[1].FailureReason != "boom" {
		t.Fatalf("got %+v", got)
	}
	r.Reset()
	if len(r.Entries()) != 0 {
		t.Fatal("reset")
	}
	if (NoActor{}).Actor(context.Background()) != (Actor{}) {
		t.Fatal("NoActor is nobody")
	}
}

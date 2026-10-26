package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/scope"
)

type created struct{ id string }

func (c created) EventName() string          { return "demo.thing.created" }
func (c created) SubjectID() string          { return c.id }
func (c created) OccurredAt() time.Time      { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) }
func (c created) Payload() map[string]any    { return map[string]any{"id": c.id, "name": "x"} }
func (c created) RequiresPermission() string { return "" }

type internalOnly struct{ created }

func (internalOnly) RequiresPermission() string { panic("not a Broadcast") }

type gated struct{ created }

func (gated) RequiresPermission() string { return "demo.manage" }

func TestEnvelopeIsWhatTheBrowserReads(t *testing.T) {
	data, _ := json.Marshal(NewEnvelope(created{"s1"}))
	want := `{"event":"demo.thing.created","subjectId":"s1","occurredAt":"2026-10-22T12:00:00Z","payload":{"id":"s1","name":"x"},"requiresPermission":null}`
	if string(data) != want {
		t.Fatalf("got  %s\nwant %s", data, want)
	}
	if e := NewEnvelope(gated{created{"s2"}}); e.RequiresPermission == nil || *e.RequiresPermission != "demo.manage" {
		t.Fatalf("permission not carried: %+v", e)
	}
	if !ValidName("demo.thing.created") || ValidName("Created") || ValidName("demo") {
		t.Fatal("name shape")
	}
}

func TestBroadcasterPublishesOnTheTenantTopicOnly(t *testing.T) {
	var hub Recorder
	var buf bytes.Buffer
	b := Broadcaster{Hub: &hub, Log: slog.New(slog.NewJSONHandler(&buf, nil))}

	b.Handle(context.Background(), created{"s1"}) // no tenant: dropped, warned
	if len(hub.Updates()) != 0 || !strings.Contains(buf.String(), "broadcast.skipped_no_tenant") {
		t.Fatalf("no tenant must drop with a warning; updates=%d log=%s", len(hub.Updates()), buf.String())
	}

	ctx := scope.WithTenant(context.Background(), "t1")
	b.Handle(ctx, created{"s1"})
	ups := hub.Updates()
	if len(ups) != 1 || ups[0].Topic != "/tenants/t1/events" || !strings.Contains(string(ups[0].Data), `"event":"demo.thing.created"`) {
		t.Fatalf("got %+v", ups)
	}

	// A plain Event that happens to have the method but is not asserted as
	// Broadcast... is a Broadcast: the interface is structural. What is NOT
	// published is a type without the method at all.
	type plain struct{ Event }
	b.Handle(ctx, plain{created{"s3"}})
	if len(hub.Updates()) != 1 {
		t.Fatal("a non-broadcast event must not be published")
	}

	buf.Reset()
	hub.Err = errors.New("hub down")
	b.Handle(ctx, created{"s4"}) // must not panic or return the error
	if !strings.Contains(buf.String(), "broadcast.failed") || !strings.Contains(buf.String(), "hub down") {
		t.Fatalf("a publish failure is logged: %s", buf.String())
	}
}

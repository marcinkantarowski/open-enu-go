package logctx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/logctx"
	"github.com/open-enu/kernel/scope"
)

func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("not JSON: %q", l)
		}
		out = append(out, m)
	}
	return out
}

func TestALineCarriesWhatItsContextKnows(t *testing.T) {
	var buf bytes.Buffer
	extra := func(context.Context) []slog.Attr { return []slog.Attr{slog.String("shard", "eu-1")} }
	log := slog.New(logctx.Handler(slog.NewJSONHandler(&buf, nil), append(logctx.Standard(), extra)...))

	ctx := httpx.WithRequestID(context.Background(), "req-1")
	ctx = scope.WithTenant(ctx, "tenant-1")
	ctx = auth.WithPrincipal(ctx, auth.Principal{Subject: "user-1", Realm: auth.RealmTenant, Impersonator: "op-1"})

	log.InfoContext(ctx, "with everything")
	log.InfoContext(ctx, "the caller's own value wins", "request_id", "mine")
	log.With("tenant_id", "preset").InfoContext(ctx, "so does With's")
	log.InfoContext(auth.WithPrincipal(context.Background(), auth.Principal{Subject: "key-1", APIKey: true, Realm: auth.RealmTenant}), "a machine")
	log.InfoContext(auth.WithPrincipal(context.Background(), auth.Principal{Subject: "op-2", Realm: auth.RealmManager}), "an operator")
	log.InfoContext(context.Background(), "nothing known")

	got := lines(t, &buf)
	want := map[string]any{"request_id": "req-1", "tenant_id": "tenant-1", "user_id": "user-1", "impersonator_id": "op-1", "shard": "eu-1"}
	for k, v := range want {
		if got[0][k] != v {
			t.Errorf("line 1 %s = %v, want %v (%v)", k, got[0][k], v, got[0])
		}
	}
	if got[1]["request_id"] != "mine" || strings.Count(buf.String(), `"request_id"`) != 3 {
		t.Errorf("a key the line has is not added again: %v", got[1])
	}
	if got[2]["tenant_id"] != "preset" || strings.Count(strings.Split(buf.String(), "\n")[2], `"tenant_id"`) != 1 {
		t.Errorf("With's key is not added again: %v", got[2])
	}
	if got[3]["api_key_id"] != "key-1" || got[3]["user_id"] != nil {
		t.Errorf("a key is not a person: %v", got[3])
	}
	if got[4]["operator_id"] != "op-2" || got[4]["user_id"] != nil {
		t.Errorf("an operator is not a person: %v", got[4])
	}
	for _, k := range []string{"request_id", "tenant_id", "user_id", "command"} {
		if _, has := got[5][k]; has {
			t.Errorf("an empty context adds %s: %v", k, got[5])
		}
	}
}

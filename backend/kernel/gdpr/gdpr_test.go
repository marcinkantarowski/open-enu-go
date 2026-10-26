package gdpr

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/open-enu/kernel/audit"
)

type subj struct {
	name  string
	order *[]string
	fail  bool
}

func (s subj) ExportFor(_ context.Context, userID string) (map[string][]map[string]any, error) {
	if s.fail {
		return nil, errors.New("timeout")
	}
	return map[string][]map[string]any{"profile": {{"user": userID, "from": s.name}}}, nil
}

func (s subj) EraseFor(_ context.Context, _ string) (int, error) {
	*s.order = append(*s.order, s.name)
	if s.fail {
		return 0, errors.New("locked")
	}
	return len(s.name), nil
}

func TestExportNamespacesAndAudits(t *testing.T) {
	var order []string
	var trail audit.Recorder
	// Two registrations of the same module name collide on `identity.profile`
	// and must both survive.
	w := NewWalker(&trail, Registered{"identity", subj{"identity", &order, false}}, Registered{"identity", subj{"identity2", &order, false}})
	bundle, err := w.Export(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle) != 2 || bundle["identity.profile"] == nil || bundle["identity.profile#2"] == nil {
		t.Fatalf("got %v", bundle)
	}
	if e := trail.Entries(); len(e) != 1 || e[0].Action != "gdpr.subject.exported" || e[0].SubjectID != "u1" || e[0].Context["sections"] != 2 {
		t.Fatalf("audit: %+v", e)
	}
	w = NewWalker(&trail, Registered{"x", subj{"x", &order, true}})
	if _, err := w.Export(context.Background(), "u1"); err == nil || !strings.Contains(err.Error(), `module "x"`) {
		t.Fatalf("names the module: %v", err)
	}
}

func TestEraseRunsInReverseAndStops(t *testing.T) {
	var order []string
	var trail audit.Recorder
	w := NewWalker(&trail, Registered{"tenant", subj{"tenant", &order, false}}, Registered{"identity", subj{"identity", &order, false}}, Registered{"example", subj{"example", &order, false}})
	affected, err := w.Erase(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, ","); got != "example,identity,tenant" {
		t.Fatalf("reverse order, got %s", got)
	}
	if affected["example"] != 7 || affected["tenant"] != 6 {
		t.Fatalf("got %v", affected)
	}
	if e := trail.Entries(); len(e) != 1 || e[0].Action != "gdpr.subject.erased" || e[0].Context["rows"] != 7+8+6 {
		t.Fatalf("audit: %+v", e)
	}
	order = nil
	w = NewWalker(&trail, Registered{"a", subj{"a", &order, false}}, Registered{"b", subj{"b", &order, true}}, Registered{"c", subj{"c", &order, false}})
	if _, err := w.Erase(context.Background(), "u1"); err == nil || !strings.Contains(err.Error(), `module "b"`) {
		t.Fatalf("names the module: %v", err)
	}
	if got := strings.Join(order, ","); got != "c,b" {
		t.Fatalf("stopped at b, got %s", got)
	}
}

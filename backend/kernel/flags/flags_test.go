package flags

import (
	"context"
	"strings"
	"testing"

	"github.com/open-enu/kernel/scope"
)

func TestStaticCoercion(t *testing.T) {
	ctx := context.Background()
	s := Static{"a": true, "b": "yes", "c": "off", "d": 2, "e": 0, "f": "7", "g": map[string]any{"k": 1}, "h": 2.0, "i": "-3"}
	cases := map[string]bool{"a": true, "b": true, "c": false, "d": true, "e": false, "missing": true}
	for id, want := range cases {
		if got := s.Enabled(ctx, id, true); got != want {
			t.Errorf("Enabled(%s) = %v, want %v", id, got, want)
		}
	}
	if s.Int(ctx, "f", 0) != 7 || s.Int(ctx, "d", 0) != 2 || s.Int(ctx, "h", 0) != 2 || s.Int(ctx, "i", 9) != 9 || s.Int(ctx, "b", 9) != 9 {
		t.Error("Int coercion: digits-only strings and integral numbers, nothing else")
	}
	if s.String(ctx, "b", "") != "yes" || s.String(ctx, "d", "dflt") != "dflt" {
		t.Error("String")
	}
	if s.JSON(ctx, "g", nil)["k"] != 1 || s.JSON(ctx, "a", map[string]any{"z": 0})["z"] != 0 {
		t.Error("JSON")
	}
}

func TestResolverDefaultsThenTenantOverride(t *testing.T) {
	store := &MemoryStore{}
	r, err := NewResolver(store,
		Definition{Identifier: "example.archive", Name: "Archive", Default: false},
		Definition{Identifier: "example.page_size", Default: 20, Type: TypeInt},
	)
	if err != nil {
		t.Fatal(err)
	}
	bare := context.Background()
	t1 := scope.WithTenant(bare, "t1")
	t2 := scope.WithTenant(bare, "t2")
	if r.Enabled(t1, "example.archive", true) {
		t.Fatal("declared default (false) beats the call's default")
	}
	if r.Enabled(t1, "example.undeclared", true) != true {
		t.Fatal("undeclared resolves to the call's default")
	}
	store.Set("t1", "example.archive", "on")
	if !r.Enabled(t1, "example.archive", false) || r.Enabled(t2, "example.archive", false) || r.Enabled(bare, "example.archive", false) {
		t.Fatal("an override applies to its tenant only; no tenant means the default")
	}
	store.Set("t2", "example.page_size", "50")
	if r.Int(t2, "example.page_size", 0) != 50 || r.Int(t1, "example.page_size", 0) != 20 {
		t.Fatal("int override")
	}
	store.Unset("t1", "example.archive")
	if r.Enabled(t1, "example.archive", false) {
		t.Fatal("unset restores the default")
	}
	if len(r.Definitions()) != 2 {
		t.Fatal("definitions")
	}
}

func TestResolverRefusesBadDeclarations(t *testing.T) {
	if _, err := NewResolver(nil, Definition{Identifier: "Archive"}); err == nil || !strings.Contains(err.Error(), "dotted lowercase") {
		t.Fatalf("shape: %v", err)
	}
	if _, err := NewResolver(nil, Definition{Identifier: "a.b", Type: "float"}); err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("type: %v", err)
	}
	if _, err := NewResolver(nil, Definition{Identifier: "a.b"}, Definition{Identifier: "a.b"}); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("dup: %v", err)
	}
	var _ Flags = (*Resolver)(nil)
	var _ Flags = Static(nil)
}

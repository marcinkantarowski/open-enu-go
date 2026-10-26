package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/open-enu/kernel/scope"
)

func counter() (func(context.Context) ([]byte, error), *int) {
	n := 0
	return func(context.Context) ([]byte, error) { n++; return []byte{byte(n)}, nil }, &n
}

func TestTenantsNeverShareAnEntry(t *testing.T) {
	mem := &Memory{}
	c := &Tenant{Backend: mem}
	a := scope.WithTenant(context.Background(), "a")
	b := scope.WithTenant(context.Background(), "b")
	compute, calls := counter()

	v1, _ := c.Get(a, "demo", "totals", 0, compute)
	v2, _ := c.Get(a, "demo", "totals", 0, compute)
	v3, _ := c.Get(b, "demo", "totals", 0, compute)
	v4, _ := c.Get(context.Background(), "demo", "totals", 0, compute)
	if *calls != 3 || v1[0] != 1 || v2[0] != 1 || v3[0] != 2 || v4[0] != 3 {
		t.Fatalf("same name, three tenants (a, b, global) = three computations: calls=%d %v %v %v %v", *calls, v1, v2, v3, v4)
	}
	if mem.Len() != 3 {
		t.Fatalf("entries: %d", mem.Len())
	}
	_ = c.InvalidateTenant(context.Background(), "a")
	if mem.Len() != 2 {
		t.Fatal("tenant invalidation drops a's entry only")
	}
	if _, _ = c.Get(a, "demo", "totals", 0, compute); *calls != 4 {
		t.Fatal("recomputed after invalidation")
	}
	_ = c.InvalidateModule(context.Background(), "demo")
	if mem.Len() != 0 {
		t.Fatal("module invalidation drops every tenant's entries of that module")
	}
}

func TestTTLAndDeleteAndTags(t *testing.T) {
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	mem := &Memory{Now: func() time.Time { return now }}
	c := &Tenant{Backend: mem}
	ctx := scope.WithTenant(context.Background(), "a")
	compute, calls := counter()
	_, _ = c.Get(ctx, "demo", "k", time.Minute, compute, "list:projects")
	_, _ = c.Get(ctx, "demo", "k", time.Minute, compute)
	if *calls != 1 {
		t.Fatal("hit within ttl")
	}
	now = now.Add(61 * time.Second)
	_, _ = c.Get(ctx, "demo", "k", time.Minute, compute)
	if *calls != 2 {
		t.Fatal("miss after ttl")
	}
	if err := mem.InvalidateTags(ctx, "list.projects"); err != nil {
		t.Fatal(err)
	}
	// The second Set carried no extra tag, so the entry survives.
	if mem.Len() != 1 {
		t.Fatal("only tagged entries are dropped")
	}
	_, _ = c.Get(ctx, "demo", "k2", 0, compute, "list:projects")
	_ = mem.InvalidateTags(ctx, "list.projects")
	if mem.Len() != 1 {
		t.Fatal("sanitised extra tag drops the entry")
	}
	_ = c.Delete(ctx, "demo", "k")
	if mem.Len() != 0 {
		t.Fatal("delete")
	}
	if SanitiseTag("a:b/c@d") != "a.b.c.d" || ModuleTag("demo") != "module.demo" || TenantTag("t:1") != "tenant.t.1" {
		t.Fatal("tags")
	}
}

func TestGetJSONAndErrors(t *testing.T) {
	c := &Tenant{Backend: &Memory{}}
	ctx := scope.WithTenant(context.Background(), "a")
	type totals struct{ Open, Closed int }
	n := 0
	compute := func(context.Context) (totals, error) { n++; return totals{Open: 3, Closed: n}, nil }
	v, err := GetJSON(ctx, c, "demo", "totals", 0, compute)
	if err != nil || v.Open != 3 || v.Closed != 1 {
		t.Fatalf("%+v %v", v, err)
	}
	v, _ = GetJSON(ctx, c, "demo", "totals", 0, compute)
	if v.Closed != 1 {
		t.Fatal("served from cache")
	}
	boom := errors.New("db down")
	if _, err := c.Get(ctx, "demo", "x", 0, func(context.Context) ([]byte, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("compute error is returned, nothing cached: %v", err)
	}
	if _, ok, _ := c.Backend.Get(ctx, "a.demo.x"); ok {
		t.Fatal("a failed computation must not be cached")
	}
	if _, _, err := c.Backend.Get(ctx, "a.demo.weird_key_"); err != nil {
		t.Fatal(err)
	}
	_, _ = c.Get(ctx, "demo", "weird key!", 0, func(context.Context) ([]byte, error) { return []byte("v"), nil })
	if v, ok, _ := c.Backend.Get(ctx, "a.demo.weird_key_"); !ok || string(v) != "v" {
		t.Fatal("keys are sanitised to [A-Za-z0-9_.]")
	}
}

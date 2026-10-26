package search

import (
	"context"
	"strings"
	"testing"

	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"
)

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

func TestSearchIsScopedAndRanked(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil); err != nil {
		t.Fatal(err)
	}
	pool := &scope.Pool{Pool: db.App}
	x := Indexer{}
	index := func(ctx context.Context, tenant, id string, fields ...string) error {
		return pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			return x.Index(ctx, tx, "example.project", id, fields, tenant)
		})
	}
	a, b := scope.WithTenant(ctx, tenantA), scope.WithTenant(ctx, tenantB)
	for _, step := range []struct {
		ctx          context.Context
		tenant, id   string
		fields       []string
		wantContains string
	}{
		{a, tenantA, "p1", []string{"Quarterly invoice run", "finance"}, ""},
		{a, tenantA, "p2", []string{"Invoice template redesign", ""}, ""},
		{b, tenantB, "p3", []string{"Invoice archive for tenant b"}, ""},
		{a, tenantB, "smuggled", []string{"invoice"}, "row-level security"},
	} {
		err := index(step.ctx, step.tenant, step.id, step.fields...)
		if step.wantContains == "" && err != nil {
			t.Fatalf("%s: %v", step.id, err)
		}
		if step.wantContains != "" && (err == nil || !strings.Contains(err.Error(), step.wantContains)) {
			t.Fatalf("%s: indexing another tenant's record from A's scope must be refused, got %v", step.id, err)
		}
	}
	search := func(ctx context.Context, q string, types ...string) []Hit {
		var hits []Hit
		if err := pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			var err error
			hits, err = x.Search(ctx, tx, q, types, 20)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return hits
	}
	hits := search(a, "invoice")
	if len(hits) != 2 || hits[0].EntityType != "example.project" || !strings.Contains(strings.ToLower(hits[0].Excerpt), "invoice") {
		t.Fatalf("A sees its two: %+v", hits)
	}
	if hits := search(b, "invoice"); len(hits) != 1 || hits[0].EntityID != "p3" {
		t.Fatalf("B sees its one: %+v", hits)
	}
	if hits := search(ctx, "invoice"); len(hits) != 0 {
		t.Fatalf("no scope, no hits: %+v", hits)
	}
	if hits := search(a, "invoice -template"); len(hits) != 1 || hits[0].EntityID != "p1" {
		t.Fatalf("websearch syntax: %+v", hits)
	}
	if hits := search(a, "invoice", "other.type"); len(hits) != 0 {
		t.Fatal("type filter")
	}
	if hits := search(a, "   "); hits != nil {
		t.Fatal("empty query is no query")
	}
	// Re-indexing with empty text removes the record.
	if err := index(a, tenantA, "p2", "", " "); err != nil {
		t.Fatal(err)
	}
	if hits := search(a, "invoice"); len(hits) != 1 || hits[0].EntityID != "p1" {
		t.Fatalf("p2 removed: %+v", hits)
	}
}

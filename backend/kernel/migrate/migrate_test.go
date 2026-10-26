package migrate

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/open-enu/kernel/testdb"
)

func TestRunAppliesRiverKernelAndModulesOnceEach(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	demo := Source{Name: "demo", FS: fstest.MapFS{"0001_thing.sql": {Data: []byte("-- +goose Up\nCREATE TABLE demo_thing (id int);\n-- +goose Down\nDROP TABLE demo_thing;\n")}}}

	applied, err := Run(ctx, db.Admin, nil, demo, Source{Name: "notables"})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, a := range applied {
		seen[a.Source]++
	}
	if seen["river"] == 0 || seen["kernel"] != 3 || seen["demo"] != 1 {
		t.Fatalf("applied %v", seen)
	}
	for _, table := range []string{"river_job", "audit_entry", "search_index", "demo_thing", "goose_kernel", "goose_demo"} {
		var n int
		if err := db.Admin.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_name = $1", table).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s: %d %v", table, n, err)
		}
	}
	// The application role can use what was created (default privileges).
	if _, err := db.App.Exec(ctx, "INSERT INTO demo_thing VALUES (1)"); err != nil {
		t.Fatalf("app role cannot write a migrated table: %v", err)
	}
	again, err := Run(ctx, db.Admin, nil, demo)
	if err != nil || len(again) != 0 {
		t.Fatalf("second run is a no-op: %v %v", again, err)
	}
}

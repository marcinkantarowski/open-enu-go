package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/testdb"
)

// The binary's composition against a real database: the env is read, the
// schema is applied, the platform validates, deep health sees Postgres.
func TestServicesFromEnvAgainstPostgres(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", db.AppDSN)
	t.Setenv("APP_ENCRYPTION_KEY", platform.TestKey)
	t.Setenv("APP_SECRET", platform.TestKey)
	t.Setenv("STORAGE_ROOT", t.TempDir())
	t.Setenv("REDIS_ADDR", "")
	log := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	reg, err := registry()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := newServices(ctx, log, reg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.DB.Close()
	h, err := newHandler(reg, svc, log)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/health/deep", nil))
	var body struct {
		Status string `json:"status"`
		Checks map[string]struct {
			OK bool `json:"ok"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 || body.Status != "ok" || !body.Checks["postgres"].OK {
		t.Fatalf("/health/deep = %d %s", rec.Code, rec.Body.String())
	}

	t.Setenv("DATABASE_URL", "")
	if _, err := newServices(ctx, log, reg); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("no database, no start: %v", err)
	}
}

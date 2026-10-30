package scope

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/open-enu/kernel/testdb"
)

// The application's pool must be the one role Postgres holds to the
// policies: the container's superuser and a role with BYPASSRLS are refused
// with the role named, the application role is accepted.
func TestVerifyApplicationRoleRefusesEveryRoleThatBypassesRLS(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()

	if err := VerifyApplicationRole(ctx, &Pool{Pool: db.App}); err != nil {
		t.Fatalf("the application role must be accepted: %v", err)
	}

	err := VerifyApplicationRole(ctx, &Pool{Pool: db.Admin})
	if !errors.Is(err, ErrPrivilegedRole) || !strings.Contains(err.Error(), "superuser=true") || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("the superuser must be refused with the fix named, got %v", err)
	}

	role := "t_bypass_" + strings.ToLower(rand.Text()[:8])
	if _, err := db.Admin.Exec(ctx, "CREATE ROLE "+role+" LOGIN PASSWORD 'bypass' NOSUPERUSER BYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Admin.Exec(context.Background(), "DROP ROLE IF EXISTS "+role) })
	cfg, err := pgxpool.ParseConfig(db.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User, cfg.ConnConfig.Password = role, "bypass"
	bypass, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer bypass.Close()
	err = VerifyApplicationRole(ctx, &Pool{Pool: bypass})
	if !errors.Is(err, ErrPrivilegedRole) || !strings.Contains(err.Error(), role) || !strings.Contains(err.Error(), "bypassrls=true") {
		t.Fatalf("a BYPASSRLS role must be refused by name, got %v", err)
	}
}

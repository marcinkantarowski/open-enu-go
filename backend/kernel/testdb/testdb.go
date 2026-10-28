// Package testdb starts the real backing services for a test: one Postgres
// and one Redis, through testcontainers, shared by every test binary of a
// `go test ./...` run (reused by name) and removed by the reaper afterwards.
// Two runs back to back - the kernel's then the app's, as `make test` does -
// are two reaper sessions, and the first removes the containers while the
// second is using them; `make test` runs with the reaper off and removes
// them itself. A lone `go test` of one module needs nothing.
//
// Tests here never mock Postgres: the questions the kernel asks - does the
// RLS policy hide the other tenant, does SET LOCAL leak across a pooled
// connection, does a rollback take the outbox row with it - are questions
// about Postgres. One container for the duration of the run is the price.
//
// Each test gets its own database, created by the container's superuser and
// then used through a NON-superuser role: a superuser bypasses row-level
// security no matter what, so a test connected as one would prove nothing
// about isolation. The dev and prod stacks connect as the same kind of role.
package testdb

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// AppRole is the non-superuser role the application connects as. It has no
// BYPASSRLS, so FORCE ROW LEVEL SECURITY applies to everything it does.
const AppRole = "open_enu_app"

const appPassword = "app"

// DB is one test's database: Admin owns it and runs migrations; App is what
// the code under test uses.
type DB struct {
	Admin *pgxpool.Pool
	App   *pgxpool.Pool
	// AppDSN connects as AppRole - for code that builds its own pool.
	AppDSN string
	// AdminDSN connects as the superuser - for migrations.
	AdminDSN string
}

var (
	pgOnce sync.Once
	pgDSN  string // superuser, database "postgres"
	pgErr  error

	redisOnce sync.Once
	redisAddr string
	redisErr  error
)

// Postgres returns a fresh database on the shared container, with AppRole in
// place and granted on it. Under -short (`make check`, the inner loop) the
// test is skipped, visibly; without it (`make test`, `make ci`) a missing
// Docker FAILS the test - a green gate with the isolation proofs skipped is
// the worst outcome available.
func Postgres(t testing.TB) *DB {
	t.Helper()
	if testing.Short() {
		t.Skip("needs Postgres through testcontainers: runs in `make test` and `make ci`, not under -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pgOnce.Do(func() { pgDSN, pgErr = startPostgres(ctx) })
	if pgErr != nil {
		t.Fatalf("testdb: postgres container: %v (is Docker running?)", pgErr)
	}

	root, err := pgxpool.New(ctx, pgDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	name := "t_" + strings.ToLower(rand.Text()[:12])
	if _, err := root.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	// Roles are cluster-wide; several test binaries race to create it.
	_, _ = root.Exec(ctx, fmt.Sprintf(`DO $$ BEGIN
		IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '%s') THEN
			CREATE ROLE %s LOGIN PASSWORD '%s' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
		END IF; END $$`, AppRole, AppRole, appPassword))

	adminDSN := replaceDB(pgDSN, name)
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"GRANT ALL ON SCHEMA public TO " + AppRole,
		"GRANT ALL ON ALL TABLES IN SCHEMA public TO " + AppRole,
		"GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO " + AppRole,
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO " + AppRole,
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO " + AppRole,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("testdb: %s: %v", q, err)
		}
	}
	appDSN := replaceUser(adminDSN, AppRole, appPassword)
	app, err := pgxpool.New(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		app.Close()
		admin.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if root, err := pgxpool.New(ctx, pgDSN); err == nil {
			_, _ = root.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			root.Close()
		}
	})
	return &DB{Admin: admin, App: app, AppDSN: appDSN, AdminDSN: adminDSN}
}

// Redis returns the address (host:port) of the shared Redis container, with
// the database flushed for this test. Tests sharing the container must not
// run in parallel within a binary; across binaries each uses its own key
// prefix.
func Redis(t testing.TB) string {
	t.Helper()
	if testing.Short() {
		t.Skip("needs Redis through testcontainers: runs in `make test` and `make ci`, not under -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	redisOnce.Do(func() { redisAddr, redisErr = startRedis(ctx) })
	if redisErr != nil {
		t.Fatalf("testdb: redis container: %v (is Docker running?)", redisErr)
	}
	return redisAddr
}

func startPostgres(ctx context.Context) (string, error) {
	c, err := postgres.Run(ctx, envOr("TESTDB_POSTGRES_IMAGE", "postgres:18-alpine"),
		postgres.WithDatabase("postgres"), postgres.WithUsername("postgres"), postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithReuseByName(containerName("postgres")),
	)
	if err != nil {
		return "", err
	}
	return c.ConnectionString(ctx, "sslmode=disable")
}

func startRedis(ctx context.Context) (string, error) {
	c, err := tcredis.Run(ctx, envOr("TESTDB_REDIS_IMAGE", "redis:8.10-alpine"),
		testcontainers.WithReuseByName(containerName("redis")))
	if err != nil {
		return "", err
	}
	s, err := c.ConnectionString(ctx) // redis://host:port
	return strings.TrimPrefix(s, "redis://"), err
}

// containerName is the name a container is reused by. Projects sharing a
// Docker host must not share it: one run's cleanup would remove the container
// another run is using, so `make` sets TESTDB_CONTAINER_PREFIX to the slug.
func containerName(service string) string {
	return envOr("TESTDB_CONTAINER_PREFIX", "open-enu-go") + "-test-" + service
}

func envOr(env, def string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return def
}

// replaceDB swaps the database name in a postgres:// URL.
func replaceDB(dsn, db string) string {
	i := strings.LastIndex(dsn, "/")
	j := strings.Index(dsn[i:], "?")
	if j < 0 {
		return dsn[:i+1] + db
	}
	return dsn[:i+1] + db + dsn[i+j:]
}

// replaceUser swaps the credentials in a postgres://user:pass@host URL.
func replaceUser(dsn, user, pass string) string {
	scheme, rest, _ := strings.Cut(dsn, "://")
	_, host, _ := strings.Cut(rest, "@")
	return scheme + "://" + user + ":" + pass + "@" + host
}

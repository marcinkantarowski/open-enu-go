package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/open-enu/kernel/migrate"
)

func init() {
	subcommands["migrate"] = command{summary: "apply River's, the kernel's and every module's migrations (DATABASE_ADMIN_URL)", run: runMigrate}
}

// runMigrate applies the schema as the database owner. It takes the admin
// DSN, not the application's: the application role has no DDL rights, which
// is also what makes the RLS policies binding on it. In the dev stack this
// runs once before `api` and `worker` start; in production, `make deploy`
// runs it between the backup and the health gate.
func runMigrate(args []string) int {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	log := newLogger(os.Stdout, nil)
	dsn := os.Getenv("DATABASE_ADMIN_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "openenu migrate: DATABASE_ADMIN_URL must be set - the owner's DSN; migrations create tables and policies")
		return 2
	}
	reg, err := registry()
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu migrate:", err)
		return 1
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu migrate:", err)
		return 1
	}
	defer admin.Close()
	var sources []migrate.Source
	for _, m := range reg.All() {
		sources = append(sources, migrate.Source{Name: m.Manifest.Name, FS: m.Migrations})
	}
	applied, err := migrate.Run(ctx, admin, log, sources...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "openenu migrate:", err)
		return 1
	}
	if len(applied) == 0 {
		fmt.Println("migrate: nothing to apply")
	} else {
		fmt.Printf("migrate: applied %d\n", len(applied))
	}
	return 0
}

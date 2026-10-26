// Package migrate applies the schema: the kernel's own tables, River's, and
// every module's migrations/ directory, in registry order.
//
// Each source has its own goose version table (`goose_<source>`), so a
// module's migrations are numbered from 1 in its own directory and two
// modules never fight over a version number.
//
// Runs as the database owner (the admin DSN), never as the application role:
// the application role is NOSUPERUSER with no DDL rights, which is also what
// makes the RLS policies binding on it.
package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

//go:embed migrations/*.sql
var kernelMigrations embed.FS

// Source is one directory of goose SQL files, named for its version table.
type Source struct {
	Name string
	FS   fs.FS
}

// Kernel is the kernel's own source: audit_entry, search_index and progress_job.
func Kernel() Source {
	sub, err := fs.Sub(kernelMigrations, "migrations")
	if err != nil {
		panic(err)
	}
	return Source{Name: "kernel", FS: sub}
}

// Applied is what one run did, for the log and for `openenu migrate`.
type Applied struct {
	Source  string
	Version int64
}

// Run applies River's schema, then the kernel's, then each module source in
// the order given. A nil FS is a module without tables and is skipped.
func Run(ctx context.Context, admin *pgxpool.Pool, log *slog.Logger, modules ...Source) ([]Applied, error) {
	if log == nil {
		log = slog.Default()
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(admin), nil)
	if err != nil {
		return nil, fmt.Errorf("migrate: river: %w", err)
	}
	res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return nil, fmt.Errorf("migrate: river: %w", err)
	}
	var out []Applied
	for _, v := range res.Versions {
		out = append(out, Applied{Source: "river", Version: int64(v.Version)})
	}

	db := stdlib.OpenDBFromPool(admin)
	defer db.Close()
	for _, src := range append([]Source{Kernel()}, modules...) {
		if src.FS == nil {
			continue
		}
		applied, err := up(ctx, db, src)
		if err != nil {
			return out, err
		}
		for _, v := range applied {
			log.InfoContext(ctx, "migrated", "source", src.Name, "version", v)
			out = append(out, Applied{Source: src.Name, Version: v})
		}
	}
	return out, nil
}

func up(ctx context.Context, db *sql.DB, src Source) ([]int64, error) {
	p, err := goose.NewProvider(goose.DialectPostgres, db, src.FS, goose.WithTableName("goose_"+src.Name))
	if err != nil {
		return nil, fmt.Errorf("migrate: %s: %w", src.Name, err)
	}
	results, err := p.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: %s: %w", src.Name, err)
	}
	var versions []int64
	for _, r := range results {
		versions = append(versions, r.Source.Version)
	}
	return versions, nil
}

package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/open-enu/kernel/scope"
)

// A test proves what the policy did by reading the table itself: allowed.
func proveIsolation(ctx context.Context, pool *pgxpool.Pool) {
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM demo_thing")
	_ = scope.Unscoped(ctx, "test: every tenant's rows", func(context.Context) error { return nil })
	_ = scope.Unscoped(ctx, "", func(context.Context) error { return nil }) // want `scope.Unscoped reason is empty`
}

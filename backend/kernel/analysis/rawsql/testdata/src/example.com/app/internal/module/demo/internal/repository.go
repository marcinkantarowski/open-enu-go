package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/open-enu/kernel/scope"
)

func findAll(ctx context.Context, p *pgxpool.Pool) error {
	return p.Query(ctx, "SELECT 1") // fine: repository.go
}

func countAcrossTenants(ctx context.Context, p *pgxpool.Pool) error {
	return scope.Unscoped(ctx, "operator dashboard counts every tenant's projects", func(ctx context.Context) error {
		return p.QueryRow(ctx, "SELECT count(*) FROM example_project")
	})
}

func badReasons(ctx context.Context) error {
	why := "computed"
	_ = scope.Unscoped(ctx, why, nil) // want `scope.Unscoped reason must be a string literal`
	_ = scope.Unscoped(ctx, "", nil)  // want `scope.Unscoped reason is empty`
	return nil
}

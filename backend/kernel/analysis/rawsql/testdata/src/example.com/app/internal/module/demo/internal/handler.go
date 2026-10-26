package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/open-enu/kernel/scope"
)

func handle(ctx context.Context, p *pgxpool.Pool) error {
	_ = p.Exec(ctx, "DELETE FROM example_project") // want `Pool.Exec called in handler.go - SQL lives in repository.go`
	return scope.Unscoped(ctx, "cleanup", nil)     // want `scope.Unscoped called in handler.go - a cross-tenant read belongs in repository.go`
}

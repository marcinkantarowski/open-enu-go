package internal

import (
	"context"

	"github.com/open-enu/kernel/scope"
)

type repo struct{ pool *scope.Pool } // reads open their own scoped transaction here: fine

func update(ctx context.Context, tx *scope.Tx, name string) error {
	return tx.Exec(ctx, "UPDATE thing SET name = $1", name)
}

func (r repo) list(ctx context.Context) error {
	return r.pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error { return nil })
}

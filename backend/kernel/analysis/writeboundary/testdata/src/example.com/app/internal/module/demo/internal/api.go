package internal

import (
	"context"

	"github.com/open-enu/kernel/scope"
)

type api struct {
	pool *scope.Pool // want `scope.Pool in api.go - only the bus opens a write transaction`
}

func (a api) rename(ctx context.Context, name string) error {
	tx, err := a.pool.Begin(ctx) // want `\*scope.Tx in api.go - a write happens in a handler the bus called` `scope.Pool in api.go`
	if err != nil {
		return err
	}
	return update(ctx, tx, name) // want `\*scope.Tx in api.go`
}

package pgxpool

import "context"

type Pool struct{}

func (p *Pool) Exec(ctx context.Context, sql string, args ...any) error     { return nil }
func (p *Pool) Query(ctx context.Context, sql string, args ...any) error    { return nil }
func (p *Pool) QueryRow(ctx context.Context, sql string, args ...any) error { return nil }

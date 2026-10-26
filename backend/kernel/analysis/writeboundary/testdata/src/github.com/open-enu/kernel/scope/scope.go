// Stub of kernel/scope for the analyzer fixture.
package scope

import "context"

type Pool struct{}

func (p *Pool) Begin(ctx context.Context) (*Tx, error) { return &Tx{}, nil }

func (p *Pool) Run(ctx context.Context, fn func(context.Context, *Tx) error) error { return nil }

type Tx struct{}

func (t *Tx) Exec(ctx context.Context, sql string, args ...any) error { return nil }

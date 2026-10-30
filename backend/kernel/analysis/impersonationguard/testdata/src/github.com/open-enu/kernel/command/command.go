package command

import "context"

type Command interface{ CommandName() string }

type Bus struct{}

func (*Bus) Dispatch(context.Context, Command) (any, error) { return nil, nil }

func Dispatch[R any](ctx context.Context, b *Bus, cmd Command) (R, error) {
	var r R
	return r, nil
}

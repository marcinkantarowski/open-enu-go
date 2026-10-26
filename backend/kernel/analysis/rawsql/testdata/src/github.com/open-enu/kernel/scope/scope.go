package scope

import "context"

func Unscoped(ctx context.Context, reason string, fn func(context.Context) error) error {
	return fn(ctx)
}

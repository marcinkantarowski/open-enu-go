package auth

import "context"

type Guard struct{}

func (Guard) DenyImpersonated(context.Context, string) error { return nil }

func Require(context.Context, string) (struct{}, error) { return struct{}{}, nil }

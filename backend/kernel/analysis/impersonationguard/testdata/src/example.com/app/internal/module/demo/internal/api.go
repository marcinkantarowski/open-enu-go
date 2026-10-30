package internal

import (
	"context"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
)

type StrictServerInterface interface {
	Create(ctx context.Context) error
	Rename(ctx context.Context) error
	Archive(ctx context.Context) error
	Show(ctx context.Context) error
	Login(ctx context.Context) error
	Logout(ctx context.Context) error
	Late(ctx context.Context) error
	Publish(ctx context.Context) error
	Unpublish(ctx context.Context) error
}

type rename struct{}

func (rename) CommandName() string { return "demo.rename" }

type api struct {
	bus   *command.Bus
	guard auth.Guard
}

var _ StrictServerInterface = api{}

func (a api) Create(ctx context.Context) error {
	if _, err := auth.Require(ctx, "demo.manage"); err != nil {
		return err
	}
	_, err := command.Dispatch[int](ctx, a.bus, rename{}) // want `Create dispatches a command without refusing an impersonated session first`
	return err
}

func (a api) Rename(ctx context.Context) error {
	if _, err := auth.Require(ctx, "demo.manage"); err != nil {
		return err
	}
	if err := a.guard.DenyImpersonated(ctx, "demo.renamed"); err != nil {
		return err
	}
	_, err := command.Dispatch[int](ctx, a.bus, rename{})
	return err
}

func (a api) Archive(ctx context.Context) error {
	_, err := a.bus.Dispatch(ctx, rename{}) // want `Archive dispatches a command without refusing`
	return err
}

func (a api) Show(ctx context.Context) error {
	_, err := auth.Require(ctx, "demo.view")
	return err
}

// Login has no principal yet, so there is nobody to impersonate.
//
//impersonation:allowed public sign-in, no principal
func (a api) Login(ctx context.Context) error {
	_, err := a.bus.Dispatch(ctx, rename{})
	return err
}

//impersonation:allowed // want `//impersonation:allowed needs a reason`
func (a api) Logout(ctx context.Context) error {
	_, err := a.bus.Dispatch(ctx, rename{})
	return err
}

func (a api) Late(ctx context.Context) error {
	_, err := a.bus.Dispatch(ctx, rename{}) // want `Late dispatches a command without refusing`
	if err != nil {
		return err
	}
	return a.guard.DenyImpersonated(ctx, "demo.late")
}

// helper is not a controller method; it is not this analyzer's business.
func (a api) helper(ctx context.Context) error {
	_, err := a.bus.Dispatch(ctx, rename{})
	return err
}

// mayWrite requires the permission and refuses impersonation in one place.
func (a api) mayWrite(ctx context.Context, action string) error {
	if _, err := auth.Require(ctx, "demo.manage"); err != nil {
		return err
	}
	return a.guard.DenyImpersonated(ctx, action)
}

// mayRead looks like a guard and is not one.
func (a api) mayRead(ctx context.Context) error {
	_, err := auth.Require(ctx, "demo.view")
	return err
}

func (a api) Publish(ctx context.Context) error {
	if err := a.mayWrite(ctx, "demo.published"); err != nil {
		return err
	}
	_, err := a.bus.Dispatch(ctx, rename{})
	return err
}

func (a api) Unpublish(ctx context.Context) error {
	if err := a.mayRead(ctx); err != nil {
		return err
	}
	_, err := a.bus.Dispatch(ctx, rename{}) // want `Unpublish dispatches a command without refusing`
	return err
}

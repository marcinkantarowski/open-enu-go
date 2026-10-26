// Package setup is what a module needs in place before a new tenant can use
// it, and the runner that asks every module in dependency order.
//
// Order is not cosmetic: a module that depends on another may reasonably
// expect that module's defaults to exist by the time it runs. The order is
// the registry's - the same topological sort that orders migrations - so
// "depends" means one thing everywhere.
//
// A failing module stops the run. Partial provisioning is worse than none:
// the tenant exists, some of it works, and nobody knows which part.
package setup

import (
	"context"
	"fmt"
	"log/slog"
)

// TenantSetup is implemented by a module and registered on its Module value.
//
// Both methods MUST be idempotent. Provisioning is retried - a failed
// signup, a partially completed wizard, a re-run after a fix - and a second
// run that duplicates its defaults turns one bad signup into permanent bad
// data.
type TenantSetup interface {
	// OnTenantCreated writes the rows the module cannot function without:
	// default roles, settings, categories.
	OnTenantCreated(ctx context.Context, tenantID string) error
	// SeedExamples writes sample data for demos and development. Separate
	// from OnTenantCreated because production tenants want the former and
	// not the latter, and merging them is how demo data reaches a customer.
	SeedExamples(ctx context.Context, tenantID string) error
}

// Step is one module's setup, named so the log and the error can say which.
type Step struct {
	Module string
	Setup  TenantSetup
}

// Runner runs the steps in the order given.
type Runner struct {
	steps []Step
	log   *slog.Logger
}

// NewRunner takes the steps in dependency order (the registry's).
func NewRunner(log *slog.Logger, steps ...Step) *Runner {
	if log == nil {
		log = slog.Default()
	}
	return &Runner{steps: steps, log: log}
}

// OnTenantCreated runs every step; the first error stops the run and names
// the module.
func (r *Runner) OnTenantCreated(ctx context.Context, tenantID string) error {
	return r.each(ctx, tenantID, "tenant setup", func(s TenantSetup) error { return s.OnTenantCreated(ctx, tenantID) })
}

// SeedExamples runs every step's seed; the first error stops the run.
func (r *Runner) SeedExamples(ctx context.Context, tenantID string) error {
	return r.each(ctx, tenantID, "tenant seed", func(s TenantSetup) error { return s.SeedExamples(ctx, tenantID) })
}

func (r *Runner) each(ctx context.Context, tenantID, what string, fn func(TenantSetup) error) error {
	for _, s := range r.steps {
		r.log.InfoContext(ctx, what, "module", s.Module, "tenant", tenantID)
		if err := fn(s.Setup); err != nil {
			return fmt.Errorf("%s: module %q: %w", what, s.Module, err)
		}
	}
	return nil
}

// Modules lists the steps' modules in run order, for `openenu modules`.
func (r *Runner) Modules() []string {
	out := make([]string, 0, len(r.steps))
	for _, s := range r.steps {
		out = append(out, s.Module)
	}
	return out
}

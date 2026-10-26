package internal

import (
	"context"

	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/setup"
)

// seededName is the one project a fresh demo tenant gets.
const seededName = "Getting started"

// Setup is the module's tenant hook. Idempotent by identity rather than by
// count: re-running must not add a second copy, and must not stop an
// operator re-seeding after a wipe.
type Setup struct{ repo repository }

// NewSetup is what module.go registers.
func NewSetup(svc *platform.Services) setup.TenantSetup { return Setup{repo: newRepository(svc)} }

// OnTenantCreated writes nothing: there is nothing this module cannot
// function without. The distinction matters - this runs for every real
// customer, SeedExamples only where sample data is wanted.
func (Setup) OnTenantCreated(context.Context, string) error { return nil }

// SeedExamples writes the one demo project, once.
func (s Setup) SeedExamples(ctx context.Context, tenantID string) error {
	found, err := s.repo.hasNamed(scope.WithTenant(ctx, tenantID), seededName)
	if err != nil || found {
		return err
	}
	ref := "ACME-0001"
	return s.repo.seed(ctx, tenantID, newProject(createProject{Name: seededName, ClientReference: &ref}, tenantID))
}

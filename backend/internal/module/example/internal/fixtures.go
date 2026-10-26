package internal

import (
	"context"

	"github.com/open-enu/kernel/platform"
)

// Fixed ids, so a test can name a row.
const (
	FixtureAlpha = "0192f000-0000-7000-8000-00000000a001"
	FixtureBeta  = "0192f000-0000-7000-8000-00000000b002"
)

// LoadFixtures writes Alpha (an encrypted reference, an attribute) and Beta
// (bare) for tenantID. Deterministic: the same two rows every time.
func LoadFixtures(ctx context.Context, svc *platform.Services, tenantID string) error {
	ref := "ACME-0001"
	alpha := newProject(createProject{ID: FixtureAlpha, Name: "Alpha", ClientReference: &ref, Attributes: map[string]any{"priority": "high"}}, tenantID)
	beta := newProject(createProject{ID: FixtureBeta, Name: "Beta"}, tenantID)
	return newRepository(svc).seed(ctx, tenantID, alpha, beta)
}

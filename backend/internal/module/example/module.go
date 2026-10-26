// Package example is the reference module: one entity, Project, carrying
// exactly one of every platform concern - tenant scope (RLS), optimistic
// locking (409), an encrypted column, JSONB attributes, a command and
// handler per write, a broadcast event, a record-level voter, a feature
// flag, a background job with progress, an impersonation guard, a tenant
// seed, deterministic fixtures, a searchable declaration with its index
// kept in step, and a task on a clock. Nothing else. Read it, then copy it.
//
// The shape is .ai/platform/ARCHITECTURE.md §4: contract/ is what other
// modules may import, internal/ is everything else, and this file is the
// only thing the registry reads.
package example

//go:generate go run ../../../tools/genapi

import (
	"context"
	"embed"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"

	"github.com/open-enu/app/internal/module/example/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

// Module is the declaration. `example.view` reads, `example.manage` writes;
// a permission says what a ROLE may do, the voter in internal/voter.go says
// whether THIS row may be touched.
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"example.view", "example.manage"},
	Mount:       internal.Mount,
	Migrations:  module.MustSub(migrations, "migrations"),
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
	Flags:       internal.Flags,
	Setup:       internal.NewSetup,
	Search:      internal.Searchable,
	Tasks:       internal.Tasks,
}

// Fixture ids - fixed, so a test can name a row.
const (
	FixtureAlpha = internal.FixtureAlpha
	FixtureBeta  = internal.FixtureBeta
)

// LoadFixtures writes the two deterministic projects for tenantID: Alpha
// with an encrypted client reference and an attribute, Beta bare. Only tests
// call it; `openenu user create --seed` runs the setup hook's SeedExamples
// (internal/setup.go) instead, and a customer tenant sees neither.
func LoadFixtures(ctx context.Context, svc *platform.Services, tenantID string) error {
	return internal.LoadFixtures(ctx, svc, tenantID)
}

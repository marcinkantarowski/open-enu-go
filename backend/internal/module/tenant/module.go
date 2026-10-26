// Package tenant owns the workspace: the row every scoped table's tenant_id
// points at. It is the one module whose table is NOT under row-level
// security - it is the thing a scope names - and the one with no HTTP routes
// of its own in the tenant realm: a tenant is created by registration
// (Identity) or by the console, and suspended by operators (Manager). What it
// offers is contract/: a Reader every module may describe a tenant through,
// the public commands that change one, and the two events.
package tenant

import (
	"context"
	"embed"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"

	"github.com/open-enu/app/internal/module/tenant/contract"
	"github.com/open-enu/app/internal/module/tenant/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed i18n
var locales embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

// Module is the declaration. `tenant.view` reads the tenant's own record,
// `tenant.manage` edits it, `tenant.delete` is the owner's alone (Identity's
// role resolver keeps it from admins).
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"tenant.view", "tenant.manage", "tenant.delete"},
	Mount:       internal.Mount,
	Migrations:  module.MustSub(migrations, "migrations"),
	Locales:     module.MustSub(locales, "i18n"),
	Locale:      internal.LocalePreference,
	Tasks: []module.Task{{
		Name:    "tenant.purge_unverified",
		Cadence: module.Daily,
		Run: func(ctx context.Context, svc *platform.Services) error {
			_, err := internal.PurgeUnverified(ctx, svc, internal.PurgeAfterDays, false)
			return err
		},
	}},
}

// PurgeUnverified deletes every tenant still pending after `days` and
// returns what it deleted (or would delete, with dryRun). The scheduler's
// task and `openenu tenant purge-unverified` both call it.
func PurgeUnverified(ctx context.Context, svc *platform.Services, days int, dryRun bool) ([]contract.Info, error) {
	return internal.PurgeUnverified(ctx, svc, days, dryRun)
}

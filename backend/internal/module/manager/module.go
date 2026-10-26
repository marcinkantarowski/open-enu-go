// Package manager is the platform-operator realm: a separate identity table,
// a separate token audience, a separate app. Operators administer tenants,
// read the trail across all of them, watch the queues and impersonate a
// member through the one designed crossing between realms - a short-lived,
// non-refreshable tenant token that names both identities (ADR-0020).
//
// It holds the most authority in the system, so it reaches other modules
// only through their contract/ packages, where the cross-tenant reads lift
// isolation by name.
package manager

//go:generate go run ../../../tools/genapi

import (
	"context"
	"embed"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"

	"github.com/open-enu/app/internal/module/manager/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

// Module is the declaration. No permissions: operator authority is the
// realm, not a dotted permission resolved from a membership.
var Module = module.Module{
	Manifest:   module.MustManifest(manifest),
	Mount:      internal.Mount,
	Migrations: module.MustSub(migrations, "migrations"),
	Locales:    module.MustSub(locales, "i18n"),
	OpenAPI:    spec,
}

// CreateOperatorInput is `openenu manager create`'s.
type CreateOperatorInput = internal.CreateOperatorInput

// CreateOperator makes an operator account, idempotently by address.
func CreateOperator(ctx context.Context, svc *platform.Services, in CreateOperatorInput) (id string, created bool, err error) {
	return internal.CreateOperator(ctx, svc, in)
}

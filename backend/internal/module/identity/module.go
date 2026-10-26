// Package identity owns people: who they are (an encrypted address and a
// password verifier), where they belong (memberships, one role each), and
// how they prove it (a fifteen-minute access token in the body, a thirty-day
// refresh token in an HttpOnly cookie that rotates on every use - ADR-0018).
// It also owns the one rule that turns a role into permissions, because
// every token carries the result.
package identity

//go:generate go run ../../../tools/genapi

import (
	"context"
	"embed"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"

	"github.com/open-enu/app/internal/module/identity/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

// Module is the declaration. `identity.member.*` is declared for a members
// screen (invite, change role, remove) that no route in this module serves
// yet - the operator realm reads members through contract.Directory and
// needs no permission from here; `identity.profile.manage`
// guards a person editing their own profile. Under the resolver's rule a
// plain member does not hold it: a member reads and does `.use` work, and
// changing one's own name is an owner's or admin's gift until a project
// decides otherwise. An API key is not a person and has no profile at all.
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"identity.member.view", "identity.member.invite", "identity.member.manage", "identity.profile.manage"},
	Mount:       internal.Mount,
	Migrations:  module.MustSub(migrations, "migrations"),
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
	Locale:      internal.LocalePreference,
	Gdpr:        internal.NewGdpr,
}

// RefreshCookie is the refresh token's cookie name, for tests and the
// browser suite's assertions.
const RefreshCookie = internal.RefreshCookie

// CreateUserInput and CreateUserResult are `openenu user create`'s.
type (
	CreateUserInput  = internal.CreateUserInput
	CreateUserResult = internal.CreateUserResult
)

// CreateUser makes an active account with a membership, creating and
// activating the tenant by slug when missing. Idempotent; `--seed` runs
// every module's examples in the tenant.
func CreateUser(ctx context.Context, svc *platform.Services, in CreateUserInput) (CreateUserResult, error) {
	return internal.CreateUser(ctx, svc, in)
}

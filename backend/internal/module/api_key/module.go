// Package api_key is machine access: a key is minted once with a subset of
// the permissions modules declare, stored as a hash with a recognisable
// prefix, and revoked rather than deleted. It is the kernel's KeyResolver -
// a bearer `sk_...` becomes a principal that IS the key, never a person.
package api_key

//go:generate go run ../../../tools/genapi

import (
	"context"
	"embed"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"

	"github.com/open-enu/app/internal/module/api_key/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

// Module is the declaration. `api_key.manage` is owner-only under
// Identity's rule: minting a credential that outlives a session is the
// workspace owner's decision.
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"api_key.view", "api_key.manage"},
	Mount:       internal.Mount,
	Migrations:  module.MustSub(migrations, "migrations"),
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
	Keys:        internal.Keys,
}

// CreateKeyInput is `openenu apikey create`'s.
type CreateKeyInput = internal.CreateKeyInput

// CreateKey mints a key for a tenant (id or slug) and returns the secret
// once. The console prints it; nothing stores it.
func CreateKey(ctx context.Context, svc *platform.Services, in CreateKeyInput) (secret, id, prefix string, err error) {
	secret, key, err := internal.CreateKey(ctx, svc, in)
	return secret, key.Id, key.Prefix, err
}

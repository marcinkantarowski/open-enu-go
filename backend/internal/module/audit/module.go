// Package audit reads the trail. The kernel writes it - every command, both
// snapshots, the actor and whoever they acted for (kernel/audit, ADR-0013)
// - so this module owns no table and no write but one: a GDPR erasure
// anonymises a person's entries rather than deleting them, because a trail
// with holes is not a trail.
package audit

//go:generate go run ../../../tools/genapi

import (
	"embed"

	"github.com/open-enu/kernel/module"

	"github.com/open-enu/app/internal/module/audit/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

// Module is the declaration. `audit.view` reads the workspace's own trail;
// operators read across workspaces through the Manager module, which calls
// contract.Reader with no tenant.
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"audit.view"},
	Mount:       internal.Mount,
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
	Gdpr:        internal.NewGdpr,
}

// Package settings is runtime configuration and kill switches. Every module
// declares its flags in code (module.Module.Flags); this module is where a
// value for one workspace diverges from the declaration - set by the
// workspace itself where the declaration allows it, by an operator for
// anything. It implements the kernel's flags.Store, so every Flags read
// written anywhere resolves live values with no call site changing.
package settings

//go:generate go run ../../../tools/genapi

import (
	"embed"

	"github.com/open-enu/kernel/module"

	"github.com/open-enu/app/internal/module/settings/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

// Module is the declaration. `settings.manage` changes a workspace's OWN
// settings, and only those marked tenantEditable.
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"settings.view", "settings.manage"},
	Mount:       internal.Mount,
	Migrations:  module.MustSub(migrations, "migrations"),
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
	FlagStore:   internal.Store,
}

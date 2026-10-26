// Package search is the one endpoint over every module's searchable
// declarations (module.Module.Search). It owns no table: the index is the
// kernel's (search_index, kernel migration 0002) and the writes are each
// module's, in its own handlers. What this module adds is the rule the
// kernel cannot enforce alone - a type's permission is checked before the
// query runs, because an excerpt is content.
package search

//go:generate go run ../../../tools/genapi

import (
	"embed"

	"github.com/open-enu/kernel/module"

	"github.com/open-enu/app/internal/module/search/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

// Module is the declaration. No permissions of its own: the permissions
// that matter are the declaring modules', one per entity type.
var Module = module.Module{
	Manifest: module.MustManifest(manifest),
	Mount:    internal.Mount,
	Locales:  module.MustSub(locales, "i18n"),
	OpenAPI:  spec,
}

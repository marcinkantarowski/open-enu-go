// Package progress is the poll half of following a long job. The kernel's
// reporter (kernel/progress.Postgres) persists every Start, Advance, Finish
// and Fail and pushes the row to the job's topic; this module answers
// GET /api/progress/{id} with the same row, so a browser that missed the
// push, or attached late, still sees the truth.
package progress

//go:generate go run ../../../tools/genapi

import (
	"embed"

	"github.com/open-enu/kernel/module"

	"github.com/open-enu/app/internal/module/progress/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

// Module is the declaration. `progress.view` is what a plain member holds,
// so anyone who started a job can watch it.
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"progress.view"},
	Mount:       internal.Mount,
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
}

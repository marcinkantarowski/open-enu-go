// Package attachment is uploaded files: the bytes go through the kernel's
// storage (keyed under the tenant, served by signed URL), the row records
// what they are - typed by sniffing, never by the client's claim - and what
// they belong to. A file nobody claims within a day is swept, blob first.
package attachment

//go:generate go run ../../../tools/genapi

import (
	"context"
	"embed"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"

	"github.com/open-enu/app/internal/module/attachment/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

// Module is the declaration. `attachment.view` fetches a record and its
// link; `attachment.manage` uploads.
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"attachment.view", "attachment.manage"},
	Mount:       internal.Mount,
	Migrations:  module.MustSub(migrations, "migrations"),
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
	Tasks: []module.Task{{
		Name:    "attachment.sweep_orphans",
		Cadence: module.Daily,
		Run: func(ctx context.Context, svc *platform.Services) error {
			_, err := internal.SweepOrphans(ctx, svc, internal.SweepAfterHours, false)
			return err
		},
	}},
}

// SweepOrphans removes uploads with no owner older than `hours`; the daily
// task and `openenu attachment sweep-orphans` both call it.
func SweepOrphans(ctx context.Context, svc *platform.Services, hours int, dryRun bool) ([]string, error) {
	return internal.SweepOrphans(ctx, svc, hours, dryRun)
}

// SetMaxBytes lowers the upload limit; tests only.
func SetMaxBytes(n int64) { internal.MaxBytes = n }

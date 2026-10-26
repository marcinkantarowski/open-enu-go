// Package notification is telling someone that something happened - in the
// app, and by mail when the type asks for it. One catalogue of declared
// types (module.Module.Notifications, every module's), one row per
// recipient, read state per person. A row stores the type and its
// arguments, never a rendered sentence: the reader's language is a property
// of the reader, and a feed written in English at write time can never be
// read in Polish (ADR-0008).
//
// To raise one, depend on the kernel's notification.Notifier (svc.Notifier),
// never on this module: it implements the contract through Module.Notifier.
package notification

//go:generate go run ../../../tools/genapi

import (
	"embed"

	"github.com/open-enu/kernel/module"

	"github.com/open-enu/app/internal/module/notification/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

// Module is the declaration. One permission: a feed is personal, so
// "whose" is never a parameter and there is nothing a second one could say.
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"notification.view"},
	Mount:       internal.Mount,
	Migrations:  module.MustSub(migrations, "migrations"),
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
	Notifier:    internal.NewNotifier,
	Gdpr:        internal.NewGdpr,
}

// Package webhook is outbound webhooks, and outbound only. A workspace
// registers an https URL, subscribes it to event names, and this module
// signs (Standard Webhooks) and delivers every matching domain event, with
// five attempts on the jobs queue and a log of what happened to each - the
// log is the product. Nothing here knows what events exist: the publisher
// subscribes to every event, and the subscription lives in the tenant's row.
//
// Inbound webhooks are deliberately absent: receiving someone else's events
// is a per-provider problem, and a generic inbound endpoint is a generic
// security hole.
package webhook

//go:generate go run ../../../tools/genapi

import (
	"embed"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/notification"

	"github.com/open-enu/app/internal/module/webhook/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

//go:embed migrations/*.sql
var migrations embed.FS

// Module is the declaration. `webhook.view` sees endpoints and the log,
// `webhook.manage` adds, deactivates and redelivers. The one notification
// is the fifth failure: a dead endpoint is silent by definition.
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"webhook.view", "webhook.manage"},
	Mount:       internal.Mount,
	Migrations:  module.MustSub(migrations, "migrations"),
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
	Notifications: []notification.Definition{{
		Type:     internal.NotificationDeliveryFailed,
		TitleKey: "notification.webhook.delivery_failed.title",
		BodyKey:  "notification.webhook.delivery_failed.body",
		// Mail as well as the feed: the point of this one is to reach
		// somebody who is not looking at the app.
		Email: true,
	}},
}

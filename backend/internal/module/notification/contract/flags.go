package contract

import (
	"context"

	"github.com/open-enu/kernel/flags"
)

// Flags declares the module's switches. Mail behind a notification is on until
// an operator turns it off for a workspace: the feed goes on either way, and a
// mail that is not a notification (a password reset, a verification) never asks.
var Flags = []flags.Definition{{
	Identifier:     "notification.email",
	Name:           "Mail to the workspace",
	Description:    "Sends a mail behind the notifications that ask for one: reminders, deadlines, notices. Off, they stay in the app and nothing is mailed. A password reset or an address verification is not a notification and goes out either way.",
	Default:        true,
	TenantEditable: false,
	Category:       "notification",
}}

// MailsOn is whether a workspace in ctx gets mail behind its notifications, for
// whoever has to say what will happen without sending anything.
func MailsOn(ctx context.Context, f flags.Flags) bool {
	return f.Enabled(ctx, "notification.email", true)
}

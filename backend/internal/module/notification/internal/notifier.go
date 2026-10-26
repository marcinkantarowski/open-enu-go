package internal

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/mail"
	"github.com/open-enu/kernel/notification"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"

	identityc "github.com/open-enu/app/internal/module/identity/contract"
)

const keyOpen i18n.Key = "notification.email.open"

// notifier is the kernel's Notifier over this module's table: every
// `svc.Notifier` call written before this module existed now produces a row
// (and mail, when the declaration asks) with no call site changing.
type notifier struct {
	repo   repository
	defs   map[string]notification.Definition
	people identityc.Directory
	mail   mail.Mailer
	i18n   *i18n.Catalogue
	appURL string
	log    *slog.Logger
}

// NewNotifier is what module.go registers (module.Module.Notifier).
func NewNotifier(svc *platform.Services) notification.Notifier {
	return notifier{repo: newRepository(svc), defs: byType(svc.NotificationDefinitions), people: identityc.NewDirectory(svc),
		mail: svc.Mail, i18n: svc.I18n, appURL: svc.URLs.App, log: svc.Log}
}

func byType(defs []notification.Definition) map[string]notification.Definition {
	out := make(map[string]notification.Definition, len(defs))
	for _, d := range defs {
		out[d.Type] = d
	}
	return out
}

// ToUser is "this concerns you personally". The workspace is the ambient
// scope's: a notification is raised from inside the work that caused it.
func (n notifier) ToUser(ctx context.Context, userID, typ string, c map[string]any) error {
	def, ok := n.declared(ctx, typ)
	if !ok {
		return nil
	}
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		n.log.WarnContext(ctx, "notification.no_tenant", "type", typ, "to_user", userID)
		return nil
	}
	members, err := n.people.MembersOf(ctx, tenant)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.ID == userID {
			return n.store(ctx, tenant, m, def, c)
		}
	}
	// Not a member here: the row is still written (they may be reading in
	// another role), the mail is not - there is no address to send to.
	return n.store(ctx, tenant, identityc.Member{ID: userID}, def, c)
}

// ToTenant is "this concerns the workspace", fanned out here rather than by
// the caller: which people count as "the workspace" is this module's
// problem, and a module raising a notification should not have to answer it.
func (n notifier) ToTenant(ctx context.Context, tenantID, typ string, c map[string]any) error {
	def, ok := n.declared(ctx, typ)
	if !ok {
		return nil
	}
	ctx = scope.WithTenant(ctx, tenantID)
	members, err := n.people.MembersOf(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, m := range members {
		if err := n.store(ctx, tenantID, m, def, c); err != nil {
			return err
		}
	}
	return nil
}

// declared drops an undeclared type with a log line rather than storing it:
// a row whose type has no declaration has no key to translate, so it would
// be permanently unreadable.
func (n notifier) declared(ctx context.Context, typ string) (notification.Definition, bool) {
	def, ok := n.defs[typ]
	if !ok {
		n.log.WarnContext(ctx, "notification.undeclared_type", "type", typ)
	}
	return def, ok
}

func (n notifier) store(ctx context.Context, tenantID string, m identityc.Member, def notification.Definition, c map[string]any) error {
	if err := n.repo.insert(ctx, newRow(tenantID, m.ID, def.Type, c)); err != nil {
		return err
	}
	if !def.Email || m.Email == "" {
		return nil
	}
	locale := n.i18n.Default()
	if m.Locale != nil {
		locale = *m.Locale
	}
	params := arguments(c)
	msg := mail.Message{To: m.Email, Subject: n.i18n.Translate(locale, def.TitleKey, params),
		Text: n.i18n.Translate(locale, def.BodyKey, params) + "\n\n" + n.i18n.Translate(locale, keyOpen, nil) + " " + n.appURL + "/notifications\n"}
	if err := n.mail.Send(ctx, msg); err != nil {
		// The row is written; a mail that did not go out is a log line, not
		// a failure of the change that raised the notification.
		n.log.ErrorContext(ctx, "notification.mail_failed", "to_user", m.ID, "type", def.Type, "error", err.Error())
	}
	return nil
}

// arguments renders the context as translation parameters, in a stable order.
func arguments(c map[string]any) map[string]string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(c))
	for _, k := range keys {
		out[k] = strings.TrimSpace(fmt.Sprint(c[k]))
	}
	return out
}

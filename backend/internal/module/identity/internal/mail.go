package internal

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/mail"
)

const (
	keyVerifySubject i18n.Key = "identity.email.verify.subject"
	keyVerifyBody    i18n.Key = "identity.email.verify.body"
	keyResetSubject  i18n.Key = "identity.email.reset.subject"
	keyResetBody     i18n.Key = "identity.email.reset.body"
	keyResetIgnore   i18n.Key = "identity.email.reset.ignore"
	keyExpires       i18n.Key = "identity.email.expires"
	keyInviteSubject i18n.Key = "identity.email.invite.subject"
	keyInviteBody    i18n.Key = "identity.email.invite.body"
	keyInviteExpires i18n.Key = "identity.email.invite.expires"
	keyInviteAfter   i18n.Key = "identity.email.invite.after"
	keyAddedSubject  i18n.Key = "identity.email.added.subject"
	keyAddedBody     i18n.Key = "identity.email.added.body"
	keyExistsSubject i18n.Key = "identity.email.account_exists.subject"
	keyExistsBody    i18n.Key = "identity.email.account_exists.body"
	keyExistsIgnore  i18n.Key = "identity.email.account_exists.ignore"
)

// mailer writes the messages this module sends, in the person's locale
// (their stored choice, else the catalogue's default), with links into the
// tenant app. A send failure is logged, not returned: the row is committed
// either way and the person can ask again.
type mailer struct {
	mail   mail.Mailer
	i18n   *i18n.Catalogue
	appURL string
	log    *slog.Logger
}

func (m mailer) verification(ctx context.Context, u user, raw string) {
	m.send(ctx, u, keyVerifySubject, []string{m.t(u, keyVerifyBody, nil), m.appURL + "/verify?token=" + raw, m.expires(u, verifyTTL.Hours())})
}

func (m mailer) reset(ctx context.Context, u user, raw string) {
	m.send(ctx, u, keyResetSubject, []string{m.t(u, keyResetBody, nil), m.appURL + "/reset-password?token=" + raw, m.expires(u, resetTTL.Hours()), m.t(u, keyResetIgnore, nil)})
}

// accountExists tells the owner of an address that someone tried to
// register it again - what registration says instead of "that address has
// an account", which would tell anyone (ADR-0023).
func (m mailer) accountExists(ctx context.Context, u user) {
	m.send(ctx, u, keyExistsSubject, []string{m.t(u, keyExistsBody, nil), m.appURL + "/login", m.appURL + "/forgot-password", m.t(u, keyExistsIgnore, nil)})
}

// invitation gives a person an operator added the link to choose a password.
// The operator never sees it, so no operator can ever sign in as them.
func (m mailer) invitation(ctx context.Context, u user, workspace, raw string) {
	named := map[string]string{"tenant": workspace}
	days := strconv.Itoa(int(setPasswordTTL.Hours() / 24))
	m.deliver(ctx, u, keyInviteSubject, m.t(u, keyInviteSubject, named), []string{m.t(u, keyInviteBody, named), m.appURL + "/reset-password?token=" + raw,
		m.t(u, keyInviteExpires, map[string]string{"days": days}), m.t(u, keyInviteAfter, nil) + "\n" + m.appURL + "/forgot-password"}, true)
}

// added tells a person with an account they can now reach one more workspace.
// No credential in it, so the journal may keep what it says.
func (m mailer) added(ctx context.Context, u user, workspace string) {
	named := map[string]string{"tenant": workspace}
	m.deliver(ctx, u, keyAddedSubject, m.t(u, keyAddedSubject, named), []string{m.t(u, keyAddedBody, named), m.appURL + "/login"}, false)
}

func (m mailer) expires(u user, hours float64) string {
	return m.t(u, keyExpires, map[string]string{"hours": strconv.Itoa(int(hours))})
}

func (m mailer) t(u user, key i18n.Key, params map[string]string) string {
	locale := m.i18n.Default()
	if u.Locale != nil {
		locale = *u.Locale
	}
	return m.i18n.Translate(locale, key, params)
}

func (m mailer) send(ctx context.Context, u user, subject i18n.Key, paragraphs []string) {
	m.deliver(ctx, u, subject, m.t(u, subject, nil), paragraphs, true)
}

func (m mailer) deliver(ctx context.Context, u user, kind i18n.Key, subject string, paragraphs []string, sensitive bool) {
	// Sensitive: a link there is a credential, and a journal must not hold it.
	msg := mail.Message{To: u.Email, Subject: subject, Text: strings.Join(paragraphs, "\n\n") + "\n",
		Kind: string(kind), UserID: u.ID, Sensitive: sensitive}
	if err := m.mail.Send(ctx, msg); err != nil && m.log != nil {
		m.log.ErrorContext(ctx, "identity.mail_failed", "to_user", u.ID, "subject", string(kind), "error", err.Error())
	}
}

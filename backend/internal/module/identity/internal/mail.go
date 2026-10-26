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
	msg := mail.Message{To: u.Email, Subject: m.t(u, subject, nil), Text: strings.Join(paragraphs, "\n\n") + "\n"}
	if err := m.mail.Send(ctx, msg); err != nil && m.log != nil {
		m.log.ErrorContext(ctx, "identity.mail_failed", "to_user", u.ID, "subject", string(subject), "error", err.Error())
	}
}

// Package mail sends a message to a person: the verification link, the
// password reset, the invitation. One interface, three implementations -
// SMTP for the stack (Mailpit in dev, a relay in production), a log line
// when no MAILER_DSN is set, and a Recorder a test can read.
//
// Plain text only, deliberately. The three messages the platform sends are
// one sentence and a link each; templates, branding and a per-tenant sender
// are the Notification module's concern, not the kernel's.
package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/smtp"
	"net/url"
	"strings"
	"sync"
)

// Message is one email to one person.
type Message struct {
	To      string
	Subject string
	Text    string
}

// Mailer sends. Send returns once the message has been handed to the
// transport; delivery itself is the transport's business.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// FromDSN picks the transport: `smtp://host:port` or
// `smtp://user:pass@host:port`; an empty DSN logs instead of sending, which
// is what a process with no relay should do rather than fail every signup.
func FromDSN(dsn, from, replyTo string, log *slog.Logger) (Mailer, error) {
	if dsn == "" {
		return Log{Log: log}, nil
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "smtp" || u.Host == "" {
		return nil, fmt.Errorf("mail: MAILER_DSN must look like smtp://host:port, got %q", dsn)
	}
	if from == "" {
		return nil, errors.New("mail: MAILER_FROM must be set when MAILER_DSN is")
	}
	s := &SMTP{Addr: u.Host, From: from, ReplyTo: replyTo}
	if u.User != nil {
		s.Username = u.User.Username()
		s.Password, _ = u.User.Password()
	}
	if !strings.Contains(s.Addr, ":") {
		s.Addr += ":25"
	}
	return s, nil
}

// SMTP sends through a relay. STARTTLS is used when the server offers it;
// credentials, when given, are sent with PLAIN auth - which net/smtp
// refuses over a plaintext connection to anything but localhost.
type SMTP struct {
	Addr     string
	From     string
	ReplyTo  string
	Username string
	Password string
}

// Send hands the message to the relay.
func (s *SMTP) Send(_ context.Context, m Message) error {
	var auth smtp.Auth
	if s.Username != "" {
		host, _, _ := strings.Cut(s.Addr, ":")
		auth = smtp.PlainAuth("", s.Username, s.Password, host)
	}
	if err := smtp.SendMail(s.Addr, auth, s.From, []string{headerLine(m.To)}, s.message(m)); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	return nil
}

// message is the RFC 5322 text Send hands the relay.
//
// Every header value is forced onto one line: the subject carries text a
// user typed (a workspace name), and a CR LF inside it would end the header
// and let that user add headers - a Bcc, say - of their own. The subject is
// also RFC 2047 encoded, so a non-ASCII name arrives intact.
func (s *SMTP) message(m Message) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\n",
		headerLine(s.From), headerLine(m.To), mime.QEncoding.Encode("utf-8", headerLine(m.Subject)))
	if s.ReplyTo != "" {
		fmt.Fprintf(&b, "Reply-To: %s\r\n", headerLine(s.ReplyTo))
	}
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(strings.ReplaceAll(m.Text, "\n", "\r\n"))
	return []byte(b.String())
}

// headerLine strips what would start a new header: CR, LF, and the NUL a
// broken client sometimes sends along.
func headerLine(v string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', 0:
			return -1
		}
		return r
	}, v)
}

// Log writes the message to the log instead of sending it. The body is
// logged too: in a stack with no relay, the log is where the verification
// link is read from.
type Log struct{ Log *slog.Logger }

// Send logs.
func (l Log) Send(ctx context.Context, m Message) error {
	log := l.Log
	if log == nil {
		log = slog.Default()
	}
	log.InfoContext(ctx, "mail.not_sent", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}

// Recorder keeps what was sent, for tests.
type Recorder struct {
	mu   sync.Mutex
	sent []Message
}

// Send records.
func (r *Recorder) Send(_ context.Context, m Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, m)
	return nil
}

// Sent returns every message so far.
func (r *Recorder) Sent() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Message(nil), r.sent...)
}

// Last returns the most recent message to an address, if any.
func (r *Recorder) Last(to string) (Message, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.sent) - 1; i >= 0; i-- {
		if r.sent[i].To == to {
			return r.sent[i], true
		}
	}
	return Message{}, false
}

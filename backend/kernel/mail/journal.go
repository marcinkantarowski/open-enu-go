package mail

import (
	"context"
	"log/slog"
)

// Outcome is what became of a mail.
type Outcome string

const (
	Sent   Outcome = "sent"
	Failed Outcome = "failed"
	Held   Outcome = "held"
)

// Journal keeps what went out and says what must not. One module provides it
// (module.Module.MailJournal).
type Journal interface {
	Held(ctx context.Context, m Message) (bool, error)
	Record(ctx context.Context, m Message, outcome Outcome, failure error) error
}

// Journaled puts a journal in front of a transport, so no sender can forget it.
// A journal that fails costs a log line and never a mail: a statutory reminder
// is worth more than the record of it.
type Journaled struct {
	Next    Mailer
	Journal Journal
	Log     *slog.Logger
}

func (j Journaled) Send(ctx context.Context, m Message) error {
	held, err := j.Journal.Held(ctx, m)
	if err != nil {
		j.Log.ErrorContext(ctx, "mail.journal_failed", "kind", m.Kind, "error", err.Error())
	}
	if held {
		j.record(ctx, m, Held, nil)
		return nil
	}
	err = j.Next.Send(ctx, m)
	if err != nil {
		j.record(ctx, m, Failed, err)
		return err
	}
	j.record(ctx, m, Sent, nil)
	return nil
}

// A run that was cancelled after the mail left still has to say so.
func (j Journaled) record(ctx context.Context, m Message, outcome Outcome, failure error) {
	if err := j.Journal.Record(context.WithoutCancel(ctx), m, outcome, failure); err != nil {
		j.Log.ErrorContext(ctx, "mail.journal_failed", "kind", m.Kind, "error", err.Error())
	}
}

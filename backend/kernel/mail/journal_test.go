package mail

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

type book struct {
	held     bool
	askErr   error
	writeErr error
	outcomes []Outcome
}

func (b *book) Held(context.Context, Message) (bool, error) { return b.held, b.askErr }
func (b *book) Record(_ context.Context, _ Message, o Outcome, _ error) error {
	b.outcomes = append(b.outcomes, o)
	return b.writeErr
}

type refused struct{}

func (refused) Send(context.Context, Message) error { return errors.New("no relay") }

func journaled(next Mailer, b *book) Journaled {
	return Journaled{Next: next, Journal: b, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestAHeldMailDoesNotLeaveAndIsRecorded(t *testing.T) {
	rec, b := &Recorder{}, &book{held: true}
	if err := journaled(rec, b).Send(context.Background(), Message{To: "a@b.test"}); err != nil {
		t.Fatal(err)
	}
	if len(rec.Sent()) != 0 || len(b.outcomes) != 1 || b.outcomes[0] != Held {
		t.Fatalf("sent %v, recorded %v", rec.Sent(), b.outcomes)
	}
}

func TestAFailureIsRecordedAndStillReturned(t *testing.T) {
	b := &book{}
	if err := journaled(refused{}, b).Send(context.Background(), Message{}); err == nil {
		t.Fatal("the failure was swallowed")
	}
	if len(b.outcomes) != 1 || b.outcomes[0] != Failed {
		t.Fatalf("recorded %v", b.outcomes)
	}
}

func TestABrokenJournalNeverCostsAMail(t *testing.T) {
	rec, b := &Recorder{}, &book{askErr: errors.New("db"), writeErr: errors.New("db")}
	if err := journaled(rec, b).Send(context.Background(), Message{To: "a@b.test"}); err != nil {
		t.Fatal(err)
	}
	if len(rec.Sent()) != 1 {
		t.Fatalf("the mail stayed: %v", rec.Sent())
	}
}

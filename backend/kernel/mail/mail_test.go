package mail_test

import (
	"context"
	"testing"

	"github.com/open-enu/kernel/mail"
)

func TestFromDSNPicksTheTransportAndRefusesNonsense(t *testing.T) {
	m, err := mail.FromDSN("", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(mail.Log); !ok {
		t.Fatalf("no DSN should log, got %T", m)
	}
	m, err = mail.FromDSN("smtp://user:secret@relay.example:587", "noreply@example.test", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := m.(*mail.SMTP)
	if !ok || s.Addr != "relay.example:587" || s.Username != "user" || s.Password != "secret" || s.From != "noreply@example.test" {
		t.Fatalf("smtp parsed wrong: %+v", m)
	}
	if _, err := mail.FromDSN("smtp://mailpit:1025", "", "", nil); err == nil {
		t.Fatal("a DSN without a sender address was accepted")
	}
	if _, err := mail.FromDSN("imap://x", "a@b", "", nil); err == nil {
		t.Fatal("a non-smtp DSN was accepted")
	}
}

func TestRecorderKeepsTheLastMessagePerAddress(t *testing.T) {
	r := &mail.Recorder{}
	_ = r.Send(context.Background(), mail.Message{To: "a@x", Subject: "one"})
	_ = r.Send(context.Background(), mail.Message{To: "b@x", Subject: "two"})
	_ = r.Send(context.Background(), mail.Message{To: "a@x", Subject: "three"})
	if m, ok := r.Last("a@x"); !ok || m.Subject != "three" {
		t.Fatalf("last to a@x = %+v %v", m, ok)
	}
	if _, ok := r.Last("c@x"); ok || len(r.Sent()) != 3 {
		t.Fatal("recorder confused")
	}
}

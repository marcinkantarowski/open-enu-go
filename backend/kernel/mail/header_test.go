package mail

import (
	"strings"
	"testing"
)

func TestHeadersCannotBeInjectedThroughTheSubjectOrTheAddress(t *testing.T) {
	s := &SMTP{From: "noreply@example.test", ReplyTo: "support@example.test"}
	raw := string(s.message(Message{
		To:      "owner@example.test\r\nBcc: victim@example.test",
		Subject: "Welcome to Acme\r\nBcc: attacker@example.test",
		Text:    "line one\nline two",
	}))
	head, body, ok := strings.Cut(raw, "\r\n\r\n")
	if !ok {
		t.Fatalf("no header/body separator: %q", raw)
	}
	for _, line := range strings.Split(head, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "bcc:") {
			t.Fatalf("a header was injected: %q", head)
		}
	}
	if !strings.Contains(head, "Subject: Welcome to AcmeBcc: attacker@example.test") {
		t.Fatalf("the subject stays one line, with its text: %q", head)
	}
	if body != "line one\r\nline two" {
		t.Fatalf("body: %q", body)
	}
	// Non-ASCII arrives encoded rather than as raw bytes in a header.
	if got := string(s.message(Message{To: "a@example.test", Subject: "Zaz\u00f3\u0142\u0107"})); !strings.Contains(got, "Subject: =?utf-8?q?") {
		t.Fatalf("non-ASCII subject not encoded: %q", got)
	}
}

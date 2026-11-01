package mail

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"strings"
	"testing"
	"time"
)

func TestAReplySaysWhatItAnswersAndCarriesItsFiles(t *testing.T) {
	raw := Render(Message{From: "info@example.test", To: "anna@example.test", Subject: "Re: Umowa", Text: "Dzień dobry\nPozdrawiam",
		Date: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), MessageID: "a@example.test", InReplyTo: "b@klient.test", References: []string{"b@klient.test"},
		Files: []File{{Name: "umowa końcowa.pdf", ContentType: "application/pdf", Data: []byte(strings.Repeat("x", 200))}, {Name: "x.bin", ContentType: "nonsense", Data: []byte("y")}}})
	m, err := netmail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if m.Header.Get("From") != "info@example.test" || m.Header.Get("Message-ID") != "<a@example.test>" || m.Header.Get("In-Reply-To") != "<b@klient.test>" ||
		m.Header.Get("References") != "<b@klient.test>" || m.Header.Get("Reply-To") != "" || m.Header.Get("Date") == "" {
		t.Fatalf("headers: %v", m.Header)
	}
	_, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	r := multipart.NewReader(m.Body, params["boundary"])
	text, err := r.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := io.ReadAll(text); string(body) != "Dzień dobry\r\nPozdrawiam" {
		t.Fatalf("text: %q", body)
	}
	file, err := r.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, file))
	if err != nil || file.FileName() != "umowa końcowa.pdf" || len(body) != 200 {
		t.Fatalf("file: %q, %d bytes: %v", file.FileName(), len(body), err)
	}
	second, err := r.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if second.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("a broken type: %v", second.Header)
	}
}

// A name typed by a person must not become a header.
func TestAFileNameCannotAddAHeader(t *testing.T) {
	raw := string(Render(Message{From: "a@b.test", To: "c@d.test\r\nBcc: e@f.test", Subject: "x", MessageID: "i\r\nBcc: g@h.test", Files: []File{{Name: "a\r\nBcc: i@j.test", Data: []byte("x")}}}))
	if strings.Contains(raw, "\r\nBcc:") {
		t.Fatalf("a header was added:\n%s", raw)
	}
}

func TestTheConfiguredSenderStandsUnlessAMailHasItsOwn(t *testing.T) {
	s := &SMTP{From: "noreply@a.test", ReplyTo: "info@a.test"}
	plain := string(s.message(Message{To: "x@y.test", Subject: "s", Text: "t"}))
	if !strings.Contains(plain, "From: noreply@a.test") || !strings.Contains(plain, "Reply-To: info@a.test") {
		t.Fatalf("a notice: %s", plain)
	}
	own := string(s.message(Message{From: "info@a.test", To: "x@y.test", Subject: "s", Text: "t"}))
	if !strings.Contains(own, "From: info@a.test") || strings.Contains(own, "Reply-To") {
		t.Fatalf("a reply: %s", own)
	}
}

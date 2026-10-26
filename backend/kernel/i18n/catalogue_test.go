package i18n

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestKernelCatalogueLoadsAndTranslates(t *testing.T) {
	c, err := NewCatalogue("en", "en", "pl")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Load("kernel", KernelLocales()); err != nil {
		t.Fatal(err)
	}
	if got := c.Translate("pl", "kernel.error.not_found", nil); got != "Nie znaleziono." {
		t.Fatalf("pl: %q", got)
	}
	if got := c.Translate("de", "kernel.error.not_found", nil); got != "Not found." {
		t.Fatalf("unknown locale falls back to the default: %q", got)
	}
	if got := c.Translate("en", "demo.missing", nil); got != "demo.missing" {
		t.Fatalf("missing key is returned raw, visibly: %q", got)
	}
	if !c.Has("en", "kernel.error.conflict") || c.Has("en", "nope.x") {
		t.Fatal("Has")
	}
	ctx := WithLocale(context.Background(), "pl")
	if got := c.TranslateCtx(ctx, "kernel.error.forbidden"); !strings.HasPrefix(got, "Nie masz") {
		t.Fatalf("TranslateCtx: %q", got)
	}
	if Locale(context.Background()) != "" {
		t.Fatal("no locale is empty")
	}
	if _, err := NewCatalogue("de", "en", "pl"); err == nil {
		t.Fatal("default must be enabled")
	}
}

func TestPlaceholdersAndModuleLoad(t *testing.T) {
	c, _ := NewCatalogue("en", "en", "pl")
	mod := fstest.MapFS{
		"messages.en.json": {Data: []byte(`{"demo.count": "Archived %count% project(s) for %who%."}`)},
		"messages.pl.json": {Data: []byte(`{"demo.count": "Zarchiwizowano %count%."}`)},
	}
	if err := c.Load("demo", mod); err != nil {
		t.Fatal(err)
	}
	got := c.Translate("en", "demo.count", map[string]string{"count": "3", "who": "ann"})
	if got != "Archived 3 project(s) for ann." {
		t.Fatalf("got %q", got)
	}
	// The same key from another origin is a namespace violation.
	if err := c.Load("other", mod); err == nil || !strings.Contains(err.Error(), `"demo.count" is already defined by demo`) {
		t.Fatalf("duplicate across modules: %v", err)
	}
	if err := c.Load("empty", nil); err != nil {
		t.Fatal("nil FS is no catalogue")
	}
	if err := c.Load("x", fstest.MapFS{"messages.de.json": {Data: []byte(`{}`)}}); err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("locale not enabled: %v", err)
	}
	if err := c.Load("y", fstest.MapFS{"messages.en.json": {Data: []byte(`{"y.a":"a"}`)}}); err == nil || !strings.Contains(err.Error(), "messages.pl.json is missing") {
		t.Fatalf("missing locale file: %v", err)
	}
}

func TestParityNamesTheKeyAndTheLocale(t *testing.T) {
	_, err := CheckParity(fstest.MapFS{
		"messages.en.json": {Data: []byte(`{"demo.a": "A", "demo.b": "B"}`)},
		"messages.pl.json": {Data: []byte(`{"demo.a": "A"}`)},
	})
	if err == nil || !strings.Contains(err.Error(), `"demo.b" exists in messages.en.json and not in messages.pl.json`) {
		t.Fatalf("got %v", err)
	}
	_, err = CheckParity(fstest.MapFS{"messages.en.json": {Data: []byte(`{"demo": {"a": "A"}}`)}})
	if err == nil || !strings.Contains(err.Error(), "never nested") {
		t.Fatalf("nested: %v", err)
	}
	_, err = CheckParity(fstest.MapFS{"messages.en.json": {Data: []byte(`{"Demo Title": "A"}`)}})
	if err == nil || !strings.Contains(err.Error(), "dotted lowercase") {
		t.Fatalf("shape: %v", err)
	}
	files, err := CheckParity(fstest.MapFS{"README.md": {Data: []byte("x")}})
	if err != nil || len(files) != 0 {
		t.Fatalf("no catalogue files is fine: %v %v", files, err)
	}
}

func TestNegotiate(t *testing.T) {
	enabled := []string{"en", "pl"}
	cases := map[string]string{
		"":                           "en",
		"pl":                         "pl",
		"pl-PL,pl;q=0.9,en;q=0.8":    "pl",
		"de-DE,de;q=0.9,en;q=0.8":    "en",
		"de, pl;q=0.5, en;q=0.9":     "en",
		"de, pl;q=0.9, en;q=0.5":     "pl",
		"fr":                         "en",
		"*":                          "en",
		"pl;q=0, en":                 "en",
		"PL":                         "pl",
		"en-US;q=0.7, pl-PL;q=0.7":   "en",
		"garbage;;;, pl ; q = 0.3 ;": "pl",
	}
	for header, want := range cases {
		if got := Negotiate(header, enabled, "en"); got != want {
			t.Errorf("%q: got %s want %s", header, got, want)
		}
	}
}

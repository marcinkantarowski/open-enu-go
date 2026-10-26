package module

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/gdpr"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/logctx"
	"github.com/open-enu/kernel/notification"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/search"
	"github.com/open-enu/kernel/setup"
)

type nopSetup struct{}

func (nopSetup) OnTenantCreated(context.Context, string) error { return nil }
func (nopSetup) SeedExamples(context.Context, string) error    { return nil }

type nopGdpr struct{}

func (nopGdpr) ExportFor(context.Context, string) (map[string][]map[string]any, error) {
	return nil, nil
}
func (nopGdpr) EraseFor(context.Context, string) (int, error) { return 0, nil }

func newSetup(*platform.Services) setup.TenantSetup { return nopSetup{} }
func newGdpr(*platform.Services) gdpr.Subject       { return nopGdpr{} }

func locales(en, pl string) fstest.MapFS {
	return fstest.MapFS{"messages.en.json": {Data: []byte(en)}, "messages.pl.json": {Data: []byte(pl)}}
}

func TestRegistryCollectsHooksInOrder(t *testing.T) {
	r, err := NewRegistry(
		Module{Manifest: manifest("example", "tenant"), Flags: []flags.Definition{{Identifier: "example.archive"}}, Setup: newSetup, Gdpr: newGdpr,
			Locales: locales(`{"example.hi":"Hi"}`, `{"example.hi":"Cześć"}`)},
		Module{Manifest: manifest("tenant"), Setup: newSetup, Locales: locales(`{}`, `{}`)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if f := r.Flags(); len(f) != 1 || f[0].Identifier != "example.archive" {
		t.Fatalf("flags %+v", f)
	}
	if o, ok := r.FlagOwner("example.archive"); !ok || o != "example" {
		t.Fatal("flag owner")
	}
	steps := r.SetupSteps(platform.InMemory(nil))
	if len(steps) != 2 || steps[0].Module != "tenant" || steps[1].Module != "example" {
		t.Fatalf("setup order %+v", steps)
	}
	if g := r.GdprSubjects(platform.InMemory(nil)); len(g) != 1 || g[0].Module != "example" {
		t.Fatalf("gdpr %+v", g)
	}
	cat, _ := i18n.NewCatalogue("en", "en", "pl")
	if err := r.LoadLocales(cat); err != nil {
		t.Fatal(err)
	}
	if cat.Translate("pl", "example.hi", nil) != "Cześć" || cat.Translate("pl", "kernel.error.not_found", nil) != "Nie znaleziono." {
		t.Fatal("module and kernel catalogues loaded")
	}
}

func TestRegistryRefusesBadHooks(t *testing.T) {
	cases := map[string]struct {
		mods []Module
		want string
	}{
		"unprefixed flag": {[]Module{{Manifest: manifest("a"), Flags: []flags.Definition{{Identifier: "b.x"}}}}, `declares flag "b.x" - a flag is prefixed with its owner`},
		"bad flag shape":  {[]Module{{Manifest: manifest("a"), Flags: []flags.Definition{{Identifier: "a.X"}}}}, "dotted lowercase"},
		"flag twice":      {[]Module{{Manifest: manifest("a"), Flags: []flags.Definition{{Identifier: "a.x"}, {Identifier: "a.x"}}}}, `flag "a.x" is declared by both`},
		"locale parity":   {[]Module{{Manifest: manifest("a"), Locales: locales(`{"a.x":"x","a.y":"y"}`, `{"a.x":"x"}`)}}, `module "a": i18n: key "a.y" exists in messages.en.json and not in messages.pl.json`},
	}
	for label, tc := range cases {
		_, err := NewRegistry(tc.mods...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", label, err, tc.want)
		}
	}
	// Parity per module passes; the same key from two modules fails at load.
	r, err := NewRegistry(
		Module{Manifest: manifest("a"), Locales: locales(`{"shared.k":"1"}`, `{"shared.k":"1"}`)},
		Module{Manifest: manifest("b"), Locales: locales(`{"shared.k":"2"}`, `{"shared.k":"2"}`)},
	)
	if err != nil {
		t.Fatal(err)
	}
	cat, _ := i18n.NewCatalogue("en", "en", "pl")
	if err := r.LoadLocales(cat); err == nil || !strings.Contains(err.Error(), `"shared.k" is already defined by module a`) {
		t.Fatalf("got %v", err)
	}
}

func TestSearchDeclarationsNameAnOwnedPermission(t *testing.T) {
	ok := Module{Manifest: Manifest{Name: "a", Enabled: true}, Permissions: []string{"a.view"}, Search: []search.Declaration{{EntityType: "a.thing", Permission: "a.view"}}}
	reg, err := NewRegistry(ok)
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.Searchable(); len(got) != 1 || got[0].EntityType != "a.thing" {
		t.Fatalf("searchable: %+v", got)
	}
	if owner, found := reg.SearchOwner("a.thing"); !found || owner != "a" {
		t.Fatalf("owner: %q %v", owner, found)
	}
	for name, bad := range map[string]Module{
		"unprefixed type":      {Manifest: Manifest{Name: "a", Enabled: true}, Permissions: []string{"a.view"}, Search: []search.Declaration{{EntityType: "thing", Permission: "a.view"}}},
		"another's permission": {Manifest: Manifest{Name: "a", Enabled: true}, Permissions: []string{"a.view"}, Search: []search.Declaration{{EntityType: "a.thing", Permission: "b.view"}}},
		"no permission at all": {Manifest: Manifest{Name: "a", Enabled: true}, Search: []search.Declaration{{EntityType: "a.thing"}}},
		"the same type twice":  {Manifest: Manifest{Name: "a", Enabled: true}, Permissions: []string{"a.view"}, Search: []search.Declaration{{EntityType: "a.thing", Permission: "a.view"}, {EntityType: "a.thing", Permission: "a.view"}}},
	} {
		if _, err := NewRegistry(bad); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}

func TestNotificationsNeedAnOwnerPrefixAndKeysTheModuleCanTranslate(t *testing.T) {
	locales := fstest.MapFS{
		"messages.en.json": {Data: []byte(`{"notification.a.failed.title": "t", "notification.a.failed.body": "b"}`)},
		"messages.pl.json": {Data: []byte(`{"notification.a.failed.title": "t", "notification.a.failed.body": "b"}`)},
	}
	ok := Module{Manifest: Manifest{Name: "a", Enabled: true}, Locales: locales,
		Notifications: []notification.Definition{{Type: "a.failed", TitleKey: "notification.a.failed.title", BodyKey: "notification.a.failed.body"}}}
	reg, err := NewRegistry(ok)
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.Notifications(); len(got) != 1 || got[0].Type != "a.failed" {
		t.Fatalf("%+v", got)
	}
	for name, bad := range map[string]Module{
		"another's prefix":   {Manifest: Manifest{Name: "a", Enabled: true}, Locales: locales, Notifications: []notification.Definition{{Type: "b.failed", TitleKey: "notification.a.failed.title", BodyKey: "notification.a.failed.body"}}},
		"a key nobody holds": {Manifest: Manifest{Name: "a", Enabled: true}, Locales: locales, Notifications: []notification.Definition{{Type: "a.failed", TitleKey: "notification.a.missing.title", BodyKey: "notification.a.failed.body"}}},
		"no locales at all":  {Manifest: Manifest{Name: "a", Enabled: true}, Notifications: []notification.Definition{{Type: "a.failed", TitleKey: "notification.a.failed.title", BodyKey: "notification.a.failed.body"}}},
	} {
		if _, err := NewRegistry(bad); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	// Two providers of the one notifier is a configuration error, named.
	provider := func(*platform.Services) notification.Notifier { return notification.Log{} }
	if _, err := NewRegistry(Module{Manifest: Manifest{Name: "a", Enabled: true}, Notifier: provider}, Module{Manifest: Manifest{Name: "b", Enabled: true}, Notifier: provider}); err != nil {
		t.Fatal(err)
	} else if reg, _ := NewRegistry(Module{Manifest: Manifest{Name: "a", Enabled: true}, Notifier: provider}, Module{Manifest: Manifest{Name: "b", Enabled: true}, Notifier: provider}); reg != nil {
		if _, err := reg.Notifier(nil); err == nil {
			t.Fatal("two notifiers accepted")
		}
	}
}

func TestTasksArePrefixedAndReturnedByCadence(t *testing.T) {
	run := func(context.Context, *platform.Services) error { return nil }
	reg, err := NewRegistry(
		Module{Manifest: Manifest{Name: "a", Enabled: true}, Tasks: []Task{{Name: "a.sweep", Cadence: Daily, Run: run}, {Name: "a.tick", Cadence: EveryMinute, Run: run}}},
		Module{Manifest: Manifest{Name: "b", Enabled: true}, Tasks: []Task{{Name: "b.sweep", Cadence: Daily, Run: run}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	daily := reg.Tasks(Daily)
	if len(daily) != 2 || daily[0].Name != "a.sweep" || daily[1].Name != "b.sweep" {
		t.Fatalf("daily = %v", daily)
	}
	if m := reg.Tasks(EveryMinute); len(m) != 1 || m[0].Name != "a.tick" {
		t.Fatalf("minute = %v", m)
	}
	for _, bad := range []Task{
		{Name: "other.sweep", Cadence: Daily, Run: run},
		{Name: "a.", Cadence: Daily, Run: run},
		{Name: "a.x", Cadence: "hourly", Run: run},
		{Name: "a.x", Cadence: Daily},
	} {
		if _, err := NewRegistry(Module{Manifest: Manifest{Name: "a", Enabled: true}, Tasks: []Task{bad}}); err == nil {
			t.Fatalf("task %+v must be refused", bad)
		}
	}
}

// A module's log fields reach the process's logger after the kernel's, in
// module order (the dependency order a.Depends sets).
func TestLogFieldsAreCollectedInModuleOrder(t *testing.T) {
	field := func(v string) logctx.Field {
		return func(context.Context) []slog.Attr { return []slog.Attr{slog.String("from", v)} }
	}
	b := Module{Manifest: manifest("b"), LogFields: []logctx.Field{field("b")}}
	a := Module{Manifest: manifest("a"), LogFields: []logctx.Field{field("a1"), field("a2")}}
	a.Manifest.Depends = []string{"b"}
	r, err := NewRegistry(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range r.LogFields() {
		got = append(got, f(context.Background())[0].Value.String())
	}
	if strings.Join(got, " ") != "b a1 a2" {
		t.Fatalf("order: %v", got)
	}
}

// A dependent module's preference is asked first: Tenant's default is always
// set, so in plain module order a person's own choice would never be read.
func TestALocalePreferenceComesBeforeThoseOfTheModulesItDependsOn(t *testing.T) {
	prefer := func(l string) func(*platform.Services) httpx.LocalePreference {
		return func(*platform.Services) httpx.LocalePreference { return func(*http.Request) string { return l } }
	}
	tenant := Module{Manifest: manifest("tenant"), Locale: prefer("en")}
	identity := Module{Manifest: manifest("identity", "tenant"), Locale: prefer("pl")}
	r, err := NewRegistry(tenant, identity)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range r.LocalePreferences(platform.InMemory(nil)) {
		got = append(got, p(httptest.NewRequest("GET", "/", nil)))
	}
	if strings.Join(got, " ") != "pl en" {
		t.Fatalf("order: %v", got)
	}
}

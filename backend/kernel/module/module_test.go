package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func manifest(name string, deps ...string) Manifest {
	return Manifest{Name: name, Description: name, Depends: deps, Enabled: true}
}

func TestParseManifestAcceptsExactlyFourKeys(t *testing.T) {
	src := `# a comment
name: example
description: "The reference module - one of everything."
depends: [tenant, identity]   # inline
enabled: true
`
	m, err := ParseManifest([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "example" || m.Description != "The reference module - one of everything." || !m.Enabled {
		t.Errorf("parsed %+v", m)
	}
	if strings.Join(m.Depends, ",") != "tenant,identity" {
		t.Errorf("depends = %v", m.Depends)
	}
}

func TestParseManifestBlockList(t *testing.T) {
	m, err := ParseManifest([]byte("name: a\ndescription: x\ndepends:\n  - b\n  - c\nenabled: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(m.Depends, ",") != "b,c" || m.Enabled {
		t.Errorf("parsed %+v", m)
	}
}

func TestParseManifestRefusals(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"unknown key":    {"name: a\ndescription: x\ndepends: []\nenabled: true\nroutes: []\n", "unknown key \"routes\""},
		"missing key":    {"name: a\ndescription: x\nenabled: true\n", "missing key \"depends\""},
		"bad name":       {"name: Example\ndescription: x\ndepends: []\nenabled: true\n", "must match"},
		"bad enabled":    {"name: a\ndescription: x\ndepends: []\nenabled: yes\n", "enabled must be true or false"},
		"depends scalar": {"name: a\ndescription: x\ndepends: b\nenabled: true\n", "depends must be an inline list"},
		"twice":          {"name: a\nname: b\ndescription: x\ndepends: []\nenabled: true\n", "given twice"},
		"not key value":  {"name: a\njust words\ndescription: x\ndepends: []\nenabled: true\n", "expected `key: value`"},
	}
	for label, tc := range cases {
		_, err := ParseManifest([]byte(tc.src))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", label, err, tc.want)
		}
	}
}

func TestReadManifestInsistsOnTheDirectoryName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "billing")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "module.yaml"), []byte("name: invoicing\ndescription: x\ndepends: []\nenabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(dir); err == nil || !strings.Contains(err.Error(), "does not match the directory") {
		t.Errorf("err = %v", err)
	}
}

func TestRegistryOrdersByDependency(t *testing.T) {
	r, err := NewRegistry(
		Module{Manifest: manifest("webhook", "identity")},
		Module{Manifest: manifest("identity", "tenant")},
		Module{Manifest: manifest("tenant")},
		Module{Manifest: manifest("audit")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.Names(), ","); got != "audit,tenant,identity,webhook" {
		t.Errorf("order = %s", got)
	}
}

func TestRegistryDropsDisabledModules(t *testing.T) {
	off := manifest("off")
	off.Enabled = false
	r, err := NewRegistry(Module{Manifest: off}, Module{Manifest: manifest("on")})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("off"); ok || len(r.Names()) != 1 {
		t.Errorf("names = %v", r.Names())
	}
}

func TestRegistryRefusals(t *testing.T) {
	cases := map[string]struct {
		mods []Module
		want string
	}{
		"missing dependency": {[]Module{{Manifest: manifest("a", "b")}}, `depends on "b", which is not present`},
		"disabled dependency": {[]Module{{Manifest: manifest("a", "b")}, {Manifest: Manifest{Name: "b", Enabled: false}}},
			`depends on "b", which is not present or not enabled`},
		"cycle":          {[]Module{{Manifest: manifest("a", "b")}, {Manifest: manifest("b", "a")}}, "dependency cycle: a -> b -> a"},
		"self":           {[]Module{{Manifest: manifest("a", "a")}}, "depends on itself"},
		"duplicate name": {[]Module{{Manifest: manifest("a")}, {Manifest: manifest("a")}}, "declared twice"},
		"unprefixed permission": {[]Module{{Manifest: manifest("a"), Permissions: []string{"b.view"}}},
			`declares permission "b.view" - a permission is prefixed with its owner`},
		"bare prefix": {[]Module{{Manifest: manifest("a"), Permissions: []string{"a."}}}, "prefixed with its owner"},
		// The prefix rule already stops module b from claiming a.view, so the
		// only way to a double declaration is a module repeating itself.
		"permission declared twice": {[]Module{
			{Manifest: manifest("a"), Permissions: []string{"a.view", "a.manage", "a.view"}},
		}, "a permission with two owners has no meaning"},
	}
	for label, tc := range cases {
		_, err := NewRegistry(tc.mods...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", label, err, tc.want)
		}
	}
}

func TestRegistryPermissions(t *testing.T) {
	r, err := NewRegistry(
		Module{Manifest: manifest("b"), Permissions: []string{"b.manage", "b.view"}},
		Module{Manifest: manifest("a"), Permissions: []string{"a.view"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.Permissions(), ","); got != "a.view,b.manage,b.view" {
		t.Errorf("permissions = %s", got)
	}
	if o, _ := r.OwnerOf("b.manage"); o != "b" {
		t.Errorf("owner = %s", o)
	}
}

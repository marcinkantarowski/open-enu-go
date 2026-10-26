package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/module"
)

func write(t *testing.T, path, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildListsEveryDeclaration(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "demo", "internal", "entity.go"), "package internal\n\ntype widget struct{}\ntype widgetList struct{}\n")
	write(t, filepath.Join(root, "demo", "internal", "command.go"), "package internal\n\ntype createWidget struct{}\n")
	write(t, filepath.Join(root, "demo", "contract", "contract.go"), "package contract\n\nconst Topic = \"demo\"\n\ntype Reader interface{}\n\nfunc NewReader() Reader { return nil }\n\nfunc hidden() {}\n")
	m := module.Module{
		Manifest:    module.Manifest{Name: "demo", Description: "A demo.", Depends: []string{"tenant"}, Enabled: true},
		Permissions: []string{"demo.view"},
		Flags:       []flags.Definition{{Identifier: "demo.flag"}},
		Migrations:  fstest.MapFS{"0001_init.sql": {Data: []byte("CREATE TABLE demo_widget (id uuid);\ncreate table if not exists \"demo_other\" (id uuid);")}},
		OpenAPI:     []byte(`{"paths": {"/api/widgets": {"get": {"operationId": "get_widget_index"}}}}`),
	}
	doc, err := Build(root, []module.Module{m})
	if err != nil {
		t.Fatal(err)
	}
	e := doc.Modules["demo"]
	for field, got := range map[string][]string{
		"entities": e.Entities, "commands": e.Commands, "contract": e.Contract, "tables": e.Tables, "flags": e.Flags, "routes": e.Routes,
	} {
		want := map[string]string{
			"entities": "widget widgetList", "commands": "createWidget", "contract": "NewReader() Reader Topic",
			"tables": "demo_other demo_widget", "flags": "demo.flag", "routes": "GET /api/widgets (get_widget_index)",
		}[field]
		if strings.Join(got, " ") != want {
			t.Errorf("%s: %q, want %q", field, strings.Join(got, " "), want)
		}
	}
	out, err := doc.Render()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "{\n  \"modules\": {\n    \"demo\": {") || !strings.HasSuffix(string(out), "}\n") {
		t.Fatalf("render:\n%s", out)
	}
	if strings.Contains(string(out), `"tasks"`) {
		t.Fatal("an empty list is written; it should be omitted")
	}
}

func TestHintsNameNearDuplicatesOnly(t *testing.T) {
	doc := &Document{Modules: map[string]Entry{
		"a": {Entities: []string{"InvoiceFormatter", "Project"}},
		"b": {Contract: []string{"InvoiceFormater", "Projects", "Unrelated"}},
	}}
	hints := doc.Hints()
	if len(hints) != 2 || hints[0] != "a.InvoiceFormatter ~ b.InvoiceFormater" || hints[1] != "a.Project ~ b.Projects" {
		t.Fatalf("hints %v", hints)
	}
}

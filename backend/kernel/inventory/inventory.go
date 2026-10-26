// Package inventory writes .ai/inventory.json: every module and every name it
// declares, so "does something like this already exist?" is a grep rather
// than an exploration. The failure mode it addresses is REINVENTION - a
// second date formatter, a third way to page a list, written because finding
// the first meant already knowing its name.
//
// It is built from the registry and the module directories, never from a
// running process: an inventory that needs a container to boot is missing
// exactly when something is broken. `openenu inventory -check` fails when the
// committed copy is stale, which is what keeps it true.
package inventory

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/spec"
)

// Entry is one module's declarations. Every list is sorted; an empty one is
// omitted, so the document says what a module has rather than what it lacks.
type Entry struct {
	Description   string   `json:"description"`
	Depends       []string `json:"depends"`
	Entities      []string `json:"entities,omitempty"`
	Commands      []string `json:"commands,omitempty"`
	Contract      []string `json:"contract,omitempty"`
	Tables        []string `json:"tables,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
	Flags         []string `json:"flags,omitempty"`
	Tasks         []string `json:"tasks,omitempty"`
	Search        []string `json:"search,omitempty"`
	Notifications []string `json:"notifications,omitempty"`
	Routes        []string `json:"routes,omitempty"`
}

// Document is the file: modules by name.
type Document struct {
	Modules map[string]Entry `json:"modules"`
}

// Build reads every enabled module. modulesDir is backend/internal/module,
// where the source files the registry does not carry (entity.go, command.go,
// contract/) are read from.
func Build(modulesDir string, mods []module.Module) (*Document, error) {
	doc := &Document{Modules: map[string]Entry{}}
	for _, m := range mods {
		name := m.Manifest.Name
		dir := filepath.Join(modulesDir, name)
		e := Entry{Description: m.Manifest.Description, Depends: sorted(m.Manifest.Depends), Permissions: sorted(m.Permissions)}
		var err error
		if e.Entities, err = typeNames(filepath.Join(dir, "internal", "entity.go")); err != nil {
			return nil, err
		}
		if e.Commands, err = typeNames(filepath.Join(dir, "internal", "command.go")); err != nil {
			return nil, err
		}
		if e.Contract, err = exportedNames(filepath.Join(dir, "contract")); err != nil {
			return nil, err
		}
		if e.Tables, err = tables(m.Migrations); err != nil {
			return nil, err
		}
		for _, f := range m.Flags {
			e.Flags = append(e.Flags, f.Identifier)
		}
		for _, t := range m.Tasks {
			e.Tasks = append(e.Tasks, t.Name+" ("+string(t.Cadence)+")")
		}
		for _, s := range m.Search {
			e.Search = append(e.Search, s.EntityType+" ("+s.Permission+")")
		}
		for _, n := range m.Notifications {
			e.Notifications = append(e.Notifications, n.Type)
		}
		if len(m.OpenAPI) > 0 {
			ops, err := spec.Operations(m.OpenAPI)
			if err != nil {
				return nil, err
			}
			for _, op := range ops {
				e.Routes = append(e.Routes, op.Pattern()+" ("+op.ID+")")
			}
		}
		for _, l := range []*[]string{&e.Flags, &e.Tasks, &e.Search, &e.Notifications, &e.Routes} {
			sort.Strings(*l)
		}
		doc.Modules[name] = e
	}
	return doc, nil
}

// Render is the committed form: indented, keys sorted, trailing newline.
func (d *Document) Render() ([]byte, error) {
	out, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// Hints lists pairs of declared names close enough to be worth a look -
// `InvoiceFormatter` beside `InvoiceFormater`. Advisory, permanently: two
// helpers doing one job rarely share a name, so this can only ever be a hint,
// and a blocking version would be wrong often enough to get muted.
func (d *Document) Hints() []string {
	type named struct{ module, name string }
	var all []named
	for mod, e := range d.Modules {
		for _, list := range [][]string{e.Entities, e.Commands, e.Contract} {
			for _, n := range list {
				all = append(all, named{mod, n})
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].module+all[i].name < all[j].module+all[j].name })
	var out []string
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			if a.name == b.name || len(a.name) < 6 || len(b.name) < 6 {
				continue
			}
			if distance(a.name, b.name) <= 2 {
				out = append(out, a.module+"."+a.name+" ~ "+b.module+"."+b.name)
			}
		}
	}
	return out
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	if out == nil {
		out = []string{}
	}
	return out
}

// distance is Levenshtein, for names - short strings, so the simple table.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// Sync writes the rendered document to path, or with check compares it to
// what is committed there. It reports whether the committed copy differs.
// The write lives here rather than in the command because the kernel is
// where files are written (platformservice): the inventory is not tenant
// data, but the rule is simpler with no exceptions.
func (d *Document) Sync(path string, check bool) (stale bool, err error) {
	out, err := d.Render()
	if err != nil {
		return false, err
	}
	cur, err := os.ReadFile(path)
	stale = err != nil || !bytes.Equal(cur, out)
	if check || !stale {
		return stale, nil
	}
	return true, os.WriteFile(path, out, 0o644)
}

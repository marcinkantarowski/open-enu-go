package module

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Manifest is module.yaml: four keys, flat, nothing else.
//
// It is read by this package's own forty-line reader rather than a YAML
// library, on purpose. The manifest is an identity card, not a schema: the
// moment it can hold arbitrary structure, settings and routes and flags start
// migrating into it ("declaring settings in module.yaml" is refused for
// exactly that reason). A reader that refuses every key it does not know is
// the check that keeps it that way.
type Manifest struct {
	// Name is the module's identity: its directory, its Go package, the prefix
	// of every permission it declares. ^[a-z][a-z0-9_]*$.
	Name        string
	Description string
	// Depends lists modules that must be present and enabled. Only their
	// contract/ package may be imported - the compiler enforces that part.
	Depends []string
	// Enabled false keeps the module out of the registry entirely: no routes,
	// no migrations, no permissions.
	Enabled bool
}

var nameShape = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ParseManifest reads the four keys and refuses anything else.
//
// Accepted syntax, and only this:
//
//	name: example
//	description: One line of text.
//	depends: []            or   depends: [a, b]
//	enabled: true
//
// Comments (#) and blank lines are ignored. A block list (`depends:` followed
// by `- a` lines) is accepted too, because that is how a YAML-aware editor
// rewrites an inline one.
func ParseManifest(data []byte) (Manifest, error) {
	m := Manifest{}
	seen := map[string]bool{}
	var inDepends bool

	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		raw := sc.Text()
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}
		if inDepends && strings.HasPrefix(line, "- ") {
			m.Depends = append(m.Depends, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
			continue
		}
		inDepends = false

		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.ContainsAny(key, " \t") {
			return m, fmt.Errorf("module.yaml line %d: expected `key: value`, got %q", n, raw)
		}
		value = strings.TrimSpace(value)
		if seen[key] {
			return m, fmt.Errorf("module.yaml line %d: %q given twice", n, key)
		}
		seen[key] = true

		switch key {
		case "name":
			m.Name = unquote(value)
		case "description":
			m.Description = unquote(value)
		case "enabled":
			switch value {
			case "true":
				m.Enabled = true
			case "false":
				m.Enabled = false
			default:
				return m, fmt.Errorf("module.yaml line %d: enabled must be true or false, got %q", n, value)
			}
		case "depends":
			if value == "" {
				inDepends = true // block list follows
				m.Depends = []string{}
				continue
			}
			if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
				return m, fmt.Errorf("module.yaml line %d: depends must be an inline list like [a, b] or a block list of `- a` lines", n)
			}
			m.Depends = []string{}
			for _, d := range strings.Split(strings.Trim(value, "[]"), ",") {
				if d = strings.TrimSpace(unquote(d)); d != "" {
					m.Depends = append(m.Depends, d)
				}
			}
		default:
			return m, fmt.Errorf("module.yaml line %d: unknown key %q - the manifest holds exactly name, description, depends and enabled; "+
				"routes, permissions, flags and settings are declared in module.go, not here", n, key)
		}
	}
	if err := sc.Err(); err != nil {
		return m, err
	}
	for _, k := range []string{"name", "description", "depends", "enabled"} {
		if !seen[k] {
			return m, fmt.Errorf("module.yaml: missing key %q", k)
		}
	}
	if !nameShape.MatchString(m.Name) {
		return m, fmt.Errorf("module.yaml: name %q must match ^[a-z][a-z0-9_]*$ - it is the directory, the Go package and the permission prefix", m.Name)
	}
	for _, d := range m.Depends {
		if !nameShape.MatchString(d) {
			return m, fmt.Errorf("module.yaml: depends entry %q is not a module name", d)
		}
	}
	return m, nil
}

// MustManifest parses an embedded module.yaml at init. A module's module.go
// does `//go:embed module.yaml` and hands the bytes here, so the manifest
// has one source - the file - and the Go value cannot drift from it. The
// generator has already refused a malformed file; a panic here means the
// embed and the file disagree, which cannot happen without editing a build.
func MustManifest(data []byte) Manifest {
	m, err := ParseManifest(data)
	if err != nil {
		panic("module: " + err.Error())
	}
	return m
}

// ReadManifest parses <dir>/module.yaml and insists that name == basename(dir):
// a manifest that disagrees with its directory would be discovered under one
// name and imported under another.
func ReadManifest(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "module.yaml"))
	if err != nil {
		return Manifest{}, err
	}
	m, err := ParseManifest(data)
	if err != nil {
		return m, fmt.Errorf("%s: %w", dir, err)
	}
	if base := filepath.Base(dir); m.Name != base {
		return m, fmt.Errorf("%s/module.yaml: name %q does not match the directory %q - rename one of them", dir, m.Name, base)
	}
	return m, nil
}

func stripComment(s string) string {
	if i := strings.Index(s, " #"); i >= 0 {
		return s[:i]
	}
	if strings.HasPrefix(strings.TrimSpace(s), "#") {
		return ""
	}
	return s
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

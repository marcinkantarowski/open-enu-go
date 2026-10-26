// Package spec merges OpenAPI fragments: the kernel's shared components and
// each module's own paths into one document (tools/genspec), or one module's
// fragment with the shared components into the input oapi-codegen generates
// that module's server from (tools/genapi).
//
// The rule that makes merging safe is simple: a component name that appears
// twice must have identical content. A module that wants a different `Error`
// gets a build failure, not a silent override.
package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Document is the subset of OpenAPI this platform writes. Anything a
// fragment carries outside these keys is refused, so the merged file cannot
// accumulate stray top-level fields.
type Document struct {
	OpenAPI    string                                `json:"openapi"`
	Info       map[string]any                        `json:"info,omitempty"`
	Paths      map[string]json.RawMessage            `json:"paths"`
	Components map[string]map[string]json.RawMessage `json:"components,omitempty"`
	Security   []map[string][]string                 `json:"security,omitempty"`
}

// ParseFragment reads one module's openapi.json.
func ParseFragment(name string, data []byte) (*Document, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	for k := range raw {
		switch k {
		case "openapi", "info", "paths", "components", "security":
		default:
			return nil, fmt.Errorf("%s: unexpected top-level key %q - a fragment carries openapi, paths and components", name, k)
		}
	}
	var d Document
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if d.Paths == nil {
		d.Paths = map[string]json.RawMessage{}
	}
	return &d, nil
}

// ParseComponents reads the kernel's components.json (a bare components
// object: schemas, responses, securitySchemes).
func ParseComponents(data []byte) (map[string]map[string]json.RawMessage, error) {
	var c map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("shared components: %w", err)
	}
	return c, nil
}

// Merge combines fragments into one document. Paths must not collide at all;
// components must be identical when they collide.
func Merge(version string, shared map[string]map[string]json.RawMessage, fragments map[string]*Document) (*Document, error) {
	// 3.0.x, not 3.1: oapi-codegen's parser (kin-openapi) is complete for 3.0
	// and partial for 3.1, and nothing here needs 3.1.
	out := &Document{
		OpenAPI: "3.0.3",
		Info: map[string]any{
			"title":   "OpenEnu API",
			"version": version,
			"description": "Two realms share this API and one signing key. A token carries an `aud` claim - " +
				"`tenant` for workspace members, `manager` for platform operators - and is refused outside its realm " +
				"with the same 401 as no token (ADR-0019). An API key (`sk_...`) acts in the tenant realm with its own scopes.",
		},
		Paths:      map[string]json.RawMessage{},
		Components: map[string]map[string]json.RawMessage{},
		Security:   []map[string][]string{{"bearer": {}}},
	}
	owner := map[string]string{} // "schemas/Error" -> fragment name
	add := func(from string, comps map[string]map[string]json.RawMessage) error {
		for kind, items := range comps {
			if out.Components[kind] == nil {
				out.Components[kind] = map[string]json.RawMessage{}
			}
			for name, body := range items {
				key := kind + "/" + name
				if prev, ok := out.Components[kind][name]; ok {
					if !sameJSON(prev, body) {
						return fmt.Errorf("component %s is defined differently by %s and %s - one definition, shared, or two names", key, owner[key], from)
					}
					continue
				}
				out.Components[kind][name] = body
				owner[key] = from
			}
		}
		return nil
	}
	if err := add("kernel", shared); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(fragments))
	for n := range fragments {
		names = append(names, n)
	}
	sort.Strings(names)
	pathOwner := map[string]string{}
	for _, n := range names {
		f := fragments[n]
		for p, body := range f.Paths {
			if prev, ok := pathOwner[p]; ok {
				return nil, fmt.Errorf("path %s is declared by both %s and %s", p, prev, n)
			}
			pathOwner[p] = n
			out.Paths[p] = body
		}
		if err := add(n, f.Components); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Render writes the document with sorted keys and two-space indentation, so
// the committed file diffs cleanly.
func Render(d *Document) ([]byte, error) {
	// Round-trip through any so nested RawMessages are re-encoded with sorted keys.
	var generic any
	tmp, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(tmp, &generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(generic); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ca, _ := json.Marshal(x)
	cb, _ := json.Marshal(y)
	return bytes.Equal(ca, cb)
}

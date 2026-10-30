// Package mcp offers the API to an MCP client as tools generated from the
// spec and the ACL, and serves them over stdio with the official SDK.
//
// Generated, never hand-written: an endpoint added today is callable by an
// agent today, and one deleted stops being offered. A curated list would be a
// second place to remember, and the second place is always the one that is
// wrong.
package mcp

import (
	"fmt"
	"sort"

	"github.com/open-enu/kernel/spec"
)

// Tool is one operation an agent may call.
type Tool struct {
	Name        string // the operationId
	Description string
	Method      string
	Path        string
	Permission  string
	InputSchema map[string]any
}

// Catalogue derives the tools from three sources, each contributing what
// only it knows:
//
//   - Fragments: the modules' OpenAPI fragments - which operations are
//     PUBLISHED. A route that exists but is not in the spec is not offered.
//   - Permissions: operationId -> the permission its handler requires,
//     generated from the `auth.Require` literal in each api.go (tools/genacl).
//   - Declared: whether a module declares that permission (the registry).
//
// Only operations guarded by a declared module permission are offered, and
// that one rule does all the filtering. An API key is a scoped tenant
// credential: it cannot sign in, cannot refresh a browser session, and is
// refused by the operator realm on its `aud` claim (ADR-0019).
// Offering those would only produce an agent that spends its turns
// discovering which of its tools are permanently 401. An operation that
// does carry a permission but refuses every key anyway - the person's own
// profile - says so in its fragment with `"x-mcp": false`, and is skipped.
type Catalogue struct {
	Fragments   [][]byte
	Permissions map[string]string
	Declared    func(permission string) bool
}

// Tools lists what a client is offered, by name.
func (c Catalogue) Tools() ([]Tool, error) {
	if len(c.Fragments) == 0 {
		// Loud, not empty: a server offering nothing looks exactly like an
		// API with no endpoints, and the operator would look in the wrong place.
		return nil, fmt.Errorf("mcp: no spec fragments - nothing to offer")
	}
	var out []Tool
	for _, fragment := range c.Fragments {
		ops, err := spec.Operations(fragment)
		if err != nil {
			return nil, err
		}
		for _, op := range ops {
			perm := c.Permissions[op.ID]
			if op.NoMCP || perm == "" || c.Declared == nil || !c.Declared(perm) {
				continue
			}
			out = append(out, Tool{
				Name:        op.ID,
				Description: fmt.Sprintf("%s %s - requires the %q permission.", op.Method, op.Path, perm),
				Method:      op.Method,
				Path:        op.Path,
				Permission:  perm,
				InputSchema: schemaFor(op),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// schemaFor is the tool's argument schema: every path parameter, required;
// then `body` for a write or `query` for a read, free-form. The spec records
// each operation's body, but a schema copied here would be this file's guess
// validated in the client, where the reason is invisible; the server's own
// 400 names the field.
func schemaFor(op spec.Operation) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, p := range spec.PathParams(op.Path) {
		props[p] = map[string]any{"type": "string", "description": "Path parameter."}
		required = append(required, p)
	}
	switch op.Method {
	case "POST", "PUT", "PATCH":
		props["body"] = map[string]any{"type": "object", "description": "JSON request body."}
	default:
		props["query"] = map[string]any{"type": "object", "description": "Query-string parameters."}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

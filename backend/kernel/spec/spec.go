// Package spec reads what the kernel needs from an OpenAPI fragment: the
// operations it publishes. Three things ask that question - the inventory
// ("what routes exist"), the MCP catalogue ("what may an agent call") and
// route coverage ("what did the tests reach") - and one reader keeps them
// agreeing about what an operation is.
package spec

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Operation is one method on one path.
type Operation struct {
	Method string // upper-case
	Path   string // as written: /api/projects/{id}
	ID     string // operationId
	// NoTestReason is the `x-no-test-required` extension: the written reason
	// an operation is exempt from route coverage. Empty for almost all.
	NoTestReason string
}

// Pattern is the operation as net/http's ServeMux reports it in
// Request.Pattern: "GET /api/projects/{id}". The generated server registers
// exactly this string, so a trace of patterns is a trace of operations.
func (o Operation) Pattern() string { return o.Method + " " + o.Path }

// Operations lists a fragment's operations, sorted by path then method.
func Operations(fragment []byte) ([]Operation, error) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(fragment, &doc); err != nil {
		return nil, fmt.Errorf("spec: %w", err)
	}
	var out []Operation
	for path, item := range doc.Paths {
		for method, raw := range item {
			switch method {
			case "get", "post", "put", "patch", "delete", "head", "options":
			default:
				continue // parameters, summary, extensions
			}
			var op struct {
				ID       string `json:"operationId"`
				NoTestRQ string `json:"x-no-test-required"`
			}
			if err := json.Unmarshal(raw, &op); err != nil {
				return nil, fmt.Errorf("spec: %s %s: %w", method, path, err)
			}
			out = append(out, Operation{Method: strings.ToUpper(method), Path: path, ID: op.ID, NoTestReason: op.NoTestRQ})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out, nil
}

// PathParams names the {segments} of a path, in order.
func PathParams(path string) []string {
	var out []string
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			out = append(out, seg[1:len(seg)-1])
		}
	}
	return out
}

package spec

import (
	"encoding/json"
	"strings"
	"testing"
)

const shared = `{"schemas": {"Error": {"type": "object"}}, "responses": {"Error": {"description": "x"}}}`

func frag(t *testing.T, name, src string) *Document {
	t.Helper()
	d, err := ParseFragment(name, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestMergeCombinesPathsAndSharedComponents(t *testing.T) {
	sh, _ := ParseComponents([]byte(shared))
	a := frag(t, "a", `{"openapi":"3.1.0","paths":{"/api/a":{"get":{"operationId":"get_a"}}},"components":{"schemas":{"A":{"type":"string"}}}}`)
	b := frag(t, "b", `{"openapi":"3.1.0","paths":{"/api/b":{"get":{"operationId":"get_b"}}}}`)
	out, err := Merge("1.2.3", sh, map[string]*Document{"b": b, "a": a})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Paths) != 2 || out.Components["schemas"]["Error"] == nil || out.Components["schemas"]["A"] == nil || out.Components["responses"]["Error"] == nil {
		t.Errorf("merged = %+v", out)
	}
	if out.Info["version"] != "1.2.3" {
		t.Errorf("version = %v", out.Info["version"])
	}
	rendered, err := Render(out)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(rendered) || !strings.HasPrefix(string(rendered), "{\n  \"components\"") {
		t.Errorf("render is not sorted, indented JSON:\n%s", rendered)
	}
}

func TestMergeRefusesConflicts(t *testing.T) {
	sh, _ := ParseComponents([]byte(shared))
	cases := map[string]struct {
		a, b string
		want string
	}{
		"same path twice": {
			`{"openapi":"3.1.0","paths":{"/api/x":{}}}`, `{"openapi":"3.1.0","paths":{"/api/x":{}}}`,
			"path /api/x is declared by both a and b",
		},
		"shared component redefined": {
			`{"openapi":"3.1.0","paths":{},"components":{"schemas":{"Error":{"type":"string"}}}}`, `{"openapi":"3.1.0","paths":{}}`,
			"component schemas/Error is defined differently by kernel and a",
		},
	}
	for label, tc := range cases {
		_, err := Merge("0", sh, map[string]*Document{"a": frag(t, "a", tc.a), "b": frag(t, "b", tc.b)})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v", label, err)
		}
	}
}

func TestIdenticalDuplicateComponentIsFine(t *testing.T) {
	sh, _ := ParseComponents([]byte(shared))
	a := frag(t, "a", `{"openapi":"3.1.0","paths":{},"components":{"schemas":{"Error":{"type":"object"}}}}`)
	if _, err := Merge("0", sh, map[string]*Document{"a": a}); err != nil {
		t.Errorf("an identical copy must be accepted: %v", err)
	}
}

func TestFragmentRefusesStrayKeys(t *testing.T) {
	if _, err := ParseFragment("x", []byte(`{"openapi":"3.1.0","paths":{},"servers":[]}`)); err == nil || !strings.Contains(err.Error(), `unexpected top-level key "servers"`) {
		t.Errorf("err = %v", err)
	}
}

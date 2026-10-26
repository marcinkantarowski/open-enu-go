// Package flags is runtime configuration and kill switches.
//
// Resolution is global default -> tenant override. On a single host with no
// blue/green, turning a feature off without a deploy is how a bad release
// stops being an outage. A module declares the flags it reads as
// Definitions on its Module value - beside the code the flag guards, not in
// a central list - and reads them through Flags. The flagdeclared analyzer
// refuses a read of a key with no Definition in the same package: an
// undeclared flag resolves to the call's default forever, invisible in the
// operator console and impossible to turn off during an incident, which is
// the one job a kill switch has.
//
// A disabled flag on a route makes it 404, not 403: 403 says the feature
// exists and you cannot have it, which a kill switch should not be leaking.
package flags

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The value types a Definition may declare.
const (
	TypeBool   = "bool"
	TypeString = "string"
	TypeInt    = "int"
	TypeJSON   = "json"
)

// Definition is a flag a module declares, so it can be seen and switched.
type Definition struct {
	// Identifier is dotted and namespaced by module: `example.archive`.
	Identifier  string
	Name        string
	Description string
	Default     any
	Type        string // one of the Type constants; "" means bool
	// TenantEditable is whether a tenant may change this for itself. false
	// for anything that is a platform kill switch: a tenant re-enabling a
	// feature an operator turned off during an incident defeats the point.
	TenantEditable bool
	Category       string
}

var identShape = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// Validate checks the identifier's shape and the type.
func (d Definition) Validate() error {
	if !identShape.MatchString(d.Identifier) {
		return fmt.Errorf("flag %q: identifier must be dotted lowercase, namespaced by module: `example.archive`", d.Identifier)
	}
	switch d.Type {
	case "", TypeBool, TypeString, TypeInt, TypeJSON:
		return nil
	}
	return fmt.Errorf("flag %q: unknown type %q (bool, string, int, json)", d.Identifier, d.Type)
}

// Flags is what a module reads. Reads are cheap; calling this in a hot path
// is fine.
type Flags interface {
	Enabled(ctx context.Context, identifier string, def bool) bool
	String(ctx context.Context, identifier string, def string) string
	Int(ctx context.Context, identifier string, def int) int
	JSON(ctx context.Context, identifier string, def map[string]any) map[string]any
}

// Static resolves from a fixed map - configuration before the Settings
// module exists, and tests. Coercion: a string "1"/"true"/"yes"/"on" is
// enabled, an int is enabled when non-zero.
type Static map[string]any

// Enabled resolves a bool.
func (s Static) Enabled(_ context.Context, id string, def bool) bool { return asBool(s[id], def) }

// String resolves a string.
func (s Static) String(_ context.Context, id string, def string) string {
	return asString(s[id], def)
}

// Int resolves an int.
func (s Static) Int(_ context.Context, id string, def int) int { return asInt(s[id], def) }

// JSON resolves an object.
func (s Static) JSON(_ context.Context, id string, def map[string]any) map[string]any {
	return asJSON(s[id], def)
}

func asBool(v any, def bool) bool {
	switch x := v.(type) {
	case nil:
		return def
	case bool:
		return x
	case string:
		switch strings.ToLower(x) {
		case "1", "true", "yes", "on":
			return true
		}
		return false
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	}
	return def
}

func asString(v any, def string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func asInt(v any, def int) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		if x == float64(int(x)) {
			return int(x)
		}
	case string:
		if n, err := strconv.Atoi(x); err == nil && x != "" && !strings.HasPrefix(x, "-") && !strings.HasPrefix(x, "+") {
			return n
		}
	}
	return def
}

func asJSON(v any, def map[string]any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return def
}

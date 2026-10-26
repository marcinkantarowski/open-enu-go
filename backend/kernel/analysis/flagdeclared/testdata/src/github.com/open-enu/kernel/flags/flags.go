// Stub of kernel/flags for the analyzer fixture: the type names are what the
// analyzer keys on.
package flags

import "context"

type Definition struct {
	Identifier  string
	Name        string
	Description string
	Default     any
	Type        string
}

type Flags interface {
	Enabled(ctx context.Context, identifier string, def bool) bool
	String(ctx context.Context, identifier string, def string) string
	Int(ctx context.Context, identifier string, def int) int
	JSON(ctx context.Context, identifier string, def map[string]any) map[string]any
}

type Static map[string]any

func (s Static) Enabled(context.Context, string, bool) bool                  { return false }
func (s Static) String(context.Context, string, string) string               { return "" }
func (s Static) Int(context.Context, string, int) int                        { return 0 }
func (s Static) JSON(context.Context, string, map[string]any) map[string]any { return nil }

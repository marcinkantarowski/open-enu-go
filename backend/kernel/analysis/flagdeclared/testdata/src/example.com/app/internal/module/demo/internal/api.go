package internal

import (
	"context"

	"github.com/open-enu/kernel/flags"
)

type api struct {
	flags flags.Flags
}

func (a api) handle(ctx context.Context, key string) {
	_ = a.flags.Enabled(ctx, "demo.archive", false)  // declared
	_ = a.flags.Int(ctx, pageSize, 20)               // declared, via a constant
	_ = a.flags.Enabled(ctx, "demo.export", false)   // want `flag "demo.export" is read but not declared in this package`
	_ = a.flags.String(ctx, "demo.theme", "")        // want `flag "demo.theme" is read but not declared`
	_ = a.flags.Enabled(ctx, key, false)             // want `flag key must be a string literal`
	_ = flags.Static{}.JSON(ctx, "demo.layout", nil) // want `flag "demo.layout" is read but not declared`
	_ = other{}.Enabled(ctx, "demo.not_a_flag", false)
}

// other has the same method names but is not a flags type; not our business.
type other struct{}

func (other) Enabled(context.Context, string, bool) bool { return false }

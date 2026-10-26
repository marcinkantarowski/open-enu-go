package internal

import (
	"context"
	"time"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// sweepAfterDays is how long an archived project stays. Retention is the
// module's demonstration of work on a clock: nobody asks for it, it is due
// by the data's own dates, and a missed run is caught up by the next one.
const sweepAfterDays = 30

// Tasks is what module.go registers; `openenu scheduler` runs it daily.
var Tasks = []module.Task{{Name: "example.sweep", Cadence: module.Daily, Run: Sweep}}

// Sweep deletes every project archived more than sweepAfterDays ago, in
// every tenant. The cross-tenant question - WHICH tenants hold one - is the
// repository's, asked unscoped with a reason. The deletion is a command
// dispatched inside each tenant's scope: the policy still applies to the
// write, and each tenant's trail records what left it.
func Sweep(ctx context.Context, svc *platform.Services) error {
	before := time.Now().UTC().Add(-sweepAfterDays * 24 * time.Hour)
	tenants, err := newRepository(svc).tenantsWithArchivedBefore(ctx, before)
	if err != nil {
		return err
	}
	for _, tid := range tenants {
		if _, err := svc.Bus.Dispatch(scope.WithTenant(ctx, tid), sweepArchived{Before: before}); err != nil {
			return err
		}
	}
	return nil
}

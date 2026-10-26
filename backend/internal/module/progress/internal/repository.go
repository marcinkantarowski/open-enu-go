package internal

import (
	"context"

	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/progress"
)

// repository reads progress_job through the kernel's Store: the kernel's
// reporter writes the rows (kernel migration 0003), this module only reads
// them, so the shape is defined once, in kernel/progress.Snapshot.
type repository struct{ store progress.Store }

func newRepository(svc *platform.Services) repository {
	return repository{store: progress.Store{Pool: svc.DB}}
}

func (r repository) get(ctx context.Context, id string) (progress.Snapshot, bool, error) {
	if len(id) == 0 || len(id) > 32 {
		return progress.Snapshot{}, false, nil
	}
	return r.store.Get(ctx, id)
}

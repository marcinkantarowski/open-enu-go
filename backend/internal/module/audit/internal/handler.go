package internal

import (
	"context"

	"github.com/open-enu/kernel/scope"
)

// handlers are the only code that receives *scope.Tx (writeboundary).
type handlers struct{ repo repository }

func (h handlers) anonymise(ctx context.Context, _ *scope.Tx, c anonymiseActor) (int, error) {
	// Across every tenant: the person acted wherever they were a member.
	// The repository lifts isolation by name and writes in its own
	// transaction; this one carries the entry that says it happened.
	return h.repo.anonymise(ctx, c.UserID)
}

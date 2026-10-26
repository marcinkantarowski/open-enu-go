package internal

import (
	"context"

	"github.com/open-enu/kernel/scope"
)

type rename struct{ Name string }

// A handler receives the Tx and hands it to the repository: fine.
func handleRename(ctx context.Context, tx *scope.Tx, c rename) error {
	return update(ctx, tx, c.Name)
}

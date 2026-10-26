package internal

import (
	"io/fs"
	"os"
	"testing"
)

// migrationsFS is ../migrations, read from disk: module.go embeds it, and
// this package is below module.go.
func migrationsFS(t *testing.T) fs.FS {
	t.Helper()
	return os.DirFS("../migrations")
}

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/crypto/columns"
)

func init() {
	subcommands["encrypted"] = command{summary: "encrypted export|import - every encrypted value in the open on stdout, or sealed from stdin with this stack's key (make copyproddata)", run: runEncrypted}
}

// runEncrypted is how a copy of the database changes master key: `export` on
// the source, piped into `import` on the target. It goes through the owner's
// DSN, because it reads every tenant, and `import` writes in one transaction.
func runEncrypted(args []string) int {
	if len(args) != 1 || (args[0] != "export" && args[0] != "import") {
		return fail("encrypted", fmt.Errorf("usage: openenu encrypted export|import"))
	}
	if stage := os.Getenv("APP_STAGE"); args[0] == "import" && stage != "local" {
		return fail("encrypted import", fmt.Errorf("APP_STAGE is %q: a copy goes into a local stack only", stage))
	}
	ctx := context.Background()
	keys, err := crypto.NewDerivedKeys(os.Getenv("APP_ENCRYPTION_KEY"))
	if err != nil {
		return fail("encrypted", err)
	}
	enc := crypto.NewEncryptor(keys)
	db, err := pgx.Connect(ctx, os.Getenv("DATABASE_ADMIN_URL"))
	if err != nil {
		return fail("encrypted", err)
	}
	defer db.Close(ctx)
	tx, err := db.Begin(ctx)
	if err != nil {
		return fail("encrypted", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a no-op after commit
	if args[0] == "export" {
		n, err := columns.Export(ctx, tx, enc, os.Stdout)
		if err != nil {
			return fail("encrypted export", err)
		}
		fmt.Fprintf(os.Stderr, "%d value(s) exported\n", n)
		return 0
	}
	written, skipped, err := columns.Import(ctx, tx, enc, os.Stdin)
	if err != nil {
		return fail("encrypted import", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail("encrypted import", err)
	}
	fmt.Printf("%d value(s) sealed with this stack's key, %d skipped\n", written, skipped)
	return 0
}

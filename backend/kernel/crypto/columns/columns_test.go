package columns

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/testdb"
)

const tenant = "11111111-1111-1111-1111-111111111111"

func encryptor(t *testing.T, master string) *crypto.Encryptor {
	t.Helper()
	keys, err := crypto.NewDerivedKeys(master)
	if err != nil {
		t.Fatal(err)
	}
	return crypto.NewEncryptor(keys)
}

func inTx(t *testing.T, db *testdb.DB, fn func(pgx.Tx) error) error {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a no-op after commit
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func TestAValueMovesToAnotherKey(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	prod := encryptor(t, strings.Repeat("p", 40))
	dev := encryptor(t, strings.Repeat("d", 40))

	mail, _ := prod.Encrypt("jan@example.com", "")
	secret, _ := prod.Encrypt("whsec", tenant)
	for _, q := range []string{
		`CREATE TABLE person (id uuid PRIMARY KEY, email text, email_hash text, note text)`,
		`CREATE TABLE hook (id uuid PRIMARY KEY, tenant_id uuid, secret text)`,
	} {
		if _, err := db.Admin.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	const row = "22222222-2222-2222-2222-222222222222"
	if _, err := db.Admin.Exec(ctx, `INSERT INTO person VALUES ($1, $2, $3, 'v1:a plain note')`, row, mail, prod.HashForLookup("jan@example.com")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Admin.Exec(ctx, `INSERT INTO hook VALUES ($1, $2, $3)`, row, tenant, secret); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := inTx(t, db, func(tx pgx.Tx) error {
		n, err := Export(ctx, tx, prod, &out)
		if n != 2 {
			t.Errorf("exported %d values, want 2", n)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"plain":"whsec"`) {
		t.Fatalf("the export does not carry the value in the open: %s", out.String())
	}

	stream := out.String() + `{"table":"gone","column":"secret","id":"x","plain":"y"}` + "\n"
	if err := inTx(t, db, func(tx pgx.Tx) error {
		written, skipped, err := Import(ctx, tx, dev, strings.NewReader(stream))
		if written != 2 || skipped != 1 {
			t.Errorf("written %d, skipped %d, want 2 and 1", written, skipped)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	var gotMail, gotHash, gotSecret, note string
	if err := db.Admin.QueryRow(ctx, `SELECT email, email_hash, note FROM person`).Scan(&gotMail, &gotHash, &note); err != nil {
		t.Fatal(err)
	}
	if err := db.Admin.QueryRow(ctx, `SELECT secret FROM hook`).Scan(&gotSecret); err != nil {
		t.Fatal(err)
	}
	if plain, err := dev.Decrypt(gotMail, ""); err != nil || plain != "jan@example.com" {
		t.Errorf("global value under the new key: %q, %v", plain, err)
	}
	if plain, err := dev.Decrypt(gotSecret, tenant); err != nil || plain != "whsec" {
		t.Errorf("tenant value under the new key: %q, %v", plain, err)
	}
	if gotHash != dev.HashForLookup("jan@example.com") {
		t.Error("the lookup hash was not recomputed: nobody could log in to the copy")
	}
	if note != "v1:a plain note" {
		t.Errorf("a text that only starts like a value changed: %q", note)
	}
}

func TestAValueLeftUnderTheOldKeyFailsTheImport(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	prod := encryptor(t, strings.Repeat("p", 40))
	dev := encryptor(t, strings.Repeat("d", 40))
	stale, _ := prod.Encrypt("left behind", "")
	if _, err := db.Admin.Exec(ctx, `CREATE TABLE person (id uuid PRIMARY KEY, email text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Admin.Exec(ctx, `INSERT INTO person VALUES (gen_random_uuid(), $1)`, stale); err != nil {
		t.Fatal(err)
	}
	err := inTx(t, db, func(tx pgx.Tx) error {
		_, _, err := Import(ctx, tx, dev, strings.NewReader(""))
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "person.email") {
		t.Fatalf("got %v, want a failure naming the column", err)
	}
}

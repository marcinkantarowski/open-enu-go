// Package columns moves encrypted values from one master key to another
// without either key leaving its machine: Export opens them under one, Import
// seals them under the other. `make copyproddata` is what it is for.
//
// The kernel knows no module's tables, so the values are found from the data:
// every text column of the public schema is searched for the exact shape
// crypto.Encrypt writes. A new encrypted column is included by being used.
package columns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/crypto"
)

// sealed is crypto.Encrypt's output: a 12-byte IV and a 16-byte tag, base64.
const sealed = `^v1:[A-Za-z0-9+/]{16}:[A-Za-z0-9+/=]*:[A-Za-z0-9+/]{22}==$`

// Value is one encrypted value in the open, one line of the stream between
// Export and Import. TenantID is empty for a value under the global key.
type Value struct {
	Table    string `json:"table"`
	Column   string `json:"column"`
	ID       string `json:"id"`
	TenantID string `json:"tenantId"`
	Plain    string `json:"plain"`
}

type column struct {
	table, name             string
	hasID, tenanted, hashed bool
}

func (c column) key() string { return c.table + "." + c.name }

func (c column) ident() (table, name string) {
	return pgx.Identifier{c.table}.Sanitize(), pgx.Identifier{c.name}.Sanitize()
}

func discover(ctx context.Context, tx pgx.Tx) ([]column, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.table_name, c.column_name,
		       EXISTS (SELECT FROM information_schema.columns o WHERE o.table_schema = c.table_schema
		               AND o.table_name = c.table_name AND o.column_name = 'id'),
		       EXISTS (SELECT FROM information_schema.columns o WHERE o.table_schema = c.table_schema
		               AND o.table_name = c.table_name AND o.column_name = 'tenant_id'),
		       EXISTS (SELECT FROM information_schema.columns o WHERE o.table_schema = c.table_schema
		               AND o.table_name = c.table_name AND o.column_name = c.column_name || '_hash')
		FROM information_schema.columns c
		JOIN information_schema.tables t USING (table_schema, table_name)
		WHERE c.table_schema = 'public' AND t.table_type = 'BASE TABLE'
		  AND c.data_type IN ('text', 'character varying')
		ORDER BY 1, 2`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (column, error) {
		var c column
		return c, r.Scan(&c.table, &c.name, &c.hasID, &c.tenanted, &c.hashed)
	})
}

// walk calls fn with every encrypted value, opened under enc. A value that has
// the shape and does not open stops it: leaving one behind silently would be a
// record the application cannot read.
func walk(ctx context.Context, tx pgx.Tx, enc *crypto.Encryptor, fn func(Value) error) error {
	cols, err := discover(ctx, tx)
	if err != nil {
		return err
	}
	for _, c := range cols {
		table, name := c.ident()
		id, tenant := "''", "''"
		if c.hasID {
			id = "id::text"
		}
		if c.tenanted {
			tenant = "coalesce(tenant_id::text, '')"
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT %s, %s, %s FROM %s WHERE %s ~ $1`, id, tenant, name, table, name), sealed)
		if err != nil {
			return fmt.Errorf("%s: %w", c.key(), err)
		}
		values, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Value, error) {
			v := Value{Table: c.table, Column: c.name}
			return v, r.Scan(&v.ID, &v.TenantID, &v.Plain)
		})
		if err != nil {
			return fmt.Errorf("%s: %w", c.key(), err)
		}
		for _, v := range values {
			if !c.hasID {
				return fmt.Errorf("%s holds an encrypted value and the table has no id", c.key())
			}
			if v.Plain, err = enc.Decrypt(v.Plain, v.TenantID); err != nil {
				return fmt.Errorf("%s, row %s: %w", c.key(), v.ID, err)
			}
			if err := fn(v); err != nil {
				return err
			}
		}
	}
	return nil
}

// Export writes every encrypted value to w in the open, one JSON object per
// line, and returns how many.
func Export(ctx context.Context, tx pgx.Tx, enc *crypto.Encryptor, w io.Writer) (int, error) {
	out, n := json.NewEncoder(w), 0
	err := walk(ctx, tx, enc, func(v Value) error {
		n++
		return out.Encode(v)
	})
	return n, err
}

// Import seals what Export wrote under enc, recomputes the sibling
// `<column>_hash` a lookup goes through, then reads every encrypted value back.
// A value for a column this database does not have is skipped, never written;
// one that opens under neither key fails the transaction.
func Import(ctx context.Context, tx pgx.Tx, enc *crypto.Encryptor, r io.Reader) (written, skipped int, err error) {
	cols, err := discover(ctx, tx)
	if err != nil {
		return 0, 0, err
	}
	known := map[string]column{}
	for _, c := range cols {
		known[c.key()] = c
	}
	in := json.NewDecoder(r)
	for {
		var v Value
		if err := in.Decode(&v); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return written, skipped, fmt.Errorf("line %d: %w", written+skipped+1, err)
		}
		c, ok := known[v.Table+"."+v.Column]
		if !ok || !c.hasID {
			skipped++
			continue
		}
		ct, err := enc.Encrypt(v.Plain, v.TenantID)
		if err != nil {
			return written, skipped, err
		}
		table, name := c.ident()
		tag, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE id::text = $2`, table, name), ct, v.ID)
		if err != nil {
			return written, skipped, fmt.Errorf("%s: %w", c.key(), err)
		}
		if tag.RowsAffected() == 0 {
			skipped++
			continue
		}
		if c.hashed {
			hash := pgx.Identifier{c.name + "_hash"}.Sanitize()
			if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE id::text = $2`, table, hash), enc.HashForLookup(v.Plain), v.ID); err != nil {
				return written, skipped, fmt.Errorf("%s: %w", c.key(), err)
			}
		}
		written++
	}
	return written, skipped, walk(ctx, tx, enc, func(Value) error { return nil })
}

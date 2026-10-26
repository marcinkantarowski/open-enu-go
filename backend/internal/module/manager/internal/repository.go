package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// repository is the only file with SQL. platform_manager has no tenant and
// no policy; the worker tables are River's. Operator writes - creation and
// the last-login stamp - are infrastructure, outside the bus: the first
// operator exists before any actor does, and a login is in the access log.
type repository struct {
	db     *scope.Pool
	crypto *crypto.Encryptor
}

func newRepository(svc *platform.Services) repository {
	return repository{db: svc.DB, crypto: svc.Crypto}
}

var errNotFound = errors.New("manager: no such operator")

const columns = `id, email, email_hash, password, display_name, roles, active, version, created_at, last_login_at`

func (r repository) byEmailHash(ctx context.Context, hash string) (o operator, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM platform_manager WHERE email_hash = $1`, hash)
		if err != nil {
			return err
		}
		items, err := r.scan(rows)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return errNotFound
		}
		o = items[0]
		return nil
	})
	return o, err
}

func (r repository) byID(ctx context.Context, id string) (o operator, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM platform_manager WHERE id = $1`, id)
		if err != nil {
			return err
		}
		items, err := r.scan(rows)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return errNotFound
		}
		o = items[0]
		return nil
	})
	return o, err
}

func (r repository) insert(ctx context.Context, o operator) error {
	email, err := r.crypto.Encrypt(o.Email, "")
	if err != nil {
		return err
	}
	var name *string
	if o.DisplayName != nil {
		n, err := r.crypto.Encrypt(*o.DisplayName, "")
		if err != nil {
			return err
		}
		name = &n
	}
	return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO platform_manager (`+columns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			o.ID, email, o.EmailHash, o.Password, name, o.Roles, o.Active, o.Version, o.CreatedAt, o.LastLoginAt)
		return err
	})
}

// recordLogin stamps the login and, given one, replaces the verifier with a
// current one - the same password, so not a change worth auditing.
func (r repository) recordLogin(ctx context.Context, id, verifier string) error {
	return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE platform_manager SET last_login_at = now(), password = COALESCE(NULLIF($2, ''), password) WHERE id = $1`, id, verifier)
		return err
	})
}

// queueDepths is what is waiting per River queue; failed is what River
// gave up on. Platform infrastructure, no tenant.
func (r repository) queueDepths(ctx context.Context) (queues map[string]int, failed int, err error) {
	queues = map[string]int{}
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT queue, count(*) FROM river_job WHERE state IN ('available', 'scheduled', 'retryable') GROUP BY queue`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var q string
			var n int
			if err := rows.Scan(&q, &n); err != nil {
				return err
			}
			queues[q] = n
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		return tx.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE state = 'discarded'`).Scan(&failed)
	})
	return queues, failed, err
}

func (r repository) scan(rows pgx.Rows) ([]operator, error) {
	defer rows.Close()
	var out []operator
	for rows.Next() {
		var o operator
		var email string
		var name *string
		if err := rows.Scan(&o.ID, &email, &o.EmailHash, &o.Password, &name, &o.Roles, &o.Active, &o.Version, &o.CreatedAt, &o.LastLoginAt); err != nil {
			return nil, err
		}
		plain, err := r.crypto.Decrypt(email, "")
		if err != nil {
			return nil, fmt.Errorf("manager: decrypt email of %s: %w", o.ID, err)
		}
		o.Email = plain
		if name != nil {
			n, err := r.crypto.Decrypt(*name, "")
			if err != nil {
				return nil, fmt.Errorf("manager: decrypt display name of %s: %w", o.ID, err)
			}
			o.DisplayName = &n
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

package internal

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// repository is the only file with SQL. Every write here is read state or a
// side effect of an already-audited change, so none goes through the bus:
// the trail exists for changes somebody might have to answer for, and "opened
// their notifications" would add a row per glance and bury it.
type repository struct{ db *scope.Pool }

func newRepository(svc *platform.Services) repository { return repository{db: svc.DB} }

const columns = `tenant_id, id, user_id, type, context, read_at, created_at`

// feedLimit is a screen's worth; the GDPR export passes its own.
const feedLimit = 50

func (r repository) insert(ctx context.Context, n row) error {
	return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO notification (`+columns+`) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			n.TenantID, n.ID, n.UserID, n.Type, n.Context, n.ReadAt, n.CreatedAt)
		return err
	})
}

// feedFor is one person's rows in the ambient scope, newest first.
func (r repository) feedFor(ctx context.Context, userID string, limit int) (out []row, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM notification WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, userID, limit)
		if err != nil {
			return err
		}
		out, err = scan(rows)
		return err
	})
	return out, err
}

func (r repository) unreadCount(ctx context.Context, userID string) (n int, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM notification WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	})
	return n, err
}

// markRead is idempotent - two tabs marking one item must not move the
// timestamp - and false when the row is not in THIS person's feed, which is
// also the answer when it is somebody else's.
func (r repository) markRead(ctx context.Context, userID, id string) (found bool, err error) {
	if _, perr := uuid.Parse(id); perr != nil {
		return false, nil
	}
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE notification SET read_at = COALESCE(read_at, now()) WHERE id = $1 AND user_id = $2`, id, userID)
		found = tag.RowsAffected() == 1
		return err
	})
	return found, err
}

func (r repository) markAllRead(ctx context.Context, userID string) error {
	return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE notification SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID)
		return err
	})
}

// allOf is every tenant's rows for one person: a subject access request is
// about the person, not about one workspace they happen to be in.
func (r repository) allOf(ctx context.Context, userID string) (out []row, err error) {
	err = scope.Unscoped(ctx, "notification: a subject access request covers every tenant the person belongs to", func(ctx context.Context) error {
		return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			rows, err := tx.Query(ctx, `SELECT `+columns+` FROM notification WHERE user_id = $1 ORDER BY created_at DESC, id DESC`, userID)
			if err != nil {
				return err
			}
			out, err = scan(rows)
			return err
		})
	})
	return out, err
}

func (r repository) eraseOf(ctx context.Context, userID string) (n int, err error) {
	err = scope.Unscoped(ctx, "notification: erasing a person from every tenant they belong to", func(ctx context.Context) error {
		return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			tag, err := tx.Exec(ctx, `DELETE FROM notification WHERE user_id = $1`, userID)
			n = int(tag.RowsAffected())
			return err
		})
	})
	return n, err
}

func scan(rows pgx.Rows) ([]row, error) {
	defer rows.Close()
	var out []row
	for rows.Next() {
		var n row
		var raw []byte
		if err := rows.Scan(&n.TenantID, &n.ID, &n.UserID, &n.Type, &raw, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &n.Context); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

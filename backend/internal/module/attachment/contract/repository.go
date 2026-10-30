package contract

import (
	"context"

	"github.com/google/uuid"

	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// NewFiles is the Files over the platform's pool.
func NewFiles(svc *platform.Services) Files { return files{db: svc.DB} }

type files struct{ db *scope.Pool }

func (r files) Find(ctx context.Context, id string) (f File, found bool, err error) {
	if _, err := uuid.Parse(id); err != nil {
		return File{}, false, nil
	}
	err = r.db.Read(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, filename, content_type, size, COALESCE(owner_type, ''), COALESCE(owner_id, '') FROM attachment WHERE id = $1`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		if !rows.Next() {
			return rows.Err()
		}
		found = true
		return rows.Scan(&f.ID, &f.Filename, &f.ContentType, &f.Size, &f.OwnerType, &f.OwnerID)
	})
	return f, found, err
}

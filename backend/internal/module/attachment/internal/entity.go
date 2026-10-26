package internal

import (
	"time"

	"github.com/google/uuid"
)

// MaxBytes is the largest upload accepted: STORAGE_MAX_UPLOAD_MB when the
// process sets it (Mount), 25 MB otherwise. A module test lowers it.
var MaxBytes int64 = 25 << 20

// attachment is the row: where the bytes are (a storage key under the
// tenant), what they are (sniffed), and what they belong to.
type attachment struct {
	ID          string
	TenantID    string
	StorageKey  string
	Filename    string
	ContentType string
	Size        int64
	OwnerType   *string
	OwnerID     *string
	CreatedAt   time.Time
}

func (a attachment) Snapshot() map[string]any {
	return map[string]any{"id": a.ID, "filename": a.Filename, "contentType": a.ContentType, "size": a.Size, "ownerType": a.OwnerType, "ownerId": a.OwnerID, "storageKey": a.StorageKey}
}

// view is the wire shape with a freshly signed URL.
func (a attachment) view(url string) Attachment {
	return Attachment{Id: a.ID, Filename: a.Filename, ContentType: a.ContentType, Size: a.Size, OwnerType: a.OwnerType, OwnerId: a.OwnerID, CreatedAt: a.CreatedAt, Url: url}
}

func newAttachment(tenantID, key string, c uploadAttachment) attachment {
	return attachment{
		ID: uuid.Must(uuid.NewV7()).String(), TenantID: tenantID, StorageKey: key, Filename: c.Filename, ContentType: c.ContentType,
		Size: int64(len(c.Data)), OwnerType: c.OwnerType, OwnerID: c.OwnerID, CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

package internal

import (
	"time"

	"github.com/google/uuid"

	"github.com/open-enu/kernel/i18n"
)

// row is one thing one person should know about: a type and its arguments.
// The sentence is made where it is read, in the reader's language.
type row struct {
	ID        string
	TenantID  string
	UserID    string
	Type      string
	Context   map[string]any
	ReadAt    *time.Time
	CreatedAt time.Time
}

func newRow(tenantID, userID, typ string, c map[string]any) row {
	if c == nil {
		c = map[string]any{}
	}
	return row{ID: uuid.Must(uuid.NewV7()).String(), TenantID: tenantID, UserID: userID, Type: typ, Context: c, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
}

// keys is what the controller needs of a declaration: the two translation
// keys. Built in mount.go from the platform's catalogue of definitions.
type keys struct{ title, body i18n.Key }

type catalogue map[string]keys

// view is the wire shape. A type no longer declared degrades to its own
// name: a version that removed a type leaves rows behind, and a feed that
// 500s on history is worse than one that shows a key.
func (r row) view(cat catalogue) Notification {
	title, body := r.Type, r.Type
	if k, ok := cat[r.Type]; ok {
		title, body = string(k.title), string(k.body)
	}
	return Notification{Id: r.ID, Type: r.Type, TitleKey: title, BodyKey: body, Context: r.Context, Read: r.ReadAt != nil, ReadAt: r.ReadAt, CreatedAt: r.CreatedAt}
}

func (r row) export() map[string]any {
	var readAt *string
	if r.ReadAt != nil {
		s := r.ReadAt.Format(time.RFC3339)
		readAt = &s
	}
	return map[string]any{"id": r.ID, "tenantId": r.TenantID, "type": r.Type, "context": r.Context, "readAt": readAt, "createdAt": r.CreatedAt.Format(time.RFC3339)}
}

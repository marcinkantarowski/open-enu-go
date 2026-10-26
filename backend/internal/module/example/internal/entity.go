package internal

import (
	"time"

	"github.com/google/uuid"
)

const (
	statusActive   = "active"
	statusArchived = "archived"
)

// project is the row as the module reasons about it - decrypted, typed.
// The generated Project and ProjectDetail (api.gen.go) are the wire shapes;
// view() and detail() are the only places one becomes the other, because
// what leaves the server is decided per endpoint, not by the entity.
type project struct {
	ID              string
	TenantID        string
	Name            string
	ClientReference *string // plaintext here; ciphertext in the table
	Description     *string
	Attributes      map[string]any
	Status          string
	Version         int
	CreatedAt       time.Time
	ArchivedAt      *time.Time
}

// newProject is a project as a creation leaves it: version 1, active, a v7
// uuid unless the command named one (fixtures do).
func newProject(c createProject, tenantID string) project {
	id := c.ID
	if id == "" {
		id = uuid.Must(uuid.NewV7()).String()
	}
	attrs := c.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	return project{
		ID: id, TenantID: tenantID, Name: c.Name, ClientReference: c.ClientReference, Description: c.Description,
		Attributes: attrs, Status: statusActive, Version: 1, CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func (p project) archived() bool { return p.Status == statusArchived }

// Snapshot is what the audit trail keeps (ADR-0013). The encrypted column is
// deliberately absent: a trail is read by more people than the record.
func (p project) Snapshot() map[string]any {
	return map[string]any{
		"id": p.ID, "name": p.Name, "description": p.Description, "attributes": p.Attributes,
		"status": p.Status, "version": p.Version, "createdAt": p.CreatedAt.Format(time.RFC3339),
	}
}

// view is the list shape: no clientReference.
func (p project) view() Project {
	return Project{
		Id: uuid.MustParse(p.ID), Name: p.Name, Description: p.Description, Attributes: p.Attributes,
		Status: ProjectStatus(p.Status), Version: p.Version, CreatedAt: p.CreatedAt,
	}
}

// detail is the single-record shape: the view plus the decrypted reference.
func (p project) detail() ProjectDetail {
	return ProjectDetail{
		Id: uuid.MustParse(p.ID), Name: p.Name, Description: p.Description, Attributes: p.Attributes,
		Status: ProjectDetailStatus(p.Status), Version: p.Version, CreatedAt: p.CreatedAt, ClientReference: p.ClientReference,
	}
}

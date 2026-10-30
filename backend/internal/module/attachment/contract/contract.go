package contract

import (
	"context"
	"errors"

	"github.com/open-enu/kernel/command"
)

// File is what another module may know of an upload: that it exists in the
// scope's tenant, and what it was attached to. Never its storage key - the
// bytes are served by GET /api/attachments/{id}.
type File struct {
	ID          string
	Filename    string
	ContentType string
	Size        int64
	OwnerType   string // "" for a file nobody has claimed
	OwnerID     string
}

// Files is the read port. A module that stores an attachment's id checks it
// here first: an id the scope's tenant cannot see is not a file - the row is
// under row-level security, so another tenant's upload is not found either.
type Files interface {
	// Find is the file, or found=false for an id that is not one (a
	// malformed id included).
	Find(ctx context.Context, id string) (f File, found bool, err error)
}

// Claim - `attachment.claimed`: an upload made without an owner gets one,
// so the sweep (which removes unclaimed files after a day) leaves it.
// Dispatched from inside the claiming module's handler, it runs in that
// command's transaction: the record and its file's owner commit together.
// Claiming a file for the owner it already has changes nothing.
type Claim struct {
	ID        string
	OwnerType string
	OwnerID   string
}

func (Claim) AuditAction() string      { return "attachment.claimed" }
func (c Claim) AuditSubjectID() string { return c.OwnerID }

// What a claim answers besides success. The claiming module maps them onto
// its own field: attachment does not know what the id was called there.
var (
	// ErrUnknown: not an upload the scope's tenant can see.
	ErrUnknown = errors.New("attachment: no such file in this tenant")
	// ErrClaimed: the file already belongs to another owner.
	ErrClaimed = errors.New("attachment: the file belongs to another owner")
)

// Claimer is Claim as a method.
type Claimer struct{ Bus *command.Bus }

// Claim makes ownerType/ownerID the file's owner.
func (c Claimer) Claim(ctx context.Context, id, ownerType, ownerID string) error {
	_, err := c.Bus.Dispatch(ctx, Claim{ID: id, OwnerType: ownerType, OwnerID: ownerID})
	return err
}

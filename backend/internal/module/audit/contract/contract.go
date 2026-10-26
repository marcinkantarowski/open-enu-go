// Package contract is the part of audit other modules import: reading the
// trail. Writing it is the kernel's (kernel/audit), through the bus.
package contract

import (
	"context"
	"time"
)

// Entry is one row of audit_entry as a reader sees it.
type Entry struct {
	ID            string
	Action        string
	TenantID      *string
	ActorID       *string
	OnBehalfOfID  *string
	SubjectID     *string
	RequestID     *string
	Succeeded     bool
	FailureReason *string
	Before        map[string]any
	After         map[string]any
	Context       map[string]any
	RecordedAt    time.Time
}

// Reader lists entries newest first. With a tenant, under that tenant's
// scope - the row-level policy is the filter. With nil, across every tenant,
// which only the operator realm may ask for and which the implementation
// lifts isolation for by name.
type Reader interface {
	Recent(ctx context.Context, tenantID *string, limit int) ([]Entry, error)
}

// MaxLimit caps one page; DefaultLimit is what a caller gets without asking.
const (
	MaxLimit     = 200
	DefaultLimit = 50
)

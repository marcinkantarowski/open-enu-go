// Package contract is the part of tenant other modules import: what a tenant
// looks like, how to read one, the commands that change one and the events
// that say one changed. The write side is commands on the bus rather than
// methods on a service, so a tenant created from Identity's registration is
// audited exactly like one created by an operator.
package contract

import (
	"context"
	"time"

	"github.com/open-enu/kernel/command"
)

// Statuses. Pending until the owner verifies their address; suspended by an
// operator; a suspended tenant's members cannot log in.
const (
	StatusPending   = "pending"
	StatusActive    = "active"
	StatusSuspended = "suspended"
)

// Info is a tenant as other modules see it.
type Info struct {
	ID            string     `json:"id"`
	Slug          string     `json:"slug"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	Plan          string     `json:"plan"`
	DefaultLocale string     `json:"defaultLocale"`
	Version       int        `json:"version"`
	CreatedAt     time.Time  `json:"createdAt"`
	ActivatedAt   *time.Time `json:"activatedAt"`
}

// Active is whether members may log in.
func (i Info) Active() bool { return i.Status == StatusActive }

// Reader describes tenants to other modules. Describe is cached for five
// minutes under the tenant's tag; every command here invalidates it.
type Reader interface {
	Describe(ctx context.Context, id string) (Info, bool, error)
	BySlug(ctx context.Context, slug string) (Info, bool, error)
	Page(ctx context.Context, offset, limit int) ([]Info, int, error)
}

// Create - `tenant.created`. Slug "" derives one from the name; ID "" mints
// one. The result is the Info. Runs every module's OnTenantCreated inside
// the new tenant's scope before it returns, so the row and the defaults
// arrive together.
type Create struct {
	ID            string
	Slug          string
	Name          string
	DefaultLocale string
}

func (Create) AuditAction() string    { return "tenant.created" }
func (Create) AuditSubjectID() string { return "" }

// Activate - `tenant.activated`. Idempotent: activating an active tenant
// changes nothing and is not an error.
type Activate struct{ ID string }

func (Activate) AuditAction() string      { return "tenant.activated" }
func (c Activate) AuditSubjectID() string { return c.ID }
func (c Activate) AuditTenantID() string  { return c.ID }

// Suspend - `tenant.suspended`.
type Suspend struct{ ID string }

func (Suspend) AuditAction() string      { return "tenant.suspended" }
func (c Suspend) AuditSubjectID() string { return c.ID }
func (c Suspend) AuditTenantID() string  { return c.ID }

// Delete - `tenant.deleted`. Raises Deleted BEFORE the row goes, so every
// module holding rows for the tenant hears the id while it still resolves.
type Delete struct{ ID string }

func (Delete) AuditAction() string      { return "tenant.deleted" }
func (c Delete) AuditSubjectID() string { return c.ID }
func (c Delete) AuditTenantID() string  { return c.ID }

// Provisioner is the commands as methods, for a caller that wants types
// rather than the bus's `any`.
type Provisioner struct{ Bus *command.Bus }

// Create dispatches Create.
func (p Provisioner) Create(ctx context.Context, c Create) (Info, error) {
	return command.Dispatch[Info](ctx, p.Bus, c)
}

// Activate dispatches Activate.
func (p Provisioner) Activate(ctx context.Context, id string) error {
	_, err := p.Bus.Dispatch(ctx, Activate{ID: id})
	return err
}

// Suspend dispatches Suspend.
func (p Provisioner) Suspend(ctx context.Context, id string) error {
	_, err := p.Bus.Dispatch(ctx, Suspend{ID: id})
	return err
}

// Delete dispatches Delete.
func (p Provisioner) Delete(ctx context.Context, id string) error {
	_, err := p.Bus.Dispatch(ctx, Delete{ID: id})
	return err
}

// Created is raised in the creating transaction. Not a broadcast: the
// tenant's first browser does not exist yet.
type Created struct {
	ID   string
	Slug string
	Name string
	At   time.Time
}

func (Created) EventName() string       { return "tenant.created" }
func (e Created) SubjectID() string     { return e.ID }
func (e Created) OccurredAt() time.Time { return e.At }
func (e Created) Payload() map[string]any {
	return map[string]any{"tenantId": e.ID, "slug": e.Slug, "name": e.Name}
}

// Deleted is raised before the row is removed. Subscribers drop what they
// hold for the tenant (Identity its memberships).
type Deleted struct {
	ID   string
	Slug string
	At   time.Time
}

func (Deleted) EventName() string         { return "tenant.deleted" }
func (e Deleted) SubjectID() string       { return e.ID }
func (e Deleted) OccurredAt() time.Time   { return e.At }
func (e Deleted) Payload() map[string]any { return map[string]any{"tenantId": e.ID, "slug": e.Slug} }

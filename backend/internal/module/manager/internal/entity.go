package internal

import (
	"time"

	"github.com/google/uuid"
)

// Operator roles. Never a tenant role: the two vocabularies are disjoint,
// so no tenant check can be satisfied by an operator and no operator check
// by a tenant user, whatever a controller asks for.
const (
	roleOperator = "ROLE_PLATFORM_MANAGER"
	roleSuper    = "ROLE_PLATFORM_SUPER"

	// MinPasswordLength is longer than a tenant user's, deliberately: this
	// account reads every tenant's data.
	MinPasswordLength = 16

	accessTTL = 15 * time.Minute
)

// operator is a row of platform_manager: a separate table, not a role on a
// user. That separation is what makes realm isolation possible at all.
type operator struct {
	ID          string
	Email       string
	EmailHash   string
	Password    string // the verifier
	DisplayName *string
	Roles       []string
	Active      bool
	Version     int
	CreatedAt   time.Time
	LastLoginAt *time.Time
}

func (o operator) view() Operator {
	status := "active"
	if !o.Active {
		status = "deactivated"
	}
	return Operator{Id: o.ID, Email: o.Email, DisplayName: o.DisplayName, Roles: o.Roles, Active: o.Active, Status: status, LastLoginAt: o.LastLoginAt, CreatedAt: o.CreatedAt}
}

func (o operator) displayName() string {
	if o.DisplayName == nil {
		return ""
	}
	return *o.DisplayName
}

func newOperator(email, emailHash, verifier string, displayName *string, super bool) operator {
	roles := []string{roleOperator}
	if super {
		roles = append(roles, roleSuper)
	}
	return operator{ID: uuid.Must(uuid.NewV7()).String(), Email: email, EmailHash: emailHash, Password: verifier, DisplayName: displayName,
		Roles: roles, Active: true, Version: 1, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
}

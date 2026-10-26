package internal

// The writes, as data. Session issuance (login, refresh, logout) is NOT here
// - it is infrastructure that would bury the trail at one rotation per tab
// per fifteen minutes; it lives in session.go, which writes through the
// repository outside the bus.

// registerUser - `identity.user.registered`: the person, their pending
// workspace (through Tenant's Create), the owner membership and the
// verification token.
type registerUser struct {
	Email       string
	Password    string
	TenantName  string
	DisplayName *string
	Locale      *string
}

func (registerUser) AuditAction() string    { return "identity.user.registered" }
func (registerUser) AuditSubjectID() string { return "" }

// registered is what the controller answers 202 with: nothing. Ids would
// have to be invented for an address that already has an account, and the
// raw token goes in the email only (ADR-0023).
type registered struct{}

// verifyEmail - `identity.email.verified`: consumes the token, activates the
// account and the workspace.
type verifyEmail struct{ Token string }

func (verifyEmail) AuditAction() string    { return "identity.email.verified" }
func (verifyEmail) AuditSubjectID() string { return "" }

// requestPasswordReset - `identity.password.reset_requested`. Succeeds for
// an unknown address too; the email simply is not sent.
type requestPasswordReset struct{ Email string }

func (requestPasswordReset) AuditAction() string    { return "identity.password.reset_requested" }
func (requestPasswordReset) AuditSubjectID() string { return "" }

// resetPassword - `identity.password.reset`: consumes the token, sets the
// verifier, revokes every session.
type resetPassword struct {
	Token    string
	Password string
}

func (resetPassword) AuditAction() string    { return "identity.password.reset" }
func (resetPassword) AuditSubjectID() string { return "" }

// updateProfile - `identity.profile.updated`. Nil leaves a field alone;
// SetDisplayName with a nil DisplayName clears it.
type updateProfile struct {
	UserID         string
	DisplayName    *string
	SetDisplayName bool
	Locale         *string
}

func (updateProfile) AuditAction() string      { return "identity.profile.updated" }
func (c updateProfile) AuditSubjectID() string { return c.UserID }

// removeTenantMemberships - `identity.memberships.removed`: what a deleted
// tenant leaves behind.
type removeTenantMemberships struct{ TenantID string }

func (removeTenantMemberships) AuditAction() string      { return "identity.memberships.removed" }
func (c removeTenantMemberships) AuditSubjectID() string { return c.TenantID }
func (c removeTenantMemberships) AuditTenantID() string  { return c.TenantID }

// createUser - `identity.user.created`: the console's verified account with
// a membership, idempotent (`openenu user create`, the e2e fixtures).
type createUser struct {
	Email       string
	Password    string
	DisplayName *string
	TenantID    string
	Role        string
}

func (createUser) AuditAction() string     { return "identity.user.created" }
func (createUser) AuditSubjectID() string  { return "" }
func (c createUser) AuditTenantID() string { return c.TenantID }

// eraseUser - `identity.user.erased`: GDPR erasure; the row and, by cascade,
// every membership and token.
type eraseUser struct{ UserID string }

func (eraseUser) AuditAction() string      { return "identity.user.erased" }
func (c eraseUser) AuditSubjectID() string { return c.UserID }

package internal

// createKey - `api_key.created`. The result carries the secret exactly once.
type createKey struct {
	Name          string
	Permissions   []string
	ExpiresInDays *int
}

func (createKey) AuditAction() string    { return "api_key.created" }
func (createKey) AuditSubjectID() string { return "" }

// created is the handler's answer: the row and the secret, which goes into
// one response and no log.
type created struct {
	key    apiKey
	secret string
}

// revokeKey - `api_key.revoked`. Marked, not deleted: the list keeps
// showing it, so "which key was that" has an answer.
type revokeKey struct{ ID string }

func (revokeKey) AuditAction() string      { return "api_key.revoked" }
func (c revokeKey) AuditSubjectID() string { return c.ID }

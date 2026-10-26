package internal

// anonymiseActor - `audit.actor.anonymised`: GDPR erasure of a person from
// the trail. The entries stay - a trail with holes is not a trail - but the
// actor becomes `erased` and the snapshots, which may quote them, go.
type anonymiseActor struct{ UserID string }

func (anonymiseActor) AuditAction() string      { return "audit.actor.anonymised" }
func (c anonymiseActor) AuditSubjectID() string { return c.UserID }

package internal

// createEndpoint - `webhook.endpoint.created`. The secret is generated in
// the handler and is in the result, never in the command or the snapshot.
type createEndpoint struct {
	URL    string
	Events []string
}

func (createEndpoint) AuditAction() string    { return "webhook.endpoint.created" }
func (createEndpoint) AuditSubjectID() string { return "" }

// deactivateEndpoint - `webhook.endpoint.deactivated`. Never a delete: the
// delivery log points at the row, and "which URL did we send that to?" is
// what an incident asks.
type deactivateEndpoint struct{ ID string }

func (deactivateEndpoint) AuditAction() string      { return "webhook.endpoint.deactivated" }
func (c deactivateEndpoint) AuditSubjectID() string { return c.ID }

// redeliver - `webhook.delivery.redelivered`: the same row, reset and
// queued again, rather than a new one - the log stays one story.
type redeliver struct{ DeliveryID string }

func (redeliver) AuditAction() string      { return "webhook.delivery.redelivered" }
func (c redeliver) AuditSubjectID() string { return c.DeliveryID }

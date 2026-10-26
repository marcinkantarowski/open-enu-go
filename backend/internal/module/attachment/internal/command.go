package internal

// uploadAttachment - `attachment.uploaded`. The subject is the owner when
// one was named: "what was attached to this invoice" is the trail's question.
type uploadAttachment struct {
	Filename    string
	ContentType string // sniffed by the controller from the bytes
	Data        []byte
	OwnerType   *string
	OwnerID     *string
}

func (uploadAttachment) AuditAction() string { return "attachment.uploaded" }
func (c uploadAttachment) AuditSubjectID() string {
	if c.OwnerID != nil {
		return *c.OwnerID
	}
	return ""
}

// deleteAttachment - `attachment.deleted`: the sweep's write. The handler
// deletes the blob first, then the row, in the row's transaction: a blob
// delete that fails leaves the row for the next sweep, and a blob already
// gone (storage.ErrNotFound) is tolerated so a row whose blob vanished is
// still removed. Either way the sweep converges.
type deleteAttachment struct{ ID string }

func (deleteAttachment) AuditAction() string      { return "attachment.deleted" }
func (c deleteAttachment) AuditSubjectID() string { return c.ID }

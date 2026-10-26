package internal

import (
	"context"

	"github.com/open-enu/kernel/auth"

	"github.com/open-enu/app/internal/module/audit/contract"
)

// api implements the StrictServerInterface genapi derives from
// ../openapi.json: one read, through the contract's reader, under the
// caller's tenant.
type api struct{ reader contract.Reader }

var _ StrictServerInterface = api{}

func (a api) GetAuditIndex(ctx context.Context, req GetAuditIndexRequestObject) (GetAuditIndexResponseObject, error) {
	p, err := auth.Require(ctx, "audit.view")
	if err != nil {
		return nil, err
	}
	// The spec says 1..200; the generated server does not enforce it, so a
	// limit outside it falls back to the default rather than failing.
	limit := contract.DefaultLimit
	if req.Params.Limit != nil && *req.Params.Limit > 0 && *req.Params.Limit <= contract.MaxLimit {
		limit = *req.Params.Limit
	}
	// The caller's tenant, never a parameter: an operator reads the whole
	// trail through the manager realm, not through this route.
	entries, err := a.reader.Recent(ctx, &p.TenantID, limit)
	if err != nil {
		return nil, err
	}
	out := AuditList{Items: make([]AuditEntry, 0, len(entries))}
	for _, e := range entries {
		out.Items = append(out.Items, view(e))
	}
	return GetAuditIndex200JSONResponse(out), nil
}

func view(e contract.Entry) AuditEntry {
	return AuditEntry{
		Id: e.ID, Action: e.Action, TenantId: e.TenantID, ActorId: e.ActorID, OnBehalfOfId: e.OnBehalfOfID, SubjectId: e.SubjectID,
		RequestId: e.RequestID, Succeeded: e.Succeeded, FailureReason: e.FailureReason, Before: before(e.Before), After: before(e.After),
		Context: e.Context, RecordedAt: e.RecordedAt,
	}
}

func before(m map[string]any) *map[string]any {
	if m == nil {
		return nil
	}
	return &m
}

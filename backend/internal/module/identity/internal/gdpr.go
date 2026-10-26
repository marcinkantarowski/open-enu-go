package internal

import (
	"context"
	"time"

	"github.com/open-enu/kernel/command"
)

// subject answers GDPR for a person: the account, the memberships and the
// sessions; erasure removes the row and everything that cascades from it.
type subject struct {
	repo repository
	bus  *command.Bus
}

func (s subject) ExportFor(ctx context.Context, userID string) (map[string][]map[string]any, error) {
	u, err := s.repo.findUser(ctx, userID)
	if err != nil {
		return map[string][]map[string]any{}, nil
	}
	ms, err := s.repo.membershipsOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	ts, err := s.repo.refreshTokensOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := map[string][]map[string]any{
		"account":     {{"id": u.ID, "email": u.Email, "displayName": u.DisplayName, "status": u.Status, "locale": u.Locale, "createdAt": u.CreatedAt.Format(time.RFC3339)}},
		"memberships": {}, "sessions": {},
	}
	for _, m := range ms {
		out["memberships"] = append(out["memberships"], m.Snapshot())
	}
	for _, t := range ts {
		out["sessions"] = append(out["sessions"], map[string]any{"id": t.ID, "tenantId": t.TenantID, "expiresAt": t.ExpiresAt.Format(time.RFC3339), "revoked": t.Revoked})
	}
	return out, nil
}

func (s subject) EraseFor(ctx context.Context, userID string) (int, error) {
	return command.Dispatch[int](ctx, s.bus, eraseUser{UserID: userID})
}

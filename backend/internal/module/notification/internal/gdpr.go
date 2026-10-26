package internal

import (
	"context"

	"github.com/open-enu/kernel/gdpr"
	"github.com/open-enu/kernel/platform"
)

// subject answers for a person's notifications across every tenant. The
// walker audits the erasure as a whole; a command per module would fragment
// one legal action into many.
type subject struct{ repo repository }

// NewGdpr is what module.go registers.
func NewGdpr(svc *platform.Services) gdpr.Subject { return subject{repo: newRepository(svc)} }

func (s subject) ExportFor(ctx context.Context, userID string) (map[string][]map[string]any, error) {
	rows, err := s.repo.allOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := map[string][]map[string]any{"notifications": {}}
	for _, r := range rows {
		out["notifications"] = append(out["notifications"], r.export())
	}
	return out, nil
}

func (s subject) EraseFor(ctx context.Context, userID string) (int, error) {
	return s.repo.eraseOf(ctx, userID)
}

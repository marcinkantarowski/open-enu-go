package internal

import (
	"context"

	"github.com/open-enu/kernel/command"
)

type subject struct {
	repo repository
	bus  *command.Bus
}

func (s subject) ExportFor(ctx context.Context, userID string) (map[string][]map[string]any, error) {
	rows, err := s.repo.actions(ctx, userID)
	if err != nil {
		return nil, err
	}
	return map[string][]map[string]any{"actions": rows}, nil
}

func (s subject) EraseFor(ctx context.Context, userID string) (int, error) {
	return command.Dispatch[int](ctx, s.bus, anonymiseActor{UserID: userID})
}

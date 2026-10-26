package internal

import (
	"context"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/httpx"
)

// api implements the StrictServerInterface genapi derives from
// ../openapi.json: one read, under the caller's tenant - another
// workspace's job is not found, because the policy never returned it.
type api struct{ repo repository }

var _ StrictServerInterface = api{}

func (a api) GetProgressShow(ctx context.Context, req GetProgressShowRequestObject) (GetProgressShowResponseObject, error) {
	if _, err := auth.Require(ctx, "progress.view"); err != nil {
		return nil, err
	}
	snap, found, err := a.repo.get(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, httpx.NotFound(httpx.KeyNotFound)
	}
	return GetProgressShow200JSONResponse{
		Id: snap.ID, Kind: snap.Kind, Label: snap.Label, Total: snap.Total, Done: snap.Done, Percent: snap.Percent,
		Status: ProgressJobStatus(snap.Status), Result: snap.Result, FailureReason: snap.FailureReason, StartedAt: snap.StartedAt, FinishedAt: snap.FinishedAt,
	}, nil
}

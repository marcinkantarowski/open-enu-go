package internal

import (
	"context"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/httpx"
)

// api implements the StrictServerInterface genapi derives from
// ../openapi.json. A feed is personal, so "whose" is never a parameter: it
// is the principal's, and the one permission says only that a feed exists.
type api struct {
	repo repository
	cat  catalogue
}

var _ StrictServerInterface = api{}

func (a api) GetNotificationIndex(ctx context.Context, _ GetNotificationIndexRequestObject) (GetNotificationIndexResponseObject, error) {
	p, err := auth.Require(ctx, "notification.view")
	if err != nil {
		return nil, err
	}
	rows, err := a.repo.feedFor(ctx, p.Subject, feedLimit)
	if err != nil {
		return nil, err
	}
	unread, err := a.repo.unreadCount(ctx, p.Subject)
	if err != nil {
		return nil, err
	}
	out := NotificationFeed{Items: make([]Notification, 0, len(rows)), Meta: FeedMeta{Unread: unread}}
	for _, r := range rows {
		out.Items = append(out.Items, r.view(a.cat))
	}
	return GetNotificationIndex200JSONResponse(out), nil
}

func (a api) PostNotificationRead(ctx context.Context, req PostNotificationReadRequestObject) (PostNotificationReadResponseObject, error) {
	p, err := auth.Require(ctx, "notification.view")
	if err != nil {
		return nil, err
	}
	found, err := a.repo.markRead(ctx, p.Subject, req.Id)
	if err != nil {
		return nil, err
	}
	if !found {
		// Indistinguishable from a notification that does not exist: a 403
		// would make the endpoint a way to discover other people's ids.
		return nil, httpx.NotFound(httpx.KeyNotFound)
	}
	return PostNotificationRead200JSONResponse{Status: "read"}, nil
}

func (a api) PostNotificationReadAll(ctx context.Context, _ PostNotificationReadAllRequestObject) (PostNotificationReadAllResponseObject, error) {
	p, err := auth.Require(ctx, "notification.view")
	if err != nil {
		return nil, err
	}
	if err := a.repo.markAllRead(ctx, p.Subject); err != nil {
		return nil, err
	}
	return PostNotificationReadAll200JSONResponse{Status: "read", Unread: 0}, nil
}

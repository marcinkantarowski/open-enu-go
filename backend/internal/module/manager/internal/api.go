package internal

import (
	"context"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/i18n"
)

const keyInvalidCredentials i18n.Key = "manager.error.invalid_credentials"

// api implements the StrictServerInterface genapi derives from
// ../openapi.json. Every route but login requires the manager realm; the
// work is the service's, which holds the other modules' contracts.
type api struct {
	svc service
}

var _ StrictServerInterface = api{}

func (a api) PostManagerLogin(ctx context.Context, req PostManagerLoginRequestObject) (PostManagerLoginResponseObject, error) {
	session, err := a.svc.login(ctx, string(req.Body.Email), req.Body.Password)
	if err != nil {
		return nil, err
	}
	return PostManagerLogin200JSONResponse(session), nil
}

func (a api) GetManagerTenantIndex(ctx context.Context, req GetManagerTenantIndexRequestObject) (GetManagerTenantIndexResponseObject, error) {
	if _, err := auth.RequireOperator(ctx); err != nil {
		return nil, err
	}
	page, size := 1, 25
	if req.Params.Page != nil && *req.Params.Page > 0 {
		page = *req.Params.Page
	}
	if req.Params.Size != nil && *req.Params.Size > 0 && *req.Params.Size <= 100 {
		size = *req.Params.Size
	}
	out, err := a.svc.page(ctx, page, size)
	if err != nil {
		return nil, err
	}
	return GetManagerTenantIndex200JSONResponse(out), nil
}

func (a api) GetManagerTenantShow(ctx context.Context, req GetManagerTenantShowRequestObject) (GetManagerTenantShowResponseObject, error) {
	if _, err := auth.RequireOperator(ctx); err != nil {
		return nil, err
	}
	out, err := a.svc.detail(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return GetManagerTenantShow200JSONResponse(out), nil
}

func (a api) PostManagerTenantSuspend(ctx context.Context, req PostManagerTenantSuspendRequestObject) (PostManagerTenantSuspendResponseObject, error) {
	if _, err := auth.RequireOperator(ctx); err != nil {
		return nil, err
	}
	if err := a.svc.suspend(ctx, req.Id); err != nil {
		return nil, err
	}
	return PostManagerTenantSuspend200JSONResponse{Status: SuspendedStatusSuspended}, nil
}

func (a api) PostManagerImpersonate(ctx context.Context, req PostManagerImpersonateRequestObject) (PostManagerImpersonateResponseObject, error) {
	p, err := auth.RequireOperator(ctx)
	if err != nil {
		return nil, err
	}
	session, err := a.svc.impersonate(ctx, p.Subject, req.TenantId, req.UserId)
	if err != nil {
		return nil, err
	}
	return PostManagerImpersonate200JSONResponse(session), nil
}

func (a api) GetManagerAuditIndex(ctx context.Context, req GetManagerAuditIndexRequestObject) (GetManagerAuditIndexResponseObject, error) {
	if _, err := auth.RequireOperator(ctx); err != nil {
		return nil, err
	}
	limit := 100
	if req.Params.Limit != nil && *req.Params.Limit > 0 && *req.Params.Limit <= maxAuditLimit {
		limit = *req.Params.Limit
	}
	var tenant *string
	if req.Params.TenantId != nil && *req.Params.TenantId != "" {
		tenant = req.Params.TenantId
	}
	out, err := a.svc.trail(ctx, tenant, limit)
	if err != nil {
		return nil, err
	}
	return GetManagerAuditIndex200JSONResponse(out), nil
}

func (a api) GetManagerWorkerStatus(ctx context.Context, _ GetManagerWorkerStatusRequestObject) (GetManagerWorkerStatusResponseObject, error) {
	if _, err := auth.RequireOperator(ctx); err != nil {
		return nil, err
	}
	queues, failed, err := a.svc.repo.queueDepths(ctx)
	if err != nil {
		return nil, err
	}
	return GetManagerWorkerStatus200JSONResponse{Queues: queues, Failed: failed}, nil
}

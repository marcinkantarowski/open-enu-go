package internal

import (
	"context"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/i18n"
)

const keySecretOnce i18n.Key = "api_key.warning.secret_once"

// api implements the StrictServerInterface genapi derives from
// ../openapi.json.
type api struct {
	bus   *command.Bus
	repo  repository
	i18n  *i18n.Catalogue
	guard auth.Guard
}

var _ StrictServerInterface = api{}

func (a api) GetApiKeyIndex(ctx context.Context, _ GetApiKeyIndexRequestObject) (GetApiKeyIndexResponseObject, error) {
	if _, err := auth.Require(ctx, "api_key.view"); err != nil {
		return nil, err
	}
	items, err := a.repo.list(ctx)
	if err != nil {
		return nil, err
	}
	out := ApiKeyList{Items: make([]ApiKey, 0, len(items))}
	for _, k := range items {
		out.Items = append(out.Items, k.view())
	}
	return GetApiKeyIndex200JSONResponse(out), nil
}

func (a api) PostApiKeyCreate(ctx context.Context, req PostApiKeyCreateRequestObject) (PostApiKeyCreateResponseObject, error) {
	if _, err := auth.Require(ctx, "api_key.manage"); err != nil {
		return nil, err
	}
	// A support session may look at the keys, never mint one: the session
	// expires in fifteen minutes, a key does not (ADR-0020).
	if err := a.guard.DenyImpersonated(ctx, "api_key.created"); err != nil {
		return nil, err
	}
	c, err := command.Dispatch[created](ctx, a.bus, createKey{Name: req.Body.Name, Permissions: req.Body.Permissions, ExpiresInDays: req.Body.ExpiresInDays})
	if err != nil {
		return nil, err
	}
	v := c.key.view()
	return PostApiKeyCreate201JSONResponse{
		Id: v.Id, Name: v.Name, Prefix: v.Prefix, Permissions: v.Permissions, Revoked: v.Revoked, ExpiresAt: v.ExpiresAt, LastUsedAt: v.LastUsedAt, CreatedAt: v.CreatedAt,
		Secret: c.secret, Warning: a.i18n.TranslateCtx(ctx, keySecretOnce),
	}, nil
}

func (a api) DeleteApiKeyRevoke(ctx context.Context, req DeleteApiKeyRevokeRequestObject) (DeleteApiKeyRevokeResponseObject, error) {
	if _, err := auth.Require(ctx, "api_key.manage"); err != nil {
		return nil, err
	}
	if err := a.guard.DenyImpersonated(ctx, "api_key.revoked"); err != nil {
		return nil, err
	}
	if _, err := command.Dispatch[apiKey](ctx, a.bus, revokeKey{ID: req.Id}); err != nil {
		return nil, err
	}
	return DeleteApiKeyRevoke200JSONResponse{Status: RevokedStatusRevoked}, nil
}

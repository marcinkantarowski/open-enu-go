package internal

import (
	"bytes"
	"context"
	"errors"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/storage"
)

const keyNoTenant i18n.Key = "attachment.error.no_tenant"

// handlers are the only code that receives *scope.Tx (writeboundary).
type handlers struct {
	repo    repository
	storage storage.Storage
}

func (h handlers) upload(ctx context.Context, tx *scope.Tx, c uploadAttachment) (attachment, error) {
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		return attachment{}, httpx.BadRequest(keyNoTenant)
	}
	// The blob first: the kernel keys it under the tenant in scope. If the
	// row then fails, the blob is orphaned storage, which the sweep is for;
	// the other order would leave a row pointing at nothing.
	key, err := h.storage.Write(ctx, "attachment", c.Filename, bytes.NewReader(c.Data), c.ContentType)
	if err != nil {
		return attachment{}, err
	}
	a := newAttachment(tenant, key, c)
	if err := h.repo.insert(ctx, tx, a); err != nil {
		_ = h.storage.Delete(ctx, key)
		return attachment{}, err
	}
	tx.Insert(a)
	return a, nil
}

func (h handlers) remove(ctx context.Context, tx *scope.Tx, c deleteAttachment) (attachment, error) {
	old, err := h.repo.get(ctx, tx, c.ID)
	if err != nil {
		return attachment{}, notFound(err)
	}
	if err := h.storage.Delete(ctx, old.StorageKey); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return attachment{}, err
	}
	if err := h.repo.remove(ctx, tx, old.ID); err != nil {
		return attachment{}, err
	}
	tx.Delete(old)
	return old, nil
}

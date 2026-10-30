package internal

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/scope"
)

// handlers are the only code that receives *scope.Tx (writeboundary).
type handlers struct {
	repo   repository
	outbox *outbox.Outbox
	policy outbound
}

func (h handlers) create(ctx context.Context, tx *scope.Tx, c createEndpoint) (created, error) {
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		return created{}, httpx.BadRequest(keyNoTenant)
	}
	// https, and not an address inside our own network (outbound.go).
	if key := h.policy.checkURL(c.URL); key != "" {
		return created{}, httpx.BadRequest(key)
	}
	if len(c.Events) == 0 {
		return created{}, httpx.BadRequest(keyEventsRequired)
	}
	// 32 bytes, generated here and never accepted from the client: a secret
	// the caller chose is one they may have chosen badly, or reused.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return created{}, err
	}
	secret := "whsec_" + base64.RawURLEncoding.EncodeToString(raw)
	e := newEndpoint(tenant, c.URL, c.Events, secret)
	if err := h.repo.insertEndpoint(ctx, tx, e); err != nil {
		return created{}, err
	}
	tx.Insert(e)
	return created{endpoint: e, secret: secret}, nil
}

func (h handlers) deactivate(ctx context.Context, tx *scope.Tx, c deactivateEndpoint) (endpoint, error) {
	old, err := h.repo.endpoint(ctx, tx, c.ID)
	if err != nil {
		return endpoint{}, notFound(err)
	}
	updated := old
	updated.Active, updated.Version = false, old.Version+1
	if err := h.repo.deactivateEndpoint(ctx, tx, updated); err != nil {
		return endpoint{}, err
	}
	tx.Update(old, updated)
	return updated, nil
}

func (h handlers) redeliver(ctx context.Context, tx *scope.Tx, c redeliver) (delivery, error) {
	old, err := h.repo.delivery(ctx, tx, c.DeliveryID)
	if err != nil {
		return delivery{}, notFound(err)
	}
	reset := old
	reset.Status, reset.Attempts, reset.ResponseCode, reset.ResponseBody = statusPending, 0, nil, nil
	if err := h.repo.resetDelivery(ctx, tx, reset); err != nil {
		return delivery{}, err
	}
	// Queued in this transaction: a rollback takes the job with it.
	if err := h.outbox.Enqueue(ctx, tx, deliverJob{DeliveryID: reset.ID}); err != nil {
		return delivery{}, err
	}
	tx.Update(old, reset)
	return reset, nil
}

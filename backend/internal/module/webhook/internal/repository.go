package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
)

// repository is the only file with SQL. Two kinds of write live here: the
// handlers' (through the bus, with a *scope.Tx) and the delivery log's own -
// recording the outcome of an outbound HTTP call is infrastructure, not a
// domain change, and runs on the worker outside any command.
type repository struct {
	db     *scope.Pool
	crypto *crypto.Encryptor
	outbox *outbox.Outbox
}

func newRepository(svc *platform.Services) repository {
	return repository{db: svc.DB, crypto: svc.Crypto, outbox: svc.Outbox}
}

var errNotFound = errors.New("webhook: no such row")

func notFound(err error) error {
	if errors.Is(err, errNotFound) {
		return httpx.NotFound(httpx.KeyNotFound)
	}
	return err
}

func newEndpoint(tenantID, url string, events []string, secret string) endpoint {
	return endpoint{ID: uuid.Must(uuid.NewV7()).String(), TenantID: tenantID, URL: url, Events: events, Secret: secret, Active: true, Version: 1,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
}

const (
	endpointColumns = `tenant_id, id, url, events, secret, active, version, created_at`
	deliveryColumns = `tenant_id, id, endpoint_id, event_name, payload, status, attempts, response_code, response_body, last_attempt_at, created_at`
	// responseBodyLimit keeps the log readable; the first two thousand
	// characters of an error page are the part that says what went wrong.
	responseBodyLimit = 2000
)

// ── endpoints ────────────────────────────────────────────────────────────

func (r repository) endpoints(ctx context.Context) (out []endpoint, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		out, err = r.queryEndpoints(ctx, tx, `SELECT `+endpointColumns+` FROM webhook_endpoint ORDER BY created_at DESC, id DESC`)
		return err
	})
	return out, err
}

// subscribedTo is the active endpoints wanting an event, in the ambient scope.
func (r repository) subscribedTo(ctx context.Context, eventName string) ([]endpoint, error) {
	all, err := r.endpoints(ctx)
	if err != nil {
		return nil, err
	}
	var out []endpoint
	for _, e := range all {
		if e.wants(eventName) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r repository) endpoint(ctx context.Context, tx *scope.Tx, id string) (endpoint, error) {
	if _, err := uuid.Parse(id); err != nil {
		return endpoint{}, errNotFound
	}
	items, err := r.queryEndpoints(ctx, tx, `SELECT `+endpointColumns+` FROM webhook_endpoint WHERE id = $1`, id)
	if err != nil {
		return endpoint{}, err
	}
	if len(items) == 0 {
		return endpoint{}, errNotFound
	}
	return items[0], nil
}

// findEndpoint is endpoint in its own read transaction, for the worker.
func (r repository) findEndpoint(ctx context.Context, id string) (e endpoint, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		e, err = r.endpoint(ctx, tx, id)
		return err
	})
	return e, err
}

func (r repository) insertEndpoint(ctx context.Context, tx *scope.Tx, e endpoint) error {
	// Under the row's tenant key, never the ambient scope's.
	ct, err := r.crypto.Encrypt(e.Secret, e.TenantID)
	if err != nil {
		return fmt.Errorf("webhook: encrypt secret: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO webhook_endpoint (`+endpointColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.TenantID, e.ID, e.URL, e.Events, ct, e.Active, e.Version, e.CreatedAt)
	return err
}

func (r repository) deactivateEndpoint(ctx context.Context, tx *scope.Tx, e endpoint) error {
	_, err := tx.Exec(ctx, `UPDATE webhook_endpoint SET active = false, version = $2 WHERE id = $1`, e.ID, e.Version)
	return err
}

func (r repository) queryEndpoints(ctx context.Context, tx *scope.Tx, sql string, args ...any) ([]endpoint, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []endpoint
	for rows.Next() {
		var e endpoint
		var ct string
		var events []byte
		if err := rows.Scan(&e.TenantID, &e.ID, &e.URL, &events, &ct, &e.Active, &e.Version, &e.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(events, &e.Events); err != nil {
			return nil, err
		}
		if e.Secret, err = r.crypto.Decrypt(ct, e.TenantID); err != nil {
			return nil, fmt.Errorf("webhook: decrypt secret of %s: %w", e.ID, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── deliveries ───────────────────────────────────────────────────────────

// recent is the log, newest first, optionally one endpoint's.
func (r repository) recent(ctx context.Context, endpointID string, limit int) (out []delivery, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		sql, args := `SELECT `+deliveryColumns+` FROM webhook_delivery ORDER BY created_at DESC, id DESC LIMIT $1`, []any{limit}
		if _, perr := uuid.Parse(endpointID); perr == nil {
			sql, args = `SELECT `+deliveryColumns+` FROM webhook_delivery WHERE endpoint_id = $2 ORDER BY created_at DESC, id DESC LIMIT $1`, []any{limit, endpointID}
		}
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		out, err = scanDeliveries(rows)
		return err
	})
	return out, err
}

func (r repository) delivery(ctx context.Context, tx *scope.Tx, id string) (delivery, error) {
	if _, err := uuid.Parse(id); err != nil {
		return delivery{}, errNotFound
	}
	rows, err := tx.Query(ctx, `SELECT `+deliveryColumns+` FROM webhook_delivery WHERE id = $1`, id)
	if err != nil {
		return delivery{}, err
	}
	items, err := scanDeliveries(rows)
	if err != nil {
		return delivery{}, err
	}
	if len(items) == 0 {
		return delivery{}, errNotFound
	}
	return items[0], nil
}

func (r repository) findDelivery(ctx context.Context, id string) (d delivery, err error) {
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		d, err = r.delivery(ctx, tx, id)
		return err
	})
	return d, err
}

// record writes a delivery and queues its job in one transaction, from the
// publisher: an infrastructure write about an event that is already
// audited. A rollback takes the job with it.
func (r repository) record(ctx context.Context, d delivery) error {
	return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO webhook_delivery (`+deliveryColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			d.TenantID, d.ID, d.EndpointID, d.EventName, d.Payload, d.Status, d.Attempts, d.ResponseCode, d.ResponseBody, d.LastAttemptAt, d.CreatedAt)
		if err != nil {
			return err
		}
		return r.outbox.Enqueue(ctx, tx, deliverJob{DeliveryID: d.ID})
	})
}

func (r repository) resetDelivery(ctx context.Context, tx *scope.Tx, d delivery) error {
	_, err := tx.Exec(ctx, `UPDATE webhook_delivery SET status = $2, attempts = 0, response_code = NULL, response_body = NULL WHERE id = $1`, d.ID, d.Status)
	return err
}

// succeeded closes the row: delivered, with the receiver's status.
func (r repository) succeeded(ctx context.Context, id string, code int) error {
	return r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE webhook_delivery SET status = $2, attempts = attempts + 1, response_code = $3, response_body = NULL, last_attempt_at = now() WHERE id = $1`,
			id, statusDelivered, code)
		return err
	})
}

// failed records one more failed attempt and returns the row as it now
// stands: `pending` while retries remain, `failed` once they are spent.
func (r repository) failed(ctx context.Context, id string, code *int, reason string) (d delivery, err error) {
	if len(reason) > responseBodyLimit {
		reason = reason[:responseBodyLimit]
	}
	err = r.db.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE webhook_delivery
			SET attempts = attempts + 1, response_code = $2, response_body = $3, last_attempt_at = now(),
			    status = CASE WHEN attempts + 1 >= $4 THEN $5 ELSE $6 END
			WHERE id = $1 RETURNING `+deliveryColumns, id, code, reason, maxAttempts, statusFailed, statusPending)
		if err != nil {
			return err
		}
		items, err := scanDeliveries(rows)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return errNotFound
		}
		d = items[0]
		return nil
	})
	return d, err
}

func scanDeliveries(rows pgx.Rows) ([]delivery, error) {
	defer rows.Close()
	var out []delivery
	for rows.Next() {
		var d delivery
		var payload []byte
		if err := rows.Scan(&d.TenantID, &d.ID, &d.EndpointID, &d.EventName, &payload, &d.Status, &d.Attempts, &d.ResponseCode, &d.ResponseBody, &d.LastAttemptAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &d.Payload); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

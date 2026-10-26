package internal

import (
	"context"
	"errors"
	"time"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/i18n"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/progress"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/search"

	"github.com/open-enu/app/internal/module/example/contract"
)

const (
	keyArchived  i18n.Key = "example.error.archived"
	keyNoTenant  i18n.Key = "example.error.no_tenant"
	keyArchiving i18n.Key = "example.progress.archiving"
)

// handlers are the only code that receives *scope.Tx (writeboundary). Each
// one does the work, leaves its snapshots on the transaction and returns
// what the controller answers with; the bus does the rest.
type handlers struct {
	repo     repository
	outbox   *outbox.Outbox
	progress progress.Reporter
	search   search.Indexer
}

func (h handlers) create(ctx context.Context, tx *scope.Tx, c createProject) (project, error) {
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		return project{}, httpx.BadRequest(keyNoTenant)
	}
	p := newProject(c, tenant)
	if err := h.repo.insert(ctx, tx, p); err != nil {
		return project{}, err
	}
	// Indexed in the same transaction as the row: findable exactly when it
	// exists, and never a hit on a record that rolled back.
	if err := h.search.Index(ctx, tx, entityType, p.ID, indexed(p), p.TenantID); err != nil {
		return project{}, err
	}
	// No `before`: there was nothing. That is what distinguishes a creation
	// from an edit in the trail.
	tx.Insert(p)
	// Through the outbox, in this transaction: the row and the event commit
	// together or not at all (ADR-0015).
	return p, h.outbox.Publish(ctx, tx, contract.ProjectCreated{ID: p.ID, Name: p.Name, At: p.CreatedAt})
}

func (h handlers) rename(ctx context.Context, tx *scope.Tx, c renameProject) (project, error) {
	old, err := h.repo.get(ctx, tx, c.ID)
	if err != nil {
		return project{}, notFound(err)
	}
	// The permission said this ROLE may rename projects; the voter says
	// whether THIS row may be touched now.
	if !canEdit(old) {
		return project{}, httpx.Forbidden(keyArchived)
	}
	if c.Locked && c.IfMatch != old.Version {
		return project{}, conflict(old, c.IfMatch)
	}
	updated := old
	updated.Name, updated.Version = c.Name, old.Version+1
	if err := h.repo.rename(ctx, tx, old.Version, updated); err != nil {
		if errors.Is(err, scope.ErrStale) { // changed between our read and our write
			if cur, rerr := h.repo.get(ctx, tx, c.ID); rerr == nil {
				return project{}, conflict(cur, old.Version)
			}
		}
		return project{}, err
	}
	if err := h.search.Index(ctx, tx, entityType, updated.ID, indexed(updated), updated.TenantID); err != nil {
		return project{}, err
	}
	tx.Update(old, updated)
	return updated, nil
}

func (h handlers) archiveRequested(ctx context.Context, tx *scope.Tx, _ archiveProjects) (ArchiveAccepted, error) {
	active, err := h.repo.active(ctx, tx)
	if err != nil {
		return ArchiveAccepted{}, err
	}
	jobID, err := h.progress.Start(ctx, "example.archive", len(active), string(keyArchiving))
	if err != nil {
		return ArchiveAccepted{}, err
	}
	// Enqueued in this transaction, so the worker cannot start on a state
	// that then rolls back.
	if err := h.outbox.Enqueue(ctx, tx, archiveJob{JobID: jobID}); err != nil {
		return ArchiveAccepted{}, err
	}
	result := ArchiveAccepted{JobId: jobID, Total: len(active)}
	tx.Snapshot(nil, map[string]any{"jobId": jobID, "total": len(active)})
	return result, nil
}

func (h handlers) archiveActive(ctx context.Context, tx *scope.Tx, c archiveActive) (int, error) {
	active, err := h.repo.active(ctx, tx)
	if err != nil {
		return 0, err
	}
	for _, p := range active {
		if err := h.repo.archive(ctx, tx, p); err != nil {
			return 0, err
		}
		// What is archived is no longer findable.
		if err := h.search.Remove(ctx, tx, entityType, p.ID, p.TenantID); err != nil {
			return 0, err
		}
		// One per project, so a watching browser sees movement rather than a
		// bar that jumps from 0 to 100 once everything is already done.
		_ = h.progress.Advance(ctx, c.JobID, 1, p.Name)
	}
	tx.Snapshot(nil, map[string]any{"jobId": c.JobID, "archived": len(active)})
	return len(active), nil
}

// sweep is the clock's write: archived projects past retention leave the
// table and the index, one transaction per tenant (task.go enters the
// scope). A batch delete has no single subject; `before` lists the ids.
func (h handlers) sweep(ctx context.Context, tx *scope.Tx, c sweepArchived) (int, error) {
	stale, err := h.repo.archivedBefore(ctx, tx, c.Before)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(stale))
	for _, p := range stale {
		if err := h.repo.delete(ctx, tx, p); err != nil {
			return 0, err
		}
		if err := h.search.Remove(ctx, tx, entityType, p.ID, p.TenantID); err != nil {
			return 0, err
		}
		ids = append(ids, p.ID)
	}
	tx.Snapshot(map[string]any{"ids": ids, "before": c.Before.Format(time.RFC3339)}, nil)
	return len(ids), nil
}

// conflict is the 409: both versions and the saved record, so the client can
// offer reload or overwrite instead of "your work is gone".
func conflict(cur project, yours int) error {
	return httpx.Conflict(httpx.ConflictBody{
		Resource: "Project", ResourceID: cur.ID, YourVersion: yours, CurrentVersion: cur.Version, Current: cur.Snapshot(),
	})
}

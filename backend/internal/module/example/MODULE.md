# Example - `example`

**The reference module. Read this one before writing a feature, then copy it.**

One entity, `Project`, which exists to demonstrate every platform concern exactly once and
nothing else. It is not a feature anyone needs; delete it when the real modules outnumber
it. Its cap is **1200 lines including tests** (`make agents-budget`, which counts every
non-generated `.go` file outside `migrations/`, blank and comment lines excluded), and it
sits near it. The number moves only when the list of what the module must SHOW grows;
adding a demonstration otherwise means removing one.

> Budget: 8 KB. A module that needs more explaining than that is too big - split it.

## Surface

- API (`openapi.json`):
  `GET /api/projects`, `POST /api/projects`, `GET /api/projects/{id}` (ETag),
  `PATCH /api/projects/{id}` (If-Match, 409), `POST /api/projects/archive` (202, flagged)
- Permissions: `example.view` (read), `example.manage` (create, rename, archive)
- Events: `example.project.created` - broadcast to holders of `example.view`
- Flags: `example.archive` - off by default; the route is **404** while off, because
  "not built yet" and "switched off during an incident" must be indistinguishable
- Search: `example.project` behind `example.view` - name and description, never the
  encrypted reference; archived projects leave the index
- Tasks: `example.sweep`, daily - a project archived more than 30 days ago is deleted
  (`example.projects.swept`, one command per tenant)
- Tables: `example_project`, under row-level security

## Public contract

`contract.ProjectCreated` - the one event. Other modules subscribe by name through
`svc.Outbox.Subscribe`; nothing else is importable.

## One of everything, and where

| Concern | File | Cost |
|---|---|---|
| Tenant scoping | `migrations/0001_create_project.sql` | the policy; no `WHERE tenant_id` anywhere |
| Optimistic locking, 409 | `internal/handler.go` `rename`, `repository.go` `rename` | `WHERE version = $3` + `scope.CheckVersion` |
| Field encryption | `internal/repository.go` `encrypt`/`scan` | `svc.Crypto` under the row's tenant key |
| Open-ended data ([ADR-0012](../../../../.ai/platform/adr/0012-no-custom-fields-layer.md)) | `example_project.attributes` | a JSONB column |
| Write + audit | `internal/command.go`, `handler.go` | a struct and a method; `tx.Update(old, new)` |
| Event to the browser | `contract/events.go` | `RequiresPermission()` makes it a broadcast |
| Record-level authorisation | `internal/voter.go` | `canEdit`, consulted by the handler |
| Feature flag | `internal/flags.go` + `api.go` | a `Definition` and `svc.Flags.Enabled` |
| Background work with progress | `internal/job.go`, `handler.go` `archiveActive` | `outbox.Handle` / `Enqueue`; one command per batch |
| Impersonation guard | `internal/api.go` | `auth.Guard.DenyImpersonated` |
| New-tenant seed | `internal/setup.go` | `setup.TenantSetup` |
| Deterministic fixtures | `internal/fixtures.go` | fixed uuids as constants |
| Full-text search | `internal/search.go`, `handler.go` | a `search.Declaration`; `Index` in the writing transaction |
| Work on a clock | `internal/task.go`, `repository.go` `tenantsWithArchivedBefore` | a `module.Task`; the one `scope.Unscoped`, with its reason |

## Notes for agents

- **A permission and a voter answer different questions.** `example.manage` asks whether
  this *role* may edit projects; `canEdit` asks whether *this row* may be edited now.
- **A command is not a job.** `archiveProjects` is synchronous so the caller learns the
  request was accepted and gets the progress id; the work runs as `archiveJob`, which
  dispatches `archiveActive` - a command like any other, so the job's write is audited.
- **What leaves the server is decided per endpoint.** `view()` has no `clientReference`;
  `detail()` does. The snapshot never does, and neither does the search index.
- **A task asks one cross-tenant question and then enters each tenant.** `Sweep` reads
  WHICH tenants are due unscoped, in `repository.go`, with a reason; every row it touches is
  read and deleted inside that tenant's scope by a command, so the policy and the trail
  both apply.
- The controller reads through the repository and dispatches commands; it never holds a
  transaction, runs SQL or calls a handler (`thincontroller`, `writeboundary`, `rawsql`).
- The UI counterpart is `frontend/app/modules/example/`, which calls these paths through
  `useApi`.

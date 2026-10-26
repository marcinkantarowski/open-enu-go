# Progress - `progress`

The poll half of following a long job. The kernel's reporter (`kernel/progress.Postgres`)
persists every `Start`, `Advance`, `Finish` and `Fail` to `progress_job` and pushes the row to
`/tenants/{tid}/progress/{id}`; this module answers `GET /api/progress/{id}` with the same
row, so a browser that missed the push, or attached late, still sees the truth. It owns no
write and no table.

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `GET /api/progress/{id}` → `{id, kind, label, total, done, percent,
  status, result, failureReason, startedAt, finishedAt}`; another workspace's job is 404
- Permissions: `progress.view`
- Tables: none of its own (reads the kernel's `progress_job`, kernel migration 0003)

## Public contract

Nothing under `contract/`. A module starting a job uses `svc.Progress` and returns the id;
the browser does the rest with `useProgress`.

## Notes for agents

- A job started outside a tenant scope is logged, not stored: there is nobody to show it to.
- A finished or failed job is final; a late call changes nothing and pushes nothing.

## Depends on

`tenant`.

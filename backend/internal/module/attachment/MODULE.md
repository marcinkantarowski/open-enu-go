# Attachment - `attachment`

Owns uploaded files: the bytes go through the kernel's storage (keyed under the tenant,
served by a signed, short-lived URL from `GET /files/{key}`), the row records what they are
and what they belong to. The content type is **sniffed from the bytes**; the client's
claim is never stored. A file nobody claims within a day is swept, blob first.

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `POST /api/attachments` (multipart `file`, optional `ownerType`,
  `ownerId`; 201 with `url`), `GET /api/attachments/{id}` (a fresh `url`)
- Permissions: `attachment.view`, `attachment.manage`; the upload is refused under impersonation (ADR-0020)
- Commands: `attachment.uploaded` (subject = the owner), `attachment.claimed` (an unclaimed
  upload gets its owner; subject = the owner), `attachment.deleted` (the sweep)
- Tasks: `attachment.sweep_orphans` (daily)
- Tables: `attachment` (RLS)
- Console: `openenu attachment sweep-orphans [--hours 24 --dry-run]`

## Public contract

`contract.Files` (`Find`) built by `contract.NewFiles`: whether an id is an upload the
scope's tenant can see, and the `contract.File` it is - name, type, size and owner, never the
storage key. A module storing an attachment's id checks it there first. `contract.Claim`
(dispatched by `contract.Claimer`) gives an upload made without an owner one, in the
claiming command's transaction, so the sweep leaves it; `contract.ErrUnknown` and
`contract.ErrClaimed` are its refusals, which the caller maps onto its own field. The root package
exports `SweepOrphans` (console and task) and `SetMaxBytes` (tests).

## Notes for agents

- The upload limit is `MaxBytes` (20 MiB); the body is read with a limit, never whole.
- A signed URL is valid for 15 minutes. Fetch the record for a new one; do not store URLs.
- The handler writes the blob, then the row; on a failed row it deletes the blob. The
  other order would leave a row pointing at nothing.

## Depends on

`tenant`.

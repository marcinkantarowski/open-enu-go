# Attachment - `attachment`

Owns uploaded files: the bytes go through the kernel's storage (keyed under the tenant,
served by a signed, short-lived URL from `GET /files/{key}`), the row records what they are
and what they belong to. The content type is **sniffed from the bytes**; the client's
claim is never stored. A file nobody claims within a day is swept, blob first.

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `POST /api/attachments` (multipart `file`, optional `ownerType`,
  `ownerId`; 201 with `url`), `GET /api/attachments/{id}` (a fresh `url`)
- Permissions: `attachment.view`, `attachment.manage`
- Commands: `attachment.uploaded` (subject = the owner), `attachment.deleted` (the sweep)
- Tasks: `attachment.sweep_orphans` (daily)
- Tables: `attachment` (RLS)
- Console: `openenu attachment sweep-orphans [--hours 24 --dry-run]`

## Public contract

Nothing under `contract/`. The root package exports `SweepOrphans` (console and task) and
`SetMaxBytes` (tests).

## Notes for agents

- The upload limit is `MaxBytes` (20 MiB); the body is read with a limit, never whole.
- A signed URL is valid for 15 minutes. Fetch the record for a new one; do not store URLs.
- The handler writes the blob, then the row; on a failed row it deletes the blob. The
  other order would leave a row pointing at nothing.

## Depends on

`tenant`.

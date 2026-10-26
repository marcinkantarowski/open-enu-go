# Notification - `notification`

Owns telling a person that something happened: the in-app feed, and the mail behind it when
a type asks for one. It implements the kernel's `notification.Notifier` (`svc.Notifier`),
so a module raises one with `svc.Notifier.ToUser(ctx, userID, "webhook.delivery_failed",
args)` or `ToTenant(ctx, tenantID, ...)` and never imports this module. A row stores the
**type and its arguments**, never a rendered sentence: the reader's language is the
reader's property ([ADR-0008](../../../../.ai/platform/adr/0008-i18n-from-the-start.md)), and the client translates `titleKey`/`bodyKey` with
`context`.

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `GET /api/notifications` (newest 50, `meta.unread`),
  `POST /api/notifications/{id}/read` (somebody else's is **404**, not 403),
  `POST /api/notifications/read-all`
- Permissions: `notification.view` - the only one; a feed is personal, so "whose" is never
  a parameter
- Commands: none. The notifier's writes are a side effect of an already-audited change;
  read state is a repository write (auditing a glance would bury the trail)
- Tables: `notification` (RLS; `type varchar(120)`, `context jsonb`, `read_at`)
- Kernel hooks: `Module.Notifier` (the one provider; the process keeps `notification.Log`
  without it), `Module.Gdpr` (export and erase across every tenant the person is in)
- The catalogue: `svc.NotificationDefinitions` - every module's `Module.Notifications`,
  validated by the registry (owner prefix, keys present in the declaring module's
  catalogue). An **undeclared type is dropped with a log line**, never stored: a row
  nobody can translate is unreadable forever. A type declared once and removed later
  degrades to its own name as the key.

## Public contract

Nothing under `contract/`. The kernel's `notification.Notifier` is the contract; this
module implements it.

## Notes for agents

- `ToUser` takes the tenant from the ambient scope (it is raised from inside the work
  that caused it); `ToTenant` takes it as a parameter and fans out to every member - which
  people count as "the workspace" is this module's question, not the caller's.
- Mail (`Definition.Email`) is translated server-side with the same arguments, in the
  recipient's stored locale (`identity` `Member.Locale`), falling back to the default.
  A mail that fails to send is a log line, not a failure of the change that raised it.
- `markRead` is idempotent: `COALESCE(read_at, now())`. Two tabs must not move it.

## Depends on

`identity` (the directory: who is in a workspace, and their address and locale).

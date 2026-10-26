# internal/module - one package per feature

Each directory is a module. The shape is [ARCHITECTURE.md §4](../../../.ai/platform/ARCHITECTURE.md)
and `example/` is the one to read and copy.
`make module NAME=X` generates it; a module written by hand differs in ways nobody notices
until the third one. The directory name is the Go package, the `module.yaml` name and the
permission prefix - `api_key`, not `apikey`.

Every module with an `openapi.json` serves it through a generated strict server
(`internal/api.gen.go`, `tools/genapi`). Console verbs are `openenu <verb>` subcommands in
`cmd/openenu/console.go`, thin calls into functions the module's root package exports. Tasks
are `module.Task` entries the `scheduler` runs; jobs are River kinds the `worker` runs.

| Module | Owns | Surface |
|---|---|---|
| `tenant` | the workspace row: slug, name, status, plan, locale; `contract/` Reader, commands and events every other module scopes to | no API; console `tenant purge-unverified`; task `tenant.purge_unverified` (daily) |
| `identity` | people, memberships, sessions (access token + rotating refresh cookie), e-mail verification, password reset, the role-to-permission rule | `POST /api/auth/{register,verify,login,refresh,switch-tenant,logout,forgot-password,reset-password}`, `GET\|PATCH /api/profile`; console `user create` |
| `api_key` | per-tenant machine credentials, hashed, with a subset of the declared permissions; the kernel's `KeyResolver` | `GET\|POST /api/api-keys`, `DELETE /api/api-keys/{id}`; console `apikey create` |
| `audit` | reading the trail the kernel writes; GDPR erasure anonymises entries | `GET /api/audit`; `contract.Reader` for the cross-tenant read |
| `attachment` | uploads through the kernel's tenant-scoped storage, typed by their bytes, served by signed URL | `POST /api/attachments`, `GET /api/attachments/{id}`; console `attachment sweep-orphans`; task `attachment.sweep_orphans` (daily) |
| `progress` | reading a persisted job's progress | `GET /api/progress/{id}` |
| `manager` | the operator realm: its own identity table and token audience, tenant administration, the cross-tenant audit view, worker status, impersonation | `POST /api/manager/login`, `GET /api/manager/tenants[/{id}]`, `POST .../tenants/{id}/suspend`, `POST .../tenants/{tenantId}/users/{userId}/impersonate`, `GET /api/manager/audit`, `GET /api/manager/workers`; console `manager create` |
| `settings` | every module's declared flags with per-tenant overrides; the kernel's `flags.Store` (`Module.FlagStore`) | `GET /api/settings`, `PUT /api/settings/{identifier}`, `GET /api/manager/settings`, `PUT /api/manager/tenants/{tenantId}/settings/{identifier}` |
| `notification` | the in-app feed and the mail behind it; the kernel's `Notifier` (`Module.Notifier`) | `GET /api/notifications`, `POST /api/notifications/read-all`, `POST /api/notifications/{id}/read` |
| `webhook` | outbound webhooks: endpoints subscribed to event names, Standard Webhooks signing, a log per attempt | `GET\|POST /api/webhooks`, `DELETE /api/webhooks/{id}`, `GET /api/webhooks/deliveries`, `POST /api/webhooks/deliveries/{id}/redeliver`; job `webhook.deliver` |
| `search` | one endpoint over every module's `search.Declaration`, permission-checked per type | `GET /api/search` |
| `example` | the reference `Project`: one of every platform concern, the module to copy | `GET\|POST /api/projects`, `GET\|PATCH /api/projects/{id}`, `POST /api/projects/archive` (flag `example.archive`); job `example.archive`; task `example.sweep` (daily) |

Visibility is the rule: `x/contract` is importable by any module, `x/internal` by none. The
registry (`cmd/openenu/module_registry_gen.go`, written by `tools/genregistry`) sees only
`x.Module`.

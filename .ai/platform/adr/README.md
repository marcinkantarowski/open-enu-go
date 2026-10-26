# Platform ADRs

Every architectural decision the platform rests on, one per file, in the order that builds
the picture: what the system is made of, how a module is shaped, where data and tenancy
live, how a write happens, who may ask, and how it runs on a server. Several are decisions
**not** to do something - no hierarchical organisations (0011), no custom fields (0012), no
undo (0014) - and are the ones most likely to be mistaken for oversights.

Read the relevant record before proposing an architectural change. Changing one is Ask
First; the change is a new record that names the one it supersedes, never a silent edit. A
project built from this platform keeps its own decisions in [`../../adr/`](../../adr/README.md)
and departs from one of these by writing its own record that names it (ADR-0027).

## Foundation

| # | Decision |
|---|---|
| [0001](0001-go-backend-and-nuxt-apps.md) | The backend is one Go binary; the browser apps are Nuxt |
| [0002](0002-kernel-packaging.md) | The kernel is a package; its name is never the project's name |
| [0003](0003-modular-backend.md) | A modular backend, not flat layers |
| [0004](0004-module-boundaries.md) | Module boundaries are enforced by tooling, not discipline |
| [0005](0005-extension-by-named-slots.md) | Extension by named slots on `module.Module`; no general decorator |
| [0006](0006-spec-first-openapi.md) | `openapi.json` is the contract: authored first, the server generated from it |
| [0007](0007-frontend-modules-as-layers.md) | Each frontend module is a local Nuxt layer |
| [0008](0008-i18n-from-the-start.md) | Two locales from the start, enforced by checks |

## Data and tenancy

| # | Decision |
|---|---|
| [0009](0009-tenancy-by-row-level-security.md) | One database, `tenant_id`, row-level security that fails closed |
| [0010](0010-membership-from-day-one.md) | `Membership(user, tenant, role)` from day one |
| [0011](0011-no-hierarchical-orgs-in-v1.md) | No hierarchical organisations in v1; the door stays open |
| [0012](0012-no-custom-fields-layer.md) | No runtime custom-fields layer; a JSONB `attributes` column instead |

## Writes, events and the data that needs care

| # | Decision |
|---|---|
| [0013](0013-command-bus-with-explicit-snapshots.md) | Every write is a command; the handler records both states |
| [0014](0014-no-undo.md) | No undo/redo; command snapshots keep it possible |
| [0015](0015-events-via-outbox.md) | Domain events and jobs go through a transactional outbox |
| [0016](0016-light-encryption.md) | Field encryption with derived per-tenant keys, not a KMS |
| [0017](0017-search-tsvector-only.md) | Search is an interface with one Postgres implementation |

## Who may ask

| # | Decision |
|---|---|
| [0018](0018-token-transport.md) | The access token lives in memory, the refresh token in an HttpOnly cookie |
| [0019](0019-realm-isolation-by-audience.md) | Realms are isolated by the token's audience |
| [0020](0020-impersonation.md) | Impersonation is a specified crossing, not a convenience |
| [0021](0021-exact-origin-cors.md) | CORS is an exact-origin allowlist, named by the URLs the apps are served at |
| [0022](0022-argon2id-passwords.md) | Passwords are argon2id, in the PHC string format |
| [0023](0023-registration-answers-the-same-for-every-address.md) | Registration answers the same for every address |

## Running it

| # | Decision |
|---|---|
| [0024](0024-app-stage.md) | One variable says where a checkout runs: `APP_STAGE` |
| [0025](0025-remote-deploy-over-ssh.md) | Deploy over SSH, building on the server |
| [0026](0026-backups-are-offbox-and-verified.md) | Backups are off-box, encrypted, and restore-tested |
| [0027](0027-platform-and-project-knowledge-are-separate-trees.md) | The platform's knowledge lives in `.ai/platform/`; the rest of `.ai/` is the project's |

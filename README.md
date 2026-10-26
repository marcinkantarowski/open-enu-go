# open-enu-go

A foundation for multi-tenant SaaS products, built to be read and extended by people and AI
agents without accumulating technical debt: a Go API that is one static binary, three Nuxt
apps sharing one UI layer, the Docker stack to run them locally and on a server, and
guardrails that fail the build when a rule is broken.

Out of the box: registration with mail verification, login, several workspaces per person
and switching between them, password reset, scoped API keys, uploads, an audit trail of
every change with before and after state, notifications, outbound webhooks, full-text
search, feature flags with per-tenant overrides, live updates in the browser, an operator
console with impersonation, GDPR export and erasure - in English and Polish.

## Start

```bash
make check-tools         # Go 1.27 (PATH, ~/.local/go or /usr/local/go), Docker, mkcert, python3
make init NAME=acme      # make it your project: slug, domain, Go module path - never the kernel
make builddev            # the whole stack from nothing, under trusted HTTPS (~3 min cold)
make urls                # where everything answers
make e2e-fixtures        # the accounts the browser tests log in as
```

`make builddev` writes `.env` from `.env.example`, generating every secret once, trusts a
local CA, maps the hosts (`make hosts` asks for sudo), starts the stack, migrates and runs
the smoke test. Then:

```bash
make check               # the inner loop after every edit - about 15 s, no Docker
make module NAME=invoice # a new feature: backend package, spec fragment, frontend layer, docs
make ci                  # the full gate: guardrails, selftest, tests, typecheck, smoke, e2e
```

## Where things are

| Path | Contents |
|---|---|
| `backend/kernel/` | `github.com/open-enu/kernel` - the framework, a separate Go module, never renamed |
| `backend/internal/module/<name>/` | one package per feature; `contract/` public, `internal/` private |
| `backend/cmd/openenu/` | the one binary: `api`, `worker`, `scheduler`, `migrate`, `mcp`, `health`, console verbs (`openenu help`) |
| `backend/openapi.json` | the contract, merged from each module's fragment |
| `ui-kit/` | `@open-enu/ui-kit` - the Nuxt layer the apps extend |
| `frontend/` `manager/` `landing/` | the tenant app, the operator console, the marketing site |
| `e2e/` | the Playwright suite against the real stack |
| `docker/` `scripts/` | the stacks, the edge, local tooling, provisioning |
| `.ai/platform/` | architecture, decisions, docs, lessons and skills |

## Read next

- [`AGENTS.md`](AGENTS.md) - the rules and the task router, for people and agents alike.
- [`.ai/platform/ARCHITECTURE.md`](.ai/platform/ARCHITECTURE.md) - what the system is made of.
- [`.ai/platform/adr/`](.ai/platform/adr/README.md) - every decision and why, in order.
- [`.ai/platform/docs/deployment.md`](.ai/platform/docs/deployment.md) - from a bare VPS to
  production.

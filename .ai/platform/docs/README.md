# Platform docs

`AGENTS.md` holds hard rules and routing, under a 32 KB budget. Procedure that does not fit
lives here, and the Task Router points at it. Each doc describes what exists; the overall
design is [`../ARCHITECTURE.md`](../ARCHITECTURE.md), and the decisions behind it are in
[`../adr/`](../adr/README.md).

| Doc | What it answers |
|---|---|
| [platform-services.md](platform-services.md) | **read before adding any infrastructure to a module** - the kernel contracts ([ARCHITECTURE.md](../ARCHITECTURE.md) §5): what a module is handed in `Mount`, and the call it makes for each |
| [module-development.md](module-development.md) | the shape of a module, `make module`, adding a route, operator screens, what the registry checks |
| [extension-surfaces.md](extension-surfaces.md) | every way to add behaviour without editing the platform: the named slots, reaching another module, what is deliberately not extendable |
| [api-contract.md](api-contract.md) | fragments, the merge, the generated strict server, operation ids, the error envelope, the frontends' types |
| [guardrails.md](guardrails.md) | every rule, what enforces it, how it is proved to fire, how to add one |
| [tenancy.md](tenancy.md) | RLS, `SET LOCAL`, the non-superuser role, `scope.Unscoped` |
| [auth.md](auth.md) | the principal, JWT and API keys, the refresh cookie, realms, `Require`, the impersonation guard, the audit actor |
| [commands-and-audit.md](commands-and-audit.md) | the bus, handlers, `tx.Update(old, new)`, nested commands, optimistic locking |
| [events-and-outbox.md](events-and-outbox.md) | River as the outbox, broadcast opt-in, subscribers, jobs, retries |
| [i18n.md](i18n.md) | any user-facing string: where catalogues live, what parity means on each side, which language a response speaks, adding a locale |
| [frontend-conventions.md](frontend-conventions.md) | anything in the three Nuxt apps: the rules, a module's layer, what the shared layer gives you, permissions in the UI |
| [landing.md](landing.md) | changing the marketing site - it ships as a "coming soon" template: pages, sections, SEO |
| [testing.md](testing.md) | `-short` vs the gate, testcontainers, `InMemory` + `WithDatabase`, what goes where, the browser suite |
| [env-vars.md](env-vars.md) | `.env` and what `make env` derives from it, adding a variable, a project's own variables, secrets |
| [dev-stack.md](dev-stack.md) | `make builddev`, the services, where the backend's environment comes from, air, the realtime cookie, `make e2e` |
| [troubleshooting.md](troubleshooting.md) | something in the dev stack is broken - symptoms first |
| [observability.md](observability.md) | the fields on every log line, the request id a job carries, queues, what is deliberately not installed |
| [deployment.md](deployment.md) | preflight, `buildstaging`/`buildprod`, the promote loop, backups, what each command refuses, what is not tested |
| [agent-interfaces.md](agent-interfaces.md) | the inventory, the MCP server, route coverage, dead code - what an agent reads and what it is offered |
| [updating.md](updating.md) | `make platform-update`: bringing a project up to a newer platform as one staged three-way merge |

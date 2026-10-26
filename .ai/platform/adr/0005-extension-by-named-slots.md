# ADR-0005 - Extension by named slots; no general decorator

**Status:** accepted · **Date:** 2026-10-25

## Context
Modules extend the platform without editing the kernel. Two general mechanisms are common
elsewhere: a dependency-injection container in which any module may decorate any service,
and a service locator. Go has no container. `platform.Services` is a struct the process
fills in `cmd/openenu/platform.go` and hands to each module's `Mount`. "Decorate anything"
would mean either a locator keyed by type - every call site a lookup, every miss a runtime
error - or a mutable `Services` that modules rewrite while they mount, in whatever order
the registry mounts them.

## Decision
A module extends the platform only through **named slots on `module.Module`**: one field per
thing that can be contributed, each with a rule the registry checks at start-up.

- Additive slots collect from every module, in dependency order: `Permissions`, `Flags`,
  `Setup`, `Gdpr`, `Tasks`, `Search`, `Notifications`, `Locale`, `LogFields`.
- Single-provider slots take one module's implementation of a kernel interface, and the
  registry refuses two: `Keys` (API keys), `FlagStore` (per-tenant overrides), `Notifier`.
  Without a provider the kernel's default stands (no keys, declared defaults, a log line).

A new extension point is a new field, a `Registry` method that collects or picks it, a test,
and a row in [`docs/extension-surfaces.md`](../docs/extension-surfaces.md) - `Keys`,
`FlagStore` and `Notifier` are the pattern to copy.

Replacing a platform service as a whole (`Mail`, `Storage`, the cache backend) is the
process's business: wrap the interface in `cmd/openenu/platform.go`, where the value is
built. That is a decorator in plain Go, in the one file that composes the process.

## Consequences
- Every way in is listed in one struct and one doc; "can a module change X?" is answered by
  reading `module.go`, not by searching for a tag.
- A module cannot alter a guarantee: the tenant scope, the realm check, the bus's audit and
  the outbox are not fields, so there is nothing to put in them.
- An extension nobody planned for needs a kernel change: a new slot, reviewed as a public
  contract (Ask First).

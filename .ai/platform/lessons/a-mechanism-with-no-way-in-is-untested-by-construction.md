---
tags: [feature-flags, testing, completeness, registry]
date: 2026-10-25
---
# A mechanism with no way to create its inputs is untested by construction

**A test that constructs its own inputs cannot notice that nothing else can.** A mechanism can
be built and tested end to end and still be unusable, because every test supplies the first
link of the chain.

## The trap
Feature flags are a resolution chain: a tenant override, then the declared default. A route
behind a switched-off flag answers 404, a flip bites on the next read, and a security test
proves a flag turned on for tenant A leaves tenant B alone. All of that can be built, and
green, while nothing in the repository can create a flag:

```go
if !a.flags.Enabled(ctx, "example.archive", false) {
	return nil, httpx.NotFound(httpx.KeyNotFound)
}
```

With nothing declaring `example.archive`, this reads the call's default forever. The settings
endpoint lists nothing for every tenant, the operator console's flags page lists nothing, and
the route can never be enabled. Every test passes - a test that writes its own override and
asserts on the behaviour around it passes *because* it wrote the row, which is exactly the
shape that hides the gap. Fixtures are supposed to construct inputs - it is their job - so the
absence is invisible precisely where you would expect to catch it.

It shows the first time something has to switch the flag *on* rather than merely observe it
off.

## The rule
For any mechanism, ask three questions and make sure something in the repository answers each
of them without a test's help:

1. **How does an input get created** the first time, in a real environment?
2. **Who can see it** once it exists?
3. **Who can change it**, and does the change take effect?

## Where it is held
- A flag is a `flags.Definition` on the module's `Module.Flags`, declared beside the code it
  guards (`internal/module/example/internal/flags.go`). The registry validates it at start-up:
  the identifier's shape, the owner's prefix, one owner per flag.
- The Settings module lists the definitions straight from the registry (`svc.FlagDefinitions`,
  built from `reg.Flags()` in `cmd/openenu/platform.go`). There is no sync step: a declared flag
  is visible, and switchable, the moment the binary starts.
- `flagdeclared` refuses a read of a key that has no `flags.Definition` in the same package, and
  `make selftest` plants one.
- `internal/security/realm_test.go` (`TestAFlagFlippedForOneTenantDoesNotAffectAnother`)
  switches the flag through the operator API, the way a person would, instead of writing a row;
  `e2e/tests/example.spec.ts` does it in a browser through the operator console and then runs
  the job the flag guards.

## Where else this applies
Everywhere a value is read far more often than it is written:

- **Permissions** - declared on `Module.Permissions` and validated by the registry; the
  committed ACL (`cmd/openenu/acl_gen.go`) is read from each handler's `auth.Require` literal,
  and `make selftest` proves a changed guard makes it stale.
- **Notification types** - declared on `Module.Notifications`; the registry refuses one whose
  title or body key is missing from a locale.
- **Settings that are not flags** - same mechanism, same answer.
- **Any seed, default or catalogue.** If the only code that creates one lives in a test, it
  does not exist.

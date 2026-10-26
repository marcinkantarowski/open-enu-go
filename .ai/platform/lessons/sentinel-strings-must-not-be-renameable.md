---
tags: [init, tooling, self-reference]
date: 2026-10-25
---
# A sentinel built from a renameable string inverts its own meaning

Any tool that rewrites source will rewrite its own control logic if that logic is expressed
in the strings it rewrites. A check compared against the string being renamed ends up checking
the opposite of what it says.

## The trap
A script that warns while the tree is still the un-renamed boilerplate:

```bash
[ "$SLUG" = open-enu-go ] && log_warn "still the boilerplate default - run: make init"
```

A rename that rewrites `open-enu-go` across the tree rewrites this line too, into
`[ "$SLUG" = acme-crm ]`. The check then matches *only* after a rename: a freshly renamed
project is told it has not been renamed, and an un-renamed one is told it is fine. Exactly
backwards, and silently. Nothing fails - the message is just wrong - so only running the rename
on a throwaway copy and reading the output shows it.

## The rule
**State that a rewriting tool depends on must live in a form the rewrite cannot express.**
A boolean, a file's presence, a checksum, a role - not a copy of the string being replaced.

## Where it is held
- `.project.json` carries `"initialized": true|false`, flipped by `init.sh`. `init.sh`,
  `module.sh`, `check-docs.sh` and `platform-update.sh` read the flag; nothing compares a slug
  with a literal. `make selftest` asserts the manifest says `"initialized": true` after a
  rename, and that a second `init` reads its own result and is a reported no-op.
- `init.sh` replaces exact strings in the files it names - `go.mod` and import lines, the
  manifest, the `Makefile` defaults, `.env.example`, the compose fallbacks - never a pattern
  across the tree. That narrows the surface without removing it: a check placed in one of
  those files and compared with the slug would still invert.
- The smoke test recognises each Nuxt app by its `app-role` meta tag (`tenant`, `operator`,
  `marketing`) - a role, never the project name (`scripts/dev/smoke.sh`, each app's
  `nuxt.config.ts`).
- `scripts/dev/check-typography.sh` builds the long dash from its bytes, so a bulk replace
  cannot rewrite the pattern into a hyphen that matches every line in the repository.

## Where else this applies
- The kernel's own names (`github.com/open-enu/kernel`, `@open-enu/ui-kit`) share the
  boilerplate's name and must survive every rename
  ([ADR-0002](../adr/0002-kernel-packaging.md)): `init.sh` refuses the kernel's path as
  `GO_MODULE`, and `make selftest` asserts the kernel tree is byte-identical after `init`.
- Any future codemod, scaffolder or `make module` template that inspects the tree it edits.

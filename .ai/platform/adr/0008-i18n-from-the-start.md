# ADR-0008 - Two locales from the start, enforced by checks

**Status:** accepted · **Date:** 2026-10-22

## Context
Internationalisation added later is a rewrite: every string is already inline - in
components, errors, validation messages and mail templates. Added at the start, it costs a
translation key per string.

## Decision
`en` and `pl` ship from the start.

- **API.** Every user-facing string is an `i18n.Key` resolved from the module's
  `i18n/messages.{en,pl}.json` (the kernel has its own catalogue). The registry refuses to
  start when a key exists in one locale and not the other, and `openenu modules` names it.
  The `rawstring` analyzer refuses a sentence where a key belongs.
- **Response language.** The person's stored preference, then the tenant's default, then
  `Accept-Language` against the locales that ship, then the catalogue's default;
  `Content-Language` says which was chosen (`kernel/httpx/locale.go`).
- **Notifications** store a key and its arguments, never a sentence, and render in the
  recipient's language.
- **Browser apps.** Catalogues per layer in `i18n/locales/{en,pl}.json`; the landing site
  has one markdown file per page per locale. `make i18n-check` fails when a key or a page
  exists in one locale and not the other.

Translatable *entity fields* - a tenant's own content in several languages - are not part
of the foundation; that is a product feature.

## Consequences
- Both ship; a project may offer one (`LOCALES`, [`i18n.md`](../docs/i18n.md)).
- Two locale files per module, scaffolded by `make module`.
- A third language is files, not a project.
- Some friction on every string. That is the mechanism working
  ([`docs/i18n.md`](../docs/i18n.md)).

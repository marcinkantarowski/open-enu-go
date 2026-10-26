# ADR-0018 - The access token lives in memory, the refresh token in an HttpOnly cookie

**Status:** accepted · **Date:** 2026-10-22

## Context
`localStorage` is where most single-page-app tutorials keep tokens. Then an XSS in any
dependency exfiltrates a long-lived refresh token, and the attacker keeps access after the
page is closed.

## Decision
- **Access token** (15 minutes): browser memory only, in ui-kit's auth store. Never
  `localStorage`, never a cookie.
- **Refresh token** (30 days): the `open_enu_refresh` cookie - `HttpOnly; Secure;
  SameSite=Strict; Path=/api/auth` on the API host. Stored hashed, single-use: every refresh
  spends it and issues the next one in its family; logout revokes every refresh token of the
  person.
- **Operators** get no refresh cookie at all; an operator session is fifteen minutes and a
  reload is a new login.
- **Realtime:** `EventSource` cannot set headers, so the browser subscribes to the Mercure
  hub with the hub's own cookie, `__Secure-mercure_access_token`, holding a short OAuth 2.0
  access token (`typ: at+jwt`) that the API issues for exactly the tenant's topics.

`app.${DOMAIN}` and `api.${DOMAIN}` are the same site (site = registrable domain), so
`SameSite=Strict` works without a `Domain=` attribute.

## Consequences
- An XSS can act as the person while the page is open; it cannot steal a durable
  credential.
- CORS must allow exact origins with credentials - a wildcard is refused by browsers anyway
  (ADR-0021).
- A page reload costs one refresh call rather than a login.

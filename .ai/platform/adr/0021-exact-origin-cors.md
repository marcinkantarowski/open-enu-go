# ADR-0021 - CORS is an exact-origin allowlist, named by the URLs the apps are served at

**Status:** accepted · **Date:** 2026-10-22

## Context
The tenant app, the operator console and the API are three hosts, so **every** call the
browser makes is cross-origin. Two facts make the policy load-bearing rather than
boilerplate:

- The refresh token rides on a cookie (ADR-0018), so the API must answer with
  `Access-Control-Allow-Credentials: true` - which browsers refuse beside a wildcard origin.
  A wildcard is not merely unwise here; it does not work.
- With credentials allowed, the origin list is the only thing standing between a hostile
  page and a request made with the person's own cookies.

Pattern matching is the usual shortcut, and it is wrong: an unescaped `.` in
`app.example.com` also matches `appXexample.com`, which somebody can register.

## Decision
`httpx.CORS` matches origins exactly, never by pattern. The allowlist is `APP_URL` and
`MANAGER_URL` - the same two values `envgen` derives from `DOMAIN` and writes into the apps'
own configuration - so the origins a browser may call from cannot drift from the URLs the
apps are served at. There is deliberately no separate CORS variable: a second place holding
the same two values is a second place to forget.

- An allowed origin gets `Allow-Origin` (echoed), `Allow-Credentials: true`, and exposed
  `ETag`, `Location` and `X-Request-Id` - a header the browser does not expose is invisible
  to JavaScript, and `ETag` is how the client learns the version to send back in `If-Match`.
  A preflight's requested headers are echoed: the allowlist is the origin, not the header.
- An unlisted origin gets no CORS headers, which is the browser refusing it; its preflight
  is answered 403, so the failure is visible in the network tab instead of looking like a
  timeout. A request with no `Origin` (curl, a server) is untouched.

## Consequences
- A third browser origin (a mobile web build, a partner console) is a deliberate
  configuration change.
- A lookalike domain cannot slip through, because nothing is pattern-matched.
- A bare `localhost:3000` outside the Docker stack is not allowed. Intentional: the stack
  serves the apps under real hostnames with trusted TLS so that development differs from
  production as little as possible.
- `internal/security/cors_test.go` asserts the *absence* of the header for unlisted and
  lookalike origins. A browser test notices a policy that is too narrow the moment the app
  stops working, and never notices one that is too wide.

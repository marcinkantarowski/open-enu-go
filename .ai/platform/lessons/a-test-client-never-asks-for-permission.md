---
tags: [cors, security, testing, browser]
date: 2026-10-25
---
# A test client never asks the browser's permission

CORS, cookie attributes and which response headers JavaScript may read are enforced by the
browser, on the browser's side, using headers the server is merely expected to supply. A
client that talks to the server directly - `httptest`, `curl`, Playwright's `request` fixture -
asks no browser, so that whole class of failure is invisible to it by construction.

## The trap
A Go suite drives the real handler stack over HTTP against a real database, and the security
suite covers realm isolation, tenant scoping, token reuse and credential handling. All of it
can be green while the first browser that tries to sign in gets nothing at all: no error in
the server log, no failed request in the API's access log, and this in the page:

```
Failed to fetch
```

None of those clients sends an `Origin` header, issues a preflight, or cares what comes back
in `Access-Control-Allow-Origin`. And the gap is not small here: the apps and the API live on
separate hosts (`app.`, `manager.` and `api.`), so **every** call the frontend makes is
cross-origin.

## The rule
**If a behaviour is enforced by the client, a server-side test cannot cover it.** Name those
behaviours explicitly and put a browser in front of them. In this repository the list is short
and worth memorising: CORS, cookie attributes (`httpOnly`, `SameSite`, `Domain`, `Path`),
`EventSource` and its credentials, the `multipart` boundary, and which response headers
JavaScript is allowed to read.

Then test the other side on the server. A browser test notices a policy that is too narrow
the moment the app stops working, and never notices one that is too wide.

## Where it is held
- `make e2e` (`e2e/tests/`) drives a real browser against the real stack: the cookie session
  in `login.spec.ts`, the `EventSource` stream in `realtime.spec.ts`, the multipart upload in
  `upload.spec.ts`, the `If-Match` round trip and the 409 in `example.spec.ts`. It runs in
  `make ci`, never in `make check`.
- `backend/kernel/httpx/cors_test.go` covers the too-wide side: `Access-Control-Allow-Origin`
  is absent for a third origin and for a listed host on another port, and a preflight from an
  unlisted origin is answered 403.
- `backend/kernel/httpx/cors.go` allows exact origins only, never a wildcard
  ([ADR-0021](../adr/0021-exact-origin-cors.md)).

## Where else this applies
- `Access-Control-Expose-Headers`: a response header is on the wire and invisible to
  JavaScript unless it is listed. `cors.go` exposes `ETag, Location, X-Request-Id`; a client
  that learned the version from `ETag` would lose optimistic locking from the browser's side
  while every server-side test kept passing.
- `Access-Control-Allow-Headers`: the same, for `If-Match` on the way out. `cors.go` echoes
  the headers a preflight asks for, so the allowlist is the origin; a fixed list of headers
  would have to name `If-Match`.
- Anything added later that the browser gates: `Permissions-Policy`, `COEP`/`COOP`,
  `Content-Security-Policy`. None of them can fail a `go test`.

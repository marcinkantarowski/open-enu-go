# End-to-end tests

Six specs, a real browser, the real stack. They cover the behaviours that **cannot** be
tested anywhere else, because a browser is the thing enforcing them:

| Spec | What only a browser can see |
|---|---|
| `login.spec.ts` | The refresh token is an `httpOnly`, `SameSite=Strict` cookie; nothing resembling a token is in `localStorage` or `sessionStorage`; a reload restores the session instead of showing a login form |
| `tenant-switch.spec.ts` | Switching workspace re-issues the session and re-scopes what is on screen |
| `impersonation.spec.ts` | The operator console hands a genuine tenant token to the tenant app through the URL **fragment**, the banner is unmissable, and no token is left in the address bar. Plus: an operator token is refused by the tenant API |
| `realtime.spec.ts` | An `EventSource` actually connects with its cookie and a change in one session arrives in another without a reload |
| `upload.spec.ts` | The multipart boundary is set by the browser, not by us |
| `example.spec.ts` | The reference module: a create reaches another session over SSE and not another tenant's; two concurrent editors, a 409, and a conflict bar offering a real choice; an operator switches a flag on and a job reports progress |

CORS sits behind all of them: a server-side test client never sends a preflight, so only a
browser proves the API answers one - see [[a-test-client-never-asks-for-permission]].

## Running them

```bash
make e2e                 # fixtures, then the whole suite
make e2e SPEC=tests/login.spec.ts
```

`make e2e` is part of `make ci` and deliberately **not** part of `make check`: it needs the
whole stack up, and a sub-60-second inner loop cannot ask for that
([testing.md](../.ai/platform/docs/testing.md)).

## Fixtures

`scripts/dev/e2e-fixtures.sh` creates the accounts through `openenu user create` and
`openenu manager create` - both idempotent, so it runs before every suite without accumulating
state. The same accounts are handy for signing in by hand; they are listed in
`fixtures/accounts.ts`, which must stay in step with that script.

Two workspaces for one person, on purpose: without a second membership the tenant switcher
never renders, and the switch spec would assert on something that is not there.

## Rules

- **No retries.** A failure here is a failure. Retrying would turn "the SSE connection never
  arrived" into "passed on the second try", which is the bug.
- **Wait on state, never on the clock.** The one timeout that exists is the ceiling before a
  genuine hang is declared.
- **Fixtures must write values that differ from what is already there.** A test that leans on
  a write which changes nothing depends on whether the server bumps the version for a no-op,
  and a locking test can then silently stop testing anything - see
  [[a-no-op-write-does-not-bump-the-version]].
- The `@playwright/test` version in `package.json` is also the browser image's tag: `make e2e`
  reads it from there, so bumping the package bumps the browsers. Keep it an exact version -
  a range (`^1.63.0`) is not an image tag, and `make e2e` refuses it.

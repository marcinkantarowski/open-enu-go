# ADR-0023 - Registration answers the same for every address

**Status:** accepted · **Date:** 2026-10-25

## Context
The public auth surface refuses to say whether an address has an account: login is one 401
for every failure, forgot-password answers 202 either way. A registration endpoint that
answered "that address is taken" would undo both - it would tell anyone, for the price of a
request, which addresses have accounts here. A rate limiter makes that slower; it does not
make it something other than an oracle.

## Decision
`POST /api/auth/register` answers **202 `{status: "unverified"}`** for every well-formed
request.

- A new address gets an account, a pending workspace and a verification link.
- An address that has an account gets nothing created. Its owner receives a mail saying
  someone tried to register it, with a link to log in and one to reset the password.
- An address whose account was never verified gets a new verification link to the
  workspace it started: that person most likely lost the first mail, and "log in" would not
  let them in.
- Every path derives a password hash (`PasswordHasher.Spend` where nothing is stored), so
  they also take the same time.
- The answer carries no ids: there are none to give in the second case, and inventing some
  would be a lie a client could act on. The verify answer (`userId`) and the session are
  where a client learns who it is.
- The attempt is on the audit trail as `identity.user.registered`, with
  `accountExisted: true` when it was, so the trail still shows what happened.

Login spends a verification for an unknown address too, and checks the password before the
account's state, for the same reason.

## Consequences
- The registration form says "check your mail" in every case; a person who already has an
  account learns so from their mailbox, not from the form.
- A mail is sent per attempt on a known address, bounded by the auth rate limiter
  (`httpx.RateLimit` on `/api/auth/`).

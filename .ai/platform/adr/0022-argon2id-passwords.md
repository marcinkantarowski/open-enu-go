# ADR-0022 - Passwords are argon2id, in the PHC string format

**Status:** accepted · **Date:** 2026-10-25

## Context
A stolen table of password verifiers is guessed offline, at the attacker's speed. A hash
that is only slow - PBKDF2, bcrypt - lets a GPU try billions of guesses for the price of
time alone. A memory-hard one makes every guess pay in memory as well.

## Decision
`crypto.PasswordHasher` writes **argon2id** (`golang.org/x/crypto/argon2`) in the PHC string
format - `$argon2id$v=19$m=65536,t=4,p=1$<salt>$<hash>` - the format the reference
implementation and most languages' libraries read and write, so verifiers can move between
systems. The default cost, `DefaultPasswordCost`, is 64 MiB, four passes, one lane.

- The encoded form carries its parameters, so the cost can change and older verifiers still
  check. `NeedsRehash` is true for another cost, and Identity's and Manager's logins replace
  such a verifier while the password is in hand.
- Parameters read from a stored verifier are bounded (at most 1 GiB, 64 passes, 64 lanes),
  so a corrupted row cannot ask for a terabyte.
- Derivations run at most `GOMAXPROCS` at a time: each holds 64 MiB, and a burst of logins
  waits for a core instead of allocating one block per request.
- `Spend` runs a derivation and discards it, so a login for an unknown address and a
  registration of a known one take the same time as the real thing (ADR-0023).
- A test platform uses argon2's floor (8 KiB, one pass); the format and the rules are what
  a test checks.

## Consequences
- A login costs about 64 MiB and tens of milliseconds of one core. The API container's
  memory limit must allow `GOMAXPROCS` of those at once.
- A cost change is a code change to `DefaultPasswordCost`; verifiers move on their owners'
  next logins, never in a migration.

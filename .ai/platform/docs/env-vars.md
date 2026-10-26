# Environment variables

**`.env` at the repository root is the only file edited by hand.** Everything else that
holds configuration is derived from it, so hostnames, CORS origins and Mercure URLs cannot
disagree - the API's allowed origins *are* the URLs the apps are served at, built from the
same `DOMAIN`.

```
.env.example   committed, documents every variable with a comment and a safe default
.env           gitignored, chmod 600, the one you edit
   ↓ make env  (scripts/lib/envgen.sh)
frontend/.env   manager/.env   landing/.env   e2e/.env   docker/api/project.env
```

The Go backend has **no env file**: compose builds the environment of `api`, `worker` and
`scheduler` from `.env` in the `x-backend-env` anchor of each compose file, and the process
reads it with `os.Getenv` in `backend/cmd/openenu/`. Which variable reaches the backend
under which name, and who reads it: [`dev-stack.md`](dev-stack.md) → *Where the environment
comes from*.

Every derived file says `GENERATED` on line 1. Editing one is pointless - the next
`make env` overwrites it.

---

## The two invariants

**An existing value is never overwritten.** This is what makes `make buildprod` safe to
re-run against a live server: a regenerated `APP_SECRET` would invalidate every session, a
regenerated `APP_ENCRYPTION_KEY` would make every encrypted column unreadable *permanently*.

**A variable added upstream surfaces as a message, not as a 3am crash.** `make env` appends
any key `.env.example` declares and `.env` lacks, with its default, and says so.

```bash
make env         # create or repair .env, fill secrets, then derive everything from it
make env-check   # read-only: what is missing, what is still __GENERATE__, what is declared twice
```

A variable declared twice is what a merge leaves behind when two sides add the same one
with different values - a platform update and the project, typically: git sees two
additions, not a conflict, and the last line silently wins. `make env-check` refuses it in
`.env.example` and in `.env`.

`__GENERATE__` as the value in `.env.example` means "fill this with 32 random bytes on
first run". Use it for anything secret; never ship a real default.

---

## Adding one

1. Add it to `.env.example`, **with a comment** saying what it does and a safe default.
2. If the backend reads it, add it to `x-backend-env` in `docker/compose.dev.yml` **and**
   `docker/compose.prod.yml`, then read it in `backend/cmd/openenu/` and hand the value to
   the kernel or a module through `platform.Services`. No module reads the platform's
   variables itself.
3. If an app needs it, add it to that app's block in `scripts/lib/envgen.sh`. `NUXT_PUBLIC_*`
   only, for anything the browser may see - and only that: everything under `NUXT_PUBLIC_`
   is compiled into the client bundle.
4. `make env && make check`.

### A project's own variables

A product built on the platform talks to third parties the platform has never heard of - an
invoicing service, an SMS provider - and each comes with a credential. Those do not go
through steps 2 and 3. Declare them in `.env.example` **below the line**

```
# == Project variables ==
```

and `make env` writes every variable declared after it, with its value from `.env`, into
`docker/api/project.env` - an `env_file` the `api`, `worker` and `scheduler` services read in
dev and in production - where the project's module reads it with `os.Getenv`. Neither the
generator nor the compose files are edited, so a platform update never conflicts over them.
Values are passed literally: compose does not expand or unquote an `env_file`. Frontend
variables are not covered: anything under `NUXT_PUBLIC_` is public, and a credential must
never be one.

---

## What the important ones do

| Variable | Effect |
|---|---|
| `DOMAIN` | drives every host: apex, `www.`, `app.`, `manager.`, `api.` (which also serves Mercure at `/.well-known/mercure`), `mail.` and `traefik.` in dev |
| `PROJECT_NAME` / `PROJECT_SLUG` | set by `make init`, not by hand; the name is what the apps display |
| `STACK`, `COMPOSE_FILE` | the compose project and its files; on a server `<slug>-staging` or `<slug>-prod`, set by provisioning |
| `APP_STAGE` | where this checkout runs: `local`, `staging` or `production` ([ADR-0024](../adr/0024-app-stage.md)). Production alone is indexable |
| `DEMO_CONTENT` | whether the tenant app offers the platform's reference module in its menu. `true` on a fresh clone; a product sets `false`. The module itself stays - tests stand on it |
| `DEFAULT_LOCALE` | the language before anyone has chosen: the apps' default and the backend's fallback (`APP_DEFAULT_LOCALE`), `en` or `pl` |
| `APP_SECRET` | signs access tokens and storage URLs; at least 32 characters or the binary refuses to start. Rotating it logs everyone out |
| `APP_ENCRYPTION_KEY` | the root of every derived per-tenant key. **Losing it loses the data** |
| `MAILER_DSN` | Mailpit in dev; production refuses to render its compose file without a real one |
| `BACKUP_REMOTE`, `BACKUP_AGE_PUBKEY` | an rclone target and the age key the dump is encrypted to. Production refuses to finish provisioning without them |

Optional integrations are no-ops when empty - `BACKUP_REMOTE` on staging, `REDIS_ADDR` and
`MERCURE_URL` for a native `openenu` outside the stack - because a boilerplate must boot
with zero external accounts. The exception is mail in production: silently dropping
registration mail is worse than not starting.

---

## Secrets, honestly

They are environment variables. That means they are visible to `docker inspect` and to any
process inside the container. That is an accepted trade-off for a single-host Compose
deployment, not an oversight: a project that needs more moves to Docker secrets or a vault,
and `scripts/lib/envgen.sh` and the `x-backend-env` anchors are the places that change.

Nothing secret is ever committed: `.env`, `.env.*` (but `.env.example`),
`docker/api/project.env`, `docker/certs/` and `.out/` are gitignored.

---

## On a server

`make buildstaging` / `make buildprod` create the remote `.env` on the server itself, from
`.env.example`, and run `make env` there - secrets are generated on the server and never
travel through your laptop. Re-running regenerates nothing that already exists; it only
sets what provisioning owns (`DOMAIN`, `APP_STAGE`, `STACK`, `COMPOSE_FILE`, the staging
allowlist). See [`deployment.md`](deployment.md).

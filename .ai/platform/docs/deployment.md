# Deployment

One command takes a bare VPS to every subdomain live. This is what it does, what it refuses
to do, and what to reach for when something is wrong.

## Two stages, one server or two

| | staging | production |
|---|---|---|
| What runs | the **dev** stack - air, Vite, Mailpit | immutable images: one static binary for `api`, `worker`, `scheduler`; one Nuxt image per app |
| Checkout | `/opt/<slug>-staging` | `/opt/<slug>-prod` |
| Compose project, containers, DB volume | `<slug>-staging-*` | `<slug>-prod-*` |
| Who can reach it | `STAGING_ALLOW_FROM` only (IP allowlist) | everyone |
| Git deploy key | **read-write** (`github-staging`) - commits made there are pushed | read-only |
| Backups | none - staging data is disposable | nightly, off-box, required |

Both can live on one server or on one each. On one server they share exactly one thing: the
**edge** (`enu-edge`, `docker/edge/`), a single Traefik that owns ports 80/443 and Let's
Encrypt for every stack on the box. Nothing else is shared - not a directory, a network, a
database or a systemd unit.

## The first time

```bash
make preflight   HOST=deploy@203.0.113.42 DOMAIN=example.com REPO=acme/open-enu-go

make deploy-key  HOST=deploy@203.0.113.42 REPO=acme/open-enu-go                 # production, read-only
make deploy-key  HOST=deploy@203.0.113.42 REPO=acme/open-enu-go STAGE=staging   # staging, read-write

make buildstaging HOST=deploy@203.0.113.42 DOMAIN=stg.example.com REPO=acme/open-enu-go EMAIL=ops@example.com
make buildprod    HOST=deploy@203.0.113.42 DOMAIN=example.com     REPO=acme/open-enu-go EMAIL=ops@example.com
```

Point `HOST` at the same server for both, or at two. `buildstaging` allows the address you
run it from (`ALLOW=ip,cidr` to choose); change it later in the staging `.env` and run
`make edge` on the server - no restart. **One server running both needs 4 GB of RAM**: the dev
stack alone runs three Nuxt dev servers.

`preflight` is read-only and takes about ten seconds; every `build*` runs it again as step 0.
Both stages use Let's Encrypt's **production** CA - a staging-CA certificate is untrusted by
browsers, and staging is used in one. While debugging DNS, set `LE_CA_SERVER` to the staging
directory: five failed authorisations an hour lock a hostname out for a week.

## The loop: vibe code, look, promote

**Code anywhere; production only ever gets what staging ran.**

```bash
# on the staging server - the stack hot-reloads as files change
ssh deploy@…  &&  cd /opt/<slug>-staging  &&  claude
git commit -am "…"

# or on your machine, with the local dev stack (make builddev)
git push                       # then pull it into staging:
make deploystaging HOST=deploy@… DOMAIN=stg.example.com

# when staging looks right
make promote                                    # on the staging server, prod on the same box
make promote PROD=deploy@prod-ip                # on the staging server, prod elsewhere
make promote STAGING=deploy@stg [PROD=deploy@…] # from your machine
```

`promote` refuses a dirty staging checkout (a release is commits, not a working tree), refuses
when origin has commits staging has not run, pushes staging's own commits, runs
`make promote-gate` (arch, tests, typecheck) **on staging**, then moves production to that
exact SHA and releases it: build → back up → migrate → restart → health. The build is
`docker compose build --pull` of four images; the migration is the freshly built API image run
once as `openenu migrate`, so the schema that goes live was produced by the binary that will
serve it. It tags
`prod-<timestamp>`, so "what was live when" is a `git tag` away. Code only - staging's
database never goes near production.

`deploystaging` fast-forwards staging and never overwrites uncommitted or unpushed work there.
Running `promote` from the staging server with production on **another** server means that
server holds an SSH key to production - a real widening of what a compromised staging box
reaches. Promoting from your machine avoids it.

## Subsequent releases without staging

```bash
make deployprod HOST=deploy@203.0.113.42 DOMAIN=example.com REF=v1.4.0
```

Runs `make ci` locally first and refuses if it is red, then releases that ref with the same
`scripts/remote/release.sh` promote uses. `SKIP_CI=1` exists and is a footgun.

**Not zero-downtime.** `docker compose up -d` recreates changed containers and a
single-replica service is unavailable for the seconds that takes. Two replicas behind
Traefik with health-gated rotation is the documented path to changing that, and is
deliberately not in v1.

## What each command refuses to do

| It will not | Because |
|---|---|
| Change a DNS record | A `buildprod` that quietly changes a live zone is not a tool anyone should trust near production. `dns-check` writes the fixes; `make dns-apply` applies them, with a token you supplied |
| Register a deploy key as a side effect of a build | Same principle. `make deploy-key` is a separate, deliberate act |
| Harden sshd before proving you can still log in | The order is: create the user → install the key → **open a second connection and verify** → only then touch `sshd_config`. If the verification fails, the file is never edited |
| Overwrite a secret | `.env` is created once and read forever after. Regenerating `APP_SECRET` invalidates every session; regenerating `APP_ENCRYPTION_KEY` makes every encrypted column unreadable, permanently |
| Expose staging to everyone | It is the dev stack: Mailpit shows every mail sent and Vite serves source files. `edge.sh` refuses to register it with an empty `STAGING_ALLOW_FROM`, and `prod-check` fails if a staging router lacks the allowlist or the stack publishes a port |
| Overwrite work on staging | `deploystaging`, `promote` and a re-run `buildstaging` all refuse to touch uncommitted or unpushed changes in the staging checkout |
| Call `buildprod` finished without an off-box backup | The stack comes up, then the command exits non-zero naming what is missing. With `BACKUP_REMOTE` and `BACKUP_AGE_PUBKEY` set it takes a real backup and looks for it on the remote - set is not the same as working. A `pg_dump` on the same disk as the database is a hope, not a backup |
| Migrate without dumping first | Backup, then migrate, in that order, every time |
| Start production on Mailpit | `compose.prod.yml` refuses to render without `MAILER_DSN`, and `buildprod` checks the server's `.env` for a real provider before it builds - a stack that silently drops verification mail is worse than one that fails |

## Preflight, check by check

Ten checks (`scripts/remote/preflight.sh`). The one that matters is **10: can the *server* read the repo.**
Checks 8 and 9 pass on your laptop because your laptop holds your GitHub credentials - the
server does not, and that is exactly where the first `git clone` fails.

When DNS is wrong, `dns-check` writes `.out/dns/${DOMAIN}/`:

- **`zone.txt`** - the complete desired record set as a BIND file. Import it with "overwrite
  existing" and the zone converges in one action. Cloudflare: DNS → Records → Import and
  Export → Import.
- **`cloudflare.sh`** - only the deltas, created or patched by id, safe to re-run. Needs
  `CF_API_TOKEN` with `Zone:DNS:Edit` on that one zone.
- **`records.json`** - the same desired state for Terraform, Route 53 or your own tooling.

**Leave Cloudflare's proxy off.** It breaks Mercure SSE and hides the origin from HTTP-01
validation. Preflight warns when `api.` (which serves Mercure) resolves into a Cloudflare range.

## No git remote yet?

```bash
SYNC=rsync make buildstaging HOST=deploy@… DOMAIN=stg.example.com
```

Pushes your working tree instead of cloning - including anything uncommitted, which is why
it is not the default. `deploy*` refuses to run against a server provisioned this way,
because there is no ref to move to.

## Backups

Nightly at 03:17 (plus up to fifteen minutes of jitter, so the whole internet does not hit
the same bucket at 03:00): `pg_dump -Fc` → `age --encrypt` → `rclone copy` to
`BACKUP_REMOTE`. The `.env` goes up as a second encrypted bundle, because a database alone
does not bring a dead server back - without `APP_SECRET` every session breaks, and without
`APP_ENCRYPTION_KEY` every encrypted column is lost. Both live only in that file.

**The private half of the age key is deliberately not on the server.** A backup that an
attacker holding the server can decrypt protects against disk failure and nothing else.

**Setting it up.** `buildprod` installs `age` and `rclone` on the server; the rest is three
things only you can provide:

1. A key pair, made on your machine: `age-keygen -o ~/.config/age/<project>.txt`. The
   public half (`age1...`) goes in `.env` on the server as `BACKUP_AGE_PUBKEY`.
2. An rclone remote for the **deploy user** (the nightly unit runs as it):
   `ssh deploy@… rclone config`. Give it the narrowest access the storage offers - the token
   sits on the server, and whoever takes the server takes the token.
3. `BACKUP_REMOTE=<remote>:<path>` in `.env`, then `buildprod` again. It takes a backup
   and fails unless the encrypted dump is listed on the remote.

A backup run that cannot reach the remote, or would send a dump in the clear, exits 1: the
nightly unit shows failed instead of logging a warning nobody reads. Seven days of nightly
dumps and secrets bundles are kept on the remote; pre-deploy and manual dumps are not pruned.

```bash
make backup-verify HOST=deploy@…    # pulls the latest REMOTE dump, restores it into a
                                    # scratch container, checks tenant, identity_user and the goose ledgers
make restore HOST=deploy@…          # destructive; asks for the database name, and takes a
                                    # safety dump of the current state first
```

`backup-verify` reads the remote object, not a local copy - the remote one is what exists
after the disk dies.

## A project's own services and routes

`docker/project/` is the project's, and the platform ships nothing there, so an update never
touches it. Next to every compose file the stage uses goes the project's file of the same
name, when there is one - `docker/project/compose.prod.yml` after `compose.prod.yml`,
`docker/project/compose.dev.yml` after `compose.dev.yml` - through `scripts/lib/compose.sh`
and the `Makefile`. Write a router there as the platform's file next to it does: `entrypoints=websecure` and `tls.certresolver=letsencrypt` in production, `tls=true` in
dev, and on staging its `${STACK}-allow@file,noindex@file` middlewares in
`docker/project/compose.staging.yml`. `prod-check` holds them to the same rules as the
platform's routers. A bare `docker compose` does not read these files; `make` does.

## When it goes wrong

| Symptom | Look at |
|---|---|
| Provision stops at DNS | `.out/dns/${DOMAIN}/` - the fixes are already written |
| `$HOST cannot read $REPO` | `make deploy-key HOST=… REPO=…` (add `STAGE=staging` for the staging checkout) |
| Staging answers 403 | Your address is not in `STAGING_ALLOW_FROM` - edit the staging `.env`, then `make edge` on the server |
| `promote` says staging is behind origin | `make deploystaging` first: production only gets what staging has run |
| `promote` cannot push | Staging's deploy key is read-only - `make deploy-key … STAGE=staging` and tick write access |
| Certificates never issue | Port 80 reachable? CAA record naming another CA? Cloudflare proxy on? All three are preflight checks |
| Stack up, health gate red | `ssh $HOST 'cd /opt/<slug>-prod && docker compose logs --tail=80 api; docker logs --tail=80 enu-edge'`. The api container's own healthcheck is `openenu health`: the binary asks itself, because distroless has no curl |
| Deploy failed mid-migration | Nothing was restarted; the old containers are still serving. The pre-deploy dump is in `/opt/<slug>-prod/backups`; the rollback command is printed |
| Need the database locally | `make tunnel HOST=… [STAGE=staging]` → `postgres://…@127.0.0.1:15432/…` for one session |
| Gone after a reboot | `systemctl status <slug>-prod.service` / `<slug>-staging.service` - the reason a kernel update at 4am is not an outage |

## What is checked, and what is not

`make arch` runs `prod-check`, which asserts the production stack's invariants against the
**rendered** compose config: no application source bind-mounted (the image is the binary),
no published database or cache port, every third-party image pinned, every public router
resolving a certificate, no development services, no container running air, and `api`,
`worker` and `scheduler` built from the Dockerfile's `prod` target (the `dev` target carries a
toolchain and expects a source mount) - and the **staging** stack's: no published port, every
router behind the allowlist, and no script that names a compose file itself (which would
drop the staging override on the server). The project's files in `docker/project/` are
rendered in, so its routers answer to the same rules. `make selftest` proves each of those
fails when broken, a project's production and staging router included.

`shell-check` runs ShellCheck over every script at `-S warning`, because the shell here is
what runs against real servers with root.

The production API image builds with `docker build --target prod`, and `openenu health` and
`openenu version` run inside it; `compose.prod.yml` renders on every `prod-check`.

**Not covered by any automated test in this repository:** the scripts have never been run
end to end against a real VPS - `provision`, `release`, `deploy` and `promote` included.
Their logic is linted and their refusals are tested; the provisioning path itself is
verified the first time you point it at a server.

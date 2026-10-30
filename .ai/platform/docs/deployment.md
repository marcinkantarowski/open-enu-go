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

On a bare box only `root@` exists. `build*` creates the user named in `HOST` first (below,
*The deploy user and the host*); to run `preflight` or `deploy-key` before that, create it
yourself: `bash scripts/remote/deploy-user.sh deploy@203.0.113.42`. Production refuses
`HOST=root@…` (or a bare address): the stack, the backups and every unit would run as root.

`preflight` is read-only and takes about ten seconds; every `build*` runs it again as step 0.
Both stages use Let's Encrypt's **production** CA - a staging-CA certificate is untrusted by
browsers, and staging is used in one. While debugging DNS, set `LE_CA_SERVER` to the staging
directory: five failed authorisations an hour lock a hostname out for a week.

## The deploy user and the host

`scripts/remote/deploy-user.sh` (run by `build*` before preflight; the only change made
before it, and it touches nothing but the user):

1. logs in as `root@` once, creates the user with your root `authorized_keys`, no password;
2. grants passwordless sudo through `/etc/sudoers.d/90-<user>`, checked with `visudo -cf`
   before it is put in place;
3. adds it to the `docker` group. **The docker group is root-equivalent** - any member can
   mount `/` into a container. It is granted because every stack command runs as this user,
   not because it is a lesser privilege than sudo;
4. proves a **second, new login** as the user, with `sudo -n` working. Only after that does
   provisioning touch `sshd_config`.

Then the host, step by step - each one prints its own result, and a failed one is listed at
the end instead of hiding behind one green line:

| Step | What |
|---|---|
| sshd | password and keyboard-interactive auth off, root key-only, in `sshd_config` **and** a drop-in `sshd_config.d/00-enu-hardening.conf` (Ubuntu's `50-cloud-init.conf` turns password auth back on otherwise); checked with `sshd -T`, then a new login is proved again |
| ufw | 80 and 443; 22 from `SSH_ALLOW_FROM` (comma-separated IPs or CIDRs, kept in the stage's `.env`), or from anywhere with a warning when it is empty. A list that does not contain the address the run comes from is refused before anything changes; after applying it, a new login is proved |
| fail2ban | the `sshd` jail: 5 failures in 10 minutes, banned for an hour; your address and `SSH_ALLOW_FROM` are never banned |
| unattended-upgrades | daily, **security origins only** (`apt.conf.d/52enu-security-only`), never a reboot; Docker's own packages are not touched |
| time | `timedatectl show -p NTPSynchronized`; NTP is switched on and a warning printed when it is not synchronised |
| docker | log rotation (`10m` x 3); the daemon restarts only when `daemon.json` actually changed |
| swap | 2 GB under 2 GB of RAM |

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

**Release tags.** `release.sh` exports `RELEASE` (the commit, `git rev-parse --short=12`)
and every image is built as `<stack>-<service>:<RELEASE>`; the release that serves is also
tagged `latest`, which is what the boot unit, a restore and a hand-typed `docker compose`
start. `.out/releases` lists the releases that served, newest last; the last three keep
their images and older tags are removed. A rollback is therefore a restart, not a build:

```bash
cd /opt/<slug>-prod && git checkout --detach <previous commit> && bash scripts/remote/release.sh --rollback
```

`--rollback` builds nothing, dumps nothing and **migrates nothing** - older code does not
undo a migration; the pre-deploy dump does. It refuses when that commit's images are no
longer on the box (older than three releases): then a normal `release.sh` rebuilds it.

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

**The tag the images carry.** `compose.prod.yml` tags every image it builds
`${STACK}-<service>:${RELEASE:-latest}`. `scripts/remote/release.sh` exports `RELEASE` (the
commit it deploys), so each release is an image of its own and a rollback is a tag. `latest`
is only what a hand-run `docker compose` on the server falls back to.

## What the production stack asks of the box

Every container has a memory ceiling, so one leak is that container restarting, not the
kernel's OOM killer choosing Postgres:

| Service | Limit | Note |
|---|---|---|
| postgres | 1 GiB | plus `shm_size: 256m` - Docker's 64 MB `/dev/shm` fails a parallel query |
| redis | 384 MiB | `maxmemory 256mb` of data; the rest is the process and the AOF rewrite's fork |
| mercure | 128 MiB | |
| api | 512 MiB | |
| worker | 384 MiB | |
| scheduler | 256 MiB | |
| frontend, manager, landing | 384 MiB each | |
| **total** | **3840 MiB** | ceilings, not reservations; the edge (Traefik and its socket proxy) has none |

Preflight warns below **1900 MB** of RAM and provisioning adds a swapfile there. That is
enough to run the stack, because a ceiling is not a reservation and an idle stack uses well
under half of it; it is not enough for every container at its ceiling at once, which 3840
MiB plus the edge and the OS (about **4.5 GB**) is. Size the box from that if the load is
real, and remember that staging on the same server is the dev stack on top (4 GB is the
floor for both together, above).

**Every service reports its health.** `api` probes itself (`openenu health`); `worker` and
`scheduler` serve no port, so they touch `HEARTBEAT_FILE` (`/tmp/openenu-heartbeat`) every 30
seconds and `openenu health -heartbeat` fails once it is two minutes old - a process that is
up and stuck reads as unhealthy. Mercure's check reads its own page, the Nuxt apps' their
`/`. `prod-check` refuses a service without one.

**Readiness from outside needs the token.** `/health` is public and is what Traefik polls;
`/health/deep` pings the database and answers only a request carrying
`X-Health-Token: <HEALTH_TOKEN from the server's .env>` (any other caller gets a 404). The
release's health gate and provisioning's final check send it; so must an external uptime
monitor (UptimeRobot, Better Stack, a cron `curl`) - configure the header there, and keep
the monitor's copy in step if the token is ever changed by hand
([`observability.md`](observability.md)).

**Two Mercure keys.** `MERCURE_PUBLISHER_JWT_SECRET` signs what the backend publishes;
`MERCURE_SUBSCRIBER_JWT_SECRET` signs the cookie every browser holds. One key for both would
let anybody who extracted the subscriber key publish to every tenant. `make env` generates
each; on a server whose `.env` predates them it adds both (`MERCURE_JWT_SECRET` there is no
longer read and can be deleted). `prod-check` refuses one secret for both.

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
| Migrate without dumping first | Backup, then migrate, in that order, every time. Only a dump taken **by this release** counts: `release.sh` passes its own stamp (`BACKUP_STAMP`) and looks for that file, so an older dump left in `backups/` can never stand in for a failed one. Exit 2 from `backup.sh` (no dump) stops the release; 3 (on this disk only) and 4 (no storage archive) let it continue with a warning |
| Run production as root | `HOST` must name a deploy user; `build*` creates it from `root@` with sudo and proves the login |
| Restrict SSH to a list that leaves you out | `SSH_ALLOW_FROM` must contain the address the run comes from, or nothing is changed |
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
the same bucket at 03:00). Every run, one stamp, three files:

| File | What |
|---|---|
| `<db>-<stamp>-<label>.dump.age` | `pg_dump -Fc` of the database |
| `<db>-<stamp>-<label>-storage.tar.age` | the `storage` volume - attachments, scanned agreements, kept PDFs. There is no S3 backend, so this is the only other copy of every upload. The volume is found by compose's labels on it (`com.docker.compose.project=$STACK`, `volume=storage`), archived read-only with `tar --numeric-owner` in the image the database already runs |
| `<db>-<stamp>-secrets.tar.age` | `.env`: without `APP_SECRET` every session breaks, without `APP_ENCRYPTION_KEY` every encrypted column is lost |

Each is `age --encrypt`ed and `rclone copy`'d to `BACKUP_REMOTE`. Written under `umask 077`
into a `0700` `backups/`. **The secrets bundle exists only encrypted**: with no
`BACKUP_AGE_PUBKEY` (or no `age`) it is not written at all, with a warning - a copy of the
encryption key beside the data it protects is not a backup.

**The private half of the age key is deliberately not on the server.** A backup that an
attacker holding the server can decrypt protects against disk failure and nothing else.

**Exit codes** - a contract, read by `release.sh`, `restore.sh` and the systemd unit
(`scripts/remote/backup.sh` header):

| Exit | Meaning | A deploy |
|---|---|---|
| 0 | complete: dumped, archived, off the box (or no `BACKUP_REMOTE`, warned) | proceeds |
| 1 | could not start (no `.env`, a malformed stamp or label) | refuses |
| 2 | **no dump**: postgres down, `pg_dump` failed, empty dump | refuses |
| 3 | the dump is good, on **this disk only** - the copy off the box failed or was refused | proceeds, warned |
| 4 | the dump is good, the storage volume could not be archived | proceeds, warned |

**Setting it up.** `buildprod` installs `age` and `rclone` on the server; the rest is three
things only you can provide:

1. A key pair, made on your machine: `age-keygen -o ~/.config/age/<project>.txt`. The
   public half (`age1...`) goes in `.env` on the server as `BACKUP_AGE_PUBKEY`.
2. An rclone remote for the **deploy user** (the nightly unit runs as it):
   `ssh deploy@… rclone config`. Give it the narrowest access the storage offers - the token
   sits on the server, and whoever takes the server takes the token.
3. `BACKUP_REMOTE=<remote>:<path>` in `.env`, then `buildprod` again. It takes a backup
   and fails unless the encrypted dump is listed on the remote.

Seven days of nightly dumps, storage archives and secrets bundles are kept on the remote;
pre-deploy, pre-restore and manual runs are not pruned. Local copies are kept two days.

**When the nightly fails, someone hears.** Every non-zero exit fails `<stack>-backup.service`,
and `OnFailure=` starts `<stack>-notify@.service` (the stack unit has it too), which runs
`scripts/remote/notify.sh` - installed root-owned as `/usr/local/lib/<stack>-notify`, since
it runs as root and the checkout is the deploy user's: the journal at priority `err` always
(`journalctl -p err`), mail to `ALERT_EMAIL` (else `LETSENCRYPT_EMAIL`) through the host's
`sendmail` or `mail` when one is installed, otherwise `wall`. The API binary has no mail
sub-command, and the alarm must not depend on the stack that may be what failed - so for
mail, install an MTA pointed at your relay (`apt-get install msmtp-mta`, then
`/etc/msmtprc`); `install-units.sh` warns when there is none.

```bash
make backup-verify HOST=deploy@…    # the latest REMOTE dump and its storage archive, restored
                                    # into a scratch container and read as the application role
make restore HOST=deploy@…          # destructive; asks for the database name, and takes a
                                    # safety backup of the current state first
```

`backup-verify` reads the remote objects, not local copies - the remote ones are what exist
after the disk dies. It fails when the dump has no storage archive next to it, lists the
archive in full, checks the tables and the goose ledgers, and then - the check that matters -
creates the application role `NOSUPERUSER NOBYPASSRLS`, proves it is **refused** before
`docker/postgres/init/grant-app.psql` runs and **reads** after it. A superuser reads every
restored table whether or not the grants came back, so a check made only as one cannot see
the failure a restore actually has.

**What `restore` does, in order:** a `pre-restore` backup of the current state; stop `api`,
`worker` and `scheduler` (the only writers; postgres stays up); drop and create the
database, `pg_restore --no-owner --no-privileges`; apply `grant-app.psql` again - the
application role's grants are per database and went with the drop, and without them every
API query answers "permission denied for table"; empty the storage volume and unpack the
archive of the **same run** into it (if the dump has none, it says so before you confirm and
leaves the uploads as they are); `openenu migrate`, since an older dump may predate the
running schema; `stack.sh up`. The role itself survives a restore: it is cluster-wide, made
once by `docker/postgres/init/01-init.sh`.

**A dead server.** Decrypt the secrets bundle on your machine
(`age -d -i <key> <db>-<stamp>-secrets.tar.age | tar -xO .env`), create the deploy user and
`/opt/<slug>-prod` on the new box, put that `.env` there (mode 600), and only then run
`buildprod`: it keeps an existing `.env`, so the database volume it creates gets the old
`DB_APP_PASSWORD` and every secret is the old one. Then
`make restore HOST=… FILE=<db>-<stamp>-<label>.dump.age`.

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
| A release is live and wrong | `git checkout --detach <previous> && bash scripts/remote/release.sh --rollback` - the previous release's images are still on the box (the last three are kept, `.out/releases`); no migration is undone |
| "pre-deploy dump failed (backup.sh exit 2)" | No dump was written - usually a full disk (`df -h`) or postgres down. `bash scripts/remote/backup.sh manual` says which |
| The nightly backup failed | `journalctl -u <slug>-prod-backup.service`; `journalctl -p err -t <slug>-prod-notify` for what was sent |
| After a restore, "permission denied for table" | The grants did not come back: `docker compose exec -T postgres psql -U $DB_USER -d $DB_NAME -v app_role=${DB_USER}_app < docker/postgres/init/grant-app.psql` |
| Need the database locally | `make tunnel HOST=… [STAGE=staging]` → `postgres://…@127.0.0.1:15432/…` for one session |
| Gone after a reboot | `systemctl status <slug>-prod.service` / `<slug>-staging.service` - the reason a kernel update at 4am is not an outage |

## What is checked, and what is not

`make arch` runs `prod-check`, which asserts the production stack's invariants against the
**rendered** compose config: no application source bind-mounted (the image is the binary),
no published database or cache port, every third-party image pinned, every public router
resolving a certificate, no development services, no container running air, no
`WEBHOOK_ALLOW_PRIVATE` (the dev stack's allowance for its private webhook receiver, an SSRF
anywhere else), two different Mercure keys, a healthcheck on every service, an edge whose
Traefik holds no Docker socket and a server edge that states its TLS minimum, and `api`,
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

The grant file is proved against a real Postgres by every `backup-verify` (refused before,
readable after). The exit codes of `backup.sh` are not yet asserted by `make selftest`.

**Not covered by any automated test in this repository:** the scripts have never been run
end to end against a real VPS - `provision`, `deploy-user`, `release` (and `--rollback`), `restore`, `deploy` and `promote` included.
Their logic is linted and their refusals are tested; the provisioning path itself is
verified the first time you point it at a server.

# Troubleshooting

Specific failures of the dev stack, with the cause and the fix. Symptoms first - that is
what you have when you arrive here. The first moves are always the same: `make status`,
`make logs-api`, `make smoke` ([`dev-stack.md`](dev-stack.md) → *When something is wrong*).

---

## `make up` stops with "port 443 is held by the container '…'"

**Cause.** Routing belongs to the **edge** - one Traefik, `enu-edge`, shared by every project
on this Docker host (`docker/edge/`, `scripts/dev/edge.sh`). It needs host ports 80 and 443,
and something else already has them: usually another project's own Traefik.

**Fix.** Stop the container the message names, then `make up` again. A Traefik that is not
the edge can be removed for good: `docker rm -f <name>`.

---

## `make up` stops with "port 5432 is held by the container '…'"

**Cause.** Two projects on one Docker host. Names never collide - every container, network,
volume and router carries `${STACK}` - but a *published host port* is shared by the whole
machine, and every checkout defaults to `DB_PORT=5432`. `scripts/dev/check-ports.sh` reads
the published ports from the resolved compose config and runs before every `make up`.

**Fix.** Do what the message says: stop the named container, or give this project its own
port - the message names the `.env` variable and the next free port. Then `make env` and
`make up`. `make ports` runs the check alone and changes nothing. A port held outside Docker
is named too, with the command that finds the process (under WSL2 it may be on the Windows
side).

---

## One project's hosts answer 504, or the wrong project answers

**Cause.** The edge sees every project's labels at once. Two things keep them apart, and
each fails in its own way:

- **The edge is not attached to the project's network** (`${STACK}-net`) → 504. `make up`
  attaches it (`edge.sh attach`); a stack started with a bare `docker compose up` never was.
- **Two projects share a router name** → Traefik drops both routers. Routers are named
  `${STACK}-<service>`; two checkouts with the same `STACK` collide the same way, so give
  each its own name with `make init`.

**Fix.** `make edge && scripts/dev/edge.sh attach`, then `make logs-traefik` for the router
list it loaded.

---

## `docker compose down` warns that the network "has active endpoints"

**Cause.** The edge is still attached to `${STACK}-net`, so Docker refuses to delete the
network. `make down` detaches it first; a bare `docker compose down` does not.

**Fix.** `scripts/dev/edge.sh detach`, then remove the network.

---

## Every host returns 404 from Traefik, but TLS is fine

**Cause.** Traefik cannot reach the Docker daemon, so it discovers no routers and falls
through to its 404. TLS still works because the certificate comes from the file provider,
which is independent - that is why this looks like a routing bug rather than a connection one.

```
make logs-traefik
ERR Failed to retrieve information of the docker client and server host
    error="Error response from daemon: " providerName=docker
```

Two things produce this. If the message is `Cannot connect to the Docker daemon`, the
daemon was restarted (Docker Desktop, a host reboot) and the edge is holding a socket that
no longer exists. `make up` (through `make edge`) detects that and restarts the edge;
`docker restart enu-edge` does the same by hand.

An empty error message is the other tell: the daemon rejected the API version outright.
Docker 29 and later set a minimum API version (`docker version` → `MinAPIVersion`), and
older Traefik builds negotiate below it.

**Fix.** Pin a Traefik release new enough for the daemon. The edge runs the image pinned in
`docker/edge/compose.yml`; `make edge` recreates it when the pin changes.

---

## A change to the backend does not take effect

**Cause.** air rebuilds `api`, `worker` and `scheduler` on every change to a `.go`, `.sql`,
`.json`, `.yaml` or `.toml` file under `backend/` - but when the build fails it keeps the
previous binary running (`stop_on_error = false` in `backend/.air.toml`), so the old
behaviour simply continues.

**Fix.** `make logs-api` (and `make logs-worker` for the other two): the compiler's error is
there, not in the browser. A worker that is up but idle on a job a new module queues is the
same case - its build failed, or it has not restarted yet
([the lesson](../lessons/a-worker-runs-the-code-it-started-with.md)).

---

## A container healthcheck fails while the service is demonstrably serving

**Cause.** `localhost` inside a container usually resolves to `::1` first. A server bound to
IPv4 `0.0.0.0` never sees that connection, and the probe reports "connection refused" for a
perfectly healthy process.

**Fix.** Probe `127.0.0.1`, never `localhost`. Both the Nuxt and API healthchecks do.

---

## The Nuxt apps 502 right after `make deps` or a restart

**Cause.** A Nuxt dev server takes a while to compile on boot. Anything that runs
immediately after a restart races it.

**Fix.** Already handled: the three app services have healthchecks with a `start_period`,
`make wait` blocks on them, and `builddev` is ordered `up → deps → wait → migrate → smoke`.
Never add a `sleep` - wait on the healthcheck.

---

## A new module's page returns 404, though the file is there

**Cause.** A frontend module is a Nuxt layer, and Nuxt reads its list of layers once, at
startup. Editing a file inside an existing layer hot-reloads; adding a layer does not.

**Fix.** `make module` restarts the frontend for you when the stack serves this checkout.
If the directory arrived another way - a branch switch, a pull - restart it by hand:
`make restart`.

---

## Hot reload stops working (the page never updates)

Two independent causes, both silent:

1. **The HMR websocket cannot connect.** It is proxied through the edge over TLS, so the
   client must dial `wss://<host>:443`, not `ws://<host>:3000`. Set by `VITE_HMR_PROTOCOL`,
   `VITE_HMR_HOST` and `VITE_HMR_PORT` in compose, consumed by the ui-kit layer. Check the
   browser console for a failing websocket to `:3000`.
2. **File events do not cross the bind mount.** inotify is unreliable into containers on
   WSL2 and macOS, which is why the ui-kit layer sets `server.watch.usePolling` (and air
   polls too). If reloads stop, confirm polling is still enabled before looking anywhere
   else.

---

## The browser shows a certificate warning (WSL2)

**Cause.** mkcert trusts its CA for the platform it runs on. The Linux `mkcert -install`
trusts it for Linux; the browser is a Windows application reading the Windows store.
Running `mkcert.exe -install` does not help - it would create and trust a *different* CA,
while the certificate is signed by the Linux one.

**Fix.** `make certs` installs the Linux CA into the Windows user store with `certutil.exe`.
If it could not, it prints the exact command. Verify with:

```powershell
certutil -user -verifystore Root <sha1-fingerprint>
```

---

## The browser cannot resolve the hosts, but `curl` can (WSL2)

**Cause.** Two hosts files matter and they serve different clients: Linux `/etc/hosts` for
curl and this repository's scripts, the Windows hosts file for the browser.

**Fix.** `make hosts` writes both, elevating via UAC for the Windows one. If sudo was
declined, the Linux half is skipped and the block is printed for manual entry - the stack
still works in the browser.

---

## `make builddev` asks for a password twice

Expected. `sudo` for `/etc/hosts`, and a Windows UAC prompt for the Windows hosts file.
Both are the `hosts` step. Everything else is non-interactive.

---

## `make check` says Go is not installed, though it is

**Cause.** Go runs on the host, never in a container, and a tarball install is often not on
`PATH`.

**Fix.** Nothing to configure: `scripts/lib/go.sh` also looks in `~/.local/go` and
`/usr/local/go`, and the Makefile and every script use the binary it finds. Install Go 1.27
into one of those, or put it on `PATH`; `make check-tools` lists everything the host needs.

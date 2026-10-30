#!/usr/bin/env bash
# =============================================================================
#  The production AND staging stacks' invariants, checked (.ai/platform/docs/deployment.md).
# =============================================================================
#  Every one of these is a sentence the plan writes about the prod compose file,
#  and a sentence nobody re-reads. They are also all one careless edit from being
#  false - and each would be false SILENTLY, in production, until the day it
#  mattered:
#
#    a bind-mounted source   means the image is no longer what runs
#    an exposed DB port      puts Postgres on the public internet
#    an unpinned image       deploys a different system six months from now
#    a missing cert resolver makes a host answer on 443 with a wrong certificate
#
#  And for staging, which is the DEV stack on a public server:
#
#    a published port        is on the internet - Docker bypasses ufw
#    a router without the    exposes Mailpit (every mail sent) and Vite's dev
#    allowlist               server (source files) to everyone
#    a script that names     silently drops the staging override, and the next
#    compose.dev.yml itself  `up` on the server undoes both of the above
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

FILE="$ROOT/docker/compose.prod.yml"
[ -f "$FILE" ] || { log_skip "no production compose file yet"; exit 0; }

fails=0
bad() { log_fail "$@" || true; fails=$((fails + 1)); }

# with_project <file>... → `-f` for each, followed by the project's file of the
# same name in docker/project/ (scripts/lib/compose.sh). Checked as it runs: a
# project's router is held to the same rules as the platform's.
with_project() {
  local f
  for f in "$@"; do
    printf -- '-f\n%s\n' "$f"
    [ -f "$ROOT/docker/project/${f##*/}" ] && printf -- '-f\n%s\n' "$ROOT/docker/project/${f##*/}"
  done
  return 0
}
mapfile -t PROD_FILES < <(with_project "$FILE")

log_step "Production stack invariants"

# Rendered rather than grepped: compose resolves anchors, extends and variables,
# and a rule applied to the raw YAML checks the text instead of the stack.
# Both stacks read .env and the derived docker/api/project.env; a fresh clone
# has neither until `make env`, and that is not a broken stack.
for derived in .env docker/api/project.env; do
  [ -f "$ROOT/$derived" ] || { log_fail "$derived is missing - the stacks cannot be rendered without it" "run: make env"; exit 1; }
done
CONFIG="$(docker compose --project-directory "$ROOT" --env-file "$ROOT/.env" "${PROD_FILES[@]}" config 2>/dev/null)" \
  || { log_fail "compose.prod.yml does not parse" "run: docker compose -f docker/compose.prod.yml config"; exit 1; }

# ── 1. no source bind-mounts ────────────────────────────────────────────────
# The Postgres init scripts are mounted on purpose; the application tree is
# not. A bind mount over the binary's tree means the image is no longer what
# runs.
if grep -E '^\s+source: .*/(backend|frontend|manager|landing|ui-kit)' <<<"$CONFIG" | grep -q .; then
  bad "the production stack bind-mounts application source" \
      "images are immutable in production - that is what makes a rollback a tag change" \
      "$(grep -E '^\s+source: .*/(backend|frontend|manager|landing|ui-kit)' <<<"$CONFIG" | head -3)"
else
  log_ok "no application source is bind-mounted"
fi

# ── 2. no published database or cache port ──────────────────────────────────
published_ports() { # <service> -> prints its published ports, if any
  SERVICE="$1" python3 -c '
import os, re, sys
service = os.environ["SERVICE"]
config = sys.stdin.read()
block = re.search(rf"^  {service}:\n(.*?)(?=^  \S|\Z)", config, re.S | re.M)
if block:
    for line in re.findall(r"published:\s*\"?(\d+)", block.group(1)):
        print(line)
' <<<"$CONFIG"
}

for service in postgres redis; do
  ports="$(published_ports "$service")"
  if [ -n "$ports" ]; then
    bad "$service publishes port(s) $(tr '\n' ' ' <<<"$ports") in production" \
        "in dev it is exposed for a GUI client; here the only way in is \`make tunnel\`"
  else
    log_ok "$service publishes no port"
  fi
done

# ── 3. every image pinned ───────────────────────────────────────────────────
unpinned="$(grep -E '^\s+image: ' <<<"$CONFIG" | grep -vE ':[A-Za-z0-9][A-Za-z0-9._-]*$' || true)"
if [ -n "$unpinned" ]; then
  bad "an image is not pinned to a tag" \
      "\`latest\` means a pull in six months deploys a system nobody tested" \
      "$unpinned"
elif grep -E '^\s+image: .*:latest$' <<<"$CONFIG" | grep -qv "${STACK:-open-enu-go}-"; then
  bad "a third-party image is pinned to :latest" "pin a version"
else
  log_ok "every third-party image is pinned"
fi

# ── 4. everything public terminates TLS with a real resolver ────────────────
# Routers are DISCOVERED, not listed. They are named <slug>-<service>, and a
# fixed list of names meets none of them - the check would skip every router
# and report success. Finding no routers at all is a failure for the same
# reason: a check with nothing to look at has not passed.
mapfile -t routers < <(grep -oE 'traefik\.http\.routers\.[A-Za-z0-9_-]+\.rule' <<<"$CONFIG" \
                        | sed -E 's/^traefik\.http\.routers\.//; s/\.rule$//' | sort -u)
missing=0
if [ "${#routers[@]}" -eq 0 ]; then
  bad "no Traefik routers found in the production stack" "nothing is public, or the labels changed shape"
  missing=1
fi
for router in "${routers[@]}"; do
  grep -qF "traefik.http.routers.$router.tls.certresolver" <<<"$CONFIG" \
    || { bad "router $router has no certificate resolver" "it would answer on 443 with the wrong certificate"; missing=1; }
done
[ "$missing" -eq 0 ] && log_ok "every public router resolves a certificate (${#routers[@]})"

# ── 5. no dev-only services leaked in ───────────────────────────────────────
for service in mailpit webhook-echo e2e; do
  grep -qE "^  $service:" <<<"$CONFIG" \
    && bad "$service is in the production stack" "it is test or development infrastructure"
done
grep -q 'dashboard: true' "$ROOT/docker/edge/traefik.server.yml" 2>/dev/null \
  && bad "the Traefik dashboard is enabled on the server edge" "it is an unauthenticated map of every route"
[ "$fails" -eq 0 ] && log_ok "no development services"

# ── the edge: Traefik faces the internet, so it holds no Docker socket ──────
EDGE_CONFIG="$(EDGE_HOME=/nonexistent docker compose -f "$ROOT/docker/edge/compose.yml" config 2>/dev/null)" \
  || { bad "docker/edge/compose.yml does not parse" "run: EDGE_HOME=/tmp docker compose -f docker/edge/compose.yml config"; EDGE_CONFIG=""; }
edge_socket="$(python3 -c '
import re, sys
c = sys.stdin.read()
block = re.search(r"^  traefik:\n(.*?)(?=^  \S|^\S|\Z)", c, re.S | re.M)
print("missing" if not block else "socket" if "docker.sock" in block.group(1) else "clean")
' <<<"$EDGE_CONFIG")"
case "$edge_socket" in
  clean)  log_ok "Traefik reaches Docker through the socket proxy, not the socket" ;;
  socket) bad "the edge's Traefik mounts the Docker socket" \
              "the socket is root on the host, read-only or not; mount it into socket-proxy only" ;;
  *)      bad "no traefik service in docker/edge/compose.yml" "the edge changed shape" ;;
esac
grep -qE '^\s+minVersion: VersionTLS1[23]\s*$' "$ROOT/docker/edge/middlewares.yml" 2>/dev/null \
  && log_ok "the server edge refuses TLS older than 1.2" \
  || bad "the server edge does not state a TLS minimum" \
         "tls.options.default.minVersion: VersionTLS12 in docker/edge/middlewares.yml"

# ── 6. nothing in production loosens what dev loosens on purpose ───────────
# The dev stack lets a webhook reach a private address, because its receiver
# lives on the stack's own network. In production that is an SSRF into the
# database, Redis and the cloud's metadata endpoint.
if grep -qE '^\s+WEBHOOK_ALLOW_PRIVATE:' <<<"$CONFIG"; then
  bad "WEBHOOK_ALLOW_PRIVATE is set in the production stack" \
      "it lets any tenant's webhook reach the private network - Postgres, Redis, the metadata service" \
      "it belongs to compose.dev.yml only"
else
  log_ok "webhooks may not reach a private address"
fi

# make e2e raises the operator's login limit; a server never does.
if grep -qE '^\s+MANAGER_LOGIN_BURST:' <<<"$CONFIG"; then
  bad "MANAGER_LOGIN_BURST is set in the production stack" \
      "it lets anyone guess an operator's password faster, and that account reaches every tenant" \
      "it belongs to make e2e only"
else
  log_ok "the operator's login keeps its limit"
fi

# ── 7. Mercure's two keys are two keys ──────────────────────────────────────
# Compared rendered, so two variables holding one value are caught too: the
# subscriber key signs a cookie every browser holds, and must not publish.
mercure_keys="$(python3 -c '
import re, sys
c = sys.stdin.read()
pub = re.search(r"^\s+MERCURE_PUBLISHER_JWT_KEY:\s*(.*)$", c, re.M)
sub = re.search(r"^\s+MERCURE_SUBSCRIBER_JWT_KEY:\s*(.*)$", c, re.M)
print("missing" if not (pub and sub and pub.group(1).strip() and sub.group(1).strip())
      else "same" if pub.group(1).strip() == sub.group(1).strip() else "distinct")
' <<<"$CONFIG")"
case "$mercure_keys" in
  distinct) log_ok "Mercure publishes and subscribes with two different keys" ;;
  same)     bad "Mercure's publisher and subscriber keys are the same secret" \
                "the subscriber key signs every browser's cookie - with one key, that cookie's signer can publish" \
                "MERCURE_PUBLISHER_JWT_SECRET and MERCURE_SUBSCRIBER_JWT_SECRET in .env" ;;
  *)        bad "Mercure has no publisher or subscriber key in the production stack" "set both in .env: make env" ;;
esac

# ── 8. every service says whether it is well ────────────────────────────────
# Without a healthcheck a process that is up and stuck - a worker whose loop
# hangs - is "running" forever, and nothing restarts or reports it.
mapfile -t unchecked < <(python3 -c '
import re, sys
c = sys.stdin.read()
services = re.search(r"^services:\n(.*?)(?=^\S|\Z)", c, re.S | re.M)
for m in re.finditer(r"^  (\S+):\n(.*?)(?=^  \S|\Z)", services.group(1) if services else "", re.S | re.M):
    if not re.search(r"^    healthcheck:", m.group(2), re.M):
        print(m.group(1))
' <<<"$CONFIG")
if [ "${#unchecked[@]}" -gt 0 ]; then
  bad "a production service has no healthcheck: ${unchecked[*]}" \
      "a stuck process then looks running forever; the worker and scheduler use openenu health -heartbeat"
else
  log_ok "every production service has a healthcheck"
fi

# ═══ staging: the dev stack, on a public server ═════════════════════════════
log_step "Staging stack invariants"
SFILE="$ROOT/docker/compose.staging.yml"
[ -f "$SFILE" ] || { bad "docker/compose.staging.yml is missing" "staging would run the dev stack unguarded"; }
mapfile -t STAGING_FILES < <(with_project "$ROOT/docker/compose.dev.yml" "$SFILE")
SCONFIG="$(STACK=check-staging docker compose --project-directory "$ROOT" --env-file "$ROOT/.env" \
            "${STAGING_FILES[@]}" --profile e2e config 2>/dev/null)" \
  || { log_fail "compose.dev.yml + compose.staging.yml do not parse" || true; exit 1; }

# ── 9. no published port ────────────────────────────────────────────────────
published="$(grep -E '^\s+published: ' <<<"$SCONFIG" || true)"
if [ -n "$published" ]; then
  bad "the staging stack publishes a port" \
      "Docker writes its own iptables rules - ufw does not protect it; the edge is the only way in" \
      "$(head -3 <<<"$published")"
else
  log_ok "staging publishes no port"
fi

# ── 10. every router behind the allowlist ────────────────────────────────────
mapfile -t srouters < <(grep -oE 'traefik\.http\.routers\.[A-Za-z0-9_-]+\.rule' <<<"$SCONFIG" \
                         | sed -E 's/^traefik\.http\.routers\.//; s/\.rule$//' | sort -u)
unguarded=0
[ "${#srouters[@]}" -gt 0 ] || { bad "no routers found in the staging stack" "the labels changed shape"; unguarded=1; }
for router in "${srouters[@]}"; do
  grep -E "traefik\.http\.routers\.$router\.middlewares: .*check-staging-allow@file" <<<"$SCONFIG" >/dev/null \
    || { bad "staging router $router is not behind the allowlist" \
             "add it to docker/compose.staging.yml (a project's router: docker/project/compose.staging.yml): \${STACK}-allow@file,noindex@file"; unguarded=1; }
done
[ "$unguarded" -eq 0 ] && log_ok "every staging router is behind the allowlist (${#srouters[@]})"

# ── 11. nothing drops the override ───────────────────────────────────────────
# Code only: a comment explaining the rule names the pattern too.
hardcoded="$(grep -rnE -- "-f [\"']?[^ ]*compose\.[a-z]+\.yml" "$ROOT/scripts" "$ROOT/Makefile" 2>/dev/null \
             | grep -vE '^[^:]+:[0-9]+:[[:space:]]*#' | grep -vE '(check-prod-compose|selftest)\.sh:' | cut -d: -f1,2 || true)"
if [ -n "$hardcoded" ]; then
  bad "a script names a compose file itself instead of using scripts/lib/compose.sh" \
      "on a staging server that drops compose.staging.yml - the next \`up\` publishes Postgres" \
      "$(sed "s|$ROOT/||" <<<"$hardcoded")"
else
  log_ok "every script reads COMPOSE_FILE"
fi

# ── the binary and the image it runs from ───────────────────────────────────
# In development the binary runs under air, which rebuilds it from the
# bind-mounted source. In production the image IS the binary: nothing there
# may run air, and every backend container must be built from the `prod`
# target - the `dev` target carries a Go toolchain and expects a source mount.
log_step "Backend image invariants"

if grep -qE '^\s+-\s+air$|command:.*\bair\b' <<<"$CONFIG"; then
  bad "a production container runs air" \
      "air rebuilds from a source tree the image does not have; the binary is the deliverable"
else
  log_ok "no production container runs air"
fi

backend_targets="$(python3 -c '
import re, sys
config = sys.stdin.read()
for svc in ("api", "worker", "scheduler"):
    block = re.search(rf"^  {svc}:\n(.*?)(?=^  \S|\Z)", config, re.S | re.M)
    # the build block only: the long volume syntax has a target: of its own
    build = re.search(r"^    build:\n((?:      .*\n)*)", block.group(1), re.M) if block else None
    target = re.search(r"^\s+target:\s*(\S+)", build.group(1), re.M) if build else None
    print(svc, target.group(1) if target else "none")
' <<<"$CONFIG")"
if grep -vq ' prod$' <<<"$backend_targets"; then
  bad "a backend container is not built from the prod target" "$backend_targets"
else
  log_ok "api, worker and scheduler are built from the prod target"
fi

echo
[ "$fails" -eq 0 ] && { log_ok "the production and staging stacks hold every invariant"; exit 0; }
log_fail "$fails problem(s) above"
exit 1

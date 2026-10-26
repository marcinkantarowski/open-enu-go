#!/usr/bin/env bash
# Block until the stack is actually usable. Compose's `up -d` returns when
# containers are *created*, not when they serve - every step after this one
# would otherwise race it.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"
. "$ROOT/scripts/lib/compose.sh"; compose_init "$ROOT"
CO=("${COMPOSE[@]}")

# wait_healthy <service> <seconds>
wait_healthy() {
  local svc="$1" limit="${2:-90}" i=0 cid state
  cid="$("${CO[@]}" ps -q "$svc" 2>/dev/null)"
  [ -n "$cid" ] || { log_fail "$svc has no container" "run: make up"; return 1; }
  while [ "$i" -lt "$limit" ]; do
    state="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$cid" 2>/dev/null)"
    case "$state" in
      healthy|running) log_ok "$svc ($state)"; return 0 ;;
      # `unhealthy` is NOT terminal: a container restarted a moment ago reports
      # it until its first probe succeeds. Only the timeout below is terminal.
      exited|dead)     log_fail "$svc exited" "inspect: $(printf '%s ' "${CO[@]}") logs $svc"; return 1 ;;
    esac
    sleep 1; i=$((i + 1))
  done
  log_fail "$svc did not become healthy within ${limit}s (last state: ${state:-unknown})" \
    "inspect: $(printf '%s ' "${CO[@]}") logs $svc"
  return 1
}

# wait_serving <service> <path> <expected-body-fragment> <seconds>
#
# Docker's health status is a verdict on the PAST: a container turns unhealthy
# only after `retries` failed probes in a row (twelve, two minutes, for the
# API). Under air the container is "up" while the first build is still
# compiling, and a build error leaves the previous binary answering. So ask it
# now, from inside the container (no DNS, no edge), and read the body, not
# only the status. 127.0.0.1, not localhost: the Go server binds IPv4.
wait_serving() {
  local svc="$1" path="$2" want="$3" limit="${4:-60}" i=0 body=""
  while [ "$i" -lt "$limit" ]; do
    body="$("${CO[@]}" exec -T "$svc" wget -qO- -T 5 "http://127.0.0.1:8080$path" 2>/dev/null)"
    case "$body" in *"$want"*) log_ok "$svc answers $path"; return 0 ;; esac
    sleep 1; i=$((i + 1))
  done
  log_fail "$svc does not answer $path with $want after ${limit}s - it is up but not serving" \
    "last answer: $(printf '%s' "${body:-nothing}" | tr -s '[:space:]' ' ' | cut -c1-160)" \
    "inspect: make logs-api"
  return 1
}

# wait_routed <service> <host> <seconds>
#
# Healthy is not reachable. The edge adds a container's route only once Docker
# reports it healthy, and it applies provider changes in batches (Traefik's
# providersThrottleDuration, 2s by default) - until then the EDGE answers, with
# its own plain-text "404 page not found". The smoke test once ran 1.2s after
# landing turned healthy and got exactly that, six times.
#
# Only a persistent edge 404 is a failure. An edge this machine cannot reach at
# all (a server that cannot resolve its own name) is left to the smoke test.
wait_routed() {
  local svc="$1" host="$2" limit="${3:-30}" i=0 body=""
  while [ "$i" -lt "$limit" ]; do
    body="$(curl -sk --max-time 5 "https://$host/" 2>/dev/null | head -c 64)"
    if [ -n "$body" ] && [ "$body" != "404 page not found" ]; then
      log_ok "$svc routed at $host"; return 0
    fi
    sleep 1; i=$((i + 1))
  done
  if [ -z "$body" ]; then
    log_warn "$svc: https://$host/ did not answer from here - leaving it to the smoke test"
    return 0
  fi
  log_fail "the edge has no route to $svc after ${limit}s (https://$host/ answers \"404 page not found\")" \
    "inspect the router labels on $svc, then: make logs-traefik"
  return 1
}

rc=0
log_step "Waiting for infrastructure"
wait_healthy postgres 90 || rc=1
wait_healthy redis    60 || rc=1
wait_healthy mercure  60 || rc=1

log_step "Waiting for applications"
# The API's first build compiles every dependency from a cold cache; the three
# Nuxt dev servers compile on boot. Waiting on their healthchecks is what keeps
# the smoke test from racing them.
wait_healthy api      240 || rc=1
wait_serving api /health '"status":"ok"' 60 || rc=1
wait_healthy frontend 180 || rc=1
wait_healthy manager  180 || rc=1
wait_healthy landing  180 || rc=1

log_step "Waiting for the edge to route them"
DOMAIN="$(sed -n 's/^DOMAIN=//p' "$ROOT/.env" | head -1)"
wait_routed api      "api.$DOMAIN"     || rc=1
wait_routed frontend "app.$DOMAIN"     || rc=1
wait_routed manager  "manager.$DOMAIN" || rc=1
wait_routed landing  "$DOMAIN"         || rc=1
exit $rc

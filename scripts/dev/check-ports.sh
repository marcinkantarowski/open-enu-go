#!/usr/bin/env bash
# =============================================================================
#  Host port collisions - checked before `up`, named before Docker names them.
# =============================================================================
#  Container, network, volume and router names all carry ${STACK}, so two
#  projects coexist on one Docker host by construction. A PUBLISHED host port
#  is the one thing the prefix cannot separate: two checkouts both default to
#  DB_PORT=5432, and the second `make up` dies halfway with "port is already
#  allocated" - which says nothing about who holds it or which variable to
#  change.
#
#  The ports are read from the RESOLVED compose config, never listed here: a
#  port added to compose is covered the day it is added, and on staging - where
#  the override unpublishes everything - there is correctly nothing to check.
#
#  Read-only. Changes nothing.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/compose.sh"

[ -f "$ROOT/.env" ] || die ".env is missing" "run: make env"
env_value() { sed -n "s/^$1=//p" "$ROOT/.env" | head -1; }
STACK="$(env_value STACK)"; STACK="${STACK:-$(env_value PROJECT_SLUG)}"
[ -n "$STACK" ] || die "STACK must be set in .env"

compose_init "$ROOT"

log_step "Host ports"

# A config that cannot be resolved is not a config with no ports.
config="$("${COMPOSE[@]}" config 2>&1)" \
  || die "could not resolve the compose config" "run: docker compose config" "$(tail -1 <<<"$config")"
ports="$(sed -n 's/^ *published: *"\{0,1\}\([0-9]\{1,5\}\)"\{0,1\} *$/\1/p' <<<"$config" | sort -un)"

if [ -z "$ports" ]; then
  log_skip "this stack publishes no host ports"
  exit 0
fi

# listening <port> - does anything on this host accept connections on it?
# A connect rather than `ss`: under WSL2 the holder may be a Windows process,
# which no Linux-side socket table can see.
listening() {
  if command -v timeout >/dev/null 2>&1; then
    timeout 2 bash -c "exec 3<>/dev/tcp/127.0.0.1/$1" 2>/dev/null
  else
    (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
  fi
}

# next_free <port> → the first port above it that nothing holds
next_free() {
  local p=$(($1 + 1))
  while [ "$p" -lt 65535 ]; do
    if [ -z "$(docker ps --filter "publish=$p" --format '{{.Names}}')" ] && ! listening "$p"; then
      printf '%s\n' "$p"; return 0
    fi
    p=$((p + 1))
  done
  return 1
}

# how_to_move <port> → the remedy, naming the .env variable when one owns it
how_to_move() {
  local var free
  var="$(sed -n "s/^\([A-Z0-9_]*PORT\)=$1\$/\1/p" "$ROOT/.env" | head -1)"
  free="$(next_free "$1")"
  if [ -n "$var" ]; then
    printf 'or give this project its own: set %s=%s in .env, then: make env' "$var" "${free:-<a free port>}"
  else
    printf 'or publish a different host port in the compose file (%s is free)' "${free:-none found}"
  fi
}

rc=0
for p in $ports; do
  holder="$(docker ps --filter "publish=$p" --format '{{.Names}}	{{.Label "com.docker.compose.project"}}' | head -1)"
  name="${holder%%	*}"; project="${holder#*	}"

  if [ -n "$holder" ] && [ "$project" = "$STACK" ]; then
    log_ok "port $p - already ours ($name)"
  elif [ -n "$holder" ]; then
    log_fail "port $p is held by the container '$name'${project:+ (compose project '$project')}" \
      "stop it: docker stop $name" \
      "$(how_to_move "$p")" || true
    rc=1
  elif listening "$p"; then
    log_fail "port $p is held by a process outside Docker" \
      "find it: ss -ltnp 'sport = :$p'  (under WSL2 it may be on the Windows side: netstat -ano | findstr :$p)" \
      "$(how_to_move "$p")" || true
    rc=1
  else
    log_ok "port $p - free"
  fi
done

exit $rc

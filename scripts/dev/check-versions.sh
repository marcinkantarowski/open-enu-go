#!/usr/bin/env bash
# =============================================================================
#  Versions - one version of each service and toolchain, wherever it is named.
# =============================================================================
#  The same Postgres, Redis, Mercure, Go and Node are named in several files:
#  the dev and production compose files, the kernel's test containers, the
#  backup check, the go.mod toolchain lines, the Dockerfiles. Dependabot bumps
#  one file at a time and never reads a Go string or a shell script, so without
#  this they drift apart and nobody decided it - the tests pass against one
#  Redis while production runs another.
#
#  Each file's version is READ, never assumed: a file that no longer names it
#  in the shape read here is a failure, not a pass. A check that stopped looking
#  is the one that reports green forever.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

log_step "Versions"
fails=0

# pick FILE REGEX - the first capture of the first line REGEX matches whole.
pick() {
  local file="$ROOT/$1" v
  [ -f "$file" ] || { log_fail "$1 is missing" || true; return 1; }
  v="$(sed -nE "s#$2#\1#p" "$file" | head -1)"
  [ -n "$v" ] || { log_fail "$1 no longer names the version in the shape this check reads" \
    "update scripts/dev/check-versions.sh with the new shape - pattern: $2" || true; return 1; }
  printf '%s' "$v"
}

# same WHAT "FILE|REGEX"... - every file names the same version.
same() {
  local what="$1" first="" spec v differ=0 report=""
  shift
  for spec in "$@"; do
    v="$(pick "${spec%%|*}" "${spec#*|}")" || { fails=$((fails + 1)); return; }
    report+="$(printf '\n      %-38s %s' "${spec%%|*}" "$v")"
    if [ -z "$first" ]; then first="$v"; elif [ "$v" != "$first" ]; then differ=1; fi
  done
  if [ "$differ" -eq 0 ]; then
    log_ok "$what $first, in all $# places"
  else
    log_fail "$what is not the same everywhere" "bump every one of these together:" || true
    printf '%s\n' "$report" >&2
    fails=$((fails + 1))
  fi
}

S='[[:space:]]'
same "Postgres" \
  "docker/compose.dev.yml|^$S*image: pgvector/pgvector:pg([0-9]+)$S*$" \
  "docker/compose.prod.yml|^$S*image: pgvector/pgvector:pg([0-9]+)$S*$" \
  "backend/kernel/testdb/testdb.go|.*\"postgres:([0-9]+)-alpine\".*" \
  "scripts/remote/backup-verify.sh|^$S*pgvector/pgvector:pg([0-9]+)$S.*"
same "Redis" \
  "docker/compose.dev.yml|^$S*image: redis:([^[:space:]]+)$S*$" \
  "docker/compose.prod.yml|^$S*image: redis:([^[:space:]]+)$S*$" \
  "backend/kernel/testdb/testdb.go|.*\"redis:([^\"]+)\".*"
same "Mercure" \
  "docker/compose.dev.yml|^$S*image: dunglas/mercure:([^[:space:]]+)$S*$" \
  "docker/compose.prod.yml|^$S*image: dunglas/mercure:([^[:space:]]+)$S*$"
same "Go" \
  "backend/go.mod|^toolchain go([0-9.]+)$S*$" \
  "backend/kernel/go.mod|^toolchain go([0-9.]+)$S*$" \
  "docker/api/Dockerfile|^FROM golang:([0-9.]+)-alpine$S.*"
same "Node" \
  "docker/node/Dockerfile|^FROM node:([0-9]+)-alpine$S.*" \
  "package.json|^$S*\"node\": \">=([0-9]+)\"$S*$"

[ "$fails" -eq 0 ] || exit 1

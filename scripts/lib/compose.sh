#!/usr/bin/env bash
# =============================================================================
#  The one way a script invokes compose for THIS checkout.
# =============================================================================
#  Sets COMPOSE=(docker compose … -f … -f …) from COMPOSE_FILE in .env.
#
#  A checkout is not always the local dev stack. On a server, staging runs
#  compose.dev.yml PLUS compose.staging.yml - the override that unpublishes the
#  database port and puts every router behind an IP allowlist. A script that
#  hard-codes `-f compose.dev.yml` drops that override without a word, and the
#  next `up` publishes Postgres to the internet. So every script asks here.
#
#  COMPOSE_FILE uses compose's own format (paths relative to the checkout,
#  joined with `:`), so a bare `docker compose ps` typed in the checkout - by a
#  person or by an agent - reads the same files.
#
#  Each listed file brings the project's file of the same name from
#  docker/project/, when there is one, right after it: compose.prod.yml then
#  docker/project/compose.prod.yml. That is where a project adds a service or a
#  route of its own. The platform never ships docker/project/, so an update
#  never touches it, and each stage keeps its own rules - a project router on
#  staging goes behind the allowlist in docker/project/compose.staging.yml.
#  A bare `docker compose` does not know this; scripts and make targets do.
# =============================================================================

# compose_files <root> → prints one absolute path per line
compose_files() {
  local root="$1" list f
  list="$(sed -n 's/^COMPOSE_FILE=//p' "$root/.env" 2>/dev/null | head -1)"
  list="${list:-docker/compose.dev.yml}"
  local IFS=:
  for f in $list; do
    case "$f" in /*) ;; *) f="$root/$f" ;; esac
    printf '%s\n' "$f"
    [ -f "$root/docker/project/${f##*/}" ] && printf '%s\n' "$root/docker/project/${f##*/}"
  done
  return 0
}

# compose_init <root> → sets the COMPOSE array
compose_init() {
  local root="$1" f
  COMPOSE=(docker compose --project-directory "$root" --env-file "$root/.env")
  while IFS= read -r f; do COMPOSE+=(-f "$f"); done < <(compose_files "$root")
}

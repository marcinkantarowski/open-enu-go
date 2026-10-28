#!/usr/bin/env bash
# =============================================================================
#  Environment generation - .ai/platform/docs/dev-stack.md
# =============================================================================
#  .env is the only file edited by hand. Every per-app env file is DERIVED from
#  it, so api.${DOMAIN}, CORS origins and Mercure URLs cannot drift apart. The
#  Go backend gets no file: compose composes its environment from .env.
#
#  Two invariants, both load-bearing:
#    1. An existing value is never overwritten. This is what makes `buildprod`
#       safe to re-run against a live server.
#    2. A key added to .env.example upstream is reported (and, for generated
#       secrets, filled) on the next run - so a new variable surfaces as a
#       message, not as a 3am runtime crash.
#
#  Usage: envgen.sh generate   # create/repair .env, then write per-app files
#         envgen.sh check      # read-only drift report; exit 1 if keys missing
# =============================================================================
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
# shellcheck source=log.sh
. "$HERE/log.sh"; . "$HERE/secrets.sh"; . "$HERE/os.sh"

ENV_FILE="$ROOT/.env"
ENV_EXAMPLE="$ROOT/.env.example"

# ── helpers ─────────────────────────────────────────────────────────────────

# Keys declared in a file, in order.
env_keys() { grep -oE '^[A-Z_][A-Z0-9_]*=' "$1" | tr -d '=' ; }

# Value of a key, read literally (no shell evaluation of the file).
env_get() {
  local key="$1" file="$2"
  sed -n "s/^${key}=//p" "$file" | head -1
}

# Where a project's own variables begin in .env.example. Everything declared
# after this line belongs to the product built on the platform - the credentials
# of the third parties IT talks to - and is handed to the backend as it is.
PROJECT_MARKER='# == Project variables =='

# The project's variables, as lines for docker/api/project.env - an env_file
# the api, worker and scheduler services read (GENERATED, gitignored).
#
# Declared in .env.example (after the marker), valued in .env. The platform
# does not know their names and must not need to: without this a project that
# integrates anything has to edit this generator, and then every platform
# update is a merge conflict in the one script that writes secrets to disk.
project_variables() {
  grep -qxF "$PROJECT_MARKER" "$ENV_EXAMPLE" || return 0

  local key value past_marker=0 line
  printf '\n# Project variables - declared after the marker in .env.example.\n'

  while IFS= read -r line; do
    if [ "$line" = "$PROJECT_MARKER" ]; then past_marker=1; continue; fi
    [ "$past_marker" -eq 1 ] || continue

    # Not every line is a variable - most are comments - and a grep that finds
    # nothing must not end the script.
    key="$(printf '%s\n' "$line" | grep -oE '^[A-Z_][A-Z0-9_]*=' | tr -d '=' || true)"
    [ -n "$key" ] || continue

    value="$(env_get "$key" "$ENV_FILE")"
    # compose reads an env_file literally - no expansion, no quotes stripped -
    # so the value is written as it is.
    printf '%s=%s\n' "$key" "$value"
  done < "$ENV_EXAMPLE"
}

env_set() {
  local key="$1" val="$2" file="$3"
  local esc; esc=$(printf '%s' "$val" | sed -e 's/[\/&|]/\\&/g')
  if grep -qE "^${key}=" "$file"; then
    sed_inplace "s|^${key}=.*|${key}=${esc}|" "$file"
  else
    printf '%s=%s\n' "$key" "$val" >> "$file"
  fi
}

# ── 1. create or repair .env ────────────────────────────────────────────────

ensure_env_file() {
  if [ ! -f "$ENV_FILE" ]; then
    cp "$ENV_EXAMPLE" "$ENV_FILE"
    chmod 600 "$ENV_FILE"
    log_ok "created .env from .env.example"
  else
    chmod 600 "$ENV_FILE"
    log_skip ".env exists - existing values preserved"
  fi

  # Append keys that upstream added to .env.example but this .env lacks.
  local added=0 key
  while read -r key; do
    [ -n "$key" ] || continue
    if ! grep -qE "^${key}=" "$ENV_FILE"; then
      local default; default=$(env_get "$key" "$ENV_EXAMPLE")
      printf '%s=%s\n' "$key" "$default" >> "$ENV_FILE"
      log_warn "new variable added from .env.example: $key"
      added=$((added + 1))
    fi
  done < <(env_keys "$ENV_EXAMPLE")
  [ "$added" -gt 0 ] && log_info "review the $added new value(s) in .env"

  # Host identity, so containers can write into the bind mount as this user
  # rather than as root. Refreshed every run: a checkout copied between machines
  # would otherwise keep the previous machine's uid.
  env_set HOST_UID "$(id -u)" "$ENV_FILE"
  env_set HOST_GID "$(id -g)" "$ENV_FILE"

  # Fill generated secrets. Only ever touches literal __GENERATE__ markers.
  local filled=0
  while read -r key; do
    [ -n "$key" ] || continue
    if [ "$(env_get "$key" "$ENV_FILE")" = "__GENERATE__" ]; then
      env_set "$key" "$(gen_secret 32)" "$ENV_FILE"
      filled=$((filled + 1))
    fi
  done < <(env_keys "$ENV_FILE")
  [ "$filled" -gt 0 ] && log_ok "generated $filled secret(s)"
  return 0
}

# ── 2. derive the per-app env files ─────────────────────────────────────────

write_derived() {
  # Split across lines so the directive below sits directly above the command
  # it annotates: on one line it would apply to `set -a` and the source would
  # still be flagged.
  set -a
  # The path is computed, and the file is the one this script just created or
  # repaired, so there is nothing for a linter to follow.
  # shellcheck source=/dev/null
  . "$ENV_FILE"
  set +a

  local api="https://api.${DOMAIN}"
  # On the API's host, not a mercure. subdomain: the subscriber cookie can then
  # be host-only. A cookie shared by two sibling hosts needs Domain=.${DOMAIN},
  # which also reaches every host beneath it - a staging stack included.
  local mercure="https://api.${DOMAIN}/.well-known/mercure"
  local app_url="https://app.${DOMAIN}"
  local manager_url="https://manager.${DOMAIN}"
  local site_url="https://${DOMAIN}"
  local banner="# GENERATED by \`make env\` from .env - DO NOT EDIT.
# Edit .env instead and re-run; every value here is derived from it.
# See .ai/platform/docs/dev-stack.md."

  # ── backend ───────────────────────────────────────────────────────────────
  # Nothing. The Go binary's environment is composed in docker/compose.dev.yml
  # from the values in .env - service hostnames, the two database roles, the
  # Mercure hub - and a native `openenu` has no stack to talk to. One fewer
  # generated file to drift. The project's own variables (after the marker in
  # .env.example) reach the api, worker and scheduler through an env_file:
  mkdir -p "$ROOT/docker/api"
  { printf '%s\n' "$banner"; project_variables; } > "$ROOT/docker/api/project.env"

  # ── the three Nuxt apps ───────────────────────────────────────────────────
  cat > "$ROOT/frontend/.env" <<EOF
$banner

NUXT_PUBLIC_API_BASE=${api}
NUXT_PUBLIC_MERCURE_URL=${mercure}
NUXT_PUBLIC_APP_NAME=${PROJECT_NAME}
NUXT_PUBLIC_DOMAIN=${DOMAIN}
NUXT_PUBLIC_DEFAULT_LOCALE=${DEFAULT_LOCALE:-en}
NUXT_PUBLIC_SITE_URL=${site_url}
NUXT_PUBLIC_DEMO_CONTENT=${DEMO_CONTENT:-true}
EOF

  cat > "$ROOT/manager/.env" <<EOF
$banner

NUXT_PUBLIC_API_BASE=${api}
NUXT_PUBLIC_MERCURE_URL=${mercure}
NUXT_PUBLIC_APP_NAME=${PROJECT_NAME} Manager
NUXT_PUBLIC_DOMAIN=${DOMAIN}
NUXT_PUBLIC_DEFAULT_LOCALE=${DEFAULT_LOCALE:-en}
NUXT_PUBLIC_APP_URL=${app_url}
EOF

  # Only production may be indexed. A staging server is the same site on a
  # public hostname, and one that ranks next to production is a bug with a
  # long half-life. Anything that is not exactly "production" says no.
  local indexable=false
  [ "${APP_STAGE:-}" = production ] && indexable=true

  cat > "$ROOT/landing/.env" <<EOF
$banner

NUXT_PUBLIC_APP_NAME=${PROJECT_NAME}
NUXT_PUBLIC_APP_URL=${app_url}
NUXT_PUBLIC_SITE_URL=${site_url}
NUXT_PUBLIC_DOMAIN=${DOMAIN}
NUXT_PUBLIC_DEFAULT_LOCALE=${DEFAULT_LOCALE:-en}
NUXT_PUBLIC_INDEXABLE=${indexable}
EOF

  cat > "$ROOT/e2e/.env" <<EOF
$banner

E2E_APP_URL=${app_url}
E2E_MANAGER_URL=${manager_url}
E2E_API_BASE=${api}
E2E_SITE_URL=${site_url}
E2E_DEFAULT_LOCALE=${DEFAULT_LOCALE:-en}
EOF

  log_ok "derived frontend/.env, manager/.env, landing/.env, e2e/.env, docker/api/project.env"
}

# ── 3. drift check (read-only) ──────────────────────────────────────────────

check_env() {
  [ -f "$ENV_FILE" ] || die ".env does not exist" "run: make env"

  local missing=() unset_secrets=() key
  while read -r key; do
    [ -n "$key" ] || continue
    grep -qE "^${key}=" "$ENV_FILE" || missing+=("$key")
  done < <(env_keys "$ENV_EXAMPLE")

  while read -r key; do
    [ -n "$key" ] || continue
    [ "$(env_get "$key" "$ENV_FILE")" = "__GENERATE__" ] && unset_secrets+=("$key")
  done < <(env_keys "$ENV_FILE")

  # A variable declared twice. Nothing downstream objects: every reader takes
  # one of the two - the last, usually - and carries on, so the file says two
  # things and only one of them is true. It arrives by merge: two branches, or a
  # platform update and a project, each add the same variable a few lines apart
  # with different values, and git joins them without a conflict.
  local file duplicated rc=0
  for file in "$ENV_EXAMPLE" "$ENV_FILE"; do
    duplicated="$(env_keys "$file" | sort | uniq -d | tr '\n' ' ')"
    if [ -n "$duplicated" ]; then
      log_fail "$(basename "$file") declares a variable more than once" \
        "$duplicated" "keep one line for each; the last one is the one that was winning" || true
      rc=1
    fi
  done

  if [ ${#missing[@]} -gt 0 ]; then
    log_fail "${#missing[@]} variable(s) in .env.example are missing from .env" \
      "${missing[*]}" "run: make env" || true
    rc=1
  fi
  if [ ${#unset_secrets[@]} -gt 0 ]; then
    log_fail "${#unset_secrets[@]} secret(s) still set to __GENERATE__" \
      "${unset_secrets[*]}" "run: make env" || true
    rc=1
  fi
  [ $rc -eq 0 ] && log_ok ".env is complete (${#missing[@]} missing, 0 placeholders)"
  return $rc
}

# ── entrypoint ──────────────────────────────────────────────────────────────
case "${1:-generate}" in
  generate) ensure_env_file; write_derived ;;
  check)    check_env ;;
  *)        die "unknown command: $1" "usage: envgen.sh [generate|check]" ;;
esac

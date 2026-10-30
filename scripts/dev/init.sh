#!/usr/bin/env bash
# =============================================================================
#  make init NAME=myproject [DOMAIN=myproject.local] [GO_MODULE=github.com/me/myproject]
#            [PLATFORM_SOURCE=<url> PLATFORM_REF=<commit>]   (a copy, not a clone)
# =============================================================================
#  Turns the boilerplate into a named project: the manifest, the application's
#  Go module path and every import of it, the default domain.
#
#  What it deliberately does NOT touch - the reason this is a script and not a
#  `sed -i` across the tree (ADR-0002):
#
#      github.com/open-enu/kernel   the kernel's module path, in go.mod and imports
#      backend/kernel/              the kernel's own source tree
#      @open-enu/ui-kit             npm package
#      .ai/platform/                the platform's own records (ADR-0027)
#
#  Those are the FRAMEWORK's identity, not the project's. Keeping them stable
#  is what lets a project later drop the `replace` directive and `go get` a
#  tagged kernel, instead of hand-merging kernel fixes forever.
#
#  The application module path is the project's, and a Go module path is
#  expected to be unique, so it IS renamed - to GO_MODULE if given, else
#  github.com/<slug>/app. Exact-string replacement on `go.mod` and import
#  lines only; nothing pattern-based.
#
#  Also: .env.example (and .env when present), the compose files' `${STACK:-...}`
#  fallbacks, the cache prefix default, the fallback domain in the Nuxt and
#  Playwright configs and the AGENTS.md title. The apps' titles
#  come from PROJECT_NAME at runtime, so no Vue file names the project.
#
#  It also records the platform commit the project starts from (and where the
#  platform is), so `make platform-update` knows what the project already has.
#
#  Idempotent: running it twice, or on an already-named project, is a reported
#  no-op. A new place that names the project adds itself HERE, and `make
#  selftest` proves the rename reaches it and still leaves the kernel alone.
# =============================================================================
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# INIT_ROOT: rename THAT tree instead of this one (platform-update uses it).
ROOT="${INIT_ROOT:-$(cd "$HERE/../.." && pwd)}"
# shellcheck source=../lib/log.sh
. "$HERE/../lib/log.sh"; . "$HERE/../lib/os.sh"

MANIFEST="$ROOT/.project.json"
[ -f "$MANIFEST" ] || die ".project.json is missing" "this does not look like an OpenEnu Go checkout"

NEW_SLUG="${NAME:-}"
[ -n "$NEW_SLUG" ] || die "NAME is required" \
  "usage: make init NAME=myproject [DOMAIN=myproject.com] [GO_MODULE=github.com/me/myproject]" \
  "NAME must be lowercase letters, digits and dashes (it becomes the DB name" \
  "and, with dashes kept, the default domain and module path)."

grep -qE '^[a-z][a-z0-9-]{1,30}$' <<<"$NEW_SLUG" || die \
  "invalid NAME: '$NEW_SLUG'" \
  "must match ^[a-z][a-z0-9-]{1,30}\$ - start with a letter, then lowercase" \
  "letters, digits or dashes. It is used verbatim as a PostgreSQL identifier."

# ── current identity ────────────────────────────────────────────────────────
read_manifest() { sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$MANIFEST" | head -1; }
OLD_SLUG="$(read_manifest slug)"
OLD_NAME="$(read_manifest name)"
OLD_DOMAIN="$(read_manifest domain)"
OLD_DB="$(read_manifest db)"; OLD_DB="${OLD_DB:-${OLD_SLUG//-/_}}"
OLD_MODULE="$(read_manifest go_module)"
[ -n "$OLD_MODULE" ] || die ".project.json has no go_module" "it must name the application's Go module path"

# Where this project came from, for `make platform-update`. Kept across a
# re-init; on the first one, taken from git while it is still true: a fresh
# clone's HEAD is the platform commit and its origin is the platform. A copy
# committed into a new repository has neither - HEAD and origin are the
# project's own - so PLATFORM_REF and PLATFORM_SOURCE, when given, win.
PLATFORM_REF="${PLATFORM_REF:-$(read_manifest platform_ref)}"
PLATFORM_SOURCE="${PLATFORM_SOURCE:-$(read_manifest platform_source)}"
GUESSED_PLATFORM=0
if [ -z "$PLATFORM_REF" ] && grep -q '"initialized"[[:space:]]*:[[:space:]]*false' "$MANIFEST" \
   && git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1; then
  PLATFORM_REF="$(git -C "$ROOT" rev-parse --verify --quiet HEAD 2>/dev/null || true)"
  PLATFORM_SOURCE="${PLATFORM_SOURCE:-$(git -C "$ROOT" remote get-url origin 2>/dev/null || true)}"
  GUESSED_PLATFORM=1
fi

# ── target identity ─────────────────────────────────────────────────────────
NEW_NAME="$(echo "$NEW_SLUG" | awk -F'-' '{for(i=1;i<=NF;i++) printf toupper(substr($i,1,1)) substr($i,2); print ""}')"
NEW_DOMAIN="${DOMAIN:-${NEW_SLUG}.local}"
NEW_DB="${NEW_SLUG//-/_}"
NEW_MODULE="${GO_MODULE:-github.com/${NEW_SLUG}/app}"

case "$NEW_MODULE" in
  github.com/open-enu/kernel|github.com/open-enu/kernel/*) die "GO_MODULE may not be the kernel's path" "the kernel is the framework's identity and is never renamed (ADR-0002)" ;;
esac
grep -qE '^[a-z0-9.-]+(/[A-Za-z0-9._-]+)+$' <<<"$NEW_MODULE" || die "invalid GO_MODULE: '$NEW_MODULE'" "expected something like github.com/me/myproject"

if [ "$OLD_SLUG" = "$NEW_SLUG" ] && [ "$OLD_DOMAIN" = "$NEW_DOMAIN" ] && [ "$OLD_MODULE" = "$NEW_MODULE" ]; then
  log_skip "already initialized as '$NEW_SLUG' (${NEW_DOMAIN}, ${NEW_MODULE}) - nothing to do"
  exit 0
fi

log_step "Renaming project"
log_info "slug    ${OLD_SLUG}  ->  ${NEW_SLUG}"
log_info "name    ${OLD_NAME}  ->  ${NEW_NAME}"
log_info "domain  ${OLD_DOMAIN}  ->  ${NEW_DOMAIN}"
log_info "db      ${OLD_DB}  ->  ${NEW_DB}"
log_info "module  ${OLD_MODULE}  ->  ${NEW_MODULE}"

# replace_exact <file> <old> <new> - literal, never a pattern. Uses a delimiter
# that cannot appear in a module path or a domain.
replace_exact() {
  local f="$1" old="$2" new="$3"
  grep -qF -- "$old" "$f" || return 0
  local o n
  o="$(printf '%s' "$old" | sed 's/[][\.*^$/|]/\\&/g')"
  n="$(printf '%s' "$new" | sed 's/[&|\\]/\\&/g')"
  sed_inplace "s|$o|$n|g" "$f"
}

# ── 1. the application's Go module path: go.mod and every import line ──────
# Only files under backend/, never under backend/kernel/: the kernel has no
# import of the app by construction, so there is nothing of ours in there to
# rename, and touching it would be the mistake this script exists to prevent.
log_step "Go module path"
replace_exact "$ROOT/backend/go.mod" "module $OLD_MODULE" "module $NEW_MODULE"
touched=0
while IFS= read -r f; do
  if grep -qF -- "\"$OLD_MODULE" "$f"; then
    replace_exact "$f" "\"$OLD_MODULE" "\"$NEW_MODULE"
    touched=$((touched + 1))
  fi
done < <(find "$ROOT/backend" -path "$ROOT/backend/kernel" -prune -o -name '*.go' -print 2>/dev/null)
log_ok "go.mod and ${touched} import site(s)"

# ── 2. the manifest ─────────────────────────────────────────────────────────
log_step "Manifest"
replace_exact "$MANIFEST" "\"slug\": \"$OLD_SLUG\""       "\"slug\": \"$NEW_SLUG\""
replace_exact "$MANIFEST" "\"name\": \"$OLD_NAME\""       "\"name\": \"$NEW_NAME\""
replace_exact "$MANIFEST" "\"domain\": \"$OLD_DOMAIN\""   "\"domain\": \"$NEW_DOMAIN\""
replace_exact "$MANIFEST" "\"db\": \"$OLD_DB\""           "\"db\": \"$NEW_DB\""
replace_exact "$MANIFEST" "\"go_module\": \"$OLD_MODULE\"" "\"go_module\": \"$NEW_MODULE\""
replace_exact "$MANIFEST" "\"initialized\": false"        "\"initialized\": true"
if [ -n "$PLATFORM_REF" ]; then
  REF_VALUE="$PLATFORM_REF" SOURCE_VALUE="$PLATFORM_SOURCE" python3 - "$MANIFEST" <<'PY'
import json, os, sys
path = sys.argv[1]
with open(path, encoding='utf-8') as fh:
    data = json.load(fh)
data['platform_ref'] = os.environ['REF_VALUE']
if os.environ['SOURCE_VALUE']:
    data['platform_source'] = os.environ['SOURCE_VALUE']
with open(path, 'w', encoding='utf-8') as fh:
    json.dump(data, fh, indent=2, ensure_ascii=False)
    fh.write('\n')
PY
fi
log_ok ".project.json$([ -n "$PLATFORM_REF" ] && echo " (made from platform ${PLATFORM_REF:0:12})")"
if [ "$GUESSED_PLATFORM" = 1 ]; then
  log_warn "platform taken from this checkout's git: ${PLATFORM_REF:0:12} at ${PLATFORM_SOURCE:-<no origin>}"
  log_info "right for a clone of the platform; for a copy committed into a new repository, say which:"
  log_info "make init NAME=$NEW_SLUG PLATFORM_SOURCE=<platform url> PLATFORM_REF=<platform commit>"
fi

# ── 3. defaults that carry the slug or the domain ───────────────────────────
log_step "Defaults"
replace_exact "$ROOT/Makefile" "PROJECT_SLUG ?= $OLD_SLUG" "PROJECT_SLUG ?= $NEW_SLUG"
replace_exact "$ROOT/Makefile" "DOMAIN       ?= $OLD_DOMAIN" "DOMAIN       ?= $NEW_DOMAIN"
log_ok "Makefile"

# The environment the stack is built from. Whole assignments only, and only
# where the value is still the old default: a value someone already changed
# in .env is theirs. Domain before slug, because the domain contains it.
for envf in "$ROOT/.env.example" "$ROOT/.env"; do
  [ -f "$envf" ] || continue
  replace_exact "$envf" "DOMAIN=$OLD_DOMAIN"        "DOMAIN=$NEW_DOMAIN"
  replace_exact "$envf" "@$OLD_DOMAIN"              "@$NEW_DOMAIN"
  replace_exact "$envf" "PROJECT_NAME=$OLD_NAME"    "PROJECT_NAME=$NEW_NAME"
  replace_exact "$envf" "PROJECT_SLUG=$OLD_SLUG"    "PROJECT_SLUG=$NEW_SLUG"
  replace_exact "$envf" "STACK=$OLD_SLUG"           "STACK=$NEW_SLUG"
  replace_exact "$envf" "DB_NAME=$OLD_DB"           "DB_NAME=$NEW_DB"
  replace_exact "$envf" "DB_USER=$OLD_DB"           "DB_USER=$NEW_DB"
  replace_exact "$envf" "my-bucket/$OLD_SLUG"       "my-bucket/$NEW_SLUG"
done
log_ok ".env.example$([ -f "$ROOT/.env" ] && echo ' and .env')"

# The fallbacks compose uses when STACK is unset - the container names, the
# network and every router name. `-prod` included: it starts with the slug.
for cf in "$ROOT"/docker/compose.*.yml; do
  replace_exact "$cf" "\${STACK:-$OLD_SLUG" "\${STACK:-$NEW_SLUG"
done
replace_exact "$ROOT/scripts/dev/check-prod-compose.sh" "\${STACK:-$OLD_SLUG}" "\${STACK:-$NEW_SLUG}"
replace_exact "$ROOT/backend/cmd/openenu/platform.go" "\"PROJECT_SLUG\", \"$OLD_SLUG\"" "\"PROJECT_SLUG\", \"$NEW_SLUG\""
replace_exact "$ROOT/AGENTS.md" "# Agent Guidelines - $OLD_SLUG" "# Agent Guidelines - $NEW_SLUG"
# The npm workspace root (the lockfile names it twice). The workspaces keep
# their own names; @open-enu/ui-kit is the framework's.
for pj in package.json package-lock.json; do
  if [ -f "$ROOT/$pj" ]; then replace_exact "$ROOT/$pj" "\"name\": \"$OLD_SLUG\"," "\"name\": \"$NEW_SLUG\","; fi
done
# The domain the apps and the browser tests fall back to when the derived env
# files are absent. Matched with its closing quote, so only the literal moves.
for nf in frontend/nuxt.config.ts manager/nuxt.config.ts ui-kit/nuxt.config.ts landing/nuxt.config.ts \
          e2e/playwright.config.ts e2e/tests/impersonation.spec.ts e2e/tests/example.spec.ts \
          e2e/tests/consultants.spec.ts; do
  if [ -f "$ROOT/$nf" ]; then replace_exact "$ROOT/$nf" "$OLD_DOMAIN'" "$NEW_DOMAIN'"; fi
done
log_ok "compose fallbacks, the cache prefix default, the apps' fallback domain, the npm root, AGENTS.md"

echo
log_ok "renamed to '$NEW_SLUG' - the kernel (github.com/open-enu/kernel) is untouched, as it must be"
log_info "next: make check"

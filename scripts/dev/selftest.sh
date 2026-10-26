#!/usr/bin/env bash
# =============================================================================
#  make selftest - the guardrails are proved to fail when broken.
# =============================================================================
#  Every rule this repository enforces is run here against a deliberately
#  broken throwaway copy, and must FAIL there. A check that has never been seen
#  to fail is a check that may be asserting nothing. It proves, in order:
#
#    1. the kernel cannot import the application - the build itself refuses
#    2. `make init` renames the application module and NOT the kernel,
#       is idempotent, and keeps database identifiers SQL-safe
#    3. the typography check catches a long dash
#    4. the docs check catches a broken link and an unindexed record
#
#  and then, section by section: the API types' freshness, one version per
#  service, translation parity, the scaffold, the generated files, every
#  analyzer's plant, locale parity at start-up, prod-check, the
#  inventory, the ACL, route coverage, and `make platform-update` carrying a
#  platform change into a renamed project. A new guardrail appends its own
#  section here (backend/kernel/analysis/<name>/testdata holds the analyzer
#  fixtures).
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/go.sh"; . "$HERE/../lib/os.sh"
[ -n "${GO:-}" ] || die "go is not installed" "make check-tools"

PASS=0; FAIL=0
ok()  { PASS=$((PASS+1)); printf '%s  ✓%s %s\n' "$C_GRN" "$C_RESET" "$1"; }
bad() { FAIL=$((FAIL+1)); printf '%s  ✗ %s%s\n' "$C_RED$C_BOLD" "$1" "$C_RESET" >&2
        [ $# -gt 1 ] && printf '      expected: %s\n      actual:   %s\n' "$2" "$3" >&2; return 0; }
# assert_eq <label> <expected> <actual>
assert_eq()       { [ "$2" = "$3" ] && ok "$1" || bad "$1" "$2" "$3"; }
assert_contains() { grep -qF -- "$3" <<<"$2" && ok "$1" || bad "$1" "contains '$3'" "$2"; }

# passes_with <label> <output> <last-line needle> - a whole check passed; when
# it did not, the failures are what is shown, not only its last line (which
# says how many failed and never which).
passes_with() {
  if grep -qF -- "$3" <<<"$(tail -1 <<<"$2")"; then ok "$1"; return; fi
  bad "$1" "a last line containing '$3'" "$(grep -A6 '✗' <<<"$2" | head -24)"
}

# fails_when <label> <command...> - the command must exit non-zero.
fails_when() {
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then bad "$label" "a failing exit" "exit 0"; else ok "$label"; fi
}

# A throwaway copy of the tree, without .git and without anything heavy.
#
# Every script exercised below is run FROM THE COPY ($WORK/scripts/...), never
# from $ROOT. A dev script resolves its own ROOT from where it lives, so the
# real tree's init.sh renames the real tree no matter what the cwd is - which is
# exactly what happened the first time this file was run.
# (.ai/platform/lessons/a-selftest-runs-the-copys-scripts.md)
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
rsync -a --exclude .git --exclude node_modules --exclude '.nuxt' --exclude '.output' "$ROOT/" "$WORK/" 2>/dev/null \
  || { cp -r "$ROOT/." "$WORK/"; rm -rf "$WORK/.git"; }
cd "$WORK" || exit 1
INIT="$WORK/scripts/dev/init.sh"

# The one guard that cannot be a convention: if anything below has touched the
# real tree, say so before anything else.
real_manifest="$(cat "$ROOT/.project.json")"
trap 'rm -rf "$WORK"; [ "$(cat "$ROOT/.project.json")" = "$real_manifest" ] || { log_fail "selftest modified the REAL .project.json - a script ran from \$ROOT instead of the copy" "git checkout -- .project.json backend/go.mod Makefile"; exit 1; }' EXIT

kernel_refs() { grep -rF --include='*.go' --include='go.mod' -c 'github.com/open-enu/kernel' backend | awk -F: '{s+=$2} END {print s+0}'; }

# ── 1. kernel purity is a build failure ─────────────────────────────────────
log_step "The kernel cannot import the application"
before_ok="$(cd backend/kernel && "$GO" build ./... 2>&1 && echo clean)"
assert_contains "clean kernel builds" "$before_ok" "clean"
cat > backend/kernel/zz_selftest_purity.go <<'EOF'
package kernel

import _ "github.com/open-enu/app/cmd/openenu"
EOF
fails_when "kernel importing the app does not build" bash -c 'cd backend/kernel && "$0" build ./...' "$GO"
rm -f backend/kernel/zz_selftest_purity.go

# ── 2. init renames the app, never the kernel ───────────────────────────────
log_step "make init - renames the application, not the kernel"
refs_before="$(kernel_refs)"
out="$(NAME=acme-crm DOMAIN=crm.acme.test bash "$INIT" 2>&1)"
assert_contains "init ran" "$out" "renamed to 'acme-crm'"
assert_contains "app module renamed in go.mod" "$(head -20 backend/go.mod)" "module github.com/acme-crm/app"
assert_eq "no import of the old app path remains" "0" "$(grep -rlF '"github.com/open-enu/app' backend --include='*.go' | wc -l | tr -d ' ')"
# Byte-for-byte: the kernel tree is the framework's, and init has no business in it.
if diff -rq "$ROOT/backend/kernel" "$WORK/backend/kernel" >/dev/null 2>&1; then ok "kernel tree byte-identical after init"; else bad "kernel tree byte-identical after init" "no diff" "$(diff -rq "$ROOT/backend/kernel" "$WORK/backend/kernel" 2>&1 | head -3)"; fi
assert_eq "kernel references unchanged" "$refs_before" "$(kernel_refs)"
assert_contains "manifest slug" "$(cat .project.json)" '"slug": "acme-crm"'
assert_contains "manifest db is SQL-safe" "$(cat .project.json)" '"db": "acme_crm"'
assert_contains "manifest initialized" "$(cat .project.json)" '"initialized": true'
assert_contains "renamed tree still builds" "$(cd backend && "$GO" build ./... 2>&1 && echo clean)" "clean"
assert_contains ".env.example carries the new domain" "$(cat .env.example)" "DOMAIN=crm.acme.test"
assert_contains ".env.example carries the new database" "$(cat .env.example)" "DB_NAME=acme_crm"
assert_contains ".env.example carries the new stack" "$(cat .env.example)" "STACK=acme-crm"
assert_eq "no compose fallback names the old slug" "0" "$(grep -c 'STACK:-open-enu-go' docker/compose.dev.yml docker/compose.prod.yml | awk -F: '{s+=$2} END {print s}')"
assert_eq "no app or browser test falls back to the old domain" "0" "$(grep -lF 'open-enu-go.local' frontend/nuxt.config.ts manager/nuxt.config.ts ui-kit/nuxt.config.ts landing/nuxt.config.ts e2e/playwright.config.ts e2e/tests/*.spec.ts | wc -l | tr -d ' ')"
assert_contains "the tenant app falls back to the new domain" "$(cat frontend/nuxt.config.ts)" "crm.acme.test"
assert_eq "the npm workspace root is renamed, the ui-kit is not" "2 1" "$(grep -c '"name": "acme-crm",' package-lock.json) $(grep -c '"name": "@open-enu/ui-kit"' ui-kit/package.json)"
out2="$(NAME=acme-crm DOMAIN=crm.acme.test bash "$INIT" 2>&1)"
assert_contains "second run is a no-op" "$out2" "nothing to do"
fails_when "refuses the kernel path as GO_MODULE" env NAME=x-y GO_MODULE=github.com/open-enu/kernel bash "$INIT"
fails_when "refuses an unsafe NAME" env NAME='Bad Name' bash "$INIT"

# ── 3. typography ───────────────────────────────────────────────────────────
log_step "Typography check catches a long dash"
printf 'a %s b\n' "$(printf '\342\200\224')" > "$WORK/.ai/platform/zz_dash.md"
fails_when "long dash is refused" bash "$WORK/scripts/dev/check-typography.sh"
rm -f "$WORK/.ai/platform/zz_dash.md"

log_step "Types check catches API types older than the spec"
assert_contains "types check passes on the stamped copy" "$(bash "$WORK/scripts/dev/check-types.sh" 2>&1 && echo clean)" "clean"
cp "$WORK/backend/openapi.json" "$WORK/openapi.bak"
printf ' ' >> "$WORK/backend/openapi.json"
fails_when "a spec changed after make types is refused" bash "$WORK/scripts/dev/check-types.sh"
cp "$WORK/openapi.bak" "$WORK/backend/openapi.json"; rm -f "$WORK/openapi.bak"

log_step "Versions check catches a service on two versions"
assert_contains "versions check passes on the copy" "$(bash "$WORK/scripts/dev/check-versions.sh" 2>&1 && echo clean)" "clean"
cp "$WORK/docker/compose.prod.yml" "$WORK/prod.bak"
sed_inplace 's|image: redis:[^[:space:]]*|image: redis:7.4-alpine|' "$WORK/docker/compose.prod.yml"
fails_when "Redis on another version in production is refused" bash "$WORK/scripts/dev/check-versions.sh"
cp "$WORK/prod.bak" "$WORK/docker/compose.prod.yml"; rm -f "$WORK/prod.bak"
cp "$WORK/backend/kernel/testdb/testdb.go" "$WORK/testdb.bak"
sed_inplace 's|"postgres:[0-9]*-alpine"|postgresImage|' "$WORK/backend/kernel/testdb/testdb.go"
fails_when "a file that stops naming the version is a failure, not a pass" bash "$WORK/scripts/dev/check-versions.sh"
cp "$WORK/testdb.bak" "$WORK/backend/kernel/testdb/testdb.go"; rm -f "$WORK/testdb.bak"

log_step "Translation check catches a key or a page in one locale only"
assert_contains "translation check passes on the copy" "$(bash "$WORK/scripts/dev/check-i18n.sh" 2>&1 && echo clean)" "clean"
cp "$WORK/frontend/app/modules/example/i18n/locales/pl.json" "$WORK/pl.bak"
python3 -I -c 'import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8")); d.pop(sorted(d)[0])
json.dump(d, open(sys.argv[1], "w", encoding="utf-8"), ensure_ascii=False)' "$WORK/frontend/app/modules/example/i18n/locales/pl.json"
fails_when "a key missing from pl is refused" bash "$WORK/scripts/dev/check-i18n.sh"
cp "$WORK/pl.bak" "$WORK/frontend/app/modules/example/i18n/locales/pl.json"; rm -f "$WORK/pl.bak"
printf -- '---\ntitle: x\n---\n' > "$WORK/landing/content/en/zz-only-en.md"
fails_when "a landing page in one locale only is refused" bash "$WORK/scripts/dev/check-i18n.sh"
rm -f "$WORK/landing/content/en/zz-only-en.md"

# ── 4. docs ─────────────────────────────────────────────────────────────────
log_step "Docs check catches a broken link and an unindexed record"
printf '# stray\n\n[gone](./does-not-exist.md)\n' > "$WORK/.ai/platform/adr/9999-stray.md"
fails_when "broken link + unindexed ADR are refused" bash "$WORK/scripts/dev/check-docs.sh"
rm -f "$WORK/.ai/platform/adr/9999-stray.md"
docs_out="$(bash "$WORK/scripts/dev/check-docs.sh" 2>&1)"
passes_with "clean copy passes docs check (links must not leave the repository)" "$docs_out" "documentation matches"

# ── 5. make module - a scaffolded module is a working module ────────────────
log_step "make module - scaffolds a module that builds, serves and tests"
out="$(NAME=demo bash "$WORK/scripts/dev/module.sh" 2>&1)"
assert_contains "scaffold ran to the end" "$(tail -4 <<<"$out")" "module demo created" \
  || sed 's/^/      /' <<<"$out" | tail -15 >&2
assert_contains "registry lists it" "$(cd "$WORK/backend" && "$GO" run ./cmd/openenu modules 2>&1)" "demo.view"
assert_contains "merged spec carries its path" "$(cat "$WORK/backend/openapi.json")" '"/api/demo"'
assert_contains "Task Router row" "$(cat "$WORK/AGENTS.md")" "| Demo - demo |"
assert_contains "frontend layer scaffolded" "$(cd "$WORK/frontend/app/modules/demo" 2>&1 && ls nuxt.config.ts navigation.ts pages/demo/index.vue i18n/locales/en.json i18n/locales/pl.json 2>&1)" "pages/demo/index.vue"
assert_eq "the layer sets no srcDir (it would become the app's)" "0" "$(grep -c 'srcDir:' "$WORK/frontend/app/modules/demo/nuxt.config.ts" 2>/dev/null || true)"
assert_contains "its page is gated by the module's permission" "$(cat "$WORK/frontend/app/modules/demo/pages/demo/index.vue" 2>&1)" "permission: 'demo.view'"
fails_when "refuses a second scaffold with the same name" env NAME=demo bash "$WORK/scripts/dev/module.sh"
fails_when "refuses a StudlyCase name" env NAME=Demo bash "$WORK/scripts/dev/module.sh"
passes_with "the copy still passes check with the module in it" "$(cd "$WORK" && bash scripts/dev/check.sh 2>&1)" "all checks passed"

# make manager-module - an operator layer for a module that exists, and only then
out="$(NAME=demo bash "$WORK/scripts/dev/manager-module.sh" 2>&1)"
assert_contains "operator layer scaffolded" "$(cd "$WORK/manager/app/modules/demo" 2>&1 && ls nuxt.config.ts navigation.ts i18n/locales/en.json i18n/locales/pl.json 2>&1)" "navigation.ts" \
  || sed 's/^/      /' <<<"$out" | tail -10 >&2
assert_eq "the operator layer sets no srcDir (it would become the console's)" "0" "$(grep -c 'srcDir:' "$WORK/manager/app/modules/demo/nuxt.config.ts" 2>/dev/null || true)"
fails_when "refuses an operator layer for a module that does not exist" env NAME=nosuchmodule bash "$WORK/scripts/dev/manager-module.sh"
fails_when "refuses a second operator layer for the same module" env NAME=demo bash "$WORK/scripts/dev/manager-module.sh"

# ── 6. generated files must be fresh ────────────────────────────────────────
log_step "A stale generated file fails gen-check"
echo "// stale" >> "$WORK/backend/cmd/openenu/module_registry_gen.go"
fails_when "stale module_registry_gen.go is refused" bash -c 'cd "$1/backend/cmd/openenu" && "$2" run ../../tools/genregistry -check' _ "$WORK" "$GO"
(cd "$WORK/backend/cmd/openenu" && "$GO" generate ./ >/dev/null 2>&1)
# sed_inplace_copy [sed flags] <expr> <file> - in place, on either host's sed
sed_inplace_copy() { local file="${!#}" tmp; tmp="$(mktemp)"; sed "${@:1:$#-1}" "$file" > "$tmp" && mv "$tmp" "$file"; }
sed_inplace_copy 's|"/api/demo"|"/api/demo-renamed"|' "$WORK/backend/internal/module/demo/openapi.json"
fails_when "fragment changed without regenerating openapi.json" bash -c 'cd "$1/backend/cmd/openenu" && "$2" run ../../tools/genspec -check' _ "$WORK" "$GO"
fails_when "fragment changed without regenerating api.gen.go" bash -c 'cd "$1/backend/internal/module/demo" && "$2" run ../../../tools/genapi -check' _ "$WORK" "$GO"
(cd "$WORK/backend/internal/module/demo" && "$GO" generate ./ >/dev/null 2>&1; cd "$WORK/backend/cmd/openenu" && "$GO" generate ./ >/dev/null 2>&1)

# ── 7. every analyzer fails on a planted violation ──────────────────────────
# The fixtures under kernel/analysis/*/testdata prove each rule under `go
# test`; this proves the rules are actually wired into `arch` over the real
# tree, which is a different thing.
log_step "Analyzers fire on a planted violation in a real module"
ARCH='cd "$1/backend" && "$2" run github.com/open-enu/kernel/analysis/cmd/arch ./...'
assert_contains "clean scaffold passes arch" "$(bash -c "$ARCH" _ "$WORK" "$GO" 2>&1 && echo clean)" "clean"
plant() { # <label> <file-under-demo/internal> <go source>
  local label="$1" file="$WORK/backend/internal/module/demo/internal/$2"
  printf '%s' "$3" > "$file"
  fails_when "$label" bash -c "$ARCH" _ "$WORK" "$GO"
  rm -f "$file"
}
# thincontroller needs the violation IN api.go: append a call to a handler
# function to the scaffolded one, then restore it.
cp "$WORK/backend/internal/module/demo/internal/api.go" "$WORK/api.go.bak"
printf '%s' 'package internal

func findAll() []string { return nil }
' > "$WORK/backend/internal/module/demo/internal/handler.go"
printf '\nvar _ = findAll()\n' >> "$WORK/backend/internal/module/demo/internal/api.go"
fails_when "thincontroller fires on api.go using handler.go" bash -c "$ARCH" _ "$WORK" "$GO"
cp "$WORK/api.go.bak" "$WORK/backend/internal/module/demo/internal/api.go"; rm -f "$WORK/backend/internal/module/demo/internal/handler.go"

plant "platformservice fires on os.WriteFile in a module" zz_export.go 'package internal

import "os"

func export() error { return os.WriteFile("/tmp/x", nil, 0o600) }
'
plant "rawstring fires on a sentence passed as a key" zz_strings.go 'package internal

import "github.com/open-enu/kernel/httpx"

var _ = httpx.NotFound("Project not found.")
'
plant "rawstring fires on an i18n.Key conversion of a sentence" zz_keyconv.go 'package internal

import "github.com/open-enu/kernel/i18n"

var _ = i18n.Key("Not a key either")
'
plant "flagdeclared fires on a flag read with no Definition" zz_flag.go 'package internal

import (
	"context"

	"github.com/open-enu/kernel/flags"
)

var _ = flags.Static{}.Enabled(context.Background(), "demo.undeclared", false)
'
plant "rawsql fires on Unscoped with a variable reason outside repository.go" zz_unscoped.go 'package internal

import (
	"context"

	"github.com/open-enu/kernel/scope"
)

var why = "because"

var _ = scope.Unscoped(context.Background(), why, func(context.Context) error { return nil })
'
plant "writeboundary fires on *scope.Tx outside handler.go" zz_write.go 'package internal

import "github.com/open-enu/kernel/scope"

func write(tx *scope.Tx) {}
'
# textlen reads only api.go and handler.go, where input is checked; the
# scaffold writes no handler.go, so the plant is one.
plant "textlen fires on a byte count used as a text limit" handler.go 'package internal

func validName(s string) bool { return len(s) <= 200 }
'
plant "tableowner fires on SQL naming another module's table" zz_peek.go 'package internal

const peek = `SELECT name FROM tenant WHERE id = $1`
'
assert_contains "clean again after the plants are removed" "$(bash -c "$ARCH" _ "$WORK" "$GO" 2>&1 && echo clean)" "clean"

# ── 8. prod-check holds the production and staging stacks to their invariants
# Each case breaks one rule in the copy and expects the check to say so. Needs
# Docker only for `compose config`, which renders without a daemon's images.
if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  log_step "prod-check fails on each broken invariant"
  PRODCHECK="$WORK/scripts/dev/check-prod-compose.sh"
  [ -f "$WORK/.env" ] || cp "$WORK/.env.example" "$WORK/.env"
  prodcase() { # <label> <edit of the copy>
    local label="$1" edit="$2"
    cp "$WORK/docker/compose.prod.yml" "$WORK/prod.bak"; cp "$WORK/docker/compose.staging.yml" "$WORK/staging.bak"
    # eval, not bash -c: the edit uses this file's own helpers
    eval "$edit"
    fails_when "$label" bash "$PRODCHECK"
    cp "$WORK/prod.bak" "$WORK/docker/compose.prod.yml"; cp "$WORK/staging.bak" "$WORK/docker/compose.staging.yml"
  }
  prodcase "prod-check catches a published database port" \
    "sed_inplace_copy 's|^  postgres:|  postgres:\n    ports: [\"5432:5432\"]|' '$WORK/docker/compose.prod.yml'"
  prodcase "prod-check catches an unpinned image" \
    "sed_inplace_copy -E 's|image: redis:[^[:space:]]+|image: redis:latest|' '$WORK/docker/compose.prod.yml'"
  prodcase "prod-check catches a router with no certificate resolver" \
    "awk '!d && /tls.certresolver=letsencrypt/ {d=1; next} {print}' '$WORK/docker/compose.prod.yml' > '$WORK/prod.tmp' && mv '$WORK/prod.tmp' '$WORK/docker/compose.prod.yml'"
  prodcase "prod-check catches a backend container built from the dev target" \
    "awk '!d && /target: prod/ {sub(/target: prod/, \"target: dev\"); d=1} {print}' '$WORK/docker/compose.prod.yml' > '$WORK/prod.tmp' && mv '$WORK/prod.tmp' '$WORK/docker/compose.prod.yml'"
  prodcase "prod-check catches a staging stack that publishes the database port" \
    "sed_inplace_copy '/ports: !reset \\[\\]/d' '$WORK/docker/compose.staging.yml'"
  prodcase "prod-check catches a staging router outside the allowlist" \
    "sed_inplace_copy '/routers.\\\${STACK}-api.middlewares/d' '$WORK/docker/compose.staging.yml'"
  # A project's own files (docker/project/) ride along with the platform's, so
  # their routers answer to the same rules - or a project route is the one that
  # reaches production with the wrong certificate, or staging without a guard.
  projectcase() { # <label> <file name> - a project router with neither a resolver nor the allowlist
    mkdir -p "$WORK/docker/project"
    printf 'services:\n  api:\n    labels:\n      - traefik.http.routers.selftest-extra.rule=Host(`extra.test`)\n' > "$WORK/docker/project/$2"
    fails_when "$1" bash "$PRODCHECK"
    rm -rf "$WORK/docker/project"
  }
  projectcase "prod-check holds a project's production router to a certificate resolver" compose.prod.yml
  projectcase "prod-check holds a project's staging router to the allowlist" compose.dev.yml
  # Any named file, not only the dev one: restore.sh once named compose.prod.yml.
  printf '#!/usr/bin/env bash\ndocker compose -f docker/compose.prod.yml ps\n' > "$WORK/scripts/remote/zz-planted.sh"
  fails_when "prod-check catches a script that names compose.prod.yml" bash "$PRODCHECK"
  rm -f "$WORK/scripts/remote/zz-planted.sh"
  assert_contains "prod-check passes on the unbroken copy" "$(bash "$PRODCHECK" 2>&1 && echo clean)" "clean"
else
  log_skip "prod-check cases need docker compose"
fi

# ── 9. the agent interfaces stay true to the code ───────────────────────────
log_step "Inventory, ACL and route coverage fail when stale or incomplete"
(cd "$WORK/backend" && "$GO" run ./cmd/openenu inventory >/dev/null 2>&1)
assert_contains "a fresh inventory passes -check" "$(cd "$WORK/backend" && "$GO" run ./cmd/openenu inventory -check 2>&1)" "matching the code"
echo '{"modules": {}}' > "$WORK/.ai/inventory.json"
fails_when "a stale inventory fails -check" bash -c 'cd "$1/backend" && "$2" run ./cmd/openenu inventory -check' _ "$WORK" "$GO"
(cd "$WORK/backend" && "$GO" run ./cmd/openenu inventory >/dev/null 2>&1)

# The ACL is read from the handler's own auth.Require literal: change the
# guard and the committed map is stale.
cp "$WORK/backend/internal/module/example/internal/api.go" "$WORK/api.go.acl"
sed_inplace_copy 's|auth.Require(ctx, "example.manage")|auth.Require(ctx, "example.view")|' "$WORK/backend/internal/module/example/internal/api.go"
fails_when "a changed permission guard makes acl_gen.go stale" bash -c 'cd "$1/backend/cmd/openenu" && "$2" run ../../tools/genacl -check' _ "$WORK" "$GO"
cp "$WORK/api.go.acl" "$WORK/backend/internal/module/example/internal/api.go"
assert_contains "acl_gen.go matches the handlers again" "$(cd "$WORK/backend/cmd/openenu" && "$GO" run ../../tools/genacl -check 2>&1 && echo clean)" "clean"

# Coverage is spec minus trace: a trace with every pattern passes, one line
# fewer names the operation it is missing.
(cd "$WORK/backend" && "$GO" run ./cmd/openenu route-coverage -list > "$WORK/full.trace")
assert_contains "a complete trace passes route coverage" "$(cd "$WORK/backend" && "$GO" run ./cmd/openenu route-coverage "$WORK/full.trace" 2>&1)" "0 exempt"
grep -v '^POST /api/projects$' "$WORK/full.trace" > "$WORK/partial.trace"
fails_when "an operation missing from the trace fails route coverage" bash -c 'cd "$1/backend" && "$2" run ./cmd/openenu route-coverage "$1/partial.trace"' _ "$WORK" "$GO"
assert_contains "the failure names the operation" "$(cd "$WORK/backend" && "$GO" run ./cmd/openenu route-coverage "$WORK/partial.trace" 2>&1)" "POST /api/projects (post_example_project_create)"
fails_when "a missing trace is a failure, not coverage" bash -c 'cd "$1/backend" && "$2" run ./cmd/openenu route-coverage "$1/absent.trace"' _ "$WORK" "$GO"

# ── 10. the registry refuses what the analyzers cannot see ──────────────────
log_step "Registry refuses a catalogue key missing from one locale"
echo '{"demo.only_en": "x"}' > "$WORK/backend/internal/module/demo/i18n/messages.en.json"
parity_out="$(cd "$WORK/backend" && "$GO" run ./cmd/openenu modules 2>&1)"
fails_when "openenu modules fails on locale parity" bash -c 'cd "$1/backend" && "$2" run ./cmd/openenu modules' _ "$WORK" "$GO"
assert_contains "the error names the key and the locale" "$parity_out" 'key "demo.only_en" exists in messages.en.json and not in messages.pl.json'
echo '{}' > "$WORK/backend/internal/module/demo/i18n/messages.en.json"

# ── 11. a project follows the platform it was made from ─────────────────────
log_step "make platform-update - a project follows the platform it was made from"
# Two real repositories, built from this checkout: a platform, and a project
# cloned from it and renamed. The platform then moves on, and the project has to
# follow - in the project's spelling, without touching the kernel's, and without
# overwriting what the project changed itself. Both repositories are throwaway
# ones in a temporary directory; nothing is committed anywhere else.
UPD="$(mktemp -d)"
trap 'rm -rf "$WORK" "$UPD"; [ "$(cat "$ROOT/.project.json")" = "$real_manifest" ] || { log_fail "selftest modified the REAL .project.json - a script ran from \$ROOT instead of the copy" "git checkout -- .project.json backend/go.mod Makefile"; exit 1; }' EXIT
GT=(git -c user.name=selftest -c user.email=selftest@localhost -c commit.gpgsign=false -c init.defaultBranch=main)
jget() { python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get(sys.argv[2], ""))' "$1" "$2"; }

mkdir "$UPD/platform"
tar --exclude=.git --exclude=node_modules --exclude=.nuxt --exclude=.output --exclude=.data --exclude=.env \
    --exclude=backend/bin --exclude=backend/var --exclude=docker/certs -C "$ROOT" -cf - . | tar -C "$UPD/platform" -xf -
"${GT[@]}" -C "$UPD/platform" init -q
"${GT[@]}" -C "$UPD/platform" add -A . >/dev/null
"${GT[@]}" -C "$UPD/platform" commit -q -m "platform, as cloned"
upd_base="$("${GT[@]}" -C "$UPD/platform" rev-parse HEAD)"

"${GT[@]}" clone -q "$UPD/platform" "$UPD/project"
( cd "$UPD/project" && make init NAME=upd-shop DOMAIN=shop.example.com GO_MODULE=example.com/shop/app >/dev/null 2>&1 )
assert_eq "init records the platform commit it started from" "$upd_base" "$(jget "$UPD/project/.project.json" platform_ref)"
assert_eq "init records where the platform is"                "$UPD/platform" "$(jget "$UPD/project/.project.json" platform_source)"
echo 'the project owns this line' >> "$UPD/project/README.md"
"${GT[@]}" -C "$UPD/project" add -A . >/dev/null
"${GT[@]}" -C "$UPD/project" commit -q -m "the project, renamed"

# The platform moves on: a new Go file importing the application by the
# platform's module path, a platform doc, and the kernel.
printf 'package example\n\nimport _ "github.com/open-enu/app/internal/module/tenant/contract"\n' > "$UPD/platform/backend/internal/module/example/update_probe.go"
echo 'A line the platform added.' >> "$UPD/platform/.ai/platform/docs/README.md"
printf '\n// update probe\n' >> "$UPD/platform/backend/kernel/kernel.go"
"${GT[@]}" -C "$UPD/platform" add -A . >/dev/null
"${GT[@]}" -C "$UPD/platform" commit -q -m "the platform moves on"
upd_next="$("${GT[@]}" -C "$UPD/platform" rev-parse HEAD)"

if out="$(cd "$UPD/project" && make platform-update 2>&1)"; then ok "the update applies cleanly"; else bad "the update applies cleanly" "exit 0" "$out"; fi
assert_contains "a new platform file arrives in the project's spelling" "$(cat "$UPD/project/backend/internal/module/example/update_probe.go" 2>/dev/null)" '"example.com/shop/app/internal/module/tenant/contract"'
assert_contains "a platform doc is updated" "$(tail -1 "$UPD/project/.ai/platform/docs/README.md")" "A line the platform added."
assert_contains "the kernel is updated" "$(tail -1 "$UPD/project/backend/kernel/kernel.go")" "update probe"
assert_eq "the kernel keeps its own name"    "module github.com/open-enu/kernel" "$(grep '^module ' "$UPD/project/backend/kernel/go.mod")"
assert_eq "the project keeps its own change" "the project owns this line" "$(tail -1 "$UPD/project/README.md")"
assert_eq "the new platform commit is recorded" "$upd_next" "$(jget "$UPD/project/.project.json" platform_ref)"
assert_eq "nothing is committed for the user" "the project, renamed" "$("${GT[@]}" -C "$UPD/project" log -1 --format=%s)"
"${GT[@]}" -C "$UPD/project" commit -q -m "platform update"

# Both sides change the same line: the update must stop and say where, not pick.
sed_inplace '1s/.*/# changed by the platform/' "$UPD/platform/README.md"
"${GT[@]}" -C "$UPD/platform" commit -q -am "the platform edits a line"
sed_inplace '1s/.*/# changed by the project/' "$UPD/project/README.md"
"${GT[@]}" -C "$UPD/project" commit -q -am "the project edits the same line"
if out="$(cd "$UPD/project" && make platform-update 2>&1)"; then
  bad "a change on both sides is reported as a conflict" "non-zero exit" "exit 0"
else
  assert_contains "a change on both sides is reported as a conflict" "$out" "README.md"
fi
assert_contains "the conflict is marked in the file" "$(cat "$UPD/project/README.md")" "<<<<<<<"

# A dirty tree is refused: the update has to be the only thing in the diff.
"${GT[@]}" -C "$UPD/project" reset -q --hard
echo dirty >> "$UPD/project/README.md"
if out="$(cd "$UPD/project" && make platform-update 2>&1)"; then
  bad "an update over uncommitted work is refused" "non-zero exit" "exit 0"
else
  assert_contains "an update over uncommitted work is refused" "$out" "uncommitted"
fi

# The base's own init cannot run on this host - a platform from before a
# portability fix, rendered on the host that needed it. The target's init
# renames it instead, and the run says so.
"${GT[@]}" -C "$UPD/project" reset -q --hard
printf '#!/usr/bin/env bash\nexit 7\n' > "$UPD/platform/scripts/dev/init.sh"
"${GT[@]}" -C "$UPD/platform" commit -q -am "the platform breaks its init"
upd_broken="$("${GT[@]}" -C "$UPD/platform" rev-parse HEAD)"
"${GT[@]}" -C "$UPD/platform" checkout -q "$upd_next" -- scripts/dev/init.sh
"${GT[@]}" -C "$UPD/platform" commit -q -am "the platform repairs its init"
upd_repaired="$("${GT[@]}" -C "$UPD/platform" rev-parse HEAD)"
if out="$(cd "$UPD/project" && make platform-update BASE="$upd_broken" 2>&1)"; then
  ok "a base whose own init cannot run is rendered with the target's"
  assert_contains "the fallback is reported"      "$out" "target's init"
  assert_eq "the repaired commit is recorded" "$upd_repaired" "$(jget "$UPD/project/.project.json" platform_ref)"
else
  bad "a base whose own init cannot run is rendered with the target's" "exit 0" "$out"
fi
rm -rf "$UPD"

echo
if [ "$FAIL" -eq 0 ]; then log_ok "selftest: $PASS passed"; exit 0; fi
log_fail "selftest: $FAIL failed, $PASS passed"; exit 1

#!/usr/bin/env bash
# =============================================================================
#  make manager-module NAME=billing
# =============================================================================
#  Gives an EXISTING module a place in the operator console: a Nuxt layer under
#  manager/app/modules/, discovered by the console the same way the tenant app
#  discovers its own (manager/nuxt.config.ts).
#
#  Separate from `make module` on purpose. Every feature has tenant screens;
#  few have anything for an operator to do. Scaffolding an empty operator layer
#  for all of them would be a dozen directories that say nothing, and a console
#  menu nobody can read.
#
#  A layer contributes any of three things, each a file the console finds by
#  itself:
#    pages/<name>/*.vue   a screen, at /<name>
#    navigation.ts        an entry in the console's menu
#    injections.ts        a card on another screen - `manager.tenant` is the
#                         tenant page, and receives `tenantId`
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

NAME="${NAME:-}"

[ -n "$NAME" ] || die "NAME is required" \
  "usage: make manager-module NAME=billing" \
  "NAME is the module's own name - the directory under backend/internal/module/."

grep -qE '^[a-z][a-z0-9_]*$' <<<"$NAME" || die \
  "invalid NAME: '$NAME'" \
  "must match ^[a-z][a-z0-9_]*\$ - the same lowercase name the module was created with."

[ -d "$ROOT/backend/internal/module/$NAME" ] || die "backend/internal/module/$NAME does not exist" \
  "an operator layer belongs to a module that is already there." \
  "create the module first: make module NAME=$NAME"

KEBAB="${NAME//_/-}"
STUDLY="$(awk -F'_' '{for(i=1;i<=NF;i++) printf toupper(substr($i,1,1)) substr($i,2); print ""}' <<<"$NAME")"
LAYER="$ROOT/manager/app/modules/$KEBAB"

[ -d "$LAYER" ] && die "manager/app/modules/$KEBAB already exists" \
  "delete it first if you mean to start over"

log_step "Creating the operator layer of $NAME"

mkdir -p "$LAYER/components" "$LAYER/i18n/locales"
: > "$LAYER/components/.gitkeep"

cat > "$LAYER/nuxt.config.ts" <<EOF
// A module's operator screens are a Nuxt layer of the console, so its pages and
// components register themselves - and deleting this directory removes them
// cleanly (ADR-0007).
//
// Imported rather than relied on as a global: this file lives under the app's
// srcDir, so it is type-checked in the app project, where Nuxt's config globals
// are not declared.
import { defineNuxtConfig } from 'nuxt/config'

export default defineNuxtConfig({
  // Deliberately no \`srcDir\`. \`extends\` merges a layer's config into the app, so
  // setting it here would silently become the APP's srcDir, and the console's
  // own pages/ and middleware/ would stop being scanned - no error, just a
  // router that matches nothing.

  i18n: {
    // The loaders point at the .ts wrappers, never the .json - see
    // i18n/locales/en.ts for why.
    locales: [
      { code: 'en', files: ['en.ts'] },
      { code: 'pl', files: ['pl.ts'] },
    ],
  },
})
EOF

cat > "$LAYER/navigation.ts" <<EOF
import { defineNavigation } from '@ui-kit/composables/defineNavigation'

// Entries this module adds to the console's menu, after the console's own.
// Empty is a fine answer: a module that only adds a card to the tenant page
// (an injections.ts, see module-development.md) has nothing to put here.
//
//   { label: '$NAME.title', to: '/$KEBAB', order: 100 },
//
// A screen for it goes in pages/$KEBAB/index.vue.
export default defineNavigation([])
EOF

printf '{\n  "%s.title": "%s"\n}\n' "$NAME" "$STUDLY" > "$LAYER/i18n/locales/en.json"
printf '{\n  "%s.title": "%s"\n}\n' "$NAME" "$STUDLY" > "$LAYER/i18n/locales/pl.json"

for loc in en pl; do
  cat > "$LAYER/i18n/locales/$loc.ts" <<EOF
// The catalogue is the .json next door; this file only re-exports it.
//
// Registering the .json directly makes Vite's json plugin wrap the output of
// @nuxtjs/i18n's own transform in JSON.parse(), which breaks rendering.
// Pointing the loader at a .ts keeps the data in a file the parity check can
// read while taking it out of that plugin's path.
import messages from './$loc.json'

export default messages
EOF
done

log_ok "manager/app/modules/$KEBAB"

# ── let the running console see its new layer ───────────────────────────────
# A NEW layer is only discovered at startup; until then its pages answer 404,
# which looks exactly like a routing mistake in the code just written.
if command -v docker >/dev/null 2>&1; then
  # shellcheck source=../lib/compose.sh
  . "$HERE/../lib/compose.sh"
  compose_init "$ROOT"
  running="$("${COMPOSE[@]}" ps -q --status running manager 2>/dev/null | head -1)"
  # Only a container serving THIS checkout - see module.sh for why.
  if [ -n "$running" ] && [ "$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/app"}}{{.Source}}{{end}}{{end}}' "$running" 2>/dev/null)" != "$ROOT" ]; then
    running=""
  fi
  if [ -n "$running" ]; then
    if "${COMPOSE[@]}" restart manager >/dev/null 2>&1; then
      log_ok "manager restarted - a new layer is only discovered at startup"
    else
      log_warn "could not restart the manager - its pages will 404 until you do: make restart"
    fi
  else
    log_skip "manager is not running here - the new layer is picked up when it starts"
  fi
fi

log_step "Next"
log_info "1. a screen:            $LAYER/pages/$KEBAB/index.vue  + an entry in navigation.ts"
log_info "2. a card on a tenant:  $LAYER/injections.ts  ->  defineInjection('manager.tenant', YourCard)"
log_info "3. its endpoints:       /api/manager/... in the module's openapi.json, each starting with auth.RequireOperator(ctx)"
log_info "4. verify:              make check && make typecheck"

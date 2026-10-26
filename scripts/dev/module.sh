#!/usr/bin/env bash
# =============================================================================
#  make module NAME=billing          (bash scripts/dev/module.sh --no-frontend
#                                     skips the frontend layer)
# =============================================================================
#  Scaffolds one feature across every place it must exist: the backend module,
#  its spec fragment and generated server, the frontend layer, the docs, the
#  spec stub and the Task Router row - then generates, builds and tests what it
#  wrote.
#
#  The point is not typing speed. It is that "where does this go?" has exactly
#  one answer, and that the harness grows WITH the code. A module written by
#  hand differs in ways nobody notices until the third one.
#
#  The frontend layer is frontend/app/modules/<kebab-name>/: a Nuxt layer with
#  one permission-gated page, a menu entry and both locales. It is discovered
#  from the directory, so nothing central is edited - but a running dev server
#  only sees a NEW layer after a restart, which this script does when the
#  frontend container serves this checkout.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/go.sh"
[ -n "${GO:-}" ] || die "go is not installed" "make check-tools"

NAME="${NAME:-}"
WITH_FRONTEND=1
for arg in "$@"; do [ "$arg" = "--no-frontend" ] && WITH_FRONTEND=0; done
[ -n "$NAME" ] || die "NAME is required" \
  "usage: make module NAME=billing" \
  "NAME is lowercase - it is the directory, the Go package and the permission prefix."

grep -qE '^[a-z][a-z0-9_]*$' <<<"$NAME" || die \
  "invalid NAME: '$NAME'" \
  "must match ^[a-z][a-z0-9_]*\$ - lowercase letters, digits, underscores." \
  "It is the Go package name, so no dashes; the URL path gets them (billing_reports -> /api/billing-reports)."

case "$NAME" in
  internal|kernel|module|contract|main|test) die "NAME '$NAME' collides with a Go or platform name" ;;
esac

# billing_reports -> BillingReports (Go identifiers) and billing-reports (URL, frontend dir)
STUDLY="$(awk -F'_' '{for(i=1;i<=NF;i++) printf toupper(substr($i,1,1)) substr($i,2); print ""}' <<<"$NAME")"
KEBAB="${NAME//_/-}"
BACKEND="$ROOT/backend"
BACK="$BACKEND/internal/module/$NAME"
MODPATH="$(sed -nE 's/^module[[:space:]]+(\S+)/\1/p' "$BACKEND/go.mod" | head -1)"
TODAY="$(date +%F)"

FRONT="$ROOT/frontend/app/modules/$KEBAB"
[ -d "$BACK" ] && die "backend/internal/module/$NAME already exists" "delete it first, or pick another name"
[ "$WITH_FRONTEND" -eq 1 ] && [ -d "$FRONT" ] && die "frontend/app/modules/$KEBAB already exists" "delete it first, or pick another name"
[ "$WITH_FRONTEND" -eq 1 ] && [ ! -d "$ROOT/frontend/app/modules" ] && die "frontend/app/modules is not in this tree" \
  "run with --no-frontend for a backend-only module"

# Where this project's records go: the platform's own tree while the
# repository IS the platform, the project's tree once `make init` has run.
if grep -q '"initialized": true' "$ROOT/.project.json"; then SPECS="$ROOT/.ai/specs"; else SPECS="$ROOT/.ai/platform/specs"; fi

log_step "Creating module $NAME"

# ── backend ─────────────────────────────────────────────────────────────────
mkdir -p "$BACK"/{contract,internal,migrations,i18n}

cat > "$BACK/module.yaml" <<EOF
# Read by tools/genregistry at generate time and embedded into module.go. The
# name MUST match the directory - it is the Go package and the permission prefix.
name: $NAME
description: TODO - one line on what this module owns.
depends: []
enabled: true
EOF

cat > "$BACK/module.go" <<EOF
// Package $NAME - TODO: one line on what this module owns.
//
// The shape is .ai/platform/ARCHITECTURE.md §4: contract/ is what other
// modules may import, internal/ is everything else, and this file is the
// only thing the registry reads.
package $NAME

//go:generate go run ../../../tools/genapi

import (
	"embed"

	"github.com/open-enu/kernel/module"

	"$MODPATH/internal/module/$NAME/internal"
)

//go:embed module.yaml
var manifest []byte

//go:embed openapi.json
var spec []byte

//go:embed i18n
var locales embed.FS

// Module is the declaration. Permissions are global and prefixed with the
// module name; \`$NAME.view\` is what every member holds, anything else needs
// a role somebody granted.
var Module = module.Module{
	Manifest:    module.MustManifest(manifest),
	Permissions: []string{"$NAME.view"},
	Mount:       internal.Mount,
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
	// Migrations: when the first table arrives, add
	//   //go:embed migrations/*.sql
	//   var migrations embed.FS
	// and set Migrations: module.MustSub(migrations, "migrations").
}
EOF

cat > "$BACK/openapi.json" <<EOF
{
  "openapi": "3.0.3",
  "paths": {
    "/api/$KEBAB": {
      "get": {
        "operationId": "get_${NAME}_index",
        "summary": "Requires \`$NAME.view\`.",
        "tags": ["$NAME"],
        "responses": {
          "200": {
            "description": "",
            "content": { "application/json": { "schema": { "\$ref": "#/components/schemas/${STUDLY}Index" } } }
          },
          "default": { "\$ref": "#/components/responses/Error" }
        }
      }
    }
  },
  "components": {
    "schemas": {
      "${STUDLY}Index": {
        "type": "object",
        "required": ["module"],
        "properties": {
          "module": { "type": "string" }
        }
      }
    }
  }
}
EOF

cat > "$BACK/contract/doc.go" <<EOF
// Package contract is the part of $NAME other modules may import: exported
// events, identifiers and read ports. Everything else is internal/ and the
// compiler refuses it from outside. Every exported identifier here must be
// named in MODULE.md - \`make docs-check\` insists.
package contract
EOF

cat > "$BACK/internal/api.go" <<EOF
package internal

import "context"

// api implements the StrictServerInterface that genapi derives from
// ../openapi.json. A method that disagrees with the spec does not compile -
// that is the contract, enforced by the type checker rather than a diff.
//
// This file stays thin: it decodes nothing by hand, holds no transaction,
// runs no SQL and never calls a handler. It turns a request into a command
// and a result into a response; reads go through the repository. Copy
// backend/internal/module/example/internal/api.go for the shape of each.
type api struct{}

var _ StrictServerInterface = api{}

// Get${STUDLY}Index answers GET /api/$KEBAB.
func (api) Get${STUDLY}Index(ctx context.Context, _ Get${STUDLY}IndexRequestObject) (Get${STUDLY}IndexResponseObject, error) {
	return Get${STUDLY}Index200JSONResponse{Module: "$NAME"}, nil
}
EOF

cat > "$BACK/internal/mount.go" <<EOF
package internal

import (
	"net/http"

	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/platform"
)

// Mount wires the module: handlers on the bus, jobs on the outbox, the
// controller on the mux. Both \`openenu api\` and \`openenu worker\` call it.
// See .ai/platform/docs/platform-services.md for what svc carries and
// backend/internal/module/example/internal/mount.go for a full wiring.
func Mount(mux *http.ServeMux, svc *platform.Services) {
	_ = svc // TODO: repo := newRepository(svc); command.Register(svc.Bus, h.create); ...
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
	}
	strict := NewStrictHandlerWithOptions(api{}, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  badRequest,
		ResponseErrorHandlerFunc: httpx.WriteError,
	})
	HandlerWithOptions(strict, StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest})
}
EOF

echo '{}' > "$BACK/i18n/messages.en.json"
echo '{}' > "$BACK/i18n/messages.pl.json"
: > "$BACK/migrations/.gitkeep"

cat > "$BACK/${NAME}_test.go" <<EOF
package ${NAME}_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-enu/kernel/platform"

	"$MODPATH/internal/module/$NAME"
)

// Every route, through real HTTP against the mounted module. Extend this as
// routes arrive: the route-coverage check (\`make route-coverage\`) fails on an
// operation no test reached.
func TestIndexAnswersWithTheModuleName(t *testing.T) {
	mux := http.NewServeMux()
	$NAME.Module.Mount(mux, platform.InMemory(nil))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/$KEBAB", nil))

	var body struct {
		Module string \`json:"module"\`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Module != "$NAME" {
		t.Fatalf("GET /api/$KEBAB = %d %s", rec.Code, rec.Body.String())
	}
}
EOF

cat > "$BACK/MODULE.md" <<EOF
# $STUDLY - \`$NAME\`

TODO - one paragraph: what this module owns, and what it deliberately does not.

## Surface

- API: \`GET /api/$KEBAB\` (\`get_${NAME}_index\`) - see \`openapi.json\`
- Permissions: \`$NAME.view\`
- Events: none yet
- Flags: none yet
- Tables: none yet

## Public contract

Nothing exported from \`contract/\` yet. Every exported identifier added there is listed
here - other modules may import it, so it is published surface.

## Depends on

Nothing. (\`depends:\` in \`module.yaml\`; only the dependency's \`contract/\` may be imported.)

## Spec

[.ai/platform/specs/$TODAY-$KEBAB.md](../../../../.ai/platform/specs/$TODAY-$KEBAB.md)
EOF
if [ "$SPECS" = "$ROOT/.ai/specs" ]; then
  sed_inplace_safe() { tmp="$(mktemp)"; sed "$1" "$2" > "$tmp" && mv "$tmp" "$2"; }
  sed_inplace_safe "s|.ai/platform/specs/$TODAY-$KEBAB.md|.ai/specs/$TODAY-$KEBAB.md|g" "$BACK/MODULE.md"
fi
log_ok "backend/internal/module/$NAME"

# ── spec stub ───────────────────────────────────────────────────────────────
SPEC="$SPECS/$TODAY-$KEBAB.md"
cat > "$SPEC" <<EOF
# $STUDLY - \`$NAME\`

**Status:** draft · **Date:** $TODAY

## Problem

TODO - what is missing, and why it matters.

## Approach

TODO - the shape of the solution. Which kernel contracts it uses
(.ai/platform/ARCHITECTURE.md §5); what the entity carries; which commands exist.

## Rejected

- TODO - the alternatives, and why not.

## Surface

- API: \`GET /api/$KEBAB\`
- Permissions: \`$NAME.view\`
- Events: -
- Flags: -

## Tests that ship with it

- [ ] functional: every route, as a person and as an API key
- [ ] unit: the handler's rules

## Changelog

- $TODAY - scaffolded by \`make module NAME=$NAME\`
EOF
printf '| [%s](%s) | module %s | draft |\n' "$(basename "$SPEC")" "$(basename "$SPEC")" "$NAME" >> "$SPECS/README.md"
log_ok "${SPEC#"$ROOT"/}"

# ── Task Router row ─────────────────────────────────────────────────────────
# Appended as the last row of the Task Router table: the first blank line
# after the heading ends the table.
python3 -I - "$ROOT/AGENTS.md" "$STUDLY" "$NAME" <<'PY'
import sys
path, studly, name = sys.argv[1:]
lines = open(path).read().split('\n')
row = f"| {studly} - {name} | `backend/internal/module/{name}/MODULE.md` |"
if row in lines:
    sys.exit(0)
try:
    start = next(i for i, l in enumerate(lines) if l.strip() == '## Task Router')
except StopIteration:
    sys.exit("AGENTS.md has no '## Task Router' heading")
i = start + 1
while i < len(lines) and not lines[i].startswith('|'):
    i += 1
while i < len(lines) and lines[i].startswith('|'):
    i += 1
lines.insert(i, row)
open(path, 'w').write('\n'.join(lines))
PY
log_ok "AGENTS.md Task Router row"

# ── frontend ────────────────────────────────────────────────────────────────
if [ "$WITH_FRONTEND" -eq 1 ]; then
  mkdir -p "$FRONT"/{components,composables,stores,types} "$FRONT/pages/$KEBAB" "$FRONT/i18n/locales"
  for d in components composables stores types; do : > "$FRONT/$d/.gitkeep"; done

  cat > "$FRONT/nuxt.config.ts" <<EOF
// A module is a Nuxt layer, so its pages, components and composables register
// themselves - and deleting this directory removes the feature cleanly, with no
// dangling imports left behind (ADR-0007).
//
// Imported rather than relied on as a global: this file lives under the app's
// srcDir, so it is type-checked in the app project, where Nuxt's config globals
// are not declared.
import { defineNuxtConfig } from 'nuxt/config'

export default defineNuxtConfig({
  // Deliberately no \`srcDir\`. \`extends\` merges a layer's config into the app, so
  // setting it here would silently become the APP's srcDir, and the app's own
  // pages/ and middleware/ would stop being scanned - no error, just a router
  // that matches nothing.

  i18n: {
    // \`<module>/i18n/locales\` is where @nuxtjs/i18n looks by default, so a
    // feature keeps its translations beside its pages rather than in one file
    // every branch edits. The loaders point at the .ts wrappers, never the
    // .json - see i18n/locales/en.ts for why.
    locales: [
      { code: 'en', files: ['en.ts'] },
      { code: 'pl', files: ['pl.ts'] },
    ],
  },
})
EOF

  cat > "$FRONT/navigation.ts" <<EOF
import { defineNavigation } from '@ui-kit/composables/defineNavigation'

// Menu entries this module contributes. Merged and permission-filtered by the
// shell - an extension surface, so nothing central needs editing.
export default defineNavigation([
  { label: '$NAME.title', to: '/$KEBAB', permission: '$NAME.view' },
])
EOF

  cat > "$FRONT/pages/$KEBAB/index.vue" <<EOF
<script setup lang="ts">
// Auto-registered at /$KEBAB by the layer - no re-export shim needed.
// The path comes from the directory, so pages live under pages/$KEBAB/.
definePageMeta({ middleware: 'permission', permission: '$NAME.view' })

const { t } = useI18n()
</script>

<template>
  <div class="flex flex-col gap-6">
    <!-- TODO: real UI. Every string is an i18n key in both locales (make i18n-check).
         frontend/app/modules/example/pages/example/index.vue is the screen to copy. -->
    <h1 class="text-lg font-semibold text-fg">{{ t('$NAME.title') }}</h1>
  </div>
</template>
EOF

  printf '{\n  "%s.title": "%s"\n}\n' "$NAME" "$STUDLY" > "$FRONT/i18n/locales/en.json"
  printf '{\n  "%s.title": "%s"\n}\n' "$NAME" "$STUDLY" > "$FRONT/i18n/locales/pl.json"
  for loc in en pl; do
    cat > "$FRONT/i18n/locales/$loc.ts" <<EOF
// The catalogue is the .json next door; this file only re-exports it.
//
// Registering the .json directly makes Vite's json plugin wrap the output of
// @nuxtjs/i18n's own transform in JSON.parse(), which breaks server rendering.
// Pointing the loader at a .ts keeps the data in a file the parity check can
// read while taking it out of that plugin's path.
import messages from './$loc.json'

export default messages
EOF
  done
  log_ok "frontend/app/modules/$KEBAB"
else
  log_skip "frontend layer (--no-frontend)"
fi

# ── generate, build, test what was written ──────────────────────────────────
log_step "Generating and verifying"
(cd "$BACK" && "$GO" generate ./ 2>&1 | sed 's/^/      /') || die "genapi failed for $NAME"
(cd "$BACKEND" && "$GO" generate ./cmd/openenu 2>&1 | sed 's/^/      /') || die "registry or spec generation failed"
log_ok "generated: internal/api.gen.go, module_registry_gen.go, backend/openapi.json"
(cd "$BACKEND" && "$GO" build ./... && "$GO" vet "./internal/module/$NAME/..." && "$GO" test "./internal/module/$NAME/...") 2>&1 | sed 's/^/      /' \
  || die "the scaffolded module does not build or its test fails - this is a platform bug, not yours"
log_ok "builds, vets and passes its own test"

# ── let the running app see its new layer ───────────────────────────────────
# Editing a file inside a layer hot-reloads. A NEW layer does not: Nuxt reads
# its list of layers once, at startup, so until the dev server restarts the
# module's pages answer 404 - which looks exactly like a routing mistake in the
# code just generated. Restart it here, or say why not.
if [ "$WITH_FRONTEND" -eq 1 ] && command -v docker >/dev/null 2>&1; then
  # shellcheck source=../lib/compose.sh
  . "$HERE/../lib/compose.sh"
  compose_init "$ROOT"
  running="$("${COMPOSE[@]}" ps -q --status running frontend 2>/dev/null | head -1)"
  # Only a container serving THIS checkout. `make selftest` runs this generator
  # in a throwaway copy that can share the stack's name, and restarting the real
  # dev server from there would be a side effect nobody asked for.
  if [ -n "$running" ] && [ "$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/app"}}{{.Source}}{{end}}{{end}}' "$running" 2>/dev/null)" != "$ROOT" ]; then
    running=""
  fi
  if [ -n "$running" ]; then
    if "${COMPOSE[@]}" restart frontend >/dev/null 2>&1; then
      log_ok "frontend restarted - a new layer is only discovered at startup"
    else
      log_warn "could not restart the frontend - its pages will 404 until you do: make restart"
    fi
  else
    log_skip "frontend is not running here - the new layer is picked up when it starts"
  fi
fi

echo
log_ok "module $NAME created"
log_info "next: fill in module.yaml's description and MODULE.md, write the spec, then \`make check\`"
log_info "the route changed the contract: \`make types inventory\` before \`make ci\`"
log_info "see:  openenu modules  |  backend/internal/module/example/ - one of everything, the thing to copy"

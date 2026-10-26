#!/usr/bin/env bash
# Install the Node dependencies INSIDE the container, so versions match what
# runs in production rather than whatever the host happens to have. The Go
# backend has no step here: `go build` under air fetches modules into the
# shared gomod volume on its first run.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"
. "$ROOT/scripts/lib/compose.sh"; compose_init "$ROOT"
CO=("${COMPOSE[@]}")

# Every install goes through `quiet` (scripts/lib/log.sh): a bare `| tail -4`
# cut npm's actual error off and left only "A complete log of this run can be
# found in..." - a path inside a container that `run --rm` has already deleted.

log_step "Node dependencies (npm workspaces)"
# One install at the workspace root covers ui-kit and all three apps, and
# symlinks @open-enu/ui-kit into node_modules so `extends` resolves it as a
# package rather than a relative path.
#
# `run --rm --no-deps` rather than `exec`: on a fresh checkout the Nuxt
# containers have no `nuxt` binary yet, so there is nothing to exec into. The
# image entrypoint installs on first boot anyway; this target is the explicit
# re-install after a package.json change.
#
# Under the entrypoint's own lock. `up` has just started the three Nuxt
# containers, and on a fresh checkout their entrypoints are installing into
# this same node_modules right now; `--entrypoint sh` bypasses that lock, so two
# installs wrote one tree at once and left it half-deleted (ENOTEMPTY, then a
# postinstall that cannot find a package's own files). Waiting on the lock makes
# this install run after theirs, and the stamp stops them repeating it.
INSTALL='mkdir -p /app/node_modules \
  && flock /app/node_modules/.install.lock sh -c "cd /app && npm install --no-audit --no-fund && touch /app/node_modules/.install-stamp"'
if quiet "${CO[@]}" run --rm --no-deps --entrypoint sh frontend -c "$INSTALL"; then
  log_ok "npm install (root workspace)"
else
  log_fail "npm install failed" "inspect: make logs-ui"; exit 1
fi

log_step "Restarting the Nuxt apps so they pick the new dependencies up"
"${CO[@]}" restart frontend manager landing >/dev/null 2>&1
log_ok "frontend, manager, landing restarted"

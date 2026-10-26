#!/usr/bin/env bash
# =============================================================================
#  make check-tools - what the host needs, split by WHEN it is needed.
# =============================================================================
#  `make check` needs Go (the toolchain go.mod pins), git and python3 and
#  nothing else, so the inner loop runs on a machine with no Docker daemon.
#  Docker, mkcert and openssl are wanted for the dev stack and the container
#  tests (`make builddev`, `make test`, `make ci`) and reported, not required.
#  The provisioning tools (ssh, rsync, dig, curl, gh, age) are advisory unless
#  DEPLOY=1, which turns them into requirements.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/require.sh"; . "$HERE/../lib/go.sh"

rc=0
log_step "Needed now"
if [ -n "${GO:-}" ]; then
  want="$(sed -nE 's/^toolchain go([0-9.]+)$/\1/p' "$ROOT/backend/go.mod")"
  have="$("$GO" version | sed -nE 's/^go version go([0-9.]+) .*/\1/p')"
  if [ "$have" = "$want" ]; then
    log_ok "go $have ($GO)"
  else
    # Not a failure: `go` honours the toolchain line and fetches the pinned
    # version itself. Said out loud because the first run will be slow.
    log_warn "go $have found, go.mod pins $want - go will download the pinned toolchain on first use"
  fi
else
  log_fail "go is not installed" \
    "download https://go.dev/dl/ (linux-amd64 tarball) and unpack into ~/.local/go:" \
    "  tar -C ~/.local -xzf go1.27.3.linux-amd64.tar.gz" \
    "no PATH change needed - scripts/lib/go.sh looks there" || true
  rc=1
fi
require_tool git "apt-get install git" || rc=1
require_tool python3 "apt-get install python3" "check-docs.sh parses markdown with it" || rc=1

log_step "Needed for the dev stack and the tests against a database"
want_tool docker "https://docs.docker.com/engine/install/" "make builddev, make test and make ci need a daemon; make check does not"
want_tool mkcert "https://github.com/FiloSottile/mkcert#installation" "make certs - the dev stack's locally-trusted certificate"
want_tool openssl "apt-get install openssl" "make env generates secrets with it; make certs reads the certificate"

# Only the machine that provisions a server needs these - the inner loop and
# CI never do. DEPLOY=1 turns the wants into requirements.
if [ "${DEPLOY:-0}" = 1 ]; then
  log_step "Needed to provision and deploy (DEPLOY=1)"
  require_tool ssh "apt-get install openssh-client" || rc=1
  require_tool rsync "apt-get install rsync" "SYNC=rsync provisioning; the deploy key is still copied with it" || rc=1
  require_tool dig "apt-get install dnsutils" "dns-check.sh asks the authoritative nameserver" || rc=1
  require_tool curl "apt-get install curl" "preflight reads the repository and GitHub's metadata with it" || rc=1
  want_tool gh "https://cli.github.com" "make deploy-key registers the key; without it the key is printed to add by hand"
  want_tool age "apt-get install age" "make backup-verify decrypts the off-box dump locally"
else
  log_step "Needed to provision and deploy (advisory - DEPLOY=1 makes them required)"
  want_tool ssh "apt-get install openssh-client" "every remote target"
  want_tool dig "apt-get install dnsutils" "make dns-check, make preflight"
fi

echo
[ $rc -eq 0 ] && log_ok "the host can run make check" || log_fail "install what is missing above"
exit $rc

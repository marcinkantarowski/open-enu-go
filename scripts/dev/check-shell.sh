#!/usr/bin/env bash
# =============================================================================
#  ShellCheck over every script in the repository.
# =============================================================================
#  This project is a lot of shell, and the shell is the part that runs against
#  real servers with root. The failure modes it catches - an unquoted expansion,
#  a `cd` that silently did not happen, a masked exit status - are exactly the
#  ones that turn "provision a box" into "delete the wrong directory".
#
#  Run through Docker rather than requiring a local install: Docker is already a
#  hard requirement here, and a linter that is optional is a linter that is not
#  run.
#
#  A run that cannot happen is a FAILURE, never a pass. Treating "no output" as
#  "clean" makes a Docker error and a clean tree indistinguishable - the same
#  shape as a smoke test that reports a dead API as healthy.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

IMAGE="koalaman/shellcheck:stable"

log_step "ShellCheck"

# A bare in-place sed is GNU-only: BSD sed (macOS) reads the expression as the
# backup suffix and the file name as the script, and `make env` died that way
# on the first developer Mac. scripts/lib/os.sh has sed_inplace for it.
# scripts/remote is not scanned: its sed runs on the Linux server, over ssh.
# Comment lines are skipped, and so is the helper's own definition.
bare="$(cd "$ROOT" && grep -rnE '(^|[^[:alnum:]_])sed +-[A-Za-z]*i( |$)' scripts/dev scripts/lib --include='*.sh' \
        | grep -v '^scripts/lib/os.sh:' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*#' || true)"
if [ -n "$bare" ]; then
  printf '%s\n' "$bare" >&2
  echo
  log_fail "a bare in-place sed in the scripts above - BSD sed on macOS rejects it" \
    "call sed_inplace from scripts/lib/os.sh instead (source it first)"
  exit 1
fi
log_ok "every in-place sed goes through sed_inplace"

command -v docker >/dev/null 2>&1 || die "docker is required to run shellcheck"

mapfile -t FILES < <(cd "$ROOT" && find scripts docker -name '*.sh' -not -path '*/node_modules/*' | sort)
[ ${#FILES[@]} -gt 0 ] || die "found no shell scripts to check - this script is in the wrong place"

OUT="$(mktemp)"; trap 'rm -f "$OUT"' EXIT

# -S warning: style notes are opinions, warnings are bugs. Raising this to
# `info` would flood the output and train people to ignore it.
docker run --rm -v "$ROOT:/mnt" -w /mnt "$IMAGE" -S warning -x "${FILES[@]}" >"$OUT" 2>&1
status=$?

case "$status" in
  0)
    log_ok "${#FILES[@]} scripts clean at -S warning"
    ;;
  1)
    # Exit 1 is the tool's own code for "found something" - the only non-zero
    # status that means it actually ran.
    cat "$OUT" >&2
    echo
    log_fail "shellcheck found problems in the scripts above" \
      "each one links to an explanation; fix the code rather than adding a disable directive"
    exit 1
    ;;
  *)
    cat "$OUT" >&2
    echo
    log_fail "shellcheck could not run (exit $status)" \
      "a linter that cannot run is not a linter that passed" \
      "check that docker can pull and mount: docker run --rm $IMAGE --version"
    exit 1
    ;;
esac

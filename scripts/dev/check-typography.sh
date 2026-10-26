#!/usr/bin/env bash
# =============================================================================
#  Typography - one dash, and it is the one on the keyboard.
# =============================================================================
#  This project writes "-" everywhere: code, comments, docs, translations. The
#  long dash (U+2014) is what editors and language models reach for on their
#  own, so without a check it is back within a week, and a tree with both is
#  worse than a tree with either - every grep for a phrase has to be run twice.
#
#  The character is never written in this file: it is built from its bytes, so
#  that the check cannot trip over itself and a future bulk replace cannot
#  rewrite the pattern into a hyphen that matches every line in the repository.
#  (.ai/platform/lessons/sentinel-strings-must-not-be-renameable.md)
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

log_step "Typography"

LONG_DASH="$(printf '\342\200\224')"
ESCAPED='\\u''2014'   # how a JSON encoder writes it - split, so this line is not a hit

hits="$(grep -rnI -e "$LONG_DASH" -e "$ESCAPED" "$ROOT" \
  --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=vendor --exclude-dir=var \
  --exclude-dir=.nuxt --exclude-dir=.output --exclude-dir=.out --exclude-dir=.idea \
  --exclude-dir=.data --exclude-dir=dist --exclude-dir=.cache --exclude-dir=.results \
  --exclude-dir=.report 2>/dev/null)"
status=$?

# grep: 0 = found, 1 = clean, anything else = it could not look.
if [ "$status" -gt 1 ]; then
  log_fail "could not search the tree (grep exited $status)"
  exit 1
fi
if [ -n "$hits" ]; then
  log_fail "$(wc -l <<<"$hits" | tr -d ' ') line(s) use the long dash (U+2014)" \
    "write a plain hyphen instead: ' - '" || true
  sed "s|^$ROOT/||" <<<"$hits" | cut -c1-140 | head -20 | sed 's/^/      /' >&2
  exit 1
fi
log_ok "no long dashes"

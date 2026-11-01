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

# A date cut from toISOString is the day in UTC: for hours every night it is not
# the day the person is living in, and a deadline is written one day off.
days="$(grep -rnI -e 'toISOString().slice(0, 10)' -e 'toISOString().substring(0, 10)' -e "toISOString().split('T')" \
  "$ROOT/frontend/app" "$ROOT/manager/app" "$ROOT/ui-kit/app" "$ROOT/landing/app" \
  --include='*.vue' --include='*.ts' --exclude-dir=node_modules 2>/dev/null)"
if [ -n "$days" ]; then
  log_fail "$(wc -l <<<"$days" | tr -d ' ') line(s) take a calendar day from UTC" \
    "ask Intl.DateTimeFormat for the day in the user's zone" || true
  sed "s|^$ROOT/||" <<<"$days" | cut -c1-140 | head -20 | sed 's/^/      /' >&2
  exit 1
fi
log_ok "no calendar day taken from UTC"

# A date written the way each browser or locale likes is a different date on
# every screen. One format, from one place in each project.
local_dates="$(grep -rnIE 'new Date\([^)]*\)\.toLocale|"02\.01\.2006' \
  "$ROOT/frontend/app" "$ROOT/manager/app" "$ROOT/ui-kit/app" "$ROOT/backend/internal/module" \
  --include='*.vue' --include='*.ts' --include='*.go' --exclude='*_test.go' --exclude-dir=node_modules 2>/dev/null)"
if [ -n "$local_dates" ]; then
  log_fail "$(wc -l <<<"$local_dates" | tr -d ' ') line(s) write a date in a format of their own" \
    "write YYYY-MM-DD: Intl.DateTimeFormat('sv-SE') on a screen, \"2006-01-02\" in Go" || true
  sed "s|^$ROOT/||" <<<"$local_dates" | cut -c1-140 | head -20 | sed 's/^/      /' >&2
  exit 1
fi
log_ok "every date is written one way"

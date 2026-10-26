#!/usr/bin/env bash
# =============================================================================
#  Translation completeness (ADR-0008).
# =============================================================================
#  Two locales ship from the start so that adding a third is a file rather than
#  a project. That only holds while they stay in step: a key added to en and
#  forgotten in pl surfaces as a raw translation key in the UI, in production,
#  to exactly the people who read that language.
#
#  Every catalogue is compared with its counterpart, keys flattened to dotted
#  paths:
#    backend   <module>/i18n/messages.<locale>.json  (the registry also refuses
#              a mismatch at start-up; this says so before anything starts)
#    apps      <layer>/i18n/locales/<locale>.json    (ui-kit, the three apps and
#              every module layer - where @nuxtjs/i18n looks by default)
#    landing   landing/content/<locale>/ - one markdown file per page; the site
#              has no fallback to English, so a page missing in one locale is a
#              404 in that language
#
#  Finding nothing to compare is a failure, not a pass: a check that stopped
#  looking would report green forever.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

LOCALES=(en pl)
BASE="${LOCALES[0]}"
fails=0
compared=0

log_step "Translation key parity"

# compare <label> <base file> <other file>
compare() {
  local label="$1" base="$2" other="$3" report
  compared=$((compared + 1))
  if [ ! -f "$other" ]; then
    log_fail "$label: ${other#"$ROOT"/} is missing" "every locale file needs its counterpart" || true
    fails=$((fails + 1)); return
  fi
  report="$(python3 -I - "$base" "$other" <<'PY'
import json, sys

def keys(path):
    try:
        with open(path, encoding='utf-8') as fh:
            data = json.load(fh)
    except Exception as exc:
        print(f"cannot parse {path}: {exc}")
        sys.exit(0)
    if not isinstance(data, dict):
        print(f"{path} must contain a JSON object")
        sys.exit(0)
    out = set()
    def walk(prefix, node):
        for k, v in node.items():
            path = f"{prefix}.{k}" if prefix else k
            if isinstance(v, dict):
                walk(path, v)
            else:
                out.add(path)
    walk('', data)
    return out

a, b = keys(sys.argv[1]), keys(sys.argv[2])
name = lambda p: p.rsplit('/', 1)[-1]
for k in sorted(a - b):
    print(f"only in {name(sys.argv[1])}: {k}")
for k in sorted(b - a):
    print(f"only in {name(sys.argv[2])}: {k}")
PY
)"
  if [ -n "$report" ]; then
    log_fail "$label: ${base##*/} and ${other##*/} disagree" || true
    sed 's/^/      /' <<<"$report" >&2
    fails=$((fails + 1))
  else
    log_ok "$label"
  fi
}

while IFS= read -r base; do
  rel="${base#"$ROOT"/}"
  dir="$(dirname "$base")"
  case "$base" in
    */messages."$BASE".json) prefix="messages."; label="${rel%/i18n/*}" ;;
    *)                       prefix="";          label="${rel%/i18n/locales/*}" ;;
  esac
  for loc in "${LOCALES[@]:1}"; do
    compare "$label ($BASE - $loc)" "$base" "$dir/${prefix}${loc}.json"
  done
done < <(find "$ROOT/backend" "$ROOT/ui-kit" "$ROOT/frontend" "$ROOT/manager" "$ROOT/landing" \
           \( -name node_modules -o -name .nuxt -o -name .output -o -name testdata \) -prune -o \
           \( -path "*/i18n/locales/$BASE.json" -o -path "*/i18n/messages.$BASE.json" \) -print 2>/dev/null | sort)

content="$ROOT/landing/content"
if [ -d "$content/$BASE" ]; then
  for loc in "${LOCALES[@]:1}"; do
    compared=$((compared + 1))
    report="$(diff <(cd "$content/$BASE" && find . -name '*.md' | sort) \
                   <(cd "$content/$loc" 2>/dev/null && find . -name '*.md' | sort) \
              | sed -n "s|^< \./|only in $BASE: |p; s|^> \./|only in $loc: |p")"
    if [ -n "$report" ]; then
      log_fail "landing/content: $BASE and $loc hold different pages" || true
      sed 's/^/      /' <<<"$report" >&2
      fails=$((fails + 1))
    else
      log_ok "landing/content ($BASE - $loc)"
    fi
  done
fi

echo
if [ "$compared" -eq 0 ]; then
  log_fail "no locale catalogue found to compare" "is this the repository root? the layout this check reads may have moved" || true
  exit 1
fi
if [ "$fails" -eq 0 ]; then
  log_ok "$compared locale pair(s) agree"
  exit 0
fi
log_fail "$fails locale mismatch(es)"
exit 1

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
#  One more thing surfaces as a raw key: a violation. An error's message is
#  translated by the API, but details.violations carries keys and the page
#  translates them with t(key), so each key a module can put there needs a text
#  in an app's catalogue too: a key that is a map's value, is assigned to a field
#  (f["name"] = keyX) or follows a field's name in a call (taken(err, "name", keyX)).
#  A key passed to an httpx constructor is the API's to translate.
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

log_step "Translation of validation messages"
report="$(python3 -I - "$ROOT" <<'PY'
import glob, json, os, re, sys

root = sys.argv[1]

def flat(node, prefix=''):
    for k, v in node.items():
        path = f"{prefix}.{k}" if prefix else k
        if isinstance(v, dict):
            yield from flat(v, path)
        else:
            yield path

texts = set()
for app in ('frontend', 'manager', 'ui-kit'):
    for path, dirs, files in os.walk(os.path.join(root, app), followlinks=True):
        dirs[:] = [d for d in dirs if d not in ('node_modules', '.nuxt', '.output')]
        if path.endswith(os.path.join('i18n', 'locales')) and 'en.json' in files:
            with open(os.path.join(path, 'en.json'), encoding='utf-8') as fh:
                texts.update(flat(json.load(fh)))

for module in sorted(glob.glob(os.path.join(root, 'backend/internal/module/*/'))):
    source = ''
    for path, dirs, files in os.walk(module):
        dirs[:] = [d for d in dirs if d != 'testdata']
        for name in files:
            if name.endswith('.go') and not name.endswith(('_test.go', '.gen.go')):
                with open(os.path.join(path, name), encoding='utf-8') as fh:
                    source += re.sub(r'//.*', '', fh.read()) + '\n'
    if 'ValidationFailed(' not in source:
        continue
    for name, key in re.findall(r'\b(\w+)\s+i18n\.Key\s*=\s*"([^"]+)"', source):
        violation = re.search(rf'(?::\s*|\]\s*=\s*|",\s*){name}\b', source)
        if violation and key not in texts:
            print(f"{os.path.basename(module.rstrip('/'))}: {key}")
PY
)"
if [ -n "$report" ]; then
  log_fail "a validation message has no text in an app" "add it to the module's i18n/locales/{en,pl}.json, or the page shows the raw key" || true
  sed 's/^/      /' <<<"$report" >&2
  fails=$((fails + 1))
else
  log_ok "every validation message has a text in an app"
fi

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

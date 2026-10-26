#!/usr/bin/env bash
# =============================================================================
#  Documentation integrity.
# =============================================================================
#  Three failures, each of which makes documentation actively worse than none:
#
#   1. A link to a file that no longer exists. An agent routed to a missing
#      guide does not fall back gracefully - it improvises.
#   2. A MODULE.md that does not name something the module's contract/ package
#      exports. contract/ is the surface other modules may import; an exported
#      identifier nobody documented is a coupling nobody agreed to.
#   3. A record on the wrong side of the platform/project line. .ai/platform/
#      is replaced by a platform update, so a project's own record left there
#      is lost - and one left outside it here is shipped to everybody.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

fails=0

# ── 1. relative links resolve ───────────────────────────────────────────────
log_step "Markdown link integrity"

broken="$(python3 -I - "$ROOT" <<'PY'
import os, re, sys
root = sys.argv[1]
skip = {'.git', 'node_modules', 'vendor', '.out', '.nuxt', '.output', '.idea', 'var', 'bin'}
link = re.compile(r'\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)')
bad = []
for dirpath, dirnames, filenames in os.walk(root):
    dirnames[:] = [d for d in dirnames if d not in skip]
    for fn in filenames:
        if not fn.endswith('.md'):
            continue
        path = os.path.join(dirpath, fn)
        try:
            text = open(path, encoding='utf-8').read()
        except Exception:
            continue
        for m in link.finditer(text):
            target = m.group(1)
            if target.startswith(('http://', 'https://', 'mailto:', '#')):
                continue
            target = target.split('#', 1)[0]
            if not target:
                continue
            rel = os.path.relpath(dirpath, root).split(os.sep)
            if target.startswith('/') and rel[:2] == ['landing', 'content'] and len(rel) > 2:
                page = target.strip('/') or 'index'
                resolved = os.path.join(root, 'landing', 'content', rel[2], page + '.md')
            else:
                resolved = os.path.normpath(os.path.join(dirpath, target))
            if not os.path.exists(resolved):
                line = text[:m.start()].count('\n') + 1
                bad.append(f"{os.path.relpath(path, root)}:{line} -> {m.group(1)}")
for b in bad:
    print(b)
PY
)"

if [ -n "$broken" ]; then
  log_fail "$(wc -l <<<"$broken") broken link(s)" || true
  sed 's/^/      /' <<<"$broken" >&2
  fails=$((fails + 1))
else
  log_ok "every relative link resolves"
fi

# ── 2. platform and project knowledge stay apart ────────────────────────────
log_step "Platform and project knowledge"

initialized="$(sed -nE 's/.*"initialized"[[:space:]]*:[[:space:]]*(true|false).*/\1/p' "$ROOT/.project.json" 2>/dev/null | head -1)"
if [ -z "$initialized" ]; then
  log_fail ".project.json does not say whether this is the platform or a project" \
    "it must carry \"initialized\": true or false" || true
  fails=$((fails + 1))
fi

# knowledge_set <directory> <index>: every record is a row in its OWN index.
knowledge_set() {
  local dir="$1" index="$2" f ok=1
  if [ ! -f "$index" ]; then
    log_fail "${index#"$ROOT"/} is missing" "it is the index of ${dir#"$ROOT"/}/" || true
    fails=$((fails + 1)); return
  fi
  shopt -s nullglob
  for f in "$dir"/*.md; do
    [ "$f" = "$index" ] && continue
    if ! grep -qF "$(basename "$f"))" "$index"; then
      log_fail "${f#"$ROOT"/} is not a row in ${index#"$ROOT"/}" \
        "an unindexed record is never read - add the row, or delete the file" || true
      ok=0; fails=$((fails + 1))
    fi
  done
  shopt -u nullglob
  [ "$ok" -eq 1 ] && log_ok "${dir#"$ROOT"/}/ is fully indexed"
}

for kind in adr specs lessons analysis; do
  knowledge_set "$ROOT/.ai/platform/$kind" "$ROOT/.ai/platform/$kind/README.md"
done
for kind in adr specs lessons analysis; do
  knowledge_set "$ROOT/.ai/$kind" "$ROOT/.ai/$kind/README.md"
done

if [ "$initialized" = false ]; then
  strays="$(find "$ROOT/.ai" -path "$ROOT/.ai/platform" -prune -o -type f ! -name README.md ! -name inventory.json -print 2>/dev/null | sort)"
  if [ -n "$strays" ]; then
    while IFS= read -r f; do
      log_fail "${f#"$ROOT"/} is a project record in the platform's repository" \
        "this repository is the platform (\"initialized\": false) - it belongs under .ai/platform/" || true
      fails=$((fails + 1))
    done <<<"$strays"
  else
    log_ok ".ai/ outside platform/ is empty, as the platform ships it"
  fi
fi

# ── 3. MODULE.md names everything contract/ exports ─────────────────────────
log_step "MODULE.md against the module's exported contract"

shopt -s nullglob
modules=("$ROOT"/backend/internal/module/*/)
shopt -u nullglob

for dir in "${modules[@]}"; do
  name="$(basename "$dir")"
  doc="$dir/MODULE.md"
  [ -f "$doc" ] || continue
  ok=1
  # Exported top-level identifiers: `type X`, `func X`, `var X`, `const X`,
  # and the names inside a grouped `var (` / `const (` block are not caught -
  # contract/ is expected to stay small enough that a grouped block is a smell.
  while IFS= read -r ident; do
    if ! grep -qF "$ident" "$doc"; then
      log_fail "$name: contract/ exports $ident, and MODULE.md does not mention it" \
        "other modules may import it, so it must be documented as published surface" || true
      ok=0; fails=$((fails + 1))
    fi
  done < <(grep -hoE '^(type|func|var|const) [A-Z][A-Za-z0-9_]*' "$dir"contract/*.go 2>/dev/null | awk '{print $2}' | sort -u)
  [ "$ok" -eq 1 ] && log_ok "$name"
done

echo
if [ "$fails" -eq 0 ]; then
  log_ok "documentation matches the code"
  exit 0
fi
log_fail "$fails documentation problem(s)"
exit 1

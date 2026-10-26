#!/usr/bin/env bash
# Context budgets (.ai/platform/docs/guardrails.md). A file past its budget is a
# design signal, not a formatting problem: AGENTS.md past 32 KB is silently
# truncated by tooling, and the rules in the truncated part stop applying without any error.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

rc=0
check() { # <path> <max-bytes> <why>
  local f="$1" max="$2" why="$3"
  [ -f "$ROOT/$f" ] || { log_skip "$f (does not exist yet)"; return 0; }
  local n; n=$(wc -c < "$ROOT/$f")
  local pct=$(( n * 100 / max ))
  if [ "$n" -gt "$max" ]; then
    log_fail "$f is ${n}B, over its ${max}B budget" "$why"; rc=1
  elif [ "$pct" -ge 85 ]; then
    log_warn "$f is ${n}B - ${pct}% of its ${max}B budget"
  else
    log_ok "$f ${n}B (${pct}% of ${max}B)"
  fi
}

log_step "Context budgets"
check AGENTS.md 32768 "move long-form procedure into .ai/platform/docs/ and link it from the Task Router"

while IFS= read -r f; do
  check "${f#"$ROOT"/}" 8192 "a module needing more than 8KB of explanation is too big - split it"
done < <(find "$ROOT/backend/internal/module" -name MODULE.md 2>/dev/null | sort)

# ── module size ─────────────────────────────────────────────────────────────
# A module is the unit an agent holds in working memory. Over the line is a
# design signal, not a documentation problem: the fix is to split it.
#
# `example` gets a much tighter budget than the rest because it is read on EVERY
# task, not only when working on it. The cap is the module as it stands - one
# demonstration of every platform concern in .ai/platform/ARCHITECTURE.md §4,
# the searchable declaration and a task on a clock included - measured and
# rounded up to the next 50. Adding a demonstration means removing one, or
# arguing that the specification grew and raising the cap in the same change.
# (0 means "not set - report, do not fail".)
MODULE_LOC=3000
MODULE_FILES=40
EXAMPLE_LOC=1150

# Counted: every .go file including tests - tests are part of what a reader
# holds. Excluded, deliberately: migrations (an append-only ledger nobody reads
# to understand a module) and generated code (`*.gen.go`, `*_gen.go` - read by
# the compiler, not by people; it is regenerated, never maintained).
module_files() { # <dir>
  find "$1" -name '*.go' -not -path '*/migrations/*' -not -name '*.gen.go' -not -name '*_gen.go' 2>/dev/null
}

# Non-blank, non-comment lines.
count_loc() { # files on stdin
  tr '\n' '\0' | xargs -0 -r grep -hcve '^\s*$' -e '^\s*//' -e '^\s*/\*' -e '^\s*\*' \
    | awk '{s+=$1} END {print s+0}'
}
module_loc() { module_files "$1" | count_loc; }

log_step "Module size"
while IFS= read -r dir; do
  name="$(basename "$dir")"
  loc=$(module_loc "$dir")
  files=$(module_files "$dir" | wc -l)

  if [ "$name" = example ]; then
    if [ "$EXAMPLE_LOC" -eq 0 ]; then
      log_skip "example ${loc} lines, ${files} files - cap not set (EXAMPLE_LOC=0)"
    elif [ "$loc" -gt "$EXAMPLE_LOC" ]; then
      log_fail "example is ${loc} lines, over its ${EXAMPLE_LOC}-line budget (tests included)" \
        "it is read on every task, so every line here is paid for on every task" \
        "cut the demonstration, not the comments - the comments are what it is for"
      rc=1
    elif [ $(( loc * 100 / EXAMPLE_LOC )) -ge 85 ]; then
      log_warn "example is ${loc} lines - $(( loc * 100 / EXAMPLE_LOC ))% of its ${EXAMPLE_LOC}-line budget"
    else
      log_ok "example ${loc} lines ($(( loc * 100 / EXAMPLE_LOC ))% of ${EXAMPLE_LOC}), ${files} files"
    fi
    continue
  fi

  if [ "$loc" -gt "$MODULE_LOC" ] || [ "$files" -gt "$MODULE_FILES" ]; then
    log_fail "$name is ${loc} lines across ${files} files, over ${MODULE_LOC}/${MODULE_FILES}" \
      "a module past this is two modules; splitting it is the fix, not a bigger budget"
    rc=1
  else
    log_ok "$name ${loc} lines, ${files} files"
  fi
done < <(find "$ROOT/backend/internal/module" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort)

# ── kernel size ─────────────────────────────────────────────────────────────
# The kernel is read by every agent working on any module, and its whole claim
# is that each platform service is a THIN contract. A file drifting past this
# cap means an implementation is growing where a module should be.
#
# The file cap keeps each contract readable in one sitting. The total is sized
# for the contracts .ai/platform/ARCHITECTURE.md §5 lists - mail, rate limiting,
# password hashing, the download route and persisted progress among them - with
# room to grow, but not for an implementation that belongs in a module.
# Generated code and tests are not counted.
KERNEL_FILE_LOC=260
KERNEL_TOTAL_LOC=8000

log_step "Kernel size"
over=0
total=0
while IFS= read -r f; do
  loc=$(grep -cve '^\s*$' -e '^\s*//' -e '^\s*/\*' -e '^\s*\*' "$f" 2>/dev/null || echo 0)
  total=$(( total + loc ))
  if [ "$loc" -gt "$KERNEL_FILE_LOC" ]; then
    log_fail "${f#"$ROOT"/} is ${loc} lines, over the ${KERNEL_FILE_LOC}-line cap for a kernel file" \
      "platform services are contracts, not implementations - if this one needs that much code," \
      "the behaviour probably belongs in a module that implements the contract"
    over=$((over + 1)); rc=1
  fi
done < <(find "$ROOT/backend/kernel" -name '*.go' -not -name '*_test.go' -not -name '*.gen.go' -not -name '*_gen.go' -not -path '*/testdata/*' 2>/dev/null | sort)

if [ "$total" -gt "$KERNEL_TOTAL_LOC" ]; then
  log_fail "the kernel is ${total} lines, over its ${KERNEL_TOTAL_LOC}-line budget" \
    "every agent reads this package; growth here is paid on every task"
  rc=1
elif [ "$over" -eq 0 ]; then
  pct=$(( total * 100 / KERNEL_TOTAL_LOC ))
  log_ok "kernel ${total} lines (${pct}% of ${KERNEL_TOTAL_LOC}), largest file under ${KERNEL_FILE_LOC}"
fi

echo
[ $rc -eq 0 ] && log_ok "all budgets within limits" || log_fail "fix the files above"
exit $rc

#!/usr/bin/env bash
# Consistent, greppable output for every script in this repo.
# Colour is disabled automatically when stdout is not a TTY (CI, pipes, logs).

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_RESET=$'\033[0m'; C_DIM=$'\033[2m';  C_BOLD=$'\033[1m'
  C_RED=$'\033[31m';  C_GRN=$'\033[32m'; C_YEL=$'\033[33m'; C_BLU=$'\033[34m'
else
  C_RESET=''; C_DIM=''; C_BOLD=''; C_RED=''; C_GRN=''; C_YEL=''; C_BLU=''
fi

log_step()  { printf '%s==>%s %s\n'  "$C_BLU$C_BOLD" "$C_RESET" "$*"; }
log_ok()    { printf '%s  ✓%s %s\n'  "$C_GRN" "$C_RESET" "$*"; }
log_skip()  { printf '%s  ·%s %s\n'  "$C_DIM" "$C_RESET" "$*"; }
log_warn()  { printf '%s  !%s %s\n'  "$C_YEL" "$C_RESET" "$*" >&2; }
log_info()  { printf '    %s\n' "$*"; }

# log_fail "what went wrong" ["how to fix it" ...]
# Every failure must tell the operator what to DO, not just what happened.
log_fail() {
  printf '%s  ✗ %s%s\n' "$C_RED$C_BOLD" "$1" "$C_RESET" >&2
  shift
  for line in "$@"; do printf '    %s\n' "$line" >&2; done
  return 1
}

die() { log_fail "$@"; exit 1; }

# quiet <command...> - run a noisy command: its last few lines when it succeeds,
# its last forty when it fails. A bare `| tail -2` keeps the tail of the output
# either way, and a framework's boxed error output is mostly blank padding - the
# message it cut off was the only line that mattered ("could not generate the keypair",
# with the reason a screen further up).
quiet() {
  local out rc
  out="$(mktemp)"
  "$@" >"$out" 2>&1; rc=$?
  if [ "$rc" -eq 0 ]; then tail -4 "$out"; else grep -v '^[[:space:]]*$' "$out" | tail -40; fi
  rm -f "$out"
  return "$rc"
}

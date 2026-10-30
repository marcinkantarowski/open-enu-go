#!/usr/bin/env bash
# The record that `make ci` passed, so a deploy right after it need not run it
# again. Time alone would ship whatever changed since: the record names the
# exact content the gate saw, and a deploy compares it with what it ships.
# It lives inside .git, so it is never committed and never reaches a server.

CI_FRESH_SECONDS="${CI_FRESH_SECONDS:-900}"

ci_stamp_file() { git -C "$1" rev-parse --path-format=absolute --git-path ci-passed 2>/dev/null; }

# ci_tree <root> - the tree a commit of everything here would have, through a
# copy of the index: the real one, and so what is staged, stays as it is.
ci_tree() {
  local index tree
  index="$(mktemp)" || return 1
  cp "$(git -C "$1" rev-parse --path-format=absolute --git-path index 2>/dev/null)" "$index" 2>/dev/null \
    && GIT_INDEX_FILE="$index" git -C "$1" add -A 2>/dev/null \
    && tree="$(GIT_INDEX_FILE="$index" git -C "$1" write-tree 2>/dev/null)"
  rm -f "$index"
  [ -n "${tree:-}" ] && printf '%s' "$tree"
}

ci_stamp_clear() {
  local file
  file="$(ci_stamp_file "$1")" && rm -f "$file"
}

# ci_stamp_write <root> <tree>
ci_stamp_write() {
  local file
  file="$(ci_stamp_file "$1")" && printf '%s %s\n' "$2" "$(date +%s)" >"$file"
}

# ci_stamp_fresh <root> <tree> - the gate passed on this tree, recently.
ci_stamp_fresh() {
  local file tree stamped
  [ -n "$2" ] || return 1
  file="$(ci_stamp_file "$1")" && [ -f "$file" ] || return 1
  read -r tree stamped <"$file" || return 1
  [ "$tree" = "$2" ] && [ $(($(date +%s) - stamped)) -lt "$CI_FRESH_SECONDS" ]
}

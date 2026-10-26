#!/usr/bin/env bash
# Where the Go toolchain is. Go is installed natively, not run through Docker
# (.ai/platform/ARCHITECTURE.md §2): `make check` has to finish in seconds, and a
# toolchain behind a container start does not. It is NOT required to be on PATH - a
# tarball unpacked into ~/.local/go or /usr/local/go is found here, so the
# Makefile and every script agree on one binary.
#
# Sourced: defines go_resolve and exports GO/GOFMT.  Executed: prints the path.
go_resolve() {
  local c
  for c in "${GO:-}" "$(command -v go 2>/dev/null || true)" \
           "$HOME/.local/go/bin/go" "/usr/local/go/bin/go"; do
    if [ -n "$c" ] && [ -x "$c" ]; then printf '%s\n' "$c"; return 0; fi
  done
  return 1
}

if GO="$(go_resolve)"; then
  export GO
  GOFMT="$(dirname "$GO")/gofmt"; export GOFMT
fi

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  go_resolve || { echo "go: not found - install Go 1.27+ (https://go.dev/dl) into ~/.local/go or /usr/local/go" >&2; exit 1; }
fi

#!/usr/bin/env bash
# Tool presence checks, split by WHEN the tool is needed. A missing rclone must
# not block `make builddev` on day one, and a missing docker must not be
# discovered halfway through provisioning a server.

# require_tool <bin> <apt-package-or-url> [note]
require_tool() {
  local bin="$1" how="$2" note="${3:-}"
  if command -v "$bin" >/dev/null 2>&1; then
    log_ok "$bin"
    return 0
  fi
  log_fail "$bin is not installed" "install: $how" ${note:+"$note"}
  return 1
}

# want_tool - advisory: reports, never fails.
want_tool() {
  local bin="$1" how="$2" why="${3:-}"
  if command -v "$bin" >/dev/null 2>&1; then
    log_ok "$bin"
  else
    log_warn "$bin not installed - $why"
    log_info "install: $how"
  fi
  return 0
}

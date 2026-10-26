#!/usr/bin/env bash
# Secret generation. One rule, and it is the rule that makes `buildprod` safe to
# re-run against a live server: a value that already exists is NEVER regenerated.

# gen_secret [bytes] -> URL-safe random string
gen_secret() {
  local bytes="${1:-32}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 "$bytes" | tr -d '\n=' | tr '+/' '-_'
  else
    head -c "$bytes" /dev/urandom | base64 | tr -d '\n=' | tr '+/' '-_'
  fi
}

# gen_hex [bytes]
gen_hex() {
  local bytes="${1:-32}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$bytes"
  else
    head -c "$bytes" /dev/urandom | od -An -tx1 | tr -d ' \n'
  fi
}

#!/usr/bin/env bash
# =============================================================================
#  Locally-trusted wildcard certificate for *.${DOMAIN}
# =============================================================================
#  On WSL2 this is the step people lose an afternoon to. mkcert keeps its CA in
#  a CAROOT directory and trusts it for the platform it runs on. Running the
#  Linux mkcert trusts the CA for Linux; running mkcert.exe would create and
#  trust a DIFFERENT CA for Windows. The browser is on Windows and the
#  certificate is signed by the Linux CA, so it stays untrusted either way.
#
#  The fix is to install the *Linux* CA into the Windows store - same CA, both
#  platforms - which is what the WSL2 branch below does.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/os.sh"

[ -f "$ROOT/.env" ] || die ".env is missing" "run: make env"
DOMAIN="$(sed -n 's/^DOMAIN=//p' "$ROOT/.env" | head -1)"
[ -n "$DOMAIN" ] || die "DOMAIN is not set in .env"

CERT_DIR="$ROOT/docker/certs"
CERT="$CERT_DIR/local-cert.pem"
KEY="$CERT_DIR/local-key.pem"
mkdir -p "$CERT_DIR"

command -v mkcert >/dev/null 2>&1 || die "mkcert is not installed" \
  "install: https://github.com/FiloSottile/mkcert#installation"

# ── 1. the certificate ──────────────────────────────────────────────────────
needs_cert=1
if [ -f "$CERT" ] && [ -f "$KEY" ]; then
  # Regenerate when the domain changed (e.g. after `make init`), otherwise the
  # old certificate silently fails to match and every host shows a warning.
  if openssl x509 -in "$CERT" -noout -text 2>/dev/null | grep -qF "DNS:*.$DOMAIN"; then
    needs_cert=0
    exp="$(openssl x509 -in "$CERT" -noout -enddate | cut -d= -f2)"
    log_skip "certificate already covers *.$DOMAIN (expires $exp)"
  else
    log_warn "existing certificate does not cover *.$DOMAIN - regenerating"
  fi
fi

if [ "$needs_cert" -eq 1 ]; then
  log_step "Generating certificate for $DOMAIN and *.$DOMAIN"
  mkcert -cert-file "$CERT" -key-file "$KEY" "$DOMAIN" "*.$DOMAIN" >/dev/null 2>&1 \
    || die "mkcert failed to generate the certificate"
  chmod 644 "$CERT"; chmod 600 "$KEY"
  log_ok "wrote docker/certs/local-{cert,key}.pem"
fi

# ── 2. trust the CA on this platform ────────────────────────────────────────
CAROOT="$(mkcert -CAROOT)"

# Copied beside the certificate it signed, so the containers can trust it too.
# Without this the API cannot call its own stack over HTTPS - which is not an
# edge case: a webhook endpoint pointed at this dev machine fails with an
# unhelpful "unable to get local issuer certificate" five retries in a row.
if [ -f "$CAROOT/rootCA.pem" ]; then
  cp "$CAROOT/rootCA.pem" "$ROOT/docker/certs/rootCA.pem"
  chmod 644 "$ROOT/docker/certs/rootCA.pem"
fi

log_step "Trusting the mkcert CA"
if install_out="$(mkcert -install 2>&1)"; then
  log_ok "$(os_kind) trust store"
  # mkcert exits 0 when it could not reach the NSS store Chrome and Firefox on
  # Linux read, and says so only in its output, which is why that is kept. The
  # system store is fine, so curl works - and a Linux browser (the one an MCP
  # or a headed Playwright run drives) still rejects every host.
  if grep -q '"certutil" is not available' <<<"$install_out"; then
    log_warn "Linux Chrome/Firefox will not trust the dev CA: the NSS store needs certutil"
    log_info "  sudo apt install -y libnss3-tools && make certs"
  fi
else
  log_warn "mkcert -install failed on $(os_kind) - curl and the CLI may not trust the cert"
fi

# ── 3. WSL2: the same CA must also be trusted by Windows ────────────────────
if is_wsl2; then
  root_pem="$CAROOT/rootCA.pem"
  [ -f "$root_pem" ] || die "mkcert CA not found at $root_pem"

  # Fingerprint identifies this exact CA in the Windows store, so re-running is
  # a no-op instead of stacking duplicate root certificates.
  fp="$(openssl x509 -in "$root_pem" -noout -fingerprint -sha1 | cut -d= -f2 | tr -d ':')"

  if certutil.exe -user -verifystore Root "$fp" >/dev/null 2>&1; then
    log_skip "Windows already trusts this CA ($fp)"
  else
    log_step "Installing the Linux mkcert CA into the Windows user trust store"
    log_info "Windows may ask you to confirm adding a root certificate - that prompt is this step."
    tmp_win="$(mktemp -d -p /mnt/c/Windows/Temp 2>/dev/null || mktemp -d)"
    cp "$root_pem" "$tmp_win/open-enu-dev-ca.crt"
    win_path="$(wslpath -w "$tmp_win/open-enu-dev-ca.crt" 2>/dev/null || echo "")"
    if [ -n "$win_path" ] && certutil.exe -user -addstore Root "$win_path" >/dev/null 2>&1; then
      log_ok "Windows user trust store now contains the dev CA"
    else
      log_warn "could not install the CA into Windows automatically"
      log_info "the Windows browser will show a certificate warning until you do this:"
      log_info "  1. open PowerShell (no admin needed)"
      log_info "  2. certutil -user -addstore Root \"${win_path:-$tmp_win/open-enu-dev-ca.crt}\""
      log_info "  (or double-click the file and choose Current User -> Trusted Root)"
    fi
    rm -rf "$tmp_win" 2>/dev/null || true
  fi
fi

log_ok "certificates ready"

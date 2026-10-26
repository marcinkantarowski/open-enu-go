#!/usr/bin/env bash
# The host toolchain, declared once.
#
# check-tools.sh reports on these and install-dev-tools.sh installs them. They
# read the same table so that a tool added to one cannot be forgotten in the
# other - the checker would then demand something the installer never offers.
#
#   bin | when | apt package | brew formula | manual hint | why it matters
#
#   when   dev     builddev cannot run without it (check-tools fails)
#          deploy  needed for staging/production (check-tools only reports)
#   apt / brew    empty = not installable that way; the hint is printed instead
#
# docker is not here on purpose: how to install it depends on the host (Docker
# Desktop under WSL2 and macOS, the engine on Linux) and choosing for the user
# breaks more setups than it fixes. check-tools.sh handles it by hand.
host_tools() {
  cat <<'TOOLS'
git|dev|git|git|apt-get install git|
openssl|dev|openssl|openssl|apt-get install openssl|
mkcert|dev|mkcert|mkcert|https://github.com/FiloSottile/mkcert#installation|generates the locally-trusted wildcard certificate for *.${DOMAIN}
certutil|dev-extra|libnss3-tools|nss|apt-get install libnss3-tools|Linux Chrome/Firefox cannot trust the dev CA without it
dig|deploy|dnsutils||apt-get install dnsutils|make preflight cannot verify DNS without it
rclone|deploy|rclone|rclone|https://rclone.org/install/|make backup cannot push off-box without it
age|deploy|age|age|apt-get install age|make backup cannot encrypt dumps without it
gh|deploy|gh|gh|https://cli.github.com/|make deploy-key falls back to manual paste without it
rsync|deploy|rsync|rsync|apt-get install rsync|SYNC=rsync deploys are unavailable without it
TOOLS
}

# tool_field <row> <n>  - 1 bin, 2 when, 3 apt, 4 brew, 5 hint, 6 why
tool_field() { cut -d'|' -f"$2" <<<"$1"; }

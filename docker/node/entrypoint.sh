#!/bin/sh
# =============================================================================
#  Install workspace dependencies if they are missing, then run the command.
# =============================================================================
#  Without this the three Nuxt containers exit 127 on a fresh checkout (`nuxt`
#  is not installed yet) and crash-loop until something installs for them -
#  which means `make up` alone never works, only the full `make builddev`.
#
#  All three share one node_modules volume, so they must not install at the same
#  time: concurrent `npm install` into one tree corrupts it. The flock below
#  makes the first container install while the others wait, then find it done.
# =============================================================================
set -e

mkdir -p /app/node_modules
exec 9>/app/node_modules/.install.lock
flock 9

if [ ! -x /app/node_modules/.bin/nuxt ]; then
  echo "[entrypoint] node_modules is empty - installing workspace dependencies"
  npm install --no-audit --no-fund
  echo "[entrypoint] dependencies installed"
elif [ /app/package.json -nt /app/node_modules/.install-stamp ]; then
  echo "[entrypoint] package.json is newer than the last install - updating"
  npm install --no-audit --no-fund
fi
touch /app/node_modules/.install-stamp

flock -u 9
exec "$@"

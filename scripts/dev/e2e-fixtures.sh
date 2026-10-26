#!/usr/bin/env bash
# e2e-fixtures.sh - the accounts e2e/tests/* log in as, created idempotently
# through the console against the running dev stack. Run before `make e2e`;
# running it twice changes nothing.
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../lib/log.sh
source "$ROOT/scripts/lib/log.sh"
# shellcheck source=../lib/compose.sh
source "$ROOT/scripts/lib/compose.sh"
compose_init "$ROOT"

OWNER_EMAIL="e2e-owner@example.test"
OWNER_PASSWORD="e2e-correct-horse-battery"
OPERATOR_EMAIL="e2e-operator@example.test"
OPERATOR_PASSWORD="e2e-operator-correct-horse-battery"

openenu() { "${COMPOSE[@]}" exec -T api go run ./cmd/openenu "$@"; }

log_step "e2e fixtures"
openenu user create --email "$OWNER_EMAIL" --password "$OWNER_PASSWORD" --name "E2E Owner" \
  --tenant "E2E Primary" --slug e2e-primary --role owner --seed
openenu user create --email "$OWNER_EMAIL" --password "$OWNER_PASSWORD" --name "E2E Owner" \
  --tenant "E2E Second" --slug e2e-second --role owner --seed
log_ok "owner $OWNER_EMAIL is a member of e2e-primary and e2e-second, both seeded"
openenu manager create --email "$OPERATOR_EMAIL" --password "$OPERATOR_PASSWORD" --name "E2E Operator"
log_ok "operator $OPERATOR_EMAIL exists"

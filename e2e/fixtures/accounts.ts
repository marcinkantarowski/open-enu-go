/**
 * The accounts the suite expects to exist.
 *
 * Created by `make e2e-fixtures`, which runs `app:user:create` and
 * `app:manager:create` - both idempotent, so the target can be run before every
 * run without accumulating state.
 *
 * Two workspaces for one person, deliberately: without a second membership the
 * tenant switcher does not render, and the switch test would pass by asserting
 * on something that is never there.
 */
export const OWNER = {
  email: 'e2e-owner@example.test',
  password: 'e2e-correct-horse-battery',
  displayName: 'E2E Owner',
}

export const PRIMARY_TENANT = { name: 'E2E Primary', slug: 'e2e-primary' }
export const SECOND_TENANT = { name: 'E2E Second', slug: 'e2e-second' }

export const OPERATOR = {
  email: 'e2e-operator@example.test',
  password: 'e2e-operator-correct-horse-battery',
}

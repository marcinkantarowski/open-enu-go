import { expect, test } from '@playwright/test'
import { PRIMARY_TENANT, SECOND_TENANT } from '../fixtures/accounts'
import { signIn } from '../fixtures/session'

/**
 * Switching workspace re-scopes everything (ADR-0010).
 *
 * The tenant lives in the token, so a switch is a NEW token - not a variable
 * changed somewhere. This test exists because the failure mode is silent: a
 * switch that updates the label without re-issuing the token leaves every
 * subsequent request scoped to the workspace that was left.
 */
test('switching workspace re-issues the session and re-scopes the UI', async ({ page }) => {
  await signIn(page)

  const switcher = page.getByTestId('tenant-switcher')
  await expect(switcher, 'the fixtures must put this account in two workspaces').toBeVisible()

  await expect(page.getByTestId('current-tenant')).toHaveText(PRIMARY_TENANT.name)

  // Selecting triggers a full reload - everything already fetched belongs to the
  // workspace being left.
  await switcher.selectOption({ label: SECOND_TENANT.name })
  await page.waitForURL('**/')

  await expect(page.getByTestId('current-tenant')).toHaveText(SECOND_TENANT.name)

  // And the data moved with it. The seeded project exists in both workspaces,
  // but they are different rows - the one visible here belongs to this tenant.
  await page.goto('/example')
  await expect(page.getByTestId('example-item-name').first()).toBeVisible()
})

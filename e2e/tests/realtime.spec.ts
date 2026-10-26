import { expect, test } from '@playwright/test'
import { signIn } from '../fixtures/session'

/**
 * The live stream belongs to the tab, not to the page that listens.
 *
 * The shell holds one stream for as long as the signed-in layout is mounted.
 * A page that subscribes and is then left must not take the stream with it -
 * that once left every other page saying "Reconnecting" over a connection
 * nothing would ever reopen.
 */
test('the stream stays live after leaving the one page that listens', async ({ page }) => {
  await signIn(page)
  await page.goto('/example')

  const indicator = page.getByRole('banner').getByTestId('realtime-indicator')
  await expect(indicator).toHaveAttribute('data-status', 'live')

  // A client-side navigation, not a reload: the example page's subscription
  // ends while the shell - and its hold on the stream - stays mounted.
  await page.locator('aside a[href="/"]').first().click()
  await expect(page).toHaveURL(/\/$/)

  await expect(indicator).toHaveAttribute('data-status', 'live')
  await expect(indicator).toContainText(/live/i)
})

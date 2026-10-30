import { expect, test } from '@playwright/test'
import { OPERATOR } from '../fixtures/accounts'
import { signIn, signInAsOperator } from '../fixtures/session'

/**
 * An operator's session has no refresh and ends with its token, so a console
 * left open goes to the login instead of showing pages it can no longer read.
 */
const managerUrl = process.env.E2E_MANAGER_URL ?? 'https://manager.open-enu-go.local'

test('the console goes to the login when the token runs out', async ({ browser }) => {
  const context = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await context.newPage()

  await page.clock.install()
  await signInAsOperator(page, managerUrl, OPERATOR.email, OPERATOR.password)
  await page.clock.fastForward('15:01')

  await expect(page).toHaveURL(/\/login\?expired=1/)
  await expect(page.getByTestId('login-expired')).toBeVisible()

  await context.close()
})

test('a refused token ends the session, and a person signed in to the app does not take its place', async ({ browser }) => {
  const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: process.env.E2E_APP_URL })
  const page = await context.newPage()
  const refreshes: string[] = []

  await signIn(page)
  await signInAsOperator(page, managerUrl, OPERATOR.email, OPERATOR.password)

  page.on('request', (request) => {
    if (request.url().endsWith('/api/auth/refresh')) refreshes.push(request.url())
  })
  await page.route('**/api/manager/operators', route => route.fulfill({
    status: 401,
    json: { error: { code: 'unauthorized', message: 'no' } },
  }))
  await page.locator('a[href="/operators"]').click()

  await expect(page).toHaveURL(/\/login\?expired=1/)
  expect(refreshes).toEqual([])

  await context.close()
})

test('signing out is not an expiry', async ({ browser }) => {
  const context = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await context.newPage()

  await signInAsOperator(page, managerUrl, OPERATOR.email, OPERATOR.password)
  await page.getByTestId('sign-out').click()

  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByTestId('login-expired')).toHaveCount(0)

  await context.close()
})

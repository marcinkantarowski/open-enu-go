import { expect, test } from '@playwright/test'
import { OPERATOR, PRIMARY_TENANT } from '../fixtures/accounts'
import { signInAsOperator } from '../fixtures/session'

/**
 * The one legitimate crossing between the two realms (ADR-0020).
 *
 * Three things have to be true at once, and only a browser can check them
 * together: the operator token works on `/api/manager` and nowhere else, the
 * minted tenant token is a genuine `aud: tenant` token the tenant app accepts, and
 * the person being impersonated - or anyone looking over a shoulder - can see
 * that it is happening.
 */
const managerUrl = process.env.E2E_MANAGER_URL ?? 'https://manager.open-enu-go.local'

test('an operator can view a workspace as one of its members, and the banner says so', async ({ browser }) => {
  const context = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await context.newPage()

  // Both confirmations are deliberate friction in the product; accept them.
  page.on('dialog', dialog => dialog.accept())

  await signInAsOperator(page, managerUrl, OPERATOR.email, OPERATOR.password)

  await page.getByRole('link', { name: PRIMARY_TENANT.name }).first().click()
  await expect(page.getByTestId('impersonate').first()).toBeVisible()

  // The console hands the session to the tenant app in a new tab rather than
  // rendering tenant screens itself.
  const [impersonated] = await Promise.all([
    context.waitForEvent('page'),
    page.getByTestId('impersonate').first().click(),
  ])

  await impersonated.waitForLoadState()

  const banner = impersonated.getByTestId('impersonation-banner')
  await expect(banner, 'a support session the customer cannot see is a trust problem').toBeVisible()

  // The token arrived in the fragment and was consumed: nothing resembling a JWT
  // may remain in the address bar, where it would sit in history.
  expect(impersonated.url()).not.toMatch(/eyJ/)

  await context.close()
})

test('an operator token is refused by the tenant API', async ({ request }) => {
  // Both firewalls verify against the same signing key, so realm isolation rests
  // entirely on the `aud` claim. If that check regresses, this is the test that
  // notices (ADR-0019).
  const login = await request.post(`${process.env.E2E_API_BASE}/api/manager/login`, {
    data: { email: OPERATOR.email, password: OPERATOR.password },
  })

  expect(login.ok()).toBe(true)
  const { token } = await login.json() as { token: string }

  const refused = await request.get(`${process.env.E2E_API_BASE}/api/projects`, {
    headers: { Authorization: `Bearer ${token}` },
  })

  expect(refused.status(), 'a manager token must not open a tenant endpoint').toBe(401)
})

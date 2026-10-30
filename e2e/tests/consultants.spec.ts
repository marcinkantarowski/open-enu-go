import { expect, test } from '@playwright/test'
import { CONSULTANT, OPERATOR, OWNER, PRIMARY_TENANT } from '../fixtures/accounts'
import { signInAsOperator } from '../fixtures/session'

/**
 * Who serves whom, as an operator sees it (ADR-0028): the operators, a
 * consultant assigned on a client's page, shown in the lists, and removed.
 */
const managerUrl = process.env.E2E_MANAGER_URL ?? 'https://manager.open-enu-go.local'

test('an operator assigns a consultant to a client, sees it in the lists, and removes it', async ({ browser }) => {
  const context = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await context.newPage()
  page.on('dialog', dialog => dialog.accept())

  await signInAsOperator(page, managerUrl, OPERATOR.email, OPERATOR.password)

  await page.goto(`${managerUrl}/operators`)
  await expect(page.getByTestId('operator-email').filter({ hasText: OPERATOR.email })).toHaveCount(1)

  await page.goto(managerUrl)
  await page.getByRole('link', { name: PRIMARY_TENANT.name }).first().click()
  await page.getByTestId('consultant-email').fill(CONSULTANT.email)
  await page.getByTestId('consultant-assign').click()
  await expect(page.getByTestId('consultant-remove')).toHaveCount(1)
  await expect(page.getByTestId('member-role').filter({ hasText: /consultant|konsultant/i })).toHaveCount(1)

  // The owner of the workspace itself is refused, and says why.
  await page.getByTestId('consultant-email').fill(OWNER.email)
  await page.getByTestId('consultant-assign').click()
  await expect(page.getByTestId('toast').last()).toContainText(/owner|właściciel/i)

  await page.goto(`${managerUrl}/consultants`)
  await expect(page.getByRole('row').filter({ hasText: CONSULTANT.email })).toContainText(PRIMARY_TENANT.name)

  await page.goto(managerUrl)
  await expect(page.getByRole('row').filter({ hasText: PRIMARY_TENANT.name }).getByTestId('tenant-consultants'))
    .not.toHaveText(/^(None|Brak)$/)

  // Back to how it was found.
  await page.getByRole('link', { name: PRIMARY_TENANT.name }).first().click()
  await page.getByTestId('consultant-remove').click()
  await expect(page.getByTestId('consultant-remove')).toHaveCount(0)

  await context.close()
})

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

  const addPerson = async (email: string, role: string) => {
    await page.getByTestId('member-email').fill(email)
    await page.getByTestId('member-role-select').selectOption(role)
    await page.getByTestId('member-add').click()
  }

  await signInAsOperator(page, managerUrl, OPERATOR.email, OPERATOR.password)

  // By the menu: an operator's session is in memory, and a reload ends it.
  await page.locator('a[href="/operators"]').click()
  await expect(page.getByTestId('operator-email').filter({ hasText: OPERATOR.email })).toHaveCount(1)

  await page.locator('a[href="/"]').first().click()
  await page.getByRole('link', { name: PRIMARY_TENANT.name }).first().click()
  await addPerson(CONSULTANT.email, 'consultant')
  await expect(page.getByTestId('consultant-remove')).toHaveCount(1)
  await expect(page.getByTestId('member-role').filter({ hasText: /consultant|konsultant/i })).toHaveCount(1)

  // The owner of the workspace itself is refused, and says why.
  await addPerson(OWNER.email, 'consultant')
  await expect(page.getByTestId('toast').last()).toContainText(/owner|właściciel/i)

  await page.locator('a[href="/consultants"]').click()
  await expect(page.getByRole('row').filter({ hasText: CONSULTANT.email })).toContainText(PRIMARY_TENANT.name)

  await page.locator('a[href="/"]').first().click()
  await expect(page.getByRole('row').filter({ hasText: PRIMARY_TENANT.name }).getByTestId('tenant-consultants'))
    .not.toHaveText(/^(None|Brak)$/)

  // Back to how it was found.
  await page.getByRole('link', { name: PRIMARY_TENANT.name }).first().click()
  await page.getByTestId('consultant-remove').click()
  await expect(page.getByTestId('consultant-remove')).toHaveCount(0)

  await context.close()
})

test('an operator opens a client and a new person sets their own password from the link', async ({ browser }) => {
  const stamp = Date.now()
  const email = `nowy-${stamp}@example.test`
  const chosen = 'a-password-they-chose'
  const context = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await context.newPage()

  await signInAsOperator(page, managerUrl, OPERATOR.email, OPERATOR.password)
  await page.getByTestId('tenant-name').fill(`Żółć ${stamp}`)
  await expect(page.getByTestId('tenant-slug')).toHaveValue(`zolc-${stamp}`)
  await page.getByTestId('tenant-create').click()

  await page.getByTestId('member-email').fill(email)
  await page.getByTestId('member-name').fill('Nowa Osoba')
  await page.getByTestId('member-add').click()
  const link = await page.getByTestId('member-link-url').textContent()
  expect(link).toContain('/reset-password?token=')

  const person = await (await browser.newContext({ ignoreHTTPSErrors: true })).newPage()
  await person.goto(link ?? '')
  await person.getByLabel(/^(New password|Nowe hasło|Password|Hasło)/).first().fill(chosen)
  await person.getByRole('button').first().click()
  await person.goto(new URL('/login', link ?? '').href)
  await person.getByTestId('login-email').fill(email)
  await person.getByTestId('login-password').fill(chosen)
  await person.getByTestId('login-submit').click()
  await expect(person.getByTestId('current-user')).toHaveText('Nowa Osoba')

  await context.close()
})

import { expect, test } from '@playwright/test'
import { OWNER } from '../fixtures/accounts'
import { signIn } from '../fixtures/session'

/**
 * Which language the two apps speak before and after someone chooses.
 *
 * Until a choice, DEFAULT_LOCALE - not the browser's language, which is why
 * the browser here prefers the OTHER one: a test that agreed with the default
 * would pass whether or not the browser was asked. After a choice, that choice,
 * across a reload - it lives in a cookie, and nothing else would survive one.
 */
const defaultLocale = process.env.E2E_DEFAULT_LOCALE ?? 'en'
const otherLocale = defaultLocale === 'pl' ? 'en' : 'pl'
const managerUrl = process.env.E2E_MANAGER_URL!

test.use({
  storageState: { cookies: [], origins: [] },
  locale: otherLocale === 'pl' ? 'pl-PL' : 'en-GB',
})

test('someone who never chose is served DEFAULT_LOCALE, whatever the browser prefers', async ({ page }) => {
  await page.goto('/login')
  await expect(page.locator('html')).toHaveAttribute('lang', defaultLocale)

  await page.goto(`${managerUrl}/login`)
  await expect(page.locator('html')).toHaveAttribute('lang', defaultLocale)
})

test('a language chosen in the profile is kept across a reload', async ({ page }) => {
  await signIn(page, OWNER.email, OWNER.password)
  await page.goto('/settings/profile')

  const language = page.getByTestId('profile-locale')
  await language.selectOption(otherLocale)
  await page.getByTestId('profile-save').click()
  await expect(page.locator('html')).toHaveAttribute('lang', otherLocale)

  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('lang', otherLocale)

  // Back to the default, so the account is as the next run expects it.
  await page.getByTestId('profile-locale').selectOption(defaultLocale)
  await page.getByTestId('profile-save').click()
  await expect(page.locator('html')).toHaveAttribute('lang', defaultLocale)
})

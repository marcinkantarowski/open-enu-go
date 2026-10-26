import { expect, type Page } from '@playwright/test'
import { OWNER } from './accounts'

/**
 * Sign in through the real form.
 *
 * Deliberately not a seeded cookie or an injected token: the session mechanism
 * IS what most of these tests are about - an access token held in memory, a
 * refresh token in an httpOnly cookie, and a redirect that lands where the guard
 * interrupted.
 */
export async function signIn(page: Page, email = OWNER.email, password = OWNER.password) {
  await page.goto('/login')

  await page.getByTestId('login-email').fill(email)
  await page.getByTestId('login-password').fill(password)
  await page.getByTestId('login-submit').click()

  await expect(page.getByTestId('current-user')).toBeVisible()
}

/** Sign in to the operator console, which lives on its own host and realm. */
export async function signInAsOperator(page: Page, managerUrl: string, email: string, password: string) {
  await page.goto(`${managerUrl}/login`)

  await page.getByTestId('login-email').fill(email)
  await page.getByTestId('login-password').fill(password)
  await page.getByTestId('login-submit').click()

  await expect(page).toHaveURL(new RegExp(`^${managerUrl}/?$`))
}

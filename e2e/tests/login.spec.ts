import { expect, test } from '@playwright/test'
import { OWNER } from '../fixtures/accounts'
import { signIn } from '../fixtures/session'

/**
 * The session, as the browser actually holds it (ADR-0018).
 *
 * Three claims are made in that ADR and none of them can be checked anywhere but
 * here: the refresh token is an httpOnly cookie, the access token is NOT in any
 * storage the page can read, and a reload turns the cookie back into a session
 * without a login screen.
 */
test.describe('session', () => {
  test('an unauthenticated visit is sent to the login page', async ({ page }) => {
    await page.goto('/example')

    await expect(page).toHaveURL(/\/login/)
    // And remembers where it was going, rather than dropping the person on the
    // dashboard to navigate again.
    await expect(page).toHaveURL(/[?&]next=(%2F|\/)example/)
  })

  test('a first visit asks for a session without a single failed request', async ({ page }) => {
    // The probe is the one request every visitor makes. With no cookie it is
    // answered 204, not 401: a 4xx is a red console line in a browser that
    // did nothing wrong, and the next real error hides among them.
    const failed: string[] = []
    const errors: string[] = []
    page.on('response', (response) => {
      if (response.status() >= 400) failed.push(`${response.status()} ${response.url()}`)
    })
    page.on('console', (message) => {
      if (message.type() === 'error') errors.push(message.text())
    })
    const probe = page.waitForResponse(response => response.url().endsWith('/api/auth/refresh'))

    await page.goto('/example')

    expect((await probe).status(), 'no cookie is nothing to restore, not a failed credential').toBe(204)
    await expect(page.getByTestId('login-submit')).toBeVisible()
    expect(failed, 'no request may fail on a first visit').toEqual([])
    expect(errors, 'the console stays clean on a first visit').toEqual([])
  })

  test('the shared components arrive with their styles', async ({ page }) => {
    await page.goto('/login')

    // The button's colour is a class used only inside the shared layer. If
    // Tailwind stops scanning that layer the class is silently never generated,
    // and the one thing this screen is for becomes white text on nothing.
    const background = await page.getByTestId('login-submit')
      .evaluate(element => getComputedStyle(element).backgroundColor)

    expect(background).not.toBe('rgba(0, 0, 0, 0)')
    expect(background).not.toBe('transparent')
  })

  test('wrong credentials are refused with one indistinguishable message', async ({ page }) => {
    await page.goto('/login')
    await page.getByTestId('login-email').fill(OWNER.email)
    await page.getByTestId('login-password').fill('definitely-not-the-password')
    await page.getByTestId('login-submit').click()

    await expect(page.getByTestId('login-error')).toBeVisible()
    await expect(page).toHaveURL(/\/login/)
  })

  test('signing in stores the refresh token in an httpOnly cookie and nothing else', async ({ page, context }) => {
    await signIn(page)

    const cookies = await context.cookies()
    const refresh = cookies.find(c => c.name.includes('refresh'))

    expect(refresh, 'the refresh token must be a cookie, not a JSON field the page keeps').toBeTruthy()
    expect(refresh!.httpOnly, 'an XSS must not be able to read it').toBe(true)
    expect(refresh!.sameSite, 'it must not ride along on a cross-site request').toBe('Strict')

    // The access token lives in memory only. Anything that survives the tab is
    // a credential an XSS could steal and use later.
    const stored = await page.evaluate(() => ({
      local: JSON.stringify(localStorage),
      session: JSON.stringify(sessionStorage),
    }))

    expect(stored.local).not.toMatch(/eyJ/)
    expect(stored.session).not.toMatch(/eyJ/)
  })

  test('a reload restores the session from the cookie instead of showing a login form', async ({ page }) => {
    await signIn(page)
    await page.reload()

    await expect(page.getByTestId('current-user')).toBeVisible()
    await expect(page).not.toHaveURL(/\/login/)
  })

  test('signing out clears the session and the guard takes effect immediately', async ({ page }) => {
    await signIn(page)
    await page.getByTestId('sign-out').click()

    await expect(page).toHaveURL(/\/login/)

    await page.goto('/example')
    await expect(page).toHaveURL(/\/login/)
  })
})

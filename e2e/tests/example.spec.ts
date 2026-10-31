import { expect, test, type Page } from '@playwright/test'
import { OPERATOR, PRIMARY_TENANT, SECOND_TENANT } from '../fixtures/accounts'
import { signIn, signInAsOperator } from '../fixtures/session'

/**
 * The reference module, through a browser.
 *
 * Four behaviours, each of them invisible to a server-side test:
 *
 *   1. A create reaches an open page over SSE - **and does not reach another
 *      tenant's stream**, which is the isolation claim that matters most and the
 *      one no functional test can make.
 *   2. A stale save is refused and the conflict bar offers a real choice.
 *   3. A long job hands back an id before the work starts, and the bar follows
 *      it to completion.
 *   4. A flag that is off makes its route 404, and the page presents that as
 *      "unavailable" rather than as an error.
 */

/** Unique per run - see the version note in the conflict test below. */
const unique = (prefix: string) => `${prefix} ${Date.now()}`

async function openProjects(page: Page) {
  await page.goto('/example')
  await expect(page.getByTestId('example-new')).toBeVisible()
}

async function createProject(page: Page, name: string) {
  await page.getByTestId('example-new').click()
  await page.getByTestId('example-name').fill(name)
  await page.getByTestId('example-create-save').click()
}

test('a project created in one session appears in another, and not in another tenant', async ({ browser }) => {
  const watcherContext = await browser.newContext({ ignoreHTTPSErrors: true })
  const otherTenantContext = await browser.newContext({ ignoreHTTPSErrors: true })
  const actorContext = await browser.newContext({ ignoreHTTPSErrors: true })

  const watcher = await watcherContext.newPage()
  const otherTenant = await otherTenantContext.newPage()
  const actor = await actorContext.newPage()

  // Same person in all three, which is the point: the isolation under test is
  // the TENANT's, not the user's. A test that used two different accounts could
  // pass because of an unrelated permission check.
  await signIn(watcher)
  await signIn(otherTenant)
  await signIn(actor)

  // Move one session to the second workspace. Switching re-issues the session,
  // so from here its Mercure subscriber token is scoped to a different topic.
  const reloaded = otherTenant.waitForEvent('load')
  await otherTenant.getByTestId('tenant-switcher').selectOption({ label: SECOND_TENANT.name })
  await reloaded
  await expect(otherTenant.getByTestId('current-tenant')).toHaveText(SECOND_TENANT.name)

  // Both are on the reference screen, which shows events as they arrive, with a
  // live connection open.
  await openProjects(watcher)
  await openProjects(otherTenant)
  await expect(watcher.getByRole('banner').getByTestId('realtime-indicator')).toContainText(/live|na żywo/i)
  await expect(otherTenant.getByRole('banner').getByTestId('realtime-indicator')).toContainText(/live|na żywo/i)

  const name = unique('Broadcast')
  await openProjects(actor)
  await createProject(actor, name)

  // Arrives over the open connection, with no reload and no poll.
  await expect(watcher.getByTestId('live-feed')).toBeVisible()
  await expect(watcher.getByTestId('live-feed').locator('li').first())
    .toContainText('example.project.created')

  // And the other workspace heard nothing. Asserted AFTER the positive case, so
  // this cannot pass merely because the event had not arrived yet - by the time
  // we get here it demonstrably has, for the tenant entitled to it.
  await expect(otherTenant.getByTestId('live-feed')).toBeHidden()

  // The creating session sees it too, and only through the broadcast: the page
  // deliberately does not refresh after a successful create.
  await expect(actor.getByTestId('example-item-name').filter({ hasText: name })).toBeVisible()

  await watcherContext.close()
  await otherTenantContext.close()
  await actorContext.close()
})

test('a stale save is refused and the conflict bar offers a real choice', async ({ browser }) => {
  const first = await browser.newContext({ ignoreHTTPSErrors: true })
  const second = await browser.newContext({ ignoreHTTPSErrors: true })

  const alice = await first.newPage()
  const bob = await second.newPage()

  // Unique per run, and that matters more than it looks. The bar is asserted
  // to show Alice's name, which proves nothing if an earlier run left the same
  // name on the row. And the test must not lean on a write that changes nothing:
  // this backend bumps the version on every update, but a server that skips a
  // no-op update leaves the version alone, and Bob's If-Match is then not stale
  // (.ai/platform/lessons/a-no-op-write-does-not-bump-the-version.md).
  const aliceName = unique('Renamed by Alice')

  await signIn(alice)
  await signIn(bob)
  await openProjects(alice)
  await openProjects(bob)

  // Both open the same row, both now holding the same version number.
  await alice.getByTestId('example-rename').first().click()
  await bob.getByTestId('example-rename').first().click()

  await alice.getByTestId('example-rename-input').fill(aliceName)
  await alice.getByTestId('example-rename-save').click()

  // Wait for the write to LAND, not merely to be sent. The modal closes only on
  // success, so this is the signal; without it the two saves race and the
  // conflict lands on whichever arrived second.
  await expect(alice.getByTestId('example-rename-input')).toBeHidden()

  await bob.getByTestId('example-rename-input').fill(unique('Renamed by Bob'))
  await bob.getByTestId('example-rename-save').click()

  const bar = bob.getByTestId('conflict-bar')
  await expect(bar).toBeVisible()
  // The saved record travels with the conflict, so "theirs" is something Bob can
  // read rather than an abstraction.
  await expect(bar).toContainText(aliceName)

  await bar.getByRole('button', { name: /load theirs|wczytaj cudze/i }).click()
  await expect(bar).toBeHidden()
  await expect(bob.getByTestId('example-item-name').first()).toHaveText(aliceName)

  await first.close()
  await second.close()
})

/**
 * The kill switch and the long job, in one pass.
 *
 * They are one test on purpose. Proving "off ⇒ 404" and "on ⇒ the job runs"
 * separately would leave the suite order-dependent - whichever ran second would
 * inherit the other's flag state - so this turns it on, uses it, and turns it
 * back off, ending where it started.
 */
test('an operator can switch the archive flag on, and the job then reports progress', async ({ browser }) => {
  const tenantContext = await browser.newContext({ ignoreHTTPSErrors: true })
  const operatorContext = await browser.newContext({ ignoreHTTPSErrors: true })

  const user = await tenantContext.newPage()
  const operator = await operatorContext.newPage()

  await signIn(user)
  await openProjects(user)

  // Off by default, so the route is a 404 and the page must present that as
  // unavailable rather than as a missing page.
  await user.getByTestId('example-archive').click()
  await expect(user.getByTestId('toast')).toContainText(/switched off|wyłączon/i)
  await expect(user.getByTestId('job-progress')).toBeHidden()

  // The operator turns it on for this workspace only.
  const managerUrl = process.env.E2E_MANAGER_URL ?? 'https://manager.open-enu-go.local'
  await signInAsOperator(operator, managerUrl, OPERATOR.email, OPERATOR.password)
  await operator.getByRole('link', { name: PRIMARY_TENANT.name }).first().click()

  // Wait for the text, not just the element: the detail page fetches after
  // navigating, so the span exists - empty - before the request resolves.
  const idCell = operator.getByTestId('tenant-id')
  await expect(idCell).not.toBeEmpty()
  const tenantId = (await idCell.innerText()).trim()

  // Navigated by clicking, never by `goto`. An operator session has no refresh
  // cookie by design (.ai/platform/docs/auth.md), so a full page load signs them
  // out - which is the correct behaviour and a trap for anyone writing a test
  // against it.
  await operator.getByRole('link', { name: /flags|flagi/i }).click()
  await operator.getByTestId('flag-tenant').fill(tenantId)
  await Promise.all([
    operator.waitForResponse(r => r.url().includes('/settings/example.archive') && r.request().method() === 'PUT'),
    operator.getByTestId('flag-on-example.archive').click(),
  ])

  try {
    // Takes effect on the very next request - the cache is invalidated by tag,
    // not left to expire, which is the whole point of a kill switch.
    await user.reload()
    await user.getByTestId('example-archive').click()

    // Asserted on the OUTCOME, not on the bar being on screen. The bar is
    // transient by nature: a job over a handful of rows finishes in well under
    // a second, and `<JobProgress>` hides itself the moment it reports done, so
    // `expect(bar).toBeVisible()` is really a test of how slow the worker is -
    // it passed alone and failed inside the full suite.
    //
    // The toast is the stronger claim anyway: it only fires from the bar's
    // `@done`, so seeing it proves the component mounted, polled the progress
    // API, saw the worker finish, and reported the count back.
    await expect(user.getByTestId('toast')).toContainText(/archived|zarchiwizowano/i, { timeout: 30_000 })
  } finally {
    // Back to how it was found, so this spec can run in any order and twice.
    //
    // Awaited on the RESPONSE, not on the click: closing a context with a write
    // still in flight aborts it, which left the flag on and made whichever spec
    // ran next fail - once, and not again, which is the worst kind of failure to
    // be handed.
    await Promise.all([
      operator.waitForResponse(r => r.url().includes('/settings/example.archive') && r.request().method() === 'PUT'),
      operator.getByTestId('flag-off-example.archive').click(),
    ])
  }

  await tenantContext.close()
  await operatorContext.close()
})

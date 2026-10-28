import { defineConfig, devices } from '@playwright/test'

/**
 * Browser tests against the REAL dev stack.
 *
 * Not a mocked API and not a preview build: the five behaviours these cover -
 * cookie login, tenant switch, impersonation, server-sent events, and the 409
 * conflict bar - are precisely the ones that pass in unit tests and then fail in
 * a browser, because they depend on cookie attributes, on two realms sharing a
 * signing key, and on a connection staying open.
 *
 * Run by `make e2e`, which is in `make ci` and deliberately NOT in `make check`:
 * it needs the whole stack up, and a 60-second pre-commit gate cannot ask for
 * that (.ai/platform/docs/testing.md).
 */
const appUrl = process.env.E2E_APP_URL ?? 'https://app.open-enu-go.local'
const managerUrl = process.env.E2E_MANAGER_URL ?? 'https://manager.open-enu-go.local'

// The suite reads English, whatever DEFAULT_LOCALE a project sets: every test
// starts as someone who chose English in both apps. tests/locale.spec.ts clears
// it to see what someone who never chose gets.
const choseEnglish = (url: string) => ({
  name: 'open_enu_locale', value: 'en', domain: new URL(url).hostname, path: '/',
  expires: -1, httpOnly: false, secure: true, sameSite: 'None' as const,
})

export default defineConfig({
  testDir: './tests',
  outputDir: './.results',

  // Sequential by default. These tests share one database and one seeded
  // workspace, so parallel workers would race each other's fixtures - and the
  // failures would look like flakiness rather than like contention.
  workers: 1,
  fullyParallel: false,

  // A failure here is a real failure. Retrying would turn "the SSE connection
  // never arrived" into "passed on the second try", which is the bug.
  retries: 0,

  // Generous, because this runs against the DEV stack: Vite compiles a route on
  // first visit, and a test that signs two people in and drives both through the
  // same screen pays that twice. The tests themselves wait on state, never on
  // the clock - this is only the ceiling before a genuine hang is declared.
  timeout: 60_000,
  expect: { timeout: 10_000 },

  reporter: process.env.CI ? [['list'], ['html', { open: 'never', outputFolder: './.report' }]] : 'list',

  use: {
    baseURL: appUrl,

    // The dev stack is served under a locally-generated CA. The browser in the
    // container has no reason to trust it, and importing it would test the
    // certificate rather than the application.
    ignoreHTTPSErrors: true,

    storageState: { cookies: [choseEnglish(appUrl), choseEnglish(managerUrl)], origins: [] },

    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'off',
    locale: 'en-GB',
  },

  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
})

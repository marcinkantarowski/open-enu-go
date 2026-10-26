import { expect, test } from '@playwright/test'
import { signIn } from '../fixtures/session'

/**
 * An attachment, uploaded through the real multipart endpoint.
 *
 * Worth a browser test for one reason: the upload path is the only place the
 * client must NOT set `Content-Type`, because the browser has to add the
 * multipart boundary itself. Setting it by hand produces a body the server
 * cannot parse - and the failure is a 400 that reads like a validation problem.
 */
test('a file uploads and is attached to the record', async ({ page }) => {
  await signIn(page)
  await page.goto('/example')

  // Creates its own row rather than reusing a seeded one. The archive spec
  // leaves every existing project archived, and an archived project is
  // read-only - so a test that grabbed "the first row" would pass or fail
  // depending on which spec ran before it.
  const name = `Upload target ${Date.now()}`
  await page.getByTestId('example-new').click()
  await page.getByTestId('example-name').fill(name)
  await page.getByTestId('example-create-save').click()

  const row = page.getByRole('row').filter({ hasText: name })
  await expect(row).toBeVisible()
  await row.getByTestId('example-rename').click()

  await page.locator('input[type="file"]').setInputFiles({
    name: 'note.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('Attached by the end-to-end suite.'),
  })

  await expect(page.getByTestId('toast')).toContainText('note.txt')
})

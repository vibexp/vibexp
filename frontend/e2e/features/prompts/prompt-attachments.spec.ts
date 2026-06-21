import path from 'node:path'

import { test, expect } from '../../fixtures/auth'

/**
 * Feature Test: Prompt Attachments
 *
 * Mirrors the artifact attachment journey (artifact-attachments.spec.ts) but on
 * a prompt: the prompt detail sidebar renders the same generic
 * <ResourceAttachments ownerType="prompt"> / AttachmentCard, so the upload/list/
 * delete interaction (testids + aria-labels) is identical; only resource
 * creation and the detail URL differ. Runs against the real GCS code path via
 * the in-cluster fake-gcs-server emulator (see docker-compose.e2e.yml), which
 * gives the backend a credential-free object store so attachment upload/list/
 * delete succeed instead of returning 503. Granular validation (limits, allowed
 * types, quota math, download blob) stays in the AttachmentCard Vitest unit
 * tests; this single happy path proves the end-to-end wiring.
 */
test.describe('Prompt Attachments', () => {
  // Fixtures live next to the spec; setInputFiles resolves these absolute paths.
  const fixturesDir = path.join(process.cwd(), 'e2e', 'fixtures')
  const file1 = 'sample-attachment.txt'
  const file2 = 'sample-attachment-2.txt'

  test('user can add multiple attachments to a prompt and delete them', async ({
    authenticatedPage: page,
  }) => {
    // --- Create a prompt (mirrors the prompt-crud happy path) ---
    await page.goto('/prompts/new')
    await expect(page).toHaveURL(/prompts\/new/)

    const promptName = `Attach Test ${String(Date.now())}`
    await page.waitForSelector('input[placeholder*="Enter prompt name"]', {
      timeout: 10000,
    })
    await page
      .locator('input[placeholder*="Enter prompt name"]')
      .fill(promptName)
    await page
      .locator('textarea[placeholder*="Write your prompt here"]')
      .fill('Prompt used to exercise the attachment upload/delete flow.')
    await page.locator('[data-testid="prompt-save-button"]').click()

    // Detail view URL: /prompts/<slug> (not the /prompts/new create route)
    await expect(page).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
      timeout: 10000,
    })

    // --- Scope to the attachment card; it starts empty ---
    const card = page.getByTestId('attachment-card')
    await expect(card).toBeVisible({ timeout: 10000 })
    await expect(card.getByText('No attachments yet.')).toBeVisible()

    const fileInput = page.locator('input[aria-label="Upload attachment"]')
    const items = page.getByTestId('attachment-item')

    // --- Upload file #1 ---
    await fileInput.setInputFiles(path.join(fixturesDir, file1))
    await expect(items.filter({ hasText: file1 })).toBeVisible({
      timeout: 10000,
    })
    await expect(items).toHaveCount(1)

    // --- Upload file #2 (proves *multiple* attachments) ---
    await fileInput.setInputFiles(path.join(fixturesDir, file2))
    await expect(items.filter({ hasText: file2 })).toBeVisible({
      timeout: 10000,
    })
    await expect(items).toHaveCount(2)

    // --- Delete file #1; file #2 remains (proves *delete* is targeted) ---
    const row1 = items.filter({ hasText: file1 })
    await row1.hover()
    await row1.getByRole('button', { name: `Delete ${file1}` }).click()
    await expect(items.filter({ hasText: file1 })).toHaveCount(0, {
      timeout: 10000,
    })
    await expect(items.filter({ hasText: file2 })).toBeVisible()
    await expect(items).toHaveCount(1)

    // --- Delete file #2; back to the empty state ---
    const row2 = items.filter({ hasText: file2 })
    await row2.hover()
    await row2.getByRole('button', { name: `Delete ${file2}` }).click()
    await expect(items).toHaveCount(0, { timeout: 10000 })
    await expect(card.getByText('No attachments yet.')).toBeVisible()
  })
})

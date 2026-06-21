import path from 'node:path'

import { test, expect } from '../../fixtures/auth'
import { selectFirstProject } from '../../helpers/artifacts'

/**
 * Feature Test: Artifact Attachments
 *
 * Exercises the full attachment user journey against the real GCS code path: an
 * in-cluster fake-gcs-server emulator (see docker-compose.e2e.yml) gives the
 * backend a credential-free object store, so upload/list/delete succeed instead
 * of returning 503. Granular validation (limits, allowed types, quota math,
 * download blob) stays in the AttachmentCards Vitest unit tests; this single
 * happy path proves the end-to-end wiring: create → upload two → delete both.
 */
test.describe('Artifact Attachments', () => {
  // Fixtures live next to the spec; setInputFiles resolves these absolute paths.
  const fixturesDir = path.join(process.cwd(), 'e2e', 'fixtures')
  const file1 = 'sample-attachment.txt'
  const file2 = 'sample-attachment-2.txt'

  test('user can add multiple attachments to an artifact and delete them', async ({
    authenticatedPage: page,
  }) => {
    // --- Create an artifact (mirrors the artifact-crud happy path) ---
    await page.goto('/artifacts/new')
    await expect(page).toHaveURL(/artifacts\/new/)

    const stamp = Date.now()
    await page.waitForSelector('[data-testid="artifact-project-select"]', {
      timeout: 10000,
    })
    await page
      .locator('[data-testid="artifact-slug-input"]')
      .fill(`attach-${String(stamp)}`)
    await page
      .locator('[data-testid="artifact-title-input"]')
      .fill(`Attach Test ${String(stamp)}`)
    await page
      .locator('[data-testid="artifact-content-textarea"]')
      .fill('Artifact used to exercise the attachment upload/delete flow.')
    await selectFirstProject(page)
    await page.locator('button:has-text("Create Artifact")').click()

    // Detail view URL: /artifacts/<project>/<slug>
    await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+/, { timeout: 10000 })

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

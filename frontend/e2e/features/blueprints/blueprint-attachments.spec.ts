import path from 'node:path'

import { test, expect, type Page } from '../../fixtures/auth'
import { generateBlueprintData } from '../../fixtures/test-data'

/**
 * Feature Test: Blueprint Attachments
 *
 * Mirrors the artifact attachments e2e (see ../artifacts/artifact-attachments.spec.ts)
 * but on a blueprint: BlueprintView renders the same shared ResourceAttachments /
 * AttachmentCard UI (ownerType="blueprint"), so the upload/list/delete interaction is
 * identical — only resource creation and the detail URL differ. Runs against the real
 * GCS code path via the in-cluster fake-gcs-server emulator (docker-compose.e2e.yml),
 * which gives the backend a credential-free object store so attachments succeed instead
 * of returning 503. Granular validation (limits, allowed types, quota math) stays in the
 * AttachmentCard Vitest unit tests; this single happy path proves the end-to-end wiring:
 * create → upload two → delete both.
 */

/** Fill and submit the blueprint create form; resolves on the detail page. */
async function createBlueprint(
  page: Page,
  blueprint: { title: string; slug: string; content: string }
): Promise<void> {
  await page.goto('/blueprints/new')
  await expect(page.getByLabel('Title')).toBeVisible({ timeout: 10000 })

  await page.getByLabel('Title').fill(blueprint.title)
  await page.getByLabel('Slug').fill(blueprint.slug)
  await page.getByLabel('Content', { exact: true }).fill(blueprint.content)

  // Project is a Radix Select with no default — pick the first project.
  // (Targeted by testid: the header project switcher's accessible name also
  // contains "Project", so a role+name lookup is ambiguous since #78.)
  await page.getByTestId('blueprint-project-select').click()
  await page.getByRole('option').first().click()

  await page.getByRole('button', { name: 'Create blueprint' }).click()

  // Success navigates to /blueprints/:project/:slug
  await page.waitForURL(/\/blueprints\/[^/]+\/[^/]+$/, { timeout: 15000 })
}

test.describe('Blueprint Attachments', () => {
  // Fixtures live next to the spec; setInputFiles resolves these absolute paths.
  const fixturesDir = path.join(process.cwd(), 'e2e', 'fixtures')
  const file1 = 'sample-attachment.txt'
  const file2 = 'sample-attachment-2.txt'

  test('user can add multiple attachments to a blueprint and delete them', async ({
    authenticatedPageWithTeam: page,
  }) => {
    // --- Create a blueprint (mirrors the blueprint-crud happy path) ---
    const blueprint = generateBlueprintData()
    await createBlueprint(page, blueprint)

    // --- Scope to the attachment card; it starts empty ---
    // The card only renders once team context is loaded (BlueprintView gates it
    // on currentTeam); authenticatedPageWithTeam guarantees that context.
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

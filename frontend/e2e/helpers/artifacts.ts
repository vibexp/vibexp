import { expect, type Page } from '@playwright/test'

/**
 * Opens the artifact form's ProjectPicker and selects the first available
 * project.
 *
 * The searchable/paginated ProjectPicker (#1790) made `project_id` a required
 * field with NO default — the form pre-selects nothing — so submitting the
 * artifact form without an explicit project selection fails zod validation and
 * the page stays on `/artifacts/new`. Every dev-login team ships with a default
 * project, so the first option is always present.
 */
export async function selectFirstProject(page: Page): Promise<void> {
  await page.getByTestId('artifact-project-select').click()
  const firstProject = page.getByRole('option').first()
  await firstProject.waitFor({ state: 'visible', timeout: 10000 })
  await firstProject.click()
}

/**
 * Creates an artifact through the new-artifact form and waits for its detail
 * page (`/artifacts/<projectId>/<slug>`). The slug is `<slugPrefix>-<stamp>`
 * and the title `<titlePrefix> <slugPrefix> <stamp>`.
 */
export async function createArtifactWithContent(
  page: Page,
  titlePrefix: string,
  slugPrefix: string,
  content: string
): Promise<void> {
  await page.goto('/artifacts/new')
  await expect(page).toHaveURL(/artifacts\/new/)

  const stamp = Date.now()
  await page.waitForSelector('[data-testid="artifact-project-select"]', {
    timeout: 10000,
  })
  await page
    .locator('[data-testid="artifact-slug-input"]')
    .fill(`${slugPrefix}-${String(stamp)}`)
  await page
    .locator('[data-testid="artifact-title-input"]')
    .fill(`${titlePrefix} ${slugPrefix} ${String(stamp)}`)
  await page.locator('[data-testid="artifact-content-textarea"]').fill(content)
  await selectFirstProject(page)
  await page.locator('button:has-text("Create Artifact")').click()

  await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+/, { timeout: 10000 })
}

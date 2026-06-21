import type { Page } from '@playwright/test'

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

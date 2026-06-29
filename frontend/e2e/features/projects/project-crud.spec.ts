import { test, expect } from '../../fixtures/auth'
import { generateUniqueSlug } from '../../fixtures/test-data'

/**
 * Feature Test: Project Settings
 *
 * Closes the e2e gap on the `settings/projects` area (issue #66). One happy path
 * proves the create → persist → list wiring end-to-end: create a project through
 * the form, then confirm it shows up in the projects table. Granular form
 * validation (slug rules, required fields) stays in the ProjectForm Vitest unit
 * tests; this only exercises the user journey.
 */
test.describe('Project Settings', () => {
  test('user can create a project and see it in the list', async ({
    authenticatedPage: page,
  }) => {
    const slug = generateUniqueSlug('e2e-project')
    const name = `E2E Project ${slug}`

    await page.goto('/settings/projects/create')

    await page.getByLabel('Name', { exact: true }).fill(name)
    await page.getByLabel('Slug', { exact: true }).fill(slug)

    await page.getByRole('button', { name: /create project/i }).click()

    // On success the form navigates back to the projects list.
    await page.waitForURL('**/settings/projects', { timeout: 10000 })

    // The new project is rendered in the list (name cell links to its detail).
    await expect(page.getByText(name, { exact: true })).toBeVisible({
      timeout: 10000,
    })
  })
})

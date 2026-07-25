import { test, expect } from '../../fixtures/auth'
import { generateUniqueSlug } from '../../fixtures/test-data'

/**
 * Feature Test: Team Projects
 *
 * Closes the e2e gap on the projects area (issue #66). One happy path
 * proves the create → persist → list wiring end-to-end: create a project through
 * the form, then confirm it shows up in the projects table. Granular form
 * validation (slug rules, required fields) stays in the ProjectForm Vitest unit
 * tests; this only exercises the user journey.
 */
test.describe('Team Projects', () => {
  test('user can create a project and see it in the list', async ({
    authenticatedPage: page,
  }) => {
    const slug = generateUniqueSlug('e2e-project')
    const name = `E2E Project ${slug}`

    // #542 moved projects under the team scope, so the route needs the team id.
    // The auth fixture already waits for this key - it is its readiness signal -
    // so it is guaranteed to be present here.
    const teamId = await page.evaluate(() =>
      localStorage.getItem('vx_current_team_id')
    )
    expect(teamId, 'no current team id in localStorage').toBeTruthy()

    await page.goto(`/teams/${teamId}/projects/create`)

    await page.getByLabel('Name', { exact: true }).fill(name)
    await page.getByLabel('Slug', { exact: true }).fill(slug)

    await page.getByRole('button', { name: /create project/i }).click()

    // On success the form navigates back to the projects list.
    await page.waitForURL(`**/teams/${teamId}/projects`, { timeout: 10000 })

    // The new project is rendered in the list (name cell links to its detail).
    await expect(page.getByText(name, { exact: true })).toBeVisible({
      timeout: 10000,
    })
  })
})

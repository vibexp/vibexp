import { test, expect, type Page } from '../../fixtures/auth'

/**
 * Feature Tests: Team Management
 * Tests creating, viewing, editing, and deleting teams
 *
 * Every test here asserts unconditionally (#607). The previous version wrapped
 * each body in a count-based `if` guard, which made a test that could not find
 * its own subject report green — indistinguishable from a passing one.
 */

/** The personal team every fresh user is provisioned with. */
const PRIVATE_WORKSPACE = 'Private Workspace'

/**
 * Drive the real create-team flow and return once the new team is in the list.
 *
 * The `authenticatedPage` fixture mints a fresh user per test, so the only
 * pre-existing team is the Private Workspace — anything a test asserts on it
 * must create first.
 */
async function createTeam(page: Page, name: string): Promise<void> {
  await page.goto('/teams')
  await page.click('[data-testid="create-team-button"]')

  const nameInput = page.locator('[data-testid="team-name-input"]')
  await expect(nameInput).toBeVisible({ timeout: 10000 })
  await nameInput.fill(name)

  await page.click('[data-testid="submit-create-team-button"]')

  // The list reloads after creation; waiting on the row (rather than a fixed
  // timeout) is also what makes a failed creation fail the test.
  await expect(teamRow(page, name)).toBeVisible({ timeout: 10000 })
}

/** The list row button that opens a team — the only clickable the table renders. */
function teamRow(page: Page, name: string) {
  return page.locator('[data-testid="team-row-link"]', { hasText: name })
}

/** Open a team from the list and wait for its detail route. */
async function openTeam(page: Page, name: string): Promise<void> {
  await teamRow(page, name).click()
  await page.waitForURL(/\/teams\/[0-9a-f-]+$/, { timeout: 10000 })
}

test.describe('Team Management', () => {
  test.describe('Team Display', () => {
    test('should display teams list in settings', async ({
      authenticatedPage,
    }) => {
      // Navigate to the top-level teams page (#538).
      await authenticatedPage.goto('/teams')

      // The page header and the one team every user starts with. Asserting the
      // list actually rendered — not merely that a <body> exists.
      await expect(
        authenticatedPage.getByRole('heading', { name: 'Teams', level: 1 })
      ).toBeVisible({ timeout: 10000 })
      await expect(teamRow(authenticatedPage, PRIVATE_WORKSPACE)).toBeVisible({
        timeout: 10000,
      })
    })

    test('should show Private Workspace as default team', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/teams')

      // A fresh user is provisioned with exactly this team, so its absence is
      // a real failure rather than a reason to skip the assertion.
      await expect(teamRow(authenticatedPage, PRIVATE_WORKSPACE)).toBeVisible({
        timeout: 10000,
      })
    })
  })

  test.describe('Team Creation', () => {
    test('should create new team with name', async ({ authenticatedPage }) => {
      const teamName = `Test Team ${Date.now()}`
      await createTeam(authenticatedPage, teamName)

      await expect(teamRow(authenticatedPage, teamName)).toBeVisible()
    })

    test('should validate team name is required', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')

      const submitButton = authenticatedPage.locator(
        '[data-testid="submit-create-team-button"]'
      )
      await expect(submitButton).toBeVisible({ timeout: 10000 })

      // Submit with an empty name.
      await submitButton.click()

      // CreateTeamModal validates client-side and renders the message inline.
      await expect(
        authenticatedPage.getByText('Team name is required')
      ).toBeVisible({ timeout: 5000 })

      // And the dialog must stay open — a closed dialog would mean the empty
      // team was accepted.
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible()
    })

    test('should display newly created team in list', async ({
      authenticatedPage,
    }) => {
      const teamName = `List Team ${Date.now()}`
      await createTeam(authenticatedPage, teamName)

      // Reload rather than trusting the post-create render: this test is about
      // the team being persisted and listed, not about the optimistic update.
      await authenticatedPage.goto('/teams')
      await expect(teamRow(authenticatedPage, teamName)).toBeVisible({
        timeout: 10000,
      })
    })
  })

  test.describe('Team Editing', () => {
    test('should edit team name', async ({ authenticatedPage }) => {
      const originalName = `Edit Team ${Date.now()}`
      await createTeam(authenticatedPage, originalName)

      // "Edit team" lives on the team's own page (TeamScopeHeader), not on the
      // list — the previous selector looked for it on /teams, where it can
      // never exist.
      await openTeam(authenticatedPage, originalName)

      await authenticatedPage.getByRole('button', { name: 'Edit team' }).click()

      // EditTeamModal's field is keyed by id, not by the create modal's testid.
      const editNameInput = authenticatedPage.locator('#team-name')
      await expect(editNameInput).toBeVisible({ timeout: 10000 })

      const updatedName = `${originalName} Updated`
      await editNameInput.fill(updatedName)
      await authenticatedPage
        .getByRole('button', { name: 'Update Team' })
        .click()

      // The scope header reflects the new name once the update lands.
      await expect(
        authenticatedPage.getByRole('heading', { name: updatedName, level: 1 })
      ).toBeVisible({ timeout: 10000 })
    })

    test('should view team details (members, resources)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/teams')

      await openTeam(authenticatedPage, PRIVATE_WORKSPACE)

      // Landing on the detail route is only half of it — assert the page
      // actually rendered the team, so a blank error page fails the test.
      await expect(
        authenticatedPage.getByRole('heading', {
          name: PRIVATE_WORKSPACE,
          level: 1,
        })
      ).toBeVisible({ timeout: 10000 })
    })
  })

  test.describe('Team Deletion', () => {
    test('should delete team with confirmation', async ({
      authenticatedPage,
    }) => {
      const teamName = `Delete Team ${Date.now()}`
      await createTeam(authenticatedPage, teamName)

      // Open the team's details page, then delete it from there (deletion
      // lives on the details page, not the list).
      await openTeam(authenticatedPage, teamName)

      // Delete lives in the scope header's overflow menu since #666, so it
      // has to be opened before the item exists in the DOM.
      await authenticatedPage
        .locator('[data-testid="team-actions-menu"]')
        .click()
      await authenticatedPage
        .locator('[data-testid="delete-team-button"]')
        .click()

      // Confirm deletion in the DeleteTeamModal (a regular Dialog)
      const confirmDialog = authenticatedPage.locator('[role="dialog"]')
      await expect(confirmDialog).toBeVisible({ timeout: 5000 })

      await authenticatedPage
        .locator('[data-testid="confirm-delete-team-button"]')
        .click()

      // Should navigate back to the teams list after deletion
      await expect(authenticatedPage).toHaveURL(/\/teams$/, {
        timeout: 10000,
      })

      // And the team must actually be gone, not merely navigated away from.
      await expect(teamRow(authenticatedPage, teamName)).toHaveCount(0, {
        timeout: 10000,
      })
    })

    test('should prevent deletion of Private Workspace', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/teams')
      await openTeam(authenticatedPage, PRIVATE_WORKSPACE)

      // Positive gate first: without it, the negative assertion below would
      // also pass against a page that never rendered (#415).
      await expect(
        authenticatedPage.getByRole('heading', {
          name: PRIVATE_WORKSPACE,
          level: 1,
        })
      ).toBeVisible({ timeout: 10000 })
      await expect(
        authenticatedPage.getByRole('button', { name: 'Edit team' })
      ).toBeVisible()

      // TeamScopeHeader gates delete AND transfer on `!team.is_personal`, and
      // the overflow menu only renders when one of them is available — so a
      // personal team must expose no actions menu at all, hence no delete.
      await expect(
        authenticatedPage.locator('[data-testid="team-actions-menu"]')
      ).toHaveCount(0)
    })

    test('should show team member count', async ({ authenticatedPage }) => {
      const teamName = `Members Team ${Date.now()}`
      await createTeam(authenticatedPage, teamName)

      // The Members column renders the count for a real team (and "-" for the
      // personal one). The creator is its only member.
      const row = authenticatedPage.getByRole('row', {
        name: new RegExp(teamName),
      })
      await expect(row).toBeVisible({ timeout: 10000 })
      await expect(row.getByText(/^\d+$/)).toBeVisible()
    })
  })
})

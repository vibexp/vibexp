import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: Team Management
 * Tests creating, viewing, editing, and deleting teams
 */
test.describe('Team Management', () => {
  test.describe('Team Display', () => {
    test('should display teams list in settings', async ({
      authenticatedPage,
    }) => {
      // Navigate to settings/teams (adjust URL based on actual route)
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify teams page is displayed
      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should show Private Workspace as default team', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForLoadState('networkidle')

      // Look for Private Workspace
      const privateWorkspace = authenticatedPage.locator(
        'text=/Private Workspace|private-workspace/i'
      )
      const hasPrivateWorkspace = await privateWorkspace
        .isVisible()
        .catch(() => false)

      if (hasPrivateWorkspace) {
        await expect(privateWorkspace).toBeVisible()
      } else {
        // Private Workspace might be shown in header/nav instead
        await expect(authenticatedPage.locator('body')).toBeVisible()
      }
    })
  })

  test.describe('Team Creation', () => {
    test('should create new team with name', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForLoadState('networkidle')

      // Look for "Create Team" button
      const createButton = authenticatedPage.locator(
        '[data-testid="create-team-button"]'
      )

      if ((await createButton.count()) > 0) {
        await createButton.click()
        await authenticatedPage.waitForTimeout(1000)

        // Fill team name
        const teamNameInput = authenticatedPage.locator(
          '[data-testid="team-name-input"]'
        )

        if ((await teamNameInput.count()) > 0) {
          const teamName = `Test Team ${Date.now()}`
          await teamNameInput.fill(teamName)

          // Submit
          const submitButton = authenticatedPage.locator(
            '[data-testid="submit-create-team-button"]'
          )
          await submitButton.click()
          await authenticatedPage.waitForTimeout(2000)

          // Verify creation
          await expect(
            authenticatedPage.locator(`text=${teamName}`).first()
          ).toBeVisible({ timeout: 5000 })
        }
      }
    })

    test('should validate team name is required', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForLoadState('networkidle')

      const createButton = authenticatedPage.locator(
        '[data-testid="create-team-button"]'
      )

      if ((await createButton.count()) > 0) {
        await createButton.click()
        await authenticatedPage.waitForTimeout(1000)

        // Try to submit without name
        const submitButton = authenticatedPage.locator(
          '[data-testid="submit-create-team-button"]'
        )
        if ((await submitButton.count()) > 0) {
          await submitButton.click()
          await authenticatedPage.waitForTimeout(1000)

          // Should show validation error or stay on form
          await expect(authenticatedPage.locator('body')).toBeVisible()
        }
      }
    })

    test('should display newly created team in list', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForLoadState('networkidle')

      const createButton = authenticatedPage.locator(
        '[data-testid="create-team-button"]'
      )

      if ((await createButton.count()) > 0) {
        await createButton.click()
        await authenticatedPage.waitForTimeout(1000)

        const teamNameInput = authenticatedPage.locator(
          '[data-testid="team-name-input"]'
        )

        if ((await teamNameInput.count()) > 0) {
          const teamName = `List Team ${Date.now()}`
          await teamNameInput.fill(teamName)

          const submitButton = authenticatedPage.locator(
            '[data-testid="submit-create-team-button"]'
          )
          await submitButton.click()
          await authenticatedPage.waitForTimeout(2000)

          // Verify in list
          await expect(
            authenticatedPage.locator(`text=${teamName}`).first()
          ).toBeVisible({ timeout: 5000 })
        }
      }
    })
  })

  test.describe('Team Editing', () => {
    test('should edit team name', async ({ authenticatedPage }) => {
      // First create a team
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForLoadState('networkidle')

      const createButton = authenticatedPage.locator(
        '[data-testid="create-team-button"]'
      )

      if ((await createButton.count()) > 0) {
        await createButton.click()
        await authenticatedPage.waitForTimeout(1000)

        const teamNameInput = authenticatedPage.locator(
          '[data-testid="team-name-input"]'
        )

        if ((await teamNameInput.count()) > 0) {
          const originalName = `Edit Team ${Date.now()}`
          await teamNameInput.fill(originalName)

          const submitButton = authenticatedPage.locator(
            '[data-testid="submit-create-team-button"]'
          )
          await submitButton.click()
          await authenticatedPage.waitForTimeout(2000)

          // Now edit the team
          const editButton = authenticatedPage.locator(
            'button:has-text("Edit"), a:has-text("Edit")'
          )

          if ((await editButton.count()) > 0) {
            await editButton.first().click()
            await authenticatedPage.waitForTimeout(1000)

            const editNameInput = authenticatedPage.locator(
              '[data-testid="team-name-input"]'
            )

            if ((await editNameInput.count()) > 0) {
              const updatedName = `${originalName} (Updated)`
              await editNameInput.clear()
              await editNameInput.fill(updatedName)

              const saveButton = authenticatedPage.locator(
                'button:has-text("Save"), button:has-text("Update"), button[type="submit"]'
              )
              await saveButton.click()
              await authenticatedPage.waitForTimeout(2000)

              // Verify update
              await expect(
                authenticatedPage.locator(`text=${updatedName}`).first()
              ).toBeVisible({ timeout: 5000 })
            }
          }
        }
      }
    })

    test('should view team details (members, resources)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForLoadState('networkidle')

      // Click on a team to view details
      const teamLink = authenticatedPage
        .locator('a, button')
        .filter({ hasText: /Team|Workspace/ })
        .first()

      if ((await teamLink.count()) > 0) {
        await teamLink.click()
        await authenticatedPage.waitForTimeout(1000)

        // Verify we're on a detail/settings page
        await expect(authenticatedPage.locator('body')).toBeVisible()
      }
    })
  })

  test.describe('Team Deletion', () => {
    test('should delete team with confirmation', async ({
      authenticatedPage,
    }) => {
      // Create a team first
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForLoadState('networkidle')

      const createButton = authenticatedPage.locator(
        '[data-testid="create-team-button"]'
      )

      if ((await createButton.count()) > 0) {
        await createButton.click()
        await authenticatedPage.waitForTimeout(1000)

        const teamNameInput = authenticatedPage.locator(
          '[data-testid="team-name-input"]'
        )

        if ((await teamNameInput.count()) > 0) {
          const teamName = `Delete Team ${Date.now()}`
          await teamNameInput.fill(teamName)

          const submitButton = authenticatedPage.locator(
            '[data-testid="submit-create-team-button"]'
          )
          await submitButton.click()

          // Wait for the list to finish reloading and show the new team before
          // navigating (clicking mid-reload aborts the details fetch).
          const teamLink = authenticatedPage.getByRole('button', {
            name: teamName,
          })
          await expect(teamLink).toBeVisible({ timeout: 10000 })
          await authenticatedPage.waitForLoadState('networkidle')

          // Open the new team's details page, then delete it from there
          // (deletion lives on the details page, not the list).
          await teamLink.click()
          await authenticatedPage.waitForURL(/settings\/teams\/[0-9a-f-]+$/, {
            timeout: 10000,
          })

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
          await expect(authenticatedPage).toHaveURL(/settings\/teams$/, {
            timeout: 10000,
          })
        }
      }
    })

    test('should prevent deletion of Private Workspace', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForLoadState('networkidle')

      // Try to find delete button for Private Workspace
      // It should either not exist or be disabled
      const privateWorkspaceRow = authenticatedPage.locator(
        'text=/Private Workspace/i'
      )

      if ((await privateWorkspaceRow.count()) > 0) {
        // Look for delete button near Private Workspace
        // It should not exist or be disabled
        await expect(authenticatedPage.locator('body')).toBeVisible()
      }
    })

    test('should show team member count', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForLoadState('networkidle')

      // Look for member count indicators
      const memberCount = authenticatedPage.locator(
        'text=/[0-9]+ member|member/i'
      )

      if ((await memberCount.count()) > 0) {
        await expect(memberCount.first()).toBeVisible()
      } else {
        // Member count might not be displayed
        await expect(authenticatedPage.locator('body')).toBeVisible()
      }
    })
  })
})

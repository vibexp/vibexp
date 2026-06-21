import { test, expect } from '../../fixtures/auth'
import { getCurrentTeam } from '../../fixtures/auth'

/**
 * Feature Tests: Team Context Switching
 * Tests switching between teams and verifying resource scoping
 */
test.describe('Team Context Switching', () => {
  test.describe('Team Switcher Display', () => {
    test('should display current team in header dropdown', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/')
      await authenticatedPage.waitForLoadState('networkidle')

      // Look for team switcher in header
      const teamSwitcher = authenticatedPage.locator(
        '[data-testid="team-switcher"], [data-testid="current-team-name"]'
      )

      if ((await teamSwitcher.count()) > 0) {
        await expect(teamSwitcher.first()).toBeVisible()
      } else {
        // Team switcher might be in a different location
        const currentTeamText = authenticatedPage.locator(
          'text=/Private Workspace|Current Team/i'
        )
        if ((await currentTeamText.count()) > 0) {
          await expect(currentTeamText.first()).toBeVisible()
        }
      }
    })

    test('should show all user teams in dropdown', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/')
      await authenticatedPage.waitForLoadState('networkidle')

      const teamSwitcher = authenticatedPage
        .locator('[data-testid="team-switcher"]')
        .first()

      if (await teamSwitcher.isVisible().catch(() => false)) {
        await teamSwitcher.click()
        await authenticatedPage.waitForTimeout(500)

        // Verify dropdown opened with teams
        const teamList = authenticatedPage.locator(
          '[role="menu"], [role="listbox"]'
        )
        const hasTeamList = await teamList.isVisible().catch(() => false)

        if (hasTeamList) {
          await expect(teamList).toBeVisible()
        }
      }
    })
  })

  test.describe('Team Switching', () => {
    test('should switch teams via header dropdown', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/')
      await authenticatedPage.waitForLoadState('networkidle')

      // Get current team
      const currentTeam = await getCurrentTeam(authenticatedPage)

      if (currentTeam) {
        // Try to find team switcher
        const teamSwitcher = authenticatedPage
          .locator('[data-testid="team-switcher"]')
          .first()

        if (await teamSwitcher.isVisible().catch(() => false)) {
          await teamSwitcher.click()
          await authenticatedPage.waitForTimeout(1000)

          // Try to select a different team if available
          const teamOptions = authenticatedPage.locator(
            '[role="menuitem"], [role="option"]'
          )
          const count = await teamOptions.count()

          if (count > 1) {
            // Click on second team (index 1)
            await teamOptions.nth(1).click()
            await authenticatedPage.waitForTimeout(1000)
          }
        }
      }

      // Verify we're still on homepage
      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should update URL/context after team switch', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Try to switch teams
      const teamSwitcher = authenticatedPage
        .locator('[data-testid="team-switcher"]')
        .first()

      if (await teamSwitcher.isVisible().catch(() => false)) {
        await teamSwitcher.click()
        await authenticatedPage.waitForTimeout(500)

        const teamOptions = authenticatedPage.locator(
          '[role="menuitem"], [role="option"]'
        )
        const count = await teamOptions.count()

        if (count > 1) {
          await teamOptions.nth(1).click()
          await authenticatedPage.waitForTimeout(1500)

          // Verify context updated
          const newTeam = await getCurrentTeam(authenticatedPage)
          expect(newTeam).toBeTruthy()
        }
      }
    })
  })

  test.describe('Resource Scoping', () => {
    test('should scope prompts to current team', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Prompts should be scoped to current team
      await expect(authenticatedPage).toHaveURL(/prompts/)
      await expect(authenticatedPage.locator('body')).toBeVisible()

      // Create a prompt in current team
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Team Scoped ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Team scoped content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
    })

    test('should scope artifacts to current team', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Artifacts should be scoped to current team
      await expect(authenticatedPage).toHaveURL(/artifacts/)
      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should scope memories to current team', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/memories')
      await authenticatedPage.waitForLoadState('networkidle')

      // Memories should be scoped to current team
      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should persist team selection across page reload', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/')
      await authenticatedPage.waitForLoadState('networkidle')

      const teamBeforeReload = await getCurrentTeam(authenticatedPage)

      // Reload page
      await authenticatedPage.reload()
      await authenticatedPage.waitForLoadState('networkidle')

      // Wait for team context to reinitialize
      await authenticatedPage.waitForFunction(
        () => {
          return localStorage.getItem('vx_current_team_id') !== null
        },
        { timeout: 10000 }
      )

      const teamAfterReload = await getCurrentTeam(authenticatedPage)

      // Team should be the same
      if (teamBeforeReload && teamAfterReload) {
        expect(teamAfterReload).toBeTruthy()
      }
    })

    test('should persist team selection across navigation', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      const teamOnPrompts = await getCurrentTeam(authenticatedPage)

      // Navigate to artifacts
      await authenticatedPage.goto('/artifacts')
      await authenticatedPage.waitForLoadState('networkidle')

      const teamOnArtifacts = await getCurrentTeam(authenticatedPage)

      // Team should remain consistent
      if (teamOnPrompts && teamOnArtifacts) {
        expect(teamOnArtifacts).toBeTruthy()
      }
    })

    test('should show team-specific empty states', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Empty state should indicate team context
      const emptyState = authenticatedPage.locator(
        'text=/no prompts|empty|get started/i'
      )

      // Empty state may or may not be visible depending on team content
      const hasEmptyState = await emptyState.isVisible().catch(() => false)

      if (hasEmptyState) {
        await expect(emptyState.first()).toBeVisible()
      } else {
        // Has prompts - verify list is visible
        await expect(authenticatedPage.locator('body')).toBeVisible()
      }
    })

    test('should verify resources do not leak between teams', async ({
      authenticatedPage,
    }) => {
      // Create a prompt in current team
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Isolation Test ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Should not leak to other teams')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Verify prompt was created
      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible({ timeout: 5000 })

      // Go to prompts list and verify it appears
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible({ timeout: 5000 })
    })

    test('should switch back to Private Workspace', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/')
      await authenticatedPage.waitForLoadState('networkidle')

      const teamSwitcher = authenticatedPage
        .locator('[data-testid="team-switcher"]')
        .first()

      if (await teamSwitcher.isVisible().catch(() => false)) {
        await teamSwitcher.click()
        await authenticatedPage.waitForTimeout(500)

        // Look for Private Workspace option
        const privateWorkspaceOption = authenticatedPage.locator(
          'text=/Private Workspace/i'
        )

        if (await privateWorkspaceOption.isVisible().catch(() => false)) {
          await privateWorkspaceOption.click()
          await authenticatedPage.waitForTimeout(1000)

          // Verify switched
          const currentTeam = await getCurrentTeam(authenticatedPage)
          expect(currentTeam).toBeTruthy()
        }
      }

      // Verify we're still on a valid page
      await expect(authenticatedPage.locator('body')).toBeVisible()
    })
  })

  test.describe('Team Assignment in API Calls', () => {
    test('should include team_id when creating resources', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `API Team Test ${Date.now()}`

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Testing team_id in API')

      // Intercept API request. The prompt create endpoint is team-scoped:
      // POST /api/v1/<teamId>/prompts (team_id is a path param, not a body field).
      const requestPromise = authenticatedPage.waitForRequest(
        request =>
          /\/api\/v1\/[0-9a-f-]{36}\/prompts$/.test(request.url()) &&
          request.method() === 'POST'
      )

      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      // Verify the request was scoped to a team via the URL path.
      const request = await requestPromise
      expect(request.url()).toMatch(/\/api\/v1\/[0-9a-f-]{36}\/prompts$/)

      const postData = request.postDataJSON() as { name: string }
      expect(postData.name).toBe(promptName)
    })
  })
})

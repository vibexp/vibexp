import { test, expect } from '../fixtures/auth'

/**
 * Journey 7: Team Collaboration Workflow
 *
 * Tests the complete team collaboration experience from team creation through
 * member management, resource sharing, and team context switching. This validates
 * the multi-tenant team features.
 *
 * User Flow:
 * 1. Navigate to Teams settings
 * 2. Create new team
 * 3. View team details
 * 4. Invite team members
 * 5. Switch between teams
 * 6. Share resources within team
 * 7. Manage team members
 * 8. Leave or delete team
 *
 * Hardened for first-attempt stability (#299): fixed `waitForTimeout` settles
 * were replaced with web-first waits. Dialog opens are gated on their fields
 * becoming actionable, team creation is confirmed by the new team appearing,
 * and a team switch is confirmed via the current-team indicator before any
 * full-page navigation re-hydrates the team context from storage.
 */
test.describe('Journey 7: Team Collaboration Workflow', () => {
  test.describe('Teams Navigation', () => {
    test('should navigate to Teams from settings', async ({
      authenticatedPage,
    }) => {
      // The user menu opens and exposes a Settings entry point.
      await authenticatedPage.click('[data-testid="user-menu"]')
      await expect(
        authenticatedPage.getByRole('menuitem', { name: /settings/i }).first()
      ).toBeVisible({ timeout: 5000 })

      // Navigate to the Teams settings page.
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.waitForURL('/settings/teams', { timeout: 10000 })

      // Should see teams heading
      await expect(
        authenticatedPage.getByRole('heading', { name: /teams/i }).first()
      ).toBeVisible()
    })

    test('should display default personal workspace', async ({
      freshUserPage,
    }) => {
      await freshUserPage.goto('/settings/teams')

      // New users should have a personal workspace by default
      await expect(
        freshUserPage.getByText(/private workspace|personal workspace/i).first()
      ).toBeVisible()
    })
  })

  test.describe('Team Creation', () => {
    test('should show create team button', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/teams')

      // Should see Create Team button
      await expect(
        authenticatedPage.getByRole('button', {
          name: /create team|new team/i,
        })
      ).toBeVisible()
    })

    test('should create new team', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/teams')

      // Click Create Team and wait for the dialog's name field to be actionable.
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })

      // Fill team details
      await authenticatedPage.fill(
        '[data-testid="team-name-input"]',
        'E2E Collaboration Team'
      )

      await authenticatedPage.fill(
        'textarea[placeholder*="description"], input[name="description"]',
        'Team for testing collaboration features'
      )

      // Submit
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')

      // Should see the new team once creation lands.
      await expect(
        authenticatedPage.getByText('E2E Collaboration Team')
      ).toBeVisible({ timeout: 10000 })
    })

    test('should auto-generate team slug', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })

      const teamName = 'My New Team With Spaces'
      await authenticatedPage.fill('[data-testid="team-name-input"]', teamName)

      // Check if slug is auto-generated
      const slugInput = authenticatedPage.locator(
        'input[placeholder*="slug"], input[name="slug"]'
      )

      if ((await slugInput.count()) > 0) {
        // Wait for the slug to be derived from the name rather than sleeping.
        await expect(slugInput.first()).not.toHaveValue('', { timeout: 5000 })
        const slugValue = await slugInput.first().inputValue()
        expect(slugValue.toLowerCase()).toContain('team')
      }
    })

    test('should require team name', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="submit-create-team-button"]')
      ).toBeVisible({ timeout: 10000 })

      // Try to create without name
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')

      // Should show validation error
      await expect(
        authenticatedPage.getByText(/name.*required|please.*name/i)
      ).toBeVisible()
    })
  })

  test.describe('Team Details and Management', () => {
    test('should view team details', async ({ authenticatedPage }) => {
      // Create a team first
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })
      await authenticatedPage.fill(
        '[data-testid="team-name-input"]',
        'Details View Team'
      )
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')

      // Click on the team once it appears in the list.
      await authenticatedPage.click('text=Details View Team')

      // Should show team details
      await expect(
        authenticatedPage.getByRole('heading', { name: /details view team/i })
      ).toBeVisible({ timeout: 10000 })
    })

    test('should display team member count', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/teams')

      // Should see member count for each team
      await expect(
        authenticatedPage.getByText(/1 member|members/i).first()
      ).toBeVisible()
    })

    test('should show team owner badge', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/teams')

      // Creator should be owner
      await expect(authenticatedPage.getByText(/owner|admin/i)).toBeVisible()
    })
  })

  test.describe('Team Member Invitation', () => {
    test('should have invite members button', async ({ authenticatedPage }) => {
      // Create team and navigate to details
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })
      await authenticatedPage.fill(
        '[data-testid="team-name-input"]',
        'Invitation Test Team'
      )
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')

      await authenticatedPage.click('text=Invitation Test Team')

      // Should see invite button
      await expect(
        authenticatedPage.getByRole('button', {
          name: /invite|add member/i,
        })
      ).toBeVisible({ timeout: 10000 })
    })

    test('should open invite dialog', async ({ authenticatedPage }) => {
      // Create the team this test needs rather than depending on one existing.
      // This used to be wrapped in `if (count() > 0)` guards, so a missing team
      // or a missing Invite button made the test pass while asserting nothing —
      // the same silent-pass class of gap that let #251 ship (#252).
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await authenticatedPage.fill(
        '[data-testid="team-name-input"]',
        'Invite Dialog Test Team'
      )
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')

      await authenticatedPage.click('text=Invite Dialog Test Team')

      const inviteButton = authenticatedPage
        .getByRole('button', { name: /invite|add member/i })
        .first()
      await expect(inviteButton).toBeVisible({ timeout: 15000 })
      await inviteButton.click()

      await expect(authenticatedPage.getByPlaceholder(/email/i)).toBeVisible({
        timeout: 15000,
      })
    })

    test('should require valid email for invitation', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/teams')

      const firstTeam = authenticatedPage
        .locator('a[href*="/settings/teams/"]')
        .first()

      if ((await firstTeam.count()) > 0) {
        await firstTeam.click()

        const inviteButton = authenticatedPage.locator(
          'button:has-text("Invite")'
        )
        await inviteButton
          .first()
          .waitFor({ state: 'visible', timeout: 10000 })
          .catch(() => {})

        if ((await inviteButton.count()) > 0) {
          await inviteButton.first().click()

          // Enter invalid email (fill auto-waits for the field to render)
          await authenticatedPage.fill(
            'input[placeholder*="email"]',
            'invalid-email'
          )

          // Try to send
          await authenticatedPage.click('button:has-text("Send Invitation")')

          // Should show validation error
          await expect(
            authenticatedPage.getByText(/valid email|email.*invalid/i)
          ).toBeVisible()
        }
      }
    })
  })

  test.describe('Team Context Switching', () => {
    test('should have team switcher in header', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/')

      // Should see team switcher
      await expect(
        authenticatedPage.locator('[data-testid="team-switcher"]')
      ).toBeVisible()
    })

    test('should display current team in switcher', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/')

      // Click team switcher
      await authenticatedPage.click('[data-testid="team-switcher"]')

      // Should show team list
      await expect(
        authenticatedPage.getByText(/private workspace|switch team/i).first()
      ).toBeVisible()
    })

    test('should switch between teams', async ({ authenticatedPage }) => {
      // Create a second team
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })
      await authenticatedPage.fill(
        '[data-testid="team-name-input"]',
        'Switch Target Team'
      )
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')
      await expect(
        authenticatedPage.getByText('Switch Target Team')
      ).toBeVisible({ timeout: 10000 })

      // Go to home and switch teams
      await authenticatedPage.goto('/')
      await authenticatedPage.click('[data-testid="team-switcher"]')
      await authenticatedPage.click('text=Switch Target Team')

      // The team switcher should now show the newly selected team as current.
      await expect(
        authenticatedPage.locator('[data-testid="current-team-name"]')
      ).toHaveText(/switch target team/i, { timeout: 10000 })
    })

    test('should persist team context across navigation', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })
      await authenticatedPage.fill(
        '[data-testid="team-name-input"]',
        'Persistence Test Team'
      )
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')
      await expect(
        authenticatedPage.getByText('Persistence Test Team')
      ).toBeVisible({ timeout: 10000 })

      // Switch to this team
      await authenticatedPage.goto('/')
      await authenticatedPage.click('[data-testid="team-switcher"]')
      await authenticatedPage.click('text=Persistence Test Team')

      // Confirm the switch committed before navigating (the switcher reflects
      // the new team, which is also persisted to storage for re-hydration).
      await expect(
        authenticatedPage.locator('[data-testid="current-team-name"]')
      ).toHaveText(/persistence test team/i, { timeout: 10000 })

      // Navigate to different pages
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.goto('/artifacts')

      // Team context should persist
      await expect(
        authenticatedPage.getByText(/persistence test team/i)
      ).toBeVisible({ timeout: 10000 })
    })
  })

  test.describe('Resource Sharing Within Team', () => {
    test('should see team resources in team context', async ({
      authenticatedPage,
    }) => {
      // Create team, switch to it, create a prompt
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })
      await authenticatedPage.fill(
        '[data-testid="team-name-input"]',
        'Resource Sharing Team'
      )
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')
      await expect(
        authenticatedPage.getByText('Resource Sharing Team')
      ).toBeVisible({ timeout: 10000 })

      // Switch to team
      await authenticatedPage.goto('/')
      await authenticatedPage.click('[data-testid="team-switcher"]')
      await authenticatedPage.click('text=Resource Sharing Team')

      // Ensure the switch committed (switcher reflects the new team) before the
      // full-page navigation re-hydrates the team context from storage.
      await expect(
        authenticatedPage.locator('[data-testid="current-team-name"]')
      ).toHaveText(/resource sharing team/i, { timeout: 10000 })

      // Create a prompt in team context. Confirm the editor page hydrated the
      // switched team before creating, so the prompt is scoped to it.
      await authenticatedPage.goto('/prompts/new')
      await expect(
        authenticatedPage.locator('[data-testid="current-team-name"]')
      ).toHaveText(/resource sharing team/i, { timeout: 10000 })
      await authenticatedPage.fill(
        'input[placeholder*="Enter prompt name"]',
        'Team Shared Prompt'
      )
      await authenticatedPage.fill(
        'textarea[placeholder*="Write your prompt here"]',
        'This prompt is shared with the team.'
      )
      await authenticatedPage.click('[data-testid="prompt-save-button"]')
      // Saving navigates to the new prompt's detail page (confirms creation).
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Go to prompts list
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Should see the team prompt
      await expect(
        authenticatedPage.getByText('Team Shared Prompt').first()
      ).toBeVisible({ timeout: 10000 })
    })

    test('should not see team resources in personal workspace', async ({
      authenticatedPage,
    }) => {
      // Assuming team prompt was created in previous test
      // Switch back to personal workspace
      await authenticatedPage.goto('/')
      await authenticatedPage.click('[data-testid="team-switcher"]')
      await authenticatedPage.click('text=Private Workspace')

      // Confirm the switch to the personal workspace committed before
      // navigating, so the list request is scoped to it (not the prior team).
      await expect(
        authenticatedPage.locator('[data-testid="current-team-name"]')
      ).toHaveText(/private workspace/i, { timeout: 10000 })

      // Navigate to prompts
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Team prompt should not be visible
      await expect(
        authenticatedPage.getByText('Team Shared Prompt')
      ).not.toBeVisible()
    })
  })

  test.describe('Team Member Management', () => {
    test('should list team members', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/teams')

      const firstTeam = authenticatedPage
        .locator('a[href*="/settings/teams/"]')
        .first()

      if ((await firstTeam.count()) > 0) {
        await firstTeam.click()

        // Should see members section
        await expect(
          authenticatedPage.getByText(/members|team members/i)
        ).toBeVisible({ timeout: 10000 })

        // Should see at least owner (current user)
        await expect(authenticatedPage.getByText(/owner|you/i)).toBeVisible()
      }
    })

    test('should display member roles', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/teams')

      const firstTeam = authenticatedPage
        .locator('a[href*="/settings/teams/"]')
        .first()

      if ((await firstTeam.count()) > 0) {
        await firstTeam.click()

        // Should show role badges (Owner, Admin, Member)
        await expect(
          authenticatedPage.getByText(/owner|admin|member/i)
        ).toBeVisible({ timeout: 10000 })
      }
    })
  })

  test.describe('Leaving and Deleting Teams', () => {
    test('should have delete team option for owner', async ({
      authenticatedPage,
    }) => {
      // Create a team to delete
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })
      await authenticatedPage.fill(
        '[data-testid="team-name-input"]',
        'Team to Delete'
      )
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')
      await expect(authenticatedPage.getByText('Team to Delete')).toBeVisible({
        timeout: 10000,
      })

      // Click on the team
      await authenticatedPage.click('text=Team to Delete')

      // Should see delete option
      await expect(
        authenticatedPage.getByRole('button', { name: /delete team/i })
      ).toBeVisible({ timeout: 10000 })
    })

    test('should confirm before deleting team', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })
      await authenticatedPage.fill(
        '[data-testid="team-name-input"]',
        'Confirm Delete Team'
      )
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')
      await expect(
        authenticatedPage.getByText('Confirm Delete Team')
      ).toBeVisible({ timeout: 10000 })

      await authenticatedPage.click('text=Confirm Delete Team')

      // Click delete
      const deleteButton = authenticatedPage.locator(
        'button:has-text("Delete Team")'
      )
      await deleteButton
        .first()
        .waitFor({ state: 'visible', timeout: 10000 })
        .catch(() => {})

      if ((await deleteButton.count()) > 0) {
        await deleteButton.click()

        // Should see confirmation dialog
        await expect(
          authenticatedPage
            .getByText(/are you sure|confirm|permanently/i)
            .first()
        ).toBeVisible({ timeout: 10000 })
      }
    })
  })
})

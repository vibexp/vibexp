import type { Page } from '@playwright/test'

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

/**
 * Creates a team and opens its detail page.
 *
 * Tests that need a team create their own rather than clicking whatever is
 * first in the list (#559). Two reasons, both of which previously made tests
 * assert nothing at all:
 *
 *  - The teams list renders **no anchor** — the name cell is a `<button>` with
 *    an onClick — so the old `a[href*="/teams/"]` selector matched zero
 *    elements and its `count() > 0` guard was permanently false.
 *  - The `authenticatedPage` fixture mints a fresh user per test, so the only
 *    pre-existing team is the personal Private Workspace, which hides Invite
 *    and other team affordances. "The first team" is the wrong team.
 *
 * `teamName` must be unique per test — colliding names made a sibling spec
 * ambiguous in #597.
 */
async function createTeamAndOpen(page: Page, teamName: string): Promise<void> {
  await page.goto('/teams')
  await page.click('[data-testid="create-team-button"]')
  await page.fill('[data-testid="team-name-input"]', teamName)
  await page.click('[data-testid="submit-create-team-button"]')

  // Opens via the stable hook on the row's clickable name, so a future
  // restructure of the list fails this loudly instead of silently skipping.
  //
  // Asserted before the click rather than relying on click's auto-wait: both
  // wait, but only this reports WHICH step broke. A bare click that times out
  // says "selector not found" whether the team was never created, the list did
  // not refresh, or the hook was renamed — and a test suite whose failures do
  // not say what went wrong is how this file ended up asserting nothing.
  const row = page.locator(
    `[data-testid="team-row-link"]:has-text("${teamName}")`
  )
  await expect(row).toBeVisible({ timeout: 15000 })
  await row.click()
}
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
      await authenticatedPage.goto('/teams')
      await authenticatedPage.waitForURL('/teams', { timeout: 10000 })

      // Should see teams heading
      await expect(
        authenticatedPage.getByRole('heading', { name: /teams/i }).first()
      ).toBeVisible()
    })

    test('should display default personal workspace', async ({
      freshUserPage,
    }) => {
      await freshUserPage.goto('/teams')

      // New users should have a personal workspace by default
      await expect(
        freshUserPage.getByText(/private workspace|personal workspace/i).first()
      ).toBeVisible()
    })
  })

  test.describe('Team Creation', () => {
    test('should show create team button', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/teams')

      // Should see Create Team button
      await expect(
        authenticatedPage.getByRole('button', {
          name: /create team|new team/i,
        })
      ).toBeVisible()
    })

    test('should create new team', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/teams')

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

    // The slug is derived server-side and the SPA never surfaces it:
    // CreateTeamModal renders only name + description (`CreateTeamModal.tsx`),
    // no teams route or page displays a slug, and `/teams/:id` routes on the
    // team id. So there is no selector this test could be pointed at — it was
    // passing purely because its `count() > 0` guard skipped the whole body
    // (#607). Skipped explicitly, with the precondition named, so the report
    // says "skipped" instead of "passed"; unskip if the UI ever exposes a slug
    // field.
    test.skip('should auto-generate team slug', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })

      const teamName = 'My New Team With Spaces'
      await authenticatedPage.fill('[data-testid="team-name-input"]', teamName)

      const slugInput = authenticatedPage.locator(
        'input[placeholder*="slug"], input[name="slug"]'
      )

      // Wait for the slug to be derived from the name rather than sleeping.
      await expect(slugInput.first()).not.toHaveValue('', { timeout: 5000 })
      const slugValue = await slugInput.first().inputValue()
      expect(slugValue.toLowerCase()).toContain('team')
    })

    test('should require team name', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/teams')
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
      await authenticatedPage.goto('/teams')
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
      await authenticatedPage.goto('/teams')

      // Should see member count for each team
      await expect(
        authenticatedPage.getByText(/1 member|members/i).first()
      ).toBeVisible()
    })

    test('should show team owner badge', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/teams')

      // Creator should be owner
      await expect(authenticatedPage.getByText(/owner|admin/i)).toBeVisible()
    })
  })

  test.describe('Team Member Invitation', () => {
    test('should have invite members button', async ({ authenticatedPage }) => {
      // Create team and navigate to details
      await authenticatedPage.goto('/teams')
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
      await authenticatedPage.goto('/teams')
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
      // Creates its own team, like `should open invite dialog` above. The
      // previous version selected `a[href*="/teams/"]` — the list renders no
      // anchor at all, so it matched nothing, the count() > 0 guard was always
      // false, and this test asserted NOTHING while reporting green (#559).
      //
      // Clicking "the first team" would not fix it either: the authenticatedPage
      // fixture mints a fresh user, whose only pre-existing team is the personal
      // Private Workspace — and TeamScopeHeader hides Invite for a personal
      // team. A first-row click would legitimately find no Invite button, which
      // is what the second, nested guard was papering over.
      await createTeamAndOpen(authenticatedPage, 'Invalid Email Test Team')

      await authenticatedPage
        .getByRole('button', { name: /invite|add member/i })
        .first()
        .click()

      // Located by label, not by `input[placeholder*="email"]` as before: the
      // field is a <textarea>, so an `input[...]` selector never matches it —
      // a second reason this test could not have worked even with a team open.
      await authenticatedPage
        .getByLabel(/email addresses/i)
        .fill('invalid-email')

      // "Send Invitations", plural — the old selector's singular substring
      // happened to still match, but pin the real copy.
      await authenticatedPage
        .getByRole('button', { name: /send invitations/i })
        .click()

      // The modal reports `Invalid email at position 1: invalid-email`.
      await expect(
        authenticatedPage.getByText(/invalid email at position/i)
      ).toBeVisible({ timeout: 10000 })
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
      await authenticatedPage.goto('/teams')
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
      await authenticatedPage.goto('/teams')
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
    /**
     * Cross-team isolation, proven on ONE user session (#415).
     *
     * This was two tests. Because the `authenticatedPage` fixture provisions a
     * fresh user per test, the old "should not see team resources in personal
     * workspace" test asserted the absence of a prompt created by the *previous*
     * test — i.e. by a different user, for whom it had never existed in any
     * workspace. That assertion passed trivially and would have kept passing
     * with team scoping completely removed. Creating the resource and checking
     * its absence in another context within the same session is what actually
     * exercises the scoping.
     */
    test('team resources are visible in team context and absent from the personal workspace', async ({
      authenticatedPage,
    }) => {
      // Longest test in this file: it drives two workspace contexts in one
      // session. It measures ~4s locally against the e2e stack, but it is also
      // the one test here whose budget a slow CI runner could genuinely
      // threaten, so give it explicit headroom rather than the shared 30s.
      test.setTimeout(60_000)

      // Unique per run so the closing absence assertion cannot be satisfied —
      // or defeated — by a leftover from another run or retry (same pattern as
      // the teardown test below).
      const promptName = `Team Shared Prompt ${Date.now()}`

      // --- Team context: create the team, switch to it, create the prompt ----
      await authenticatedPage.goto('/teams')
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
        promptName
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

      // The prompt is listed in the team it was created in.
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')
      await expect(authenticatedPage.getByText(promptName).first()).toBeVisible(
        { timeout: 10000 }
      )

      // --- Personal workspace: the same prompt must NOT be listed ------------
      await authenticatedPage.goto('/')
      await authenticatedPage.click('[data-testid="team-switcher"]')
      await authenticatedPage.click('text=Private Workspace')

      // Confirm the switch to the personal workspace committed before
      // navigating, so the list request is scoped to it (not the prior team).
      await expect(
        authenticatedPage.locator('[data-testid="current-team-name"]')
      ).toHaveText(/private workspace/i, { timeout: 10000 })

      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Re-confirm on the list page itself: the full-page navigation re-hydrates
      // the team context from storage, and this list request must be the
      // personal workspace's, not the team's.
      await expect(
        authenticatedPage.locator('[data-testid="current-team-name"]')
      ).toHaveText(/private workspace/i, { timeout: 10000 })

      // Positive proof that the list rendered for this workspace before any
      // absence is asserted: the fresh user's personal workspace owns no
      // prompts, so the list's own empty state is the signal that the request
      // completed and came back empty. A bare `not.toBeVisible()` would also
      // pass against a list that never loaded — which is exactly how the
      // previous version of this test managed to assert nothing at all.
      // Matched case-insensitively on purpose: this gate is load-bearing, and
      // #595 is this repo's example of exact-case copy matching breaking a spec.
      await expect(authenticatedPage.getByText(/no prompts yet/i)).toBeVisible({
        timeout: 10000,
      })

      // The team-scoped prompt is not visible outside its team.
      await expect(authenticatedPage.getByText(promptName)).not.toBeVisible()
    })
  })

  test.describe('Team Member Management', () => {
    test('should list team members', async ({ authenticatedPage }) => {
      await createTeamAndOpen(authenticatedPage, 'Member List Test Team')

      // Scoped to the heading rather than getByText(/members/i): that regex also
      // matches the "Total members" stat card, so once the guard is gone and the
      // assertion actually runs it would fail Playwright strict mode.
      await expect(
        authenticatedPage.getByRole('heading', { name: /team members/i })
      ).toBeVisible({ timeout: 10000 })

      // Exactly one: the fixture mints a fresh user per test, so a team they
      // just created has precisely one member. An "at least one" assertion would
      // not notice the list rendering someone else's members.
      await expect(
        authenticatedPage.locator('[data-testid="team-member-row"]')
      ).toHaveCount(1)
    })

    test('should display member roles', async ({ authenticatedPage }) => {
      await createTeamAndOpen(authenticatedPage, 'Member Roles Test Team')

      // Scoped to the member row: /owner|admin|member/i unscoped matches the
      // stat card and the section heading too, so this would be a strict-mode
      // failure the moment it stopped being skipped.
      await expect(
        authenticatedPage
          .locator('[data-testid="team-member-row"]')
          .first()
          .getByText(/owner/i)
      ).toBeVisible({ timeout: 10000 })
    })
  })

  test.describe('Leaving and Deleting Teams', () => {
    test('should have delete team option for owner', async ({
      authenticatedPage,
    }) => {
      // Create a team to delete
      await authenticatedPage.goto('/teams')
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

      // Click on the team. Selected by its test hook: the list renders no
      // anchor and a bare text selector also matches the header team switcher
      // (#559, #597).
      await authenticatedPage
        .locator('[data-testid="team-row-link"]', { hasText: 'Team to Delete' })
        .click()
      await authenticatedPage.waitForURL(/\/teams\/[0-9a-f-]+$/, {
        timeout: 10000,
      })

      // Since #670 delete is a menu item in the scope header's overflow menu,
      // so it is not in the DOM until the menu is opened — and it carries role
      // `menuitem`, not `button`. This test kept asserting the pre-#670 shape
      // and was red on main until #681.
      await authenticatedPage
        .locator('[data-testid="team-actions-menu"]')
        .click()
      await expect(
        authenticatedPage.locator('[data-testid="delete-team-button"]')
      ).toBeVisible({ timeout: 10000 })
    })

    test('should confirm before deleting team', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/teams')
      await authenticatedPage.click('[data-testid="create-team-button"]')
      await expect(
        authenticatedPage.locator('[data-testid="team-name-input"]')
      ).toBeVisible({ timeout: 10000 })
      // Deliberately NOT a name that contains a UI control label: team names
      // are user data and collide with text-based selectors (#597).
      const teamName = `Journey7 Confirm Teardown ${Date.now()}`
      await authenticatedPage.fill('[data-testid="team-name-input"]', teamName)
      await authenticatedPage.click('[data-testid="submit-create-team-button"]')
      await expect(authenticatedPage.getByText(teamName)).toBeVisible({
        timeout: 10000,
      })

      await authenticatedPage
        .locator('[data-testid="team-row-link"]', { hasText: teamName })
        .click()
      await authenticatedPage.waitForURL(/\/teams\/[0-9a-f-]+$/, {
        timeout: 10000,
      })

      // Click delete. Selected by its test hook rather than by text: since
      // #544 the header team switcher renders the current team's name while
      // disabled, so `button:has-text("Delete Team")` also matches the switcher
      // for a team whose name contains that substring - which this fixture's
      // does.
      //
      // Since #666 delete sits in the scope header's overflow menu, so the menu
      // is opened first — the item is not in the DOM until then.
      await authenticatedPage
        .locator('[data-testid="team-actions-menu"]')
        .click()

      // Unconditional (#607): the fixture's user owns this team, so the delete
      // item MUST be there. Swallowing the wait and gating the click on
      // `count() > 0` meant a missing menu item passed as a green test.
      const deleteButton = authenticatedPage.locator(
        '[data-testid="delete-team-button"]'
      )
      await expect(deleteButton).toBeVisible({ timeout: 10000 })
      await deleteButton.click()

      // Should see confirmation dialog
      await expect(
        authenticatedPage.getByText(/are you sure|confirm|permanently/i).first()
      ).toBeVisible({ timeout: 10000 })
    })
  })
})

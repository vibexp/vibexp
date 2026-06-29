import { test, expect } from '../../fixtures/auth'

/**
 * Feature Test: Team Invitation Accept page
 *
 * Closes the e2e gap on the public `/invitations/accept/:token` route (issue
 * #66). The invitation token is intentionally never exposed by the API (it is
 * only delivered by email — see convertInvitationsToResponses, which omits it),
 * so a full two-user accept is a backend-integration concern, not a clean e2e.
 *
 * What this proves end-to-end: the route is reachable, the token-load path runs,
 * and an unresolvable token degrades gracefully (error card + recovery action)
 * instead of crashing or rendering a blank page.
 */
test.describe('Team Invitation Accept', () => {
  test('an unresolvable invitation token renders a graceful error', async ({
    authenticatedPage: page,
  }) => {
    await page.goto(`/invitations/accept/nonexistent-e2e-token-${Date.now()}`)

    // The page resolved the token (and failed), so it shows the error card with
    // its recovery action rather than a crash or a blank screen.
    await expect(
      page.getByRole('button', { name: /go to dashboard/i })
    ).toBeVisible({ timeout: 10000 })
    await expect(page.getByRole('alert')).toBeVisible()
  })
})

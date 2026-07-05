import { test as base, expect, type Page } from '@playwright/test'

/**
 * Shared dev-login implementation.
 *
 * The redesigned sign-in page (shadcn) renders the dev login form inside a
 * collapsed Radix `Collapsible`. `?dev_login=true` only controls whether the
 * component is rendered at all — the form fields (#dev-email / #dev-name) live
 * inside `CollapsibleContent` and stay hidden until the "Development login"
 * trigger is clicked. We therefore expand the collapsible before filling it.
 *
 * Auth itself is an httpOnly session cookie set by `/auth/dev/login` (there is
 * no JWT in localStorage anymore), so we confirm success via the team context
 * being hydrated rather than by reading a token.
 */
async function performDevLogin(
  page: Page,
  email: string,
  name: string
): Promise<void> {
  // Navigate to login page with dev_login enabled
  await page.goto('/?dev_login=true')

  // The dev login form is collapsed by default — expand it via its trigger.
  const trigger = page.getByRole('button', { name: /development login/i })
  await expect(trigger).toBeVisible()
  await trigger.click()

  // Wait for the form fields to become actionable inside the expanded panel.
  await expect(page.locator('#dev-email')).toBeVisible()

  // Fill and submit the dev login form.
  await page.fill('#dev-email', email)
  await page.fill('#dev-name', name)
  await page.click('button:has-text("Dev login")')

  // Wait for redirect to home page.
  await page.waitForURL('/', { timeout: 15000 })

  // CRITICAL: Wait for team context to be initialized.
  // The TeamContext fetches teams asynchronously, and many components depend on
  // currentTeam. The current team id is persisted to localStorage once teams
  // load, which is also our signal that the authenticated session is ready.
  // Every login provisions a brand-new user (user + personal team + default
  // project), which under CI load occasionally takes longer than one wait
  // budget — the session cookie is already set at this point, so a single
  // reload re-triggers the teams fetch without re-submitting the form (#86).
  try {
    await waitForTeamHydration(page)
  } catch {
    console.warn(
      '[e2e] team hydration slow after dev login — reloading once (#86)'
    )
    await page.reload()
    await waitForTeamHydration(page)
  }

  // Give an extra moment for React to process the team context update.
  await page.waitForTimeout(500)
}

async function waitForTeamHydration(page: Page): Promise<void> {
  await page.waitForFunction(
    () => localStorage.getItem('vx_current_team_id') !== null,
    { timeout: 15000 }
  )
}

/**
 * Custom fixture that provides authenticated context using dev login.
 *
 * Each login fixture carries its OWN timeout (tuple form) so setup is not
 * boxed by the 30s per-test timeout: `performDevLogin`'s internal waits alone
 * allow 15s (redirect) + 15s (team hydration, retried once), which cannot fit
 * the test budget and was intermittently failing random specs during fixture
 * setup under CI load (#86).
 */
export const test = base.extend({
  /**
   * Authenticated page fixture - logs in before each test using this fixture
   * This is reusable across all tests that need authentication
   */
  authenticatedPage: [
    async ({ page }, use) => {
      const testEmail = `playwright_vibexp${Date.now()}@example.com`
      await performDevLogin(page, testEmail, 'Playwright Test User')

      // Use the authenticated page in the test
      await use(page)
    },
    { timeout: 60000 },
  ],

  /**
   * Authenticated page with team context - similar to authenticatedPage
   * but explicitly provides team context for team-scoped tests
   */
  authenticatedPageWithTeam: [
    async ({ page }, use) => {
      await devLogin(page)

      // Ensure we have a team context
      const currentTeam = await getCurrentTeam(page)
      if (!currentTeam) {
        throw new Error('No team context found after login')
      }

      await use(page)
    },
    { timeout: 60000 },
  ],

  /**
   * Fresh user page fixture - creates a new user for tests that need
   * a clean user state (e.g., onboarding, first-time flows)
   */
  freshUserPage: [
    async ({ page }, use) => {
      // Create a unique email for this fresh user
      const freshEmail = `fresh_user_${Date.now()}@example.com`
      const freshName = `Fresh User ${Date.now()}`

      await devLogin(page, freshEmail, freshName)

      await use(page)
    },
    { timeout: 60000 },
  ],
})

/**
 * Helper function to perform dev login manually if needed
 * This can be used in tests that need more control over the login process
 */
export async function devLogin(
  page: Page,
  email?: string,
  name?: string
): Promise<void> {
  const testEmail = email || `playwright_vibexp${Date.now()}@example.com`
  const testName = name || 'Playwright Test User'
  await performDevLogin(page, testEmail, testName)
}

/**
 * Helper to check if user is authenticated.
 *
 * Auth is an httpOnly session cookie (not readable from JS), so we use the
 * hydrated team context (persisted to localStorage after a successful session
 * loads) as the authentication signal. This survives reloads because the app
 * re-hydrates the session from the cookie via GET /auth/me on mount.
 */
export async function isAuthenticated(page: Page): Promise<boolean> {
  try {
    return await page.evaluate(
      () => localStorage.getItem('vx_current_team_id') !== null
    )
  } catch {
    return false
  }
}

/**
 * Helper to logout by clearing the session cookie and any client state.
 */
export async function logout(page: Page): Promise<void> {
  // Clear the httpOnly session cookie from the browser context.
  await page.context().clearCookies()

  // Clear any persisted client state (team context, etc.).
  await page.evaluate(() => localStorage.clear())

  // Navigate to home/login page
  await page.goto('/')
}

/**
 * Helper to get current team name from the UI
 */
export async function getCurrentTeam(page: Page): Promise<string | null> {
  try {
    // Get the current team ID from localStorage
    const teamId = await page.evaluate(() =>
      localStorage.getItem('vx_current_team_id')
    )

    if (!teamId) {
      return null
    }

    // Try to get team name from the UI (header/nav area)
    // This selector may need adjustment based on your actual UI
    const teamNameElement = page
      .locator('[data-testid="current-team-name"]')
      .first()
    if (await teamNameElement.isVisible({ timeout: 1000 }).catch(() => false)) {
      return await teamNameElement.textContent()
    }

    // Fallback: return team ID if we can't get the name
    return teamId
  } catch {
    return null
  }
}

/**
 * Helper to switch to a different team
 */
export async function switchTeam(page: Page, teamName: string): Promise<void> {
  // Look for team switcher button (adjust selector based on your UI)
  const teamSwitcher = page.locator('[data-testid="team-switcher"]').first()

  if (await teamSwitcher.isVisible({ timeout: 1000 }).catch(() => false)) {
    await teamSwitcher.click()

    // Wait for team list to appear
    await page.waitForTimeout(500)

    // Click on the desired team
    await page.getByText(teamName, { exact: true }).click()

    // Wait for team context to update
    await page.waitForTimeout(500)
  } else {
    throw new Error(
      'Team switcher not found. User may not have multiple teams.'
    )
  }
}

export { expect }

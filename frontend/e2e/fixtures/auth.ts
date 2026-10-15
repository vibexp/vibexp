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
  name: string,
  options: DevLoginOptions = {}
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
  // The session cookie is set by this response, before the app's hard
  // redirect away from the sign-in page.
  const signedIn = page.waitForResponse(
    response =>
      response.url().includes('/api/v1/auth/dev/login') && response.ok(),
    { timeout: 15000 }
  )
  await page.click('button:has-text("Dev login")')
  await signedIn

  // Wait for redirect to home page, or to the first-run onboarding the
  // dashboard sends a new user on to. The sign-in page is `/` too
  // (`/?dev_login=true`), so the query tells the two apart.
  await page.waitForURL(
    url =>
      !url.searchParams.has('dev_login') &&
      (url.pathname === '/' || url.pathname === ONBOARDING_PATH),
    { timeout: 15000 }
  )

  if (options.keepOnboarding) {
    // The onboarding renders outside the team shell, so there is no team
    // context to wait for: the journey asserts on the flow itself.
    await page.waitForURL(url => url.pathname === ONBOARDING_PATH, {
      timeout: 15000,
    })
    return
  }

  // Before the team wait: the onboarding renders outside the team shell, so
  // a user the dashboard already sent there would never hydrate a team.
  await completeOnboarding(page)

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

/** Where the first-run onboarding lives. */
export const ONBOARDING_PATH = '/getting-started'

/**
 * Where a sign-in lands: the dashboard, or for a user who has not finished
 * the first-run onboarding, the onboarding the dashboard hands them to.
 */
export const SIGNED_IN_LANDING = /\/(getting-started\?step=\d+)?$/

/**
 * Mark the signed-in user's first-run onboarding as finished, and load the
 * dashboard.
 *
 * Every dev login provisions a brand-new user, and a new user landing on the
 * dashboard is sent to the onboarding. Specs that are not about the
 * onboarding expect the dashboard, so the shared login finishes it for them
 * the way a person would (the same endpoint "Skip setup" calls), then
 * reloads the dashboard. The onboarding journey opts out with
 * `keepOnboarding`.
 */
export async function completeOnboarding(page: Page): Promise<void> {
  const response = await page.request.post('/api/v1/user/onboarding/complete')
  expect(
    response.ok(),
    `marking onboarding complete failed: ${response.status()}`
  ).toBe(true)
  // A full load, not a client-side navigation: the app holds the user it
  // fetched at sign-in (still "not completed") until it fetches it again.
  // The app may still be finishing its own navigation (the sign-in's hard
  // redirect, or the dashboard handing a new user to the onboarding), which
  // interrupts or aborts ours; try again once the page has loaded.
  for (let attempt = 1; ; attempt++) {
    try {
      await page.goto('/')
      break
    } catch (error) {
      const interrupted = /interrupted by another navigation|ERR_ABORTED/.test(
        String(error)
      )
      if (!interrupted || attempt === 3) throw error
      await page.waitForLoadState('load')
    }
  }
  await page.waitForURL('/')
}

/** Options for {@link devLogin}. */
export interface DevLoginOptions {
  /**
   * Leave the new user's first-run onboarding unfinished, so the app opens
   * it. Only the onboarding journey wants this.
   */
  keepOnboarding?: boolean
}

async function waitForTeamHydration(page: Page): Promise<void> {
  await page.waitForFunction(
    () => localStorage.getItem('vx_current_team_id') !== null,
    { timeout: 15000 }
  )
}

/**
 * Fixture types for the extended `test`.
 *
 * Declaring these is what lets specs see the custom fixtures: without the type
 * argument `base.extend` returns a `TestType` carrying only the stock
 * Playwright fixture args, so every `test('…', async ({ authenticatedPage })`
 * signature is a type error even though Playwright resolves fixtures by name
 * at runtime (#621).
 *
 * Each fixture hands its `use()` a plain Playwright `Page` that has already
 * been through dev login.
 */
export interface AuthFixtures {
  authenticatedPage: Page
  authenticatedPageWithTeam: Page
  freshUserPage: Page
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
export const test = base.extend<AuthFixtures>({
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
  name?: string,
  options: DevLoginOptions = {}
): Promise<void> {
  const testEmail = email || `playwright_vibexp${Date.now()}@example.com`
  const testName = name || 'Playwright Test User'
  await performDevLogin(page, testEmail, testName, options)
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

// Re-exported so specs can pull `test`, `expect` and the `Page` type they
// annotate helpers with from this one fixture module.
export { expect }
export type { Page }

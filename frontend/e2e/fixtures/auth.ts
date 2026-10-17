import { test as base, expect, type Page } from '@playwright/test'

/**
 * Shared dev-login implementation.
 *
 * Signs in through the dev-login API rather than the sign-in form (#1565):
 * `page.request` shares the page's cookie jar, so the httpOnly session cookie
 * `/auth/dev/login` sets is the page's session, with no sign-in page load,
 * form fill or redirect to wait for. The sign-in form itself is covered by
 * auth.spec.ts and the dev-login specs, which drive it directly.
 *
 * There is no JWT in localStorage, so success is confirmed by the team
 * context being hydrated rather than by reading a token.
 */
async function performDevLogin(
  page: Page,
  email: string,
  name: string,
  options: DevLoginOptions = {}
): Promise<void> {
  const response = await page.request.post('/api/v1/auth/dev/login', {
    data: { email, name },
    timeout: 15000,
  })
  expect(response.ok(), `dev login failed: ${response.status()}`).toBe(true)

  if (options.keepOnboarding) {
    // A new user landing on the dashboard is sent to the onboarding, which
    // renders outside the team shell, so there is no team context to wait
    // for: the journey asserts on the flow itself.
    await page.goto('/')
    await page.waitForURL(url => url.pathname === ONBOARDING_PATH, {
      timeout: 15000,
    })
    return
  }

  // Finishes the new user's onboarding and loads the dashboard.
  await completeOnboarding(page)

  // CRITICAL: Wait for team context to be initialized.
  // The TeamContext fetches teams asynchronously, and many components depend on
  // currentTeam. The current team id is persisted to localStorage once teams
  // load, which is also our signal that the authenticated session is ready.
  // Every login provisions a brand-new user (user + personal team + default
  // project), which under CI load occasionally takes longer than one wait
  // budget — the session cookie is already set at this point, so a single
  // reload re-triggers the teams fetch (#86).
  try {
    await waitForTeamHydration(page)
  } catch {
    console.warn(
      '[e2e] team hydration slow after dev login — reloading once (#86)'
    )
    await page.reload()
    await waitForTeamHydration(page)
  }
}

/**
 * A test-user email no other test can produce, including one running at the
 * same millisecond in another worker (a `Date.now()` stamp can collide, and
 * two tests would then silently share a user).
 */
export function uniqueEmail(prefix = 'playwright_vibexp'): string {
  return `${prefix}_${crypto.randomUUID()}@example.com`
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
      const testEmail = uniqueEmail()
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
      const freshEmail = uniqueEmail('fresh_user')
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
  const testEmail = email || uniqueEmail()
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

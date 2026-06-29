import { test, expect } from '@playwright/test'
import { devLogin, isAuthenticated } from './fixtures/auth'

test.describe('Dev Login', () => {
  test('should successfully login with dev login and access dashboard', async ({
    page,
  }) => {
    // Navigate to login page with dev_login enabled
    await page.goto('/?dev_login=true')

    // Verify the dev login trigger is visible, then expand the collapsible form
    await expect(page.getByText('Development Login')).toBeVisible()
    await page.getByRole('button', { name: /development login/i }).click()
    await expect(page.locator('#dev-email')).toBeVisible()
    await expect(page.locator('#dev-name')).toBeVisible()

    // Fill in dev login credentials with playwright-specific email
    const testEmail = `playwright_vibexp${Date.now()}@example.com`
    await page.fill('#dev-email', testEmail)
    await page.fill('#dev-name', 'Playwright Test User')

    // Submit the form
    await page.click('button:has-text("Dev Login")')

    // Wait for navigation to home page
    await page.waitForURL('/', { timeout: 10000 })

    // Verify we're on the home page
    expect(page.url()).toMatch(/\/$|\/\?/)

    // Wait for the session to hydrate (team context persisted) before asserting
    await page.waitForFunction(
      () => localStorage.getItem('vx_current_team_id') !== null,
      { timeout: 10000 }
    )

    // Verify authenticated (cookie session -> hydrated team context)
    const authenticated = await isAuthenticated(page)
    expect(authenticated).toBe(true)

    // Verify home page content is visible
    await expect(page.locator('body')).toBeVisible()
  })

  test('should hide dev login when dev_login=false query param is set', async ({
    page,
  }) => {
    // Navigate to login page with dev_login disabled
    await page.goto('/?dev_login=false')

    // Verify dev login form is NOT visible
    await expect(page.getByText('Development Login')).not.toBeVisible()

    // Verify the sign-in page itself still renders. We assert the page heading
    // rather than a specific identity provider ("Continue with Google"): the
    // open-source default config has no AUTH_PROVIDERS configured, so no
    // provider button renders — the sign-in page must not depend on one.
    await expect(
      page.getByRole('heading', { name: /sign in to/i })
    ).toBeVisible()
  })

  test('should show dev login by default in development', async ({ page }) => {
    // Navigate to login page without query params
    await page.goto('/')

    // In development mode, dev login should be visible by default
    // (This test assumes you're running in development mode)
    await expect(page.getByText('Development Login')).toBeVisible()
  })

  test('should disable login button when email is empty', async ({ page }) => {
    await page.goto('/?dev_login=true')
    await page.getByRole('button', { name: /development login/i }).click()

    // Button should be disabled without email
    const loginButton = page.locator('button:has-text("Dev Login")')
    await expect(loginButton).toBeDisabled()

    // Fill email and button should become enabled
    const testEmail = `playwright_vibexp${Date.now()}@example.com`
    await page.fill('#dev-email', testEmail)
    await expect(loginButton).toBeEnabled()
  })

  test('should allow login with email only (name is optional)', async ({
    page,
  }) => {
    await page.goto('/?dev_login=true')
    await page.getByRole('button', { name: /development login/i }).click()

    const testEmail = `playwright_vibexp${Date.now()}@example.com`
    await page.fill('#dev-email', testEmail)
    // Don't fill name - it's optional

    await page.click('button:has-text("Dev Login")')

    // Should successfully redirect to home page
    await page.waitForURL('/', { timeout: 10000 })
    expect(page.url()).toMatch(/\/$|\/\?/)
  })

  test('should create new user on first login', async ({ page }) => {
    const uniqueEmail = `playwright_vibexp${Date.now()}@example.com`

    await devLogin(page, uniqueEmail, 'Playwright New User')

    // Verify we're authenticated and on home page
    expect(page.url()).toMatch(/\/$|\/\?/)
    const authenticated = await isAuthenticated(page)
    expect(authenticated).toBe(true)
  })
})

test.describe('Home Page Access', () => {
  test('authenticated user can access home page', async ({ page }) => {
    // Use the helper to login
    await devLogin(page)

    // Navigate to home explicitly
    await page.goto('/')

    // Verify home page loads
    await expect(page.locator('body')).toBeVisible()
    expect(page.url()).toMatch(/\/$|\/\?/)
  })

  test('unauthenticated user sees login page', async ({ page }) => {
    // Try to access home without authentication
    await page.goto('/')

    // Verify the login page is shown. We assert the sign-in heading rather than
    // a specific provider button — the default open-source config configures no
    // AUTH_PROVIDERS, so the page must be provider-agnostic.
    await expect(
      page.getByRole('heading', { name: /sign in to/i })
    ).toBeVisible({ timeout: 5000 })
  })
})

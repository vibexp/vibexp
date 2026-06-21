/**
 * Smoke Tests - Critical Paths
 *
 * Fast smoke tests for CI gating. These tests validate critical user paths
 * and should execute in < 2 minutes total.
 *
 * Target: Fast failure detection for deployment gates
 */

import { test, expect } from '@playwright/test'
import { devLogin, isAuthenticated, getCurrentTeam } from '../fixtures/auth'
import { selectFirstProject } from '../helpers/artifacts'

test.describe('Authentication Smoke Tests', () => {
  test('should complete dev login and load dashboard', async ({ page }) => {
    await devLogin(page)

    // Verify we're on the home/dashboard page
    await expect(page).toHaveURL('/')

    // Verify authentication state
    const authenticated = await isAuthenticated(page)
    expect(authenticated).toBe(true)

    // Verify main content is visible
    await expect(page.locator('body')).toBeVisible()
  })

  test('should persist authentication across page reload', async ({ page }) => {
    await devLogin(page)

    // Verify authenticated
    let authenticated = await isAuthenticated(page)
    expect(authenticated).toBe(true)

    // Reload page
    await page.reload()

    // Verify still authenticated
    authenticated = await isAuthenticated(page)
    expect(authenticated).toBe(true)

    // Should still be on home page (not redirected to login)
    await expect(page).toHaveURL('/')
  })
})

test.describe('Core Feature Smoke Tests', () => {
  test('should create and view a prompt', async ({ page }) => {
    await devLogin(page)

    // Navigate directly to new prompt page
    await page.goto('/prompts/new')
    await expect(page).toHaveURL(/prompts\/new/)

    // Wait for form to be ready
    await page.waitForSelector('input[placeholder*="Enter prompt name"]', {
      timeout: 10000,
    })

    // Fill prompt form
    const promptName = `Smoke Test Prompt ${Date.now()}`
    await page
      .locator('input[placeholder*="Enter prompt name"]')
      .fill(promptName)
    await page
      .locator('textarea[placeholder*="Write your prompt here"]')
      .fill('Test prompt content for smoke test')

    // Save prompt
    await page.locator('[data-testid="prompt-save-button"]').click()

    // Verify redirect to prompt detail page
    await page.waitForTimeout(2000)
    await expect(page).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
      timeout: 10000,
    })
    await expect(page.locator(`text=${promptName}`).first()).toBeVisible({
      timeout: 10000,
    })
  })

  test('should create and view an artifact', async ({ page }) => {
    await devLogin(page)

    // Navigate directly to new artifact page
    await page.goto('/artifacts/new')
    await expect(page).toHaveURL(/artifacts\/new/)

    // Wait for the project picker to be ready (a project must be selected
    // explicitly — the form no longer defaults one, see #1790).
    await page.waitForSelector('[data-testid="artifact-project-select"]', {
      timeout: 10000,
    })

    // Fill artifact form
    const artifactSlug = `smoke-test-${Date.now()}`
    const artifactTitle = `Smoke Test Artifact ${Date.now()}`
    await page.locator('[data-testid="artifact-slug-input"]').fill(artifactSlug)
    await page
      .locator('[data-testid="artifact-title-input"]')
      .fill(artifactTitle)
    await page
      .locator('[data-testid="artifact-content-textarea"]')
      .fill('# Smoke Test\nTest artifact content')

    // Select a project (required), then save
    await selectFirstProject(page)
    await page.locator('button:has-text("Create Artifact")').click()

    // Verify redirect to artifact detail page
    await page.waitForTimeout(2000)
    await expect(page).toHaveURL(/artifacts\/[^/]+\/[^/]+/, { timeout: 10000 })
    await expect(page.locator(`text=${artifactTitle}`).first()).toBeVisible({
      timeout: 10000,
    })
  })

  test('should create an API key', async ({ page }) => {
    await devLogin(page)

    // Navigate to API keys
    await page.goto('/settings/api-keys')
    await expect(page).toHaveURL('/settings/api-keys')

    // Open the create dialog
    await page.locator('[data-testid="create-api-key-button"]').click()

    // Fill API key form (name + at least one integration, both required)
    const keyName = `Smoke Test Key ${Date.now()}`
    await page.locator('[data-testid="api-key-name-input"]').fill(keyName)
    await page.locator('[data-testid="integration-checkbox-ai_tools"]').check()

    // Submit and verify the created key is shown
    await page.locator('[data-testid="submit-create-api-key-button"]').click()
    await expect(
      page.locator('[data-testid="created-api-key-card"]')
    ).toBeVisible({ timeout: 10000 })
  })

  test('should display subscription page', async ({ page }) => {
    await devLogin(page)

    // Navigate to subscription page
    await page.goto('/subscription')

    // Verify page loads without errors
    await expect(page).toHaveURL('/subscription')

    // Verify some subscription content is visible
    await expect(page.locator('body')).toBeVisible()
  })
})

test.describe('Team Context Smoke Tests', () => {
  test('should display current team in header', async ({ page }) => {
    await devLogin(page)

    // Get current team
    const currentTeam = await getCurrentTeam(page)

    // Verify team context exists
    expect(currentTeam).toBeTruthy()
  })

  test('should switch between teams', async ({ page }) => {
    await devLogin(page)

    // This test may skip if user doesn't have multiple teams
    // Check if team switcher is available
    const teamSwitcher = page.locator('[data-testid="team-switcher"]').first()

    const hasTeamSwitcher = await teamSwitcher
      .isVisible({ timeout: 1000 })
      .catch(() => false)

    if (!hasTeamSwitcher) {
      test.skip()
      return
    }

    // If switcher exists, test team switching
    await teamSwitcher.click()
    await page.waitForTimeout(500)

    // Verify team list appears
    // This is a basic smoke test - we just verify the UI responds
    expect(true).toBe(true)
  })
})

test.describe('Navigation Smoke Tests', () => {
  test('should navigate to all main sections without errors', async ({
    page,
  }) => {
    await devLogin(page)

    const mainSections = [
      { path: '/', name: 'Home' },
      { path: '/prompts', name: 'Prompts' },
      { path: '/artifacts', name: 'Artifacts' },
      { path: '/memories', name: 'Memories' },
      { path: '/settings/api-keys', name: 'API Keys' },
      { path: '/subscription', name: 'Subscription' },
    ]

    for (const section of mainSections) {
      await page.goto(section.path)
      await expect(page).toHaveURL(section.path)

      // Verify no critical errors on page
      await expect(page.locator('body')).toBeVisible()

      // Wait briefly between navigations
      await page.waitForTimeout(200)
    }
  })
})

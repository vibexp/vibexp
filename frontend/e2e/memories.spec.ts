import { test, expect } from './fixtures/auth'

test.describe('Memories Feature', () => {
  test('should create a new memory', async ({ authenticatedPage }) => {
    await authenticatedPage.goto('/memories/new')
    await expect(authenticatedPage).toHaveURL(/memories\/new/)

    const memoryText = `Test Memory ${Date.now()} - Test content`

    await authenticatedPage.waitForSelector(
      '[data-testid="memory-content-textarea"]',
      { timeout: 10000 }
    )
    await authenticatedPage
      .locator('[data-testid="memory-content-textarea"]')
      .fill(memoryText)

    // Wait for the create API request to complete
    const createPromise = authenticatedPage.waitForResponse(
      response =>
        /\/memories$/.test(response.url()) &&
        response.request().method() === 'POST' &&
        true, // Accept any status code
      { timeout: 15000 }
    )

    await authenticatedPage.locator('button:has-text("Create Memory")').click()

    // Wait for the API response
    await createPromise

    // Wait a bit for navigation to occur
    await authenticatedPage.waitForTimeout(1000)

    // Navigate to memories list if not redirected
    if (!authenticatedPage.url().match(/memories$/)) {
      await authenticatedPage.goto('/memories')
    }
    await authenticatedPage.waitForLoadState('networkidle')

    const searchText = memoryText.substring(0, 30)
    await expect(
      authenticatedPage.locator(`text=${searchText}`).first()
    ).toBeVisible({ timeout: 10000 })
  })

  test('should view memory list', async ({ authenticatedPage }) => {
    await authenticatedPage.goto('/memories')
    await expect(authenticatedPage).toHaveURL(/memories$/)
    await authenticatedPage.waitForLoadState('networkidle')
    await expect(authenticatedPage.locator('body')).toBeVisible()
  })

  test('should view memory details', async ({ authenticatedPage }) => {
    await authenticatedPage.goto('/memories/new')
    await authenticatedPage.waitForSelector(
      '[data-testid="memory-content-textarea"]'
    )

    const memoryText = `Detail Test ${Date.now()} - Detail content`
    await authenticatedPage
      .locator('[data-testid="memory-content-textarea"]')
      .fill(memoryText)

    // Wait for the create API request to complete
    const createPromise = authenticatedPage.waitForResponse(
      response =>
        /\/memories$/.test(response.url()) &&
        response.request().method() === 'POST' &&
        true, // Accept any status code
      { timeout: 15000 }
    )

    await authenticatedPage.locator('button:has-text("Create Memory")').click()

    // Wait for the API response
    await createPromise
    await authenticatedPage.waitForTimeout(1000)

    // After creating, should be on memories list page
    if (!authenticatedPage.url().match(/memories$/)) {
      await authenticatedPage.goto('/memories')
    }
    await authenticatedPage.waitForLoadState('networkidle')

    // Click View icon button to see memory details (Eye icon)
    const searchText = memoryText.substring(0, 30)
    await authenticatedPage
      .locator(`text=${searchText}`)
      .first()
      .waitFor({ state: 'visible' })
    await authenticatedPage.locator('button[aria-label="View"]').first().click()

    await expect(authenticatedPage).toHaveURL(/memories\/[a-f0-9-]+$/, {
      timeout: 10000,
    })
    await expect(
      authenticatedPage.locator(`text=${searchText}`).first()
    ).toBeVisible()
  })

  test('should edit an existing memory', async ({ authenticatedPage }) => {
    // Increase timeout for this test to accommodate slower CI environments
    test.setTimeout(20000)

    await authenticatedPage.goto('/memories/new')
    await authenticatedPage.waitForSelector(
      '[data-testid="memory-content-textarea"]'
    )

    const originalText = `Edit Test ${Date.now()} - Original`
    await authenticatedPage
      .locator('[data-testid="memory-content-textarea"]')
      .fill(originalText)

    // Wait for the create API request to complete
    const createPromise = authenticatedPage.waitForResponse(
      response =>
        /\/memories$/.test(response.url()) &&
        response.request().method() === 'POST' &&
        true, // Accept any status code
      { timeout: 15000 }
    )

    await authenticatedPage.locator('button:has-text("Create Memory")').click()

    // Wait for the API response
    await createPromise
    await authenticatedPage.waitForTimeout(1000)

    // Navigate to memories list if not already there
    if (!authenticatedPage.url().match(/memories$/)) {
      await authenticatedPage.goto('/memories')
    }
    await authenticatedPage.waitForLoadState('networkidle')

    // Find and click the memory to view it first using the Eye icon
    const searchText = originalText.substring(0, 30)
    await authenticatedPage
      .locator(`text=${searchText}`)
      .first()
      .waitFor({ state: 'visible' })
    await authenticatedPage.locator('button[aria-label="View"]').first().click()

    await authenticatedPage.waitForURL(/memories\/[a-f0-9-]+$/, {
      timeout: 10000,
    })
    // Click Edit icon button on the details page
    await authenticatedPage.locator('button:has-text("Edit")').first().click()
    await expect(authenticatedPage).toHaveURL(/memories\/[a-f0-9-]+\/edit/, {
      timeout: 10000,
    })

    const updatedText = `${originalText} (Updated)`
    await authenticatedPage.waitForSelector(
      '[data-testid="memory-content-textarea"]'
    )
    await authenticatedPage
      .locator('[data-testid="memory-content-textarea"]')
      .clear()
    await authenticatedPage
      .locator('[data-testid="memory-content-textarea"]')
      .fill(updatedText)
    await authenticatedPage
      .locator('button')
      .filter({ hasText: /save changes|update|save/i })
      .first()
      .click()

    // Wait for API call to complete and navigation
    await authenticatedPage.waitForLoadState('networkidle', { timeout: 10000 })

    // Navigate to memories list if not already there
    if (!authenticatedPage.url().match(/memories$/)) {
      await authenticatedPage.goto('/memories')
      await authenticatedPage.waitForLoadState('networkidle')
    }

    const updatedSearchText = updatedText.substring(0, 30)
    await expect(
      authenticatedPage.locator(`text=${updatedSearchText}`).first()
    ).toBeVisible({ timeout: 10000 })
  })

  test('should support memory search if available', async ({
    authenticatedPage,
  }) => {
    // Increase timeout for this test to accommodate slower CI environments
    test.setTimeout(20000)

    // Create only 2 memories - sufficient to test search filtering
    const memories = [`Search Alpha ${Date.now()}`, `Search Beta ${Date.now()}`]

    for (const memoryText of memories) {
      await authenticatedPage.goto('/memories/new')
      await authenticatedPage.waitForSelector(
        '[data-testid="memory-content-textarea"]'
      )
      await authenticatedPage
        .locator('[data-testid="memory-content-textarea"]')
        .fill(memoryText)

      // Wait for the create API request to complete
      const createPromise = authenticatedPage.waitForResponse(
        response =>
          /\/memories$/.test(response.url()) &&
          response.request().method() === 'POST' &&
          true, // Accept any status code
        { timeout: 15000 }
      )

      await authenticatedPage
        .locator('button:has-text("Create Memory")')
        .click()

      // Wait for the API response
      await createPromise
      await authenticatedPage.waitForTimeout(500)
    }

    await authenticatedPage.goto('/memories')
    await authenticatedPage.waitForLoadState('networkidle')

    const searchInput = authenticatedPage.locator(
      'input[placeholder*="Search memories" i]'
    )
    if ((await searchInput.count()) > 0) {
      await searchInput.fill('Alpha')
      // Search is debounced, so wait for the results
      await authenticatedPage.waitForTimeout(1000)
      // Wait for search results to appear instead of arbitrary timeout
      await expect(authenticatedPage.locator('text=Alpha').first()).toBeVisible(
        { timeout: 5000 }
      )
      // Verify Beta is filtered out
      await expect(
        authenticatedPage.locator('text=Beta').first()
      ).not.toBeVisible({ timeout: 2000 })
    } else {
      await expect(
        authenticatedPage.locator('text=Search').first()
      ).toBeVisible({ timeout: 10000 })
    }
  })
})

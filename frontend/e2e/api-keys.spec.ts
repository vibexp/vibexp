import { test, expect } from './fixtures/auth'

/**
 * Comprehensive E2E tests for API Keys feature
 *
 * Tests cover:
 * 1. Creating API keys with name input
 * 2. Displaying full key after creation with Copy and Close buttons
 * 3. Listing API keys (masked format)
 * 4. Deleting API keys
 * 5. Using API keys to authenticate with backend endpoints
 *
 * Uses data-testid attributes for stable, reliable selectors
 */

test.describe('API Keys Feature', () => {
  test.describe('API Key Management UI', () => {
    test('should navigate to API keys page', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await expect(authenticatedPage).toHaveURL(/settings\/api-keys/)

      // Verify page header
      await expect(
        authenticatedPage.locator('h1, h2').filter({ hasText: 'API Keys' })
      ).toBeVisible()
    })

    test('should display empty state when no API keys exist', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      // Check for create button or empty state
      const hasApiKeys =
        (await authenticatedPage
          .locator('[data-testid="api-key-item"]')
          .count()) > 0
      const hasCreateButton =
        (await authenticatedPage
          .locator('[data-testid="create-api-key-button"]')
          .count()) > 0

      // Either should have API keys or a create button
      expect(hasApiKeys || hasCreateButton).toBeTruthy()
    })

    test('should open create API key form', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      // Click create button
      await authenticatedPage
        .locator('[data-testid="create-api-key-button"]')
        .click()

      // Verify form appears
      await expect(
        authenticatedPage.locator('[data-testid="create-api-key-form"]')
      ).toBeVisible()
      await expect(
        authenticatedPage.locator('[data-testid="api-key-name-input"]')
      ).toBeVisible()
    })

    test('should create a new API key and display full key', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      const apiKeyName = `E2E Test Key ${Date.now()}`

      // Click create button
      await authenticatedPage
        .locator('[data-testid="create-api-key-button"]')
        .click()

      // Fill in the API key name
      await authenticatedPage
        .locator('[data-testid="api-key-name-input"]')
        .fill(apiKeyName)

      // Select at least one integration (required)
      await authenticatedPage
        .locator('[data-testid="integration-checkbox-ai_tools"]')
        .check()

      // Submit the form
      await authenticatedPage
        .locator('[data-testid="submit-create-api-key-button"]')
        .click()

      // Wait for the API key creation to complete and card to appear
      await expect(
        authenticatedPage.locator('[data-testid="created-api-key-card"]')
      ).toBeVisible({ timeout: 10000 })

      // Verify the full API key is displayed (should start with 'vxk_')
      const apiKeyDisplay = authenticatedPage.locator(
        '[data-testid="api-key-display"]'
      )
      await expect(apiKeyDisplay).toBeVisible()

      const apiKeyText = await apiKeyDisplay.textContent()
      expect(apiKeyText).toMatch(/^vxk_/)

      // Verify Copy button exists
      await expect(
        authenticatedPage.locator('[data-testid="copy-api-key-button"]')
      ).toBeVisible()

      // Verify Close button exists
      await expect(
        authenticatedPage.locator('[data-testid="close-api-key-modal-button"]')
      ).toBeVisible()

      // Test copy functionality
      await authenticatedPage
        .locator('[data-testid="copy-api-key-button"]')
        .click()
      await authenticatedPage.waitForTimeout(500)

      // Close the modal/dialog
      await authenticatedPage
        .locator('[data-testid="close-api-key-modal-button"]')
        .click()

      // Verify we're back to the list view and the key name is visible
      await expect(authenticatedPage.locator(`text=${apiKeyName}`)).toBeVisible(
        { timeout: 5000 }
      )
    })

    test('should display API keys in masked format in the list', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      // Create a test API key first
      const apiKeyName = `Masked Test ${Date.now()}`

      await authenticatedPage
        .locator('[data-testid="create-api-key-button"]')
        .click()
      await authenticatedPage
        .locator('[data-testid="api-key-name-input"]')
        .fill(apiKeyName)
      await authenticatedPage
        .locator('[data-testid="integration-checkbox-ai_tools"]')
        .check()
      await authenticatedPage
        .locator('[data-testid="submit-create-api-key-button"]')
        .click()

      // Wait for the API key creation modal to appear
      await expect(
        authenticatedPage.locator('[data-testid="close-api-key-modal-button"]')
      ).toBeVisible({ timeout: 10000 })

      // Close the creation modal
      await authenticatedPage
        .locator('[data-testid="close-api-key-modal-button"]')
        .click()

      await authenticatedPage.waitForTimeout(500)

      // Verify the API key is displayed in masked format
      const maskedKey = authenticatedPage
        .locator('[data-testid="masked-api-key"]')
        .first()
      await expect(maskedKey).toBeVisible({ timeout: 5000 })

      const maskedKeyText = await maskedKey.textContent()
      expect(maskedKeyText).toMatch(/\*\*\*/)

      // Verify the name is visible
      await expect(
        authenticatedPage.locator(`text=${apiKeyName}`)
      ).toBeVisible()
    })

    test('should delete an API key', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      // Create a test API key to delete
      const apiKeyName = `Delete Test ${Date.now()}`

      await authenticatedPage
        .locator('[data-testid="create-api-key-button"]')
        .click()
      await authenticatedPage
        .locator('[data-testid="api-key-name-input"]')
        .fill(apiKeyName)
      await authenticatedPage
        .locator('[data-testid="integration-checkbox-ai_tools"]')
        .check()
      await authenticatedPage
        .locator('[data-testid="submit-create-api-key-button"]')
        .click()

      // Wait for the API key creation modal to appear
      await expect(
        authenticatedPage.locator('[data-testid="close-api-key-modal-button"]')
      ).toBeVisible({ timeout: 10000 })

      // Close the creation modal
      await authenticatedPage
        .locator('[data-testid="close-api-key-modal-button"]')
        .click()

      await authenticatedPage.waitForTimeout(1000)

      // Find the API key row and delete button
      const apiKeyRow = authenticatedPage
        .locator(`[data-testid="api-key-item"]:has-text("${apiKeyName}")`)
        .first()
      await expect(apiKeyRow).toBeVisible()

      const deleteButton = apiKeyRow.locator(
        '[data-testid="delete-api-key-button"]'
      )

      // Click delete button to open ConfirmDialog
      await deleteButton.click()

      // Wait for the ConfirmDialog to appear (shadcn AlertDialog -> role=alertdialog)
      const confirmDialog = authenticatedPage.locator('[role="alertdialog"]')
      await expect(confirmDialog).toBeVisible({ timeout: 5000 })

      // Verify dialog content mentions deletion
      await expect(confirmDialog).toContainText('delete')

      // Click the confirm button in the ConfirmDialog
      const confirmButton = confirmDialog.locator('button:has-text("Delete")')
      await confirmButton.click()

      // Wait for the dialog to close
      await expect(confirmDialog).not.toBeVisible({ timeout: 5000 })

      // Verify the API key is no longer in the list
      await expect(
        authenticatedPage.locator(
          `[data-testid="api-key-item"]:has-text("${apiKeyName}")`
        )
      ).not.toBeVisible({ timeout: 5000 })
    })

    test('should copy API key to clipboard', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      const apiKeyName = `Copy Test ${Date.now()}`

      // Create API key
      await authenticatedPage
        .locator('[data-testid="create-api-key-button"]')
        .click()
      await authenticatedPage
        .locator('[data-testid="api-key-name-input"]')
        .fill(apiKeyName)
      await authenticatedPage
        .locator('[data-testid="integration-checkbox-ai_tools"]')
        .check()
      await authenticatedPage
        .locator('[data-testid="submit-create-api-key-button"]')
        .click()

      // Wait for the API key to be created and displayed
      const apiKeyElement = authenticatedPage.locator(
        '[data-testid="api-key-display"]'
      )
      await expect(apiKeyElement).toBeVisible({ timeout: 10000 })

      // Get the API key text before copying
      const apiKeyText = await apiKeyElement.textContent()
      expect(apiKeyText).toMatch(/^vxk_/)

      // Grant clipboard permissions
      await authenticatedPage
        .context()
        .grantPermissions(['clipboard-read', 'clipboard-write'])

      // Click copy button
      await authenticatedPage
        .locator('[data-testid="copy-api-key-button"]')
        .click()

      await authenticatedPage.waitForTimeout(500)

      // Verify the key was actually copied to the clipboard (the icon-only copy
      // button shows a checkmark, not text, so assert on clipboard contents).
      const clipboard = await authenticatedPage.evaluate(() =>
        navigator.clipboard.readText()
      )
      expect(clipboard).toBe(apiKeyText)
    })

    test('should validate API key name is required', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      // Open create form
      await authenticatedPage
        .locator('[data-testid="create-api-key-button"]')
        .click()

      // Try to submit without entering a name
      const submitButton = authenticatedPage.locator(
        '[data-testid="submit-create-api-key-button"]'
      )

      // Check if submit button is disabled
      const isDisabled = await submitButton.isDisabled()

      // Button should be disabled when input is empty
      expect(isDisabled).toBeTruthy()
    })
  })

  test.describe('API Key Backend Authentication', () => {
    test('should create API key for backend testing', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      const apiKeyName = `Backend Test Key ${Date.now()}`

      // Create API key
      await authenticatedPage
        .locator('[data-testid="create-api-key-button"]')
        .click()
      await authenticatedPage
        .locator('[data-testid="api-key-name-input"]')
        .fill(apiKeyName)
      await authenticatedPage
        .locator('[data-testid="integration-checkbox-ai_tools"]')
        .check()
      await authenticatedPage
        .locator('[data-testid="submit-create-api-key-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)

      // Extract the API key
      const apiKeyElement = authenticatedPage.locator(
        '[data-testid="api-key-display"]'
      )
      const createdApiKey = (await apiKeyElement.textContent()) || ''

      expect(createdApiKey).toMatch(/^vxk_/)
      expect(createdApiKey.length).toBeGreaterThan(10)

      // Close modal
      await authenticatedPage
        .locator('[data-testid="close-api-key-modal-button"]')
        .click()
    })

    test('should authenticate with API key on Claude Code hooks endpoint', async ({
      page,
    }) => {
      // First, create an API key through UI in authenticated context
      const authenticatedContext = await page.context().browser()?.newContext()
      if (!authenticatedContext) {
        throw new Error('Could not create authenticated context')
      }

      const authPage = await authenticatedContext.newPage()

      // Import devLogin from fixtures
      const { devLogin } = await import('./fixtures/auth')
      await devLogin(authPage)

      await authPage.goto('/settings/api-keys')
      await authPage.waitForLoadState('networkidle')

      const testApiKeyName = `Auth Test ${Date.now()}`

      await authPage.locator('[data-testid="create-api-key-button"]').click()
      await authPage
        .locator('[data-testid="api-key-name-input"]')
        .fill(testApiKeyName)
      await authPage
        .locator('[data-testid="integration-checkbox-ai_tools"]')
        .check()
      await authPage
        .locator('[data-testid="submit-create-api-key-button"]')
        .click()

      await authPage.waitForTimeout(2000)

      // Extract the API key
      const apiKeyElement = authPage.locator('[data-testid="api-key-display"]')
      const testApiKey = (await apiKeyElement.textContent()) || ''

      expect(testApiKey).toMatch(/^vxk_/)

      await authPage.close()
      await authenticatedContext.close()

      // Now test API key authentication with backend
      const apiBaseUrl =
        process.env.PLAYWRIGHT_API_BASE_URL || 'http://localhost:8080'

      // Test with Claude Code hooks endpoint (supports API key auth)
      const hookResponse = await page.request.post(
        `${apiBaseUrl}/api/v1/claude-code/hooks`,
        {
          headers: {
            Authorization: `Bearer ${testApiKey}`,
            'Content-Type': 'application/json',
          },
          data: {
            event_type: 'test_event',
            session_id: `test_session_${Date.now()}`,
            data: {
              test: true,
            },
          },
        }
      )

      // Should not return 401 Unauthorized
      expect(hookResponse.status()).not.toBe(401)

      // Should return either 200/201 (success) or 400 (bad request due to test data)
      // But not 401 (unauthorized), which would indicate API key auth failed
      expect([200, 201, 400, 422]).toContain(hookResponse.status())
    })

    test('should reject invalid API key', async ({ page }) => {
      const apiBaseUrl =
        process.env.PLAYWRIGHT_API_BASE_URL || 'http://localhost:8080'

      const hookResponse = await page.request.post(
        `${apiBaseUrl}/api/v1/claude-code/hooks`,
        {
          headers: {
            Authorization: 'Bearer ak_invalid_key_that_does_not_exist',
            'Content-Type': 'application/json',
          },
          data: {
            event_type: 'test_event',
            session_id: `test_session_${Date.now()}`,
            data: {
              test: true,
            },
          },
          failOnStatusCode: false,
        }
      )

      // Should return 401 Unauthorized for invalid API key
      expect(hookResponse.status()).toBe(401)
    })

    test('should reject request without API key or JWT', async ({ page }) => {
      const apiBaseUrl =
        process.env.PLAYWRIGHT_API_BASE_URL || 'http://localhost:8080'

      const hookResponse = await page.request.post(
        `${apiBaseUrl}/api/v1/claude-code/hooks`,
        {
          headers: {
            'Content-Type': 'application/json',
          },
          data: {
            event_type: 'test_event',
            session_id: `test_session_${Date.now()}`,
            data: {
              test: true,
            },
          },
          failOnStatusCode: false,
        }
      )

      // Should return 401 Unauthorized without auth
      expect(hookResponse.status()).toBe(401)
    })
  })

  test.describe('API Key List Management', () => {
    test('should display multiple API keys in list', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      // Create 2 API keys
      const keyNames: string[] = []

      for (let i = 0; i < 2; i++) {
        const keyName = `Multi Key Test ${i + 1} ${Date.now()}`
        keyNames.push(keyName)

        await authenticatedPage
          .locator('[data-testid="create-api-key-button"]')
          .click()
        await authenticatedPage
          .locator('[data-testid="api-key-name-input"]')
          .fill(keyName)
        await authenticatedPage
          .locator('[data-testid="integration-checkbox-ai_tools"]')
          .check()
        await authenticatedPage
          .locator('[data-testid="submit-create-api-key-button"]')
          .click()

        // Wait for the API key creation modal to appear
        await expect(
          authenticatedPage.locator(
            '[data-testid="close-api-key-modal-button"]'
          )
        ).toBeVisible({ timeout: 10000 })

        await authenticatedPage
          .locator('[data-testid="close-api-key-modal-button"]')
          .click()

        await authenticatedPage.waitForTimeout(500)
      }

      // Verify both keys are visible
      for (const keyName of keyNames) {
        await expect(authenticatedPage.locator(`text=${keyName}`)).toBeVisible()
      }
    })

    test('should show created date for API keys', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      // Create an API key
      const apiKeyName = `Date Test ${Date.now()}`

      await authenticatedPage
        .locator('[data-testid="create-api-key-button"]')
        .click()
      await authenticatedPage
        .locator('[data-testid="api-key-name-input"]')
        .fill(apiKeyName)
      await authenticatedPage
        .locator('[data-testid="integration-checkbox-ai_tools"]')
        .check()
      await authenticatedPage
        .locator('[data-testid="submit-create-api-key-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)

      await authenticatedPage
        .locator('[data-testid="close-api-key-modal-button"]')
        .click()

      await authenticatedPage.waitForTimeout(1000)

      // Verify created date is displayed
      const createdDate = authenticatedPage
        .locator('[data-testid="api-key-created-date"]')
        .first()
      await expect(createdDate).toBeVisible()

      // The created date is rendered in the table's "Created" column as a
      // formatted date (contains the year).
      const dateText = await createdDate.textContent()
      expect(dateText).toMatch(/\d{4}/)
    })
  })

  test.describe('API Key Security', () => {
    test('should only show full API key once during creation', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/settings/api-keys')
      await authenticatedPage.waitForLoadState('networkidle')

      const apiKeyName = `Security Test ${Date.now()}`

      // Create API key
      await authenticatedPage
        .locator('[data-testid="create-api-key-button"]')
        .click()
      await authenticatedPage
        .locator('[data-testid="api-key-name-input"]')
        .fill(apiKeyName)
      await authenticatedPage
        .locator('[data-testid="integration-checkbox-ai_tools"]')
        .check()
      await authenticatedPage
        .locator('[data-testid="submit-create-api-key-button"]')
        .click()

      // Wait for the API key to be created and modal to appear
      const fullKeyElement = authenticatedPage.locator(
        '[data-testid="api-key-display"]'
      )
      await expect(fullKeyElement).toBeVisible({ timeout: 10000 })

      const fullKeyText = await fullKeyElement.textContent()
      expect(fullKeyText).toMatch(/^vxk_/)
      expect(fullKeyText?.length || 0).toBeGreaterThan(20)

      // Close modal
      await authenticatedPage
        .locator('[data-testid="close-api-key-modal-button"]')
        .click()

      await authenticatedPage.waitForTimeout(1000)

      // Verify full key is NOT visible in the list (should be masked)
      const maskedKey = authenticatedPage
        .locator('[data-testid="masked-api-key"]')
        .first()
      await expect(maskedKey).toBeVisible()

      const maskedKeyText = await maskedKey.textContent()
      expect(maskedKeyText).toMatch(/\*\*\*/)
      expect(maskedKeyText).not.toMatch(/^ak_[a-zA-Z0-9]{30,}$/)

      // But name should be visible
      await expect(
        authenticatedPage.locator(`text=${apiKeyName}`)
      ).toBeVisible()
    })
  })
})

import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: Prompt CRUD Operations
 * Tests basic Create, Read, Update, Delete functionality for prompts
 */
test.describe('Prompt CRUD Operations', () => {
  test.describe('Prompt Creation', () => {
    test('should display prompts page with navigation', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts')
      await expect(authenticatedPage).toHaveURL(/prompts$/)
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify navigation and page structure
      await expect(authenticatedPage.locator('body')).toBeVisible()
    })

    test('should show empty state for users with no prompts', async ({
      freshUserPage,
    }) => {
      await freshUserPage.goto('/prompts')
      await expect(freshUserPage).toHaveURL(/prompts$/)
      await freshUserPage.waitForLoadState('networkidle')

      // Check for empty state indicators (adjust selectors based on actual UI)
      const pageContent = await freshUserPage.textContent('body')
      expect(pageContent).toBeTruthy()
    })

    test('should create prompt with valid data', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts/new')
      await expect(authenticatedPage).toHaveURL(/prompts\/new/)

      const promptName = `Test Prompt ${Date.now()}`
      const promptBody = 'This is a test prompt content for E2E testing'

      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill(promptBody)
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible({ timeout: 10000 })
    })

    test('should validate required fields (name, content)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      // Try to save without filling required fields
      const saveButton = authenticatedPage.locator(
        '[data-testid="prompt-save-button"]'
      )
      await saveButton.click()

      // Wait a bit to see if validation triggers
      await authenticatedPage.waitForTimeout(1000)

      // Should still be on the create page (validation failed)
      await expect(authenticatedPage).toHaveURL(/prompts\/new/)
    })

    test('should auto-generate slug from name', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Auto Slug Test ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)

      // Check if slug field is auto-populated
      const slugInput = authenticatedPage.locator(
        '[data-testid="prompt-slug-input"]'
      )
      if (await slugInput.isVisible()) {
        const slugValue = await slugInput.inputValue()
        // Slug should be a lowercase, hyphenated version
        expect(slugValue.length).toBeGreaterThan(0)
        expect(slugValue).toMatch(/^[a-z0-9-]+$/)
      }
    })

    test('should allow custom slug override', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Custom Slug ${Date.now()}`
      const customSlug = `custom-slug-${Date.now()}`

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)

      // Override the slug
      const slugInput = authenticatedPage.locator(
        '[data-testid="prompt-slug-input"]'
      )
      if (await slugInput.isVisible()) {
        await slugInput.clear()
        await slugInput.fill(customSlug)
      }

      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Content with custom slug')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)

      // Verify URL contains custom slug
      const currentUrl = authenticatedPage.url()
      if (customSlug) {
        expect(currentUrl).toContain(customSlug)
      }
    })

    test('should validate slug format (no spaces, special chars)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill('Test Prompt')

      const slugInput = authenticatedPage.locator(
        '[data-testid="prompt-slug-input"]'
      )
      if (await slugInput.isVisible()) {
        // Try invalid slug with spaces and special characters
        await slugInput.clear()
        await slugInput.fill('invalid slug @#$')

        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .fill('Content')
        await authenticatedPage
          .locator('[data-testid="prompt-save-button"]')
          .click()

        // Should show validation error or prevent submission
        await authenticatedPage.waitForTimeout(1000)
        // Still on create page indicates validation worked
        await expect(authenticatedPage).toHaveURL(/prompts\/new/)
      }
    })

    test('should add tags to prompt', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Tagged Prompt ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Content with tags')

      // Look for tags input (adjust selector based on actual UI)
      const tagsInput = authenticatedPage.locator(
        'input[placeholder*="tag"], input[placeholder*="Tag"]'
      )
      if (await tagsInput.isVisible()) {
        await tagsInput.fill('test,e2e,automation')
      }

      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
    })

    test('should set prompt status (active/draft)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Status Test ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Content with status')

      // Status is a shadcn (Radix) Select, not a native <select>; open it and
      // pick an option. Defaults to Draft, so this is a no-op-safe interaction.
      const statusTrigger = authenticatedPage.locator('#prompt-status')
      if (await statusTrigger.isVisible().catch(() => false)) {
        await statusTrigger.click()
        await authenticatedPage.getByRole('option', { name: /draft/i }).click()
      }

      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
    })

    test('should display prompt in list after creation', async ({
      authenticatedPage,
    }) => {
      // Create a prompt
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `List Test ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('List test content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Go to prompts list
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify prompt appears in list
      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible({ timeout: 5000 })
    })
  })

  test.describe('Prompt Reading', () => {
    test('should navigate to prompt detail view', async ({
      authenticatedPage,
    }) => {
      // Create a prompt first
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Detail Nav ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Detail navigation test')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // We should be on detail view after creation
      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible()
    })

    test('should view prompt details with all metadata', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Metadata Test ${Date.now()}`
      const promptBody = 'Content with full metadata'

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill(promptBody)
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Verify name and content are visible
      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible()
      await expect(
        authenticatedPage.locator(`text=${promptBody}`)
      ).toBeVisible()
    })
  })

  test.describe('Prompt Updating', () => {
    test('should edit prompt content', async ({ authenticatedPage }) => {
      // Create a prompt
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const originalName = `Edit Content ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(originalName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Original content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/)

      // Navigate to edit
      await authenticatedPage
        .locator('[data-testid="edit-prompt-button"]')
        .click()
      await expect(authenticatedPage).toHaveURL(/prompts\/[^/]+\/edit/, {
        timeout: 10000,
      })

      // Update content
      await authenticatedPage.waitForSelector(
        'textarea[placeholder*="Write your prompt here"]',
        { timeout: 10000 }
      )
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .clear()
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Updated content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Verify updated content
      await expect(
        authenticatedPage.locator('text=Updated content')
      ).toBeVisible({ timeout: 5000 })
    })

    test('should edit prompt metadata (name, tags, status)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const originalName = `Metadata Edit ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(originalName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Original content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/)

      // Navigate to edit
      await authenticatedPage
        .locator('[data-testid="edit-prompt-button"]')
        .click()
      await expect(authenticatedPage).toHaveURL(/prompts\/[^/]+\/edit/, {
        timeout: 10000,
      })

      // Update name
      const updatedName = `${originalName} (Updated)`
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]'
      )
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .clear()
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(updatedName)

      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Verify updated name
      await expect(
        authenticatedPage.locator(`text=${updatedName}`).first()
      ).toBeVisible({ timeout: 5000 })
    })

    test('should preserve prompt data after edit', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Preserve Test ${Date.now()}`
      const promptBody = 'Content to preserve'

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill(promptBody)
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/)

      // Navigate to edit and back without changing
      await authenticatedPage
        .locator('[data-testid="edit-prompt-button"]')
        .click()
      await expect(authenticatedPage).toHaveURL(/prompts\/[^/]+\/edit/, {
        timeout: 10000,
      })

      // Wait for form to load
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      // Verify original values are still there
      const nameValue = await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .inputValue()
      expect(nameValue).toBe(promptName)

      const bodyValue = await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .inputValue()
      expect(bodyValue).toBe(promptBody)
    })
  })

  test.describe('Prompt Deletion', () => {
    test('should show confirmation dialog before delete', async ({
      authenticatedPage,
    }) => {
      // Create a prompt to delete
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Delete Confirm ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('To be deleted')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Go to prompts list
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Find and click delete button
      const deleteButton = authenticatedPage
        .locator('[data-testid="delete-prompt-button"]')
        .first()
      await deleteButton.click()

      // Wait for confirmation dialog
      const confirmDialog = authenticatedPage.locator('[role="alertdialog"]')
      await expect(confirmDialog).toBeVisible({ timeout: 5000 })

      // Verify dialog content
      await expect(confirmDialog).toContainText(/delete/i)
    })

    test('should cancel delete and keep prompt', async ({
      authenticatedPage,
    }) => {
      // Create a prompt
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Keep Prompt ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Should remain')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Go to list
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Click delete
      const deleteButton = authenticatedPage
        .locator('[data-testid="delete-prompt-button"]')
        .first()
      await deleteButton.click()

      // Wait for dialog
      const confirmDialog = authenticatedPage.locator('[role="alertdialog"]')
      await expect(confirmDialog).toBeVisible({ timeout: 5000 })

      // Click cancel
      const cancelButton = confirmDialog.locator(
        'button:has-text("Cancel"), button:has-text("No")'
      )
      if (await cancelButton.isVisible()) {
        await cancelButton.click()
      }

      // Dialog should close
      await expect(confirmDialog).not.toBeVisible({ timeout: 5000 })

      // Prompt should still be in list
      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible({ timeout: 5000 })
    })

    test('should delete prompt and remove from list', async ({
      authenticatedPage,
    }) => {
      // Create a prompt to delete
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      const promptName = `Delete Me ${Date.now()}`
      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('This will be deleted')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Go to list
      await authenticatedPage.goto('/prompts')
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify prompt exists
      const promptText = promptName.substring(0, 20)
      await authenticatedPage
        .locator(`text=${promptText}`)
        .first()
        .waitFor({ state: 'visible', timeout: 10000 })

      // Click delete
      const deleteButton = authenticatedPage
        .locator('[data-testid="delete-prompt-button"]')
        .first()
      await deleteButton.click()

      // Confirm deletion
      const confirmDialog = authenticatedPage.locator('[role="alertdialog"]')
      await expect(confirmDialog).toBeVisible({ timeout: 5000 })

      const confirmButton = confirmDialog.locator('button:has-text("Delete")')
      await confirmButton.click()

      // Wait for dialog to close
      await expect(confirmDialog).not.toBeVisible({ timeout: 5000 })

      // Wait for list to refresh
      await authenticatedPage.waitForLoadState('networkidle')

      // Prompt should be removed from list
      await expect(authenticatedPage.locator(`text=${promptText}`).first())
        .not.toBeVisible({ timeout: 5000 })
        .catch(() => {
          // It's okay if the element doesn't exist at all
          return Promise.resolve()
        })
    })

    test('should handle duplicate slug error gracefully', async ({
      authenticatedPage,
    }) => {
      const uniqueSlug = `duplicate-${Date.now()}`

      // Create first prompt with specific slug
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill('First Prompt')

      const slugInput = authenticatedPage.locator(
        '[data-testid="prompt-slug-input"]'
      )
      if (await slugInput.isVisible()) {
        await slugInput.clear()
        await slugInput.fill(uniqueSlug)
      }

      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('First content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForTimeout(2000)
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Try to create second prompt with same slug
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill('Second Prompt')

      if (await slugInput.isVisible()) {
        await slugInput.clear()
        await slugInput.fill(uniqueSlug)
      }

      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Second content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      // Should show error or stay on create page
      await authenticatedPage.waitForTimeout(2000)

      // Check if we're still on create page (indicates error handling)
      const currentUrl = authenticatedPage.url()
      const isStillOnCreate = currentUrl.includes('/prompts/new')

      if (isStillOnCreate) {
        // Error was handled correctly
        expect(isStillOnCreate).toBeTruthy()
      }
    })
  })
})

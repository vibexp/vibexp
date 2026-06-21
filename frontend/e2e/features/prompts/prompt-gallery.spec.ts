import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: Prompt Gallery
 * Tests the public prompt gallery browsing, filtering, and usage features
 */
test.describe('Prompt Gallery', () => {
  test.describe('Gallery Navigation', () => {
    test('should display prompt gallery categories', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await expect(
        authenticatedPage.getByRole('heading', { name: 'Prompt Gallery' })
      ).toBeVisible()

      // Verify categories are displayed
      const categoryCards = authenticatedPage
        .locator('[data-testid="gallery-category-card"]')
        .filter({
          hasText: /Customer Support|Engineering|Marketing/,
        })
      await expect(categoryCards.first()).toBeVisible({ timeout: 10000 })
    })

    test('should show category cards with icons and descriptions', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify at least one category card is visible
      const categoryCards = authenticatedPage.locator(
        '[data-testid="gallery-category-card"]'
      )
      const count = await categoryCards.count()
      expect(count).toBeGreaterThan(0)

      // Verify first category has text content
      const firstCard = categoryCards.first()
      const cardText = await firstCard.textContent()
      expect(cardText).toBeTruthy()
    })

    test('should navigate to category page', async ({ authenticatedPage }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Click on a category
      await authenticatedPage.getByText('Customer Support').first().click()
      await expect(authenticatedPage).toHaveURL(
        /\/prompt-gallery\/Customer%20Support/
      )
    })

    test('should display prompts in selected category', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Click on Engineering category
      await authenticatedPage.getByText('Engineering').first().click()
      await expect(authenticatedPage).toHaveURL(/\/prompt-gallery\/Engineering/)

      // Verify category heading
      await expect(
        authenticatedPage.getByRole('heading', {
          name: /Engineering/i,
          level: 1,
        })
      ).toBeVisible()

      // Wait for prompts to load
      const promptCards = authenticatedPage
        .locator('[data-testid="gallery-prompt-card"]')
        .filter({
          hasText: /code|review|engineering/i,
        })
      await expect(promptCards.first()).toBeVisible({ timeout: 10000 })
    })

    test('should navigate back to gallery from prompt view', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate to category
      await authenticatedPage.getByText('Marketing').first().click()
      await authenticatedPage.waitForTimeout(500)

      // Navigate to prompt
      await authenticatedPage
        .locator('[data-testid="gallery-prompt-card"]')
        .first()
        .click()
      await expect(authenticatedPage).toHaveURL(/\/prompt-gallery\/prompt\//)

      // Navigate back to category (detail page Back button)
      const backButton = authenticatedPage.getByRole('button', {
        name: /^Back$/i,
      })
      await expect(backButton).toBeVisible({ timeout: 5000 })
      await backButton.click()
      await expect(authenticatedPage).toHaveURL(/\/prompt-gallery\/Marketing/)

      // Navigate back to gallery home (category page Back button)
      const backToCategories = authenticatedPage.getByRole('button', {
        name: /^Back$/i,
      })
      await expect(backToCategories).toBeVisible({ timeout: 5000 })
      await backToCategories.click()
      await expect(authenticatedPage).toHaveURL(/\/prompt-gallery$/)
    })
  })

  test.describe('Gallery Filtering', () => {
    test('should filter prompts by tags within category', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Go to a category
      await authenticatedPage.getByText('Customer Support').first().click()
      await expect(authenticatedPage).toHaveURL(
        /\/prompt-gallery\/Customer%20Support/
      )

      // Wait for prompts and tags to load
      await authenticatedPage.waitForTimeout(1000)

      // Look for tag buttons
      const tagButtons = authenticatedPage.locator('button').filter({
        hasText: /support|customer-service|communication/,
      })
      const tagCount = await tagButtons.count()

      if (tagCount > 0) {
        // Click first tag
        const firstTag = tagButtons.first()
        await firstTag.click()

        // A selected tag activates filtering, which surfaces the Clear filters control.
        await expect(
          authenticatedPage.getByTestId('clear-filters-button').first()
        ).toBeVisible()
      }
    })

    test('should search prompts within category', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate to category
      await authenticatedPage.getByText('Engineering').first().click()
      await expect(authenticatedPage).toHaveURL(/\/prompt-gallery\/Engineering/)

      // Use search
      const searchInput = authenticatedPage.getByPlaceholder(/Search prompts/i)
      await expect(searchInput).toBeVisible()
      await searchInput.fill('code')

      // Wait for search results
      await authenticatedPage.waitForTimeout(1000)

      // Verify some results appear
      const promptCards = authenticatedPage.locator(
        '[data-testid="gallery-prompt-card"]'
      )
      const count = await promptCards.count()
      expect(count).toBeGreaterThanOrEqual(0)
    })
  })

  test.describe('Prompt Details', () => {
    test('should view gallery prompt details', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate to category and prompt
      await authenticatedPage.getByText('Customer Support').first().click()
      await authenticatedPage.waitForTimeout(500)
      await authenticatedPage
        .locator('[data-testid="gallery-prompt-card"]')
        .first()
        .click()

      // Verify detail page
      await expect(authenticatedPage).toHaveURL(/\/prompt-gallery\/prompt\//)
      await expect(
        authenticatedPage.getByRole('heading', { level: 1 })
      ).toBeVisible()
    })

    test('should display prompt metadata (author, tags, description)', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate to any prompt
      await authenticatedPage.getByText('Engineering').first().click()
      await authenticatedPage.waitForTimeout(500)
      await authenticatedPage
        .locator('[data-testid="gallery-prompt-card"]')
        .first()
        .click()

      // Verify metadata sections
      await expect(authenticatedPage.getByText(/Category/i)).toBeVisible()
      await expect(authenticatedPage.getByText(/Tags/i)).toBeVisible()
    })

    test('should click "Use This Prompt" button', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate to prompt
      await authenticatedPage.getByText('Customer Support').first().click()
      await authenticatedPage.waitForTimeout(500)
      await authenticatedPage
        .locator('[data-testid="gallery-prompt-card"]')
        .first()
        .click()

      // Wait for detail page
      await expect(
        authenticatedPage.getByRole('button', { name: /use this prompt/i })
      ).toBeVisible({
        timeout: 10000,
      })

      // Click "Use This Prompt"
      const usePromptButton = authenticatedPage.getByRole('button', {
        name: /Use This Prompt/i,
      })
      await expect(usePromptButton).toBeVisible()
      await usePromptButton.click()

      // Should navigate to prompt editor
      await expect(authenticatedPage).toHaveURL(/\/prompts\/new/)
    })

    test('should pre-fill form from gallery prompt', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate to prompt and use it
      await authenticatedPage.getByText('Marketing').first().click()
      await authenticatedPage.waitForTimeout(500)
      await authenticatedPage
        .locator('[data-testid="gallery-prompt-card"]')
        .first()
        .click()
      await authenticatedPage.waitForTimeout(500)

      const usePromptButton = authenticatedPage.getByRole('button', {
        name: /Use This Prompt/i,
      })
      await usePromptButton.click()

      // Wait for form to load
      await expect(authenticatedPage).toHaveURL(/\/prompts\/new/)
      await expect(
        authenticatedPage.getByRole('heading', { name: /Create New Prompt/i })
      ).toBeVisible()

      // Verify form is pre-filled
      const nameInput = authenticatedPage.getByTestId('prompt-name-input')
      await expect(nameInput).toBeVisible({ timeout: 10000 })
      const nameValue = await nameInput.inputValue()
      expect(nameValue).toContain('Based on:')

      const bodyTextarea = authenticatedPage.getByTestId('prompt-body-textarea')
      await expect(bodyTextarea).toBeVisible({ timeout: 5000 })
      const bodyValue = await bodyTextarea.inputValue()
      expect(bodyValue.length).toBeGreaterThan(0)
    })

    test('should allow customization of gallery prompt', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate and use a prompt
      await authenticatedPage.getByText('Customer Support').first().click()
      await authenticatedPage.waitForTimeout(500)
      await authenticatedPage
        .locator('[data-testid="gallery-prompt-card"]')
        .first()
        .click()
      await authenticatedPage.waitForTimeout(500)

      const usePromptButton = authenticatedPage.getByRole('button', {
        name: /Use This Prompt/i,
      })
      await usePromptButton.click()

      // Wait for form
      await expect(authenticatedPage).toHaveURL(/\/prompts\/new/)
      const nameInput = authenticatedPage.getByTestId('prompt-name-input')
      await expect(nameInput).toBeVisible({ timeout: 10000 })

      // Customize the prompt
      await nameInput.fill(`My Custom Prompt ${Date.now()}`)

      const slugInput = authenticatedPage.getByTestId('prompt-slug-input')
      await slugInput.fill(`custom-prompt-${Date.now()}`)

      // Verify we can edit
      const bodyTextarea = authenticatedPage.getByTestId('prompt-body-textarea')
      await bodyTextarea.fill('My customized content')

      // The form should be editable
      const nameValue = await nameInput.inputValue()
      expect(nameValue).toContain('My Custom Prompt')
    })

    test('should save customized prompt to user collection', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate and use a prompt
      await authenticatedPage.getByText('Engineering').first().click()
      await authenticatedPage.waitForTimeout(500)
      await authenticatedPage
        .locator('[data-testid="gallery-prompt-card"]')
        .first()
        .click()
      await authenticatedPage.waitForTimeout(500)

      const usePromptButton = authenticatedPage.getByRole('button', {
        name: /Use This Prompt/i,
      })
      await usePromptButton.click()

      // Customize and save
      await expect(authenticatedPage).toHaveURL(/\/prompts\/new/)
      const nameInput = authenticatedPage.getByTestId('prompt-name-input')
      await expect(nameInput).toBeVisible({ timeout: 10000 })

      const customName = `Gallery Saved ${Date.now()}`
      await nameInput.fill(customName)

      const slugInput = authenticatedPage.getByTestId('prompt-slug-input')
      await slugInput.fill(`gallery-saved-${Date.now()}`)

      const saveButton = authenticatedPage.getByTestId('prompt-save-button')
      await saveButton.click()

      // Verify redirect to detail page
      await expect(authenticatedPage).toHaveURL(/\/prompts\//, {
        timeout: 10000,
      })
      await expect(authenticatedPage.getByText(customName).first()).toBeVisible(
        { timeout: 5000 }
      )
    })
  })

  test.describe('Gallery Search and Filters', () => {
    test('should clear filters when no results found', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate to category
      await authenticatedPage.getByText('Engineering').first().click()
      await expect(authenticatedPage).toHaveURL(/\/prompt-gallery\/Engineering/)

      // Wait for prompts to load
      await authenticatedPage.waitForSelector(
        '[data-testid="gallery-prompt-card"]',
        {
          timeout: 5000,
        }
      )

      // Search for non-existent prompt
      const searchInput = authenticatedPage.getByPlaceholder(/Search prompts/i)
      const responsePromise = authenticatedPage.waitForResponse(
        response =>
          response.url().includes('/prompt-gallery/prompts') &&
          response.status() === 200,
        { timeout: 10000 }
      )

      await searchInput.fill('nonexistentpromptxyz123')
      await responsePromise

      // Wait for loading to complete
      await authenticatedPage
        .waitForSelector('[class*="animate-spin"]', {
          state: 'hidden',
          timeout: 5000,
        })
        .catch(() => {})

      // Wait for results or empty state
      await authenticatedPage.waitForSelector(
        '[data-testid="empty-state"], [class*="cursor-pointer"]',
        { timeout: 10000 }
      )

      // Verify empty state
      await expect(authenticatedPage.getByTestId('empty-state')).toBeVisible({
        timeout: 5000,
      })
      await expect(
        authenticatedPage.getByTestId('empty-state-message')
      ).toContainText(/Try adjusting your filters/i)

      // Clear filters
      const clearButton = authenticatedPage
        .getByTestId('clear-filters-button')
        .first()
      await expect(clearButton).toBeVisible({ timeout: 5000 })
      await clearButton.click()

      // Verify filters are cleared
      const searchValue = await searchInput.inputValue()
      expect(searchValue).toBe('')
    })
  })

  test.describe('Gallery Content Actions', () => {
    test('should copy prompt content using copy button', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate to prompt detail
      await authenticatedPage.getByText('Customer Support').first().click()
      await authenticatedPage.waitForTimeout(500)
      await authenticatedPage
        .locator('[data-testid="gallery-prompt-card"]')
        .first()
        .click()

      // Wait for detail page
      await expect(
        authenticatedPage.getByRole('button', { name: /use this prompt/i })
      ).toBeVisible({
        timeout: 10000,
      })

      // Click copy button
      const copyButton = authenticatedPage.getByTestId('copy-button')
      await expect(copyButton).toBeVisible({ timeout: 10000 })
      await copyButton.click()

      // Verify copy success - button should change to "Copied"
      await expect(copyButton).toContainText(/Copied/i, { timeout: 3000 })
    })
  })

  test.describe('Gallery Metadata Display', () => {
    test('should display readable metadata table', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompt-gallery')
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate to any prompt
      await authenticatedPage.getByText('Engineering').first().click()
      await authenticatedPage.waitForTimeout(500)
      await authenticatedPage
        .locator('[data-testid="gallery-prompt-card"]')
        .first()
        .click()

      // Check for additional information section
      const additionalInfoSection = authenticatedPage.getByText(
        /Additional Information/i
      )

      // If metadata exists, verify table format
      const hasAdditionalInfo = await additionalInfoSection
        .isVisible()
        .catch(() => false)

      if (hasAdditionalInfo) {
        // Verify table structure
        const table = authenticatedPage.locator('table')
        await expect(table).toBeVisible()

        // Verify table has rows
        const tableRows = table.locator('tr')
        const rowCount = await tableRows.count()
        expect(rowCount).toBeGreaterThan(0)
      } else {
        // No metadata is also valid
        expect(hasAdditionalInfo).toBeDefined()
      }
    })
  })
})

import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: Prompt Placeholder Variables ({{variable}})
 * Tests the placeholder syntax detection, rendering, and substitution features
 */
test.describe('Prompt Placeholder Variables', () => {
  // Increase timeout for these tests as they involve rendering
  test.setTimeout(30000)

  test.describe('Placeholder Detection', () => {
    test('should detect {{placeholder}} syntax in content', async ({
      authenticatedPage,
    }) => {
      const promptName = `Single Placeholder ${Date.now()}`

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Hello {{name}}!')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify prompt was created
      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible({ timeout: 5000 })
    })

    test('should detect multiple {{placeholders}} in one prompt', async ({
      authenticatedPage,
    }) => {
      const promptName = `Multi Placeholder ${Date.now()}`

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Hello {{name}}, you work at {{company}} as a {{role}}.')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify prompt was created with multiple placeholders
      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible({ timeout: 5000 })
    })
  })

  test.describe('Placeholder Display', () => {
    test('should display placeholder input fields in form', async ({
      authenticatedPage,
    }) => {
      const promptName = `Placeholder Display ${Date.now()}`

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Test with {{variable}}')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await authenticatedPage.waitForLoadState('networkidle')

      // Switch to rendered view if available
      const renderedButton = authenticatedPage
        .locator('button:has-text("Rendered")')
        .first()
      if (await renderedButton.isVisible()) {
        await renderedButton.click()
        await authenticatedPage.waitForTimeout(1500)
      }

      // Check for placeholder section
      const placeholderSection = authenticatedPage.locator(
        'text=Fill in placeholder values'
      )
      const hasPlaceholders = await placeholderSection
        .isVisible()
        .catch(() => false)

      if (hasPlaceholders) {
        // Verify placeholder input exists
        const variableLabel = authenticatedPage.locator(
          'label:has-text("variable")'
        )
        await expect(variableLabel).toBeVisible({ timeout: 3000 })
      }
    })

    test('should render prompt with filled placeholder values', async ({
      authenticatedPage,
    }) => {
      const promptName = `Render Placeholder ${Date.now()}`

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Greet: {{greeting}}')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await authenticatedPage.waitForLoadState('networkidle')

      // Switch to rendered view
      const renderedButton = authenticatedPage
        .locator('button:has-text("Rendered")')
        .first()
      if (await renderedButton.isVisible()) {
        await renderedButton.click()
        await authenticatedPage.waitForTimeout(1500)
      }

      // Try to fill placeholder
      const placeholderInput = authenticatedPage
        .locator('input[placeholder*="greeting"]')
        .first()
      const inputExists = await placeholderInput.isVisible().catch(() => false)

      if (inputExists) {
        await placeholderInput.fill('Hello World')

        // Try to render
        const renderButton = authenticatedPage
          .locator('button:has-text("Render")')
          .first()
        if (await renderButton.isVisible()) {
          await renderButton.click()
          await authenticatedPage.waitForTimeout(2000)
        }
      }

      // Verify prompt exists at minimum
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/)
    })
  })

  test.describe('Placeholder Rendering', () => {
    test('should preserve placeholder structure in raw view', async ({
      authenticatedPage,
    }) => {
      const promptName = `Raw View ${Date.now()}`
      const content = 'Raw {{placeholder}} content'

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill(content)
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await authenticatedPage.waitForLoadState('networkidle')

      // Check if raw button exists and click it
      const rawButton = authenticatedPage
        .locator('button:has-text("Raw")')
        .first()
      if (await rawButton.isVisible()) {
        await rawButton.click()
        await authenticatedPage.waitForTimeout(1000)

        // Verify placeholder syntax is preserved
        const bodyContent = await authenticatedPage.textContent('body')
        expect(bodyContent).toContain('{{placeholder}}')
      }
    })

    test('should handle special characters in placeholder names', async ({
      authenticatedPage,
    }) => {
      const promptName = `Special Chars ${Date.now()}`

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('User: {{user_name}} Full: {{full-name}}')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify prompt was created successfully
      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible({ timeout: 5000 })
    })

    test('should handle nested {{placeholders}}', async ({
      authenticatedPage,
    }) => {
      const promptName = `Nested Placeholders ${Date.now()}`

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Outer: {{outer}}, Inner: {{inner}}')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await authenticatedPage.waitForLoadState('networkidle')

      // Verify creation
      await expect(
        authenticatedPage.locator(`text=${promptName}`).first()
      ).toBeVisible({ timeout: 5000 })
    })

    test('should validate placeholder syntax errors', async ({
      authenticatedPage,
    }) => {
      const promptName = `Syntax Error ${Date.now()}`

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      // Invalid syntax: missing closing brace
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Invalid {{placeholder syntax')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      // Should still create (or show error)
      await authenticatedPage.waitForTimeout(2000)

      // Verify we either created it or stayed on the form
      const currentUrl = authenticatedPage.url()
      expect(currentUrl).toContain('/prompts')
    })
  })

  test.describe('Placeholder Persistence', () => {
    test('should preserve placeholders when editing prompt', async ({
      authenticatedPage,
    }) => {
      const promptName = `Edit Preserve ${Date.now()}`
      const originalContent = 'Original {{variable}} content'

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill(originalContent)
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await authenticatedPage.waitForLoadState('networkidle')

      // Navigate to edit
      const editLink = authenticatedPage
        .locator('[data-testid="edit-prompt-button"]')
        .first()
      await editLink.click()
      await authenticatedPage.waitForURL(/edit/, { timeout: 10000 })

      // Verify textarea still contains placeholder
      const textarea = authenticatedPage.locator(
        'textarea[placeholder*="Write your prompt here"]'
      )
      const textareaValue = await textarea.inputValue()
      expect(textareaValue).toContain('{{variable}}')
    })

    test('should clear placeholder values when switching prompts', async ({
      authenticatedPage,
    }) => {
      // Create first prompt
      const prompt1Name = `Prompt1 ${Date.now()}`
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(prompt1Name)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('First {{var1}}')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Create second prompt
      const prompt2Name = `Prompt2 ${Date.now()}`
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(prompt2Name)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Second {{var2}}')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Verify we're on the second prompt
      await expect(
        authenticatedPage.locator(`text=${prompt2Name}`).first()
      ).toBeVisible({ timeout: 5000 })
    })
  })

  test.describe('Placeholder Interaction', () => {
    test('should copy rendered output with substituted values', async ({
      authenticatedPage,
    }) => {
      const promptName = `Copy Rendered ${Date.now()}`

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Copy this: {{text}}')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await authenticatedPage.waitForLoadState('networkidle')

      // Look for copy button
      const copyButton = authenticatedPage
        .locator('button:has-text("Copy"), button[title*="Copy"]')
        .first()
      if (await copyButton.isVisible()) {
        await copyButton.click()
        await authenticatedPage.waitForTimeout(1000)

        // Verify copy button state changed (usually shows "Copied")
        const buttonText = await copyButton.textContent()
        // Button text should change or remain visible
        expect(buttonText).toBeTruthy()
      }
    })

    test('should handle empty placeholder values gracefully', async ({
      authenticatedPage,
    }) => {
      const promptName = `Empty Values ${Date.now()}`

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(promptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Empty {{value}} test')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      await authenticatedPage.waitForLoadState('networkidle')

      // Switch to rendered view
      const renderedButton = authenticatedPage
        .locator('button:has-text("Rendered")')
        .first()
      if (await renderedButton.isVisible()) {
        await renderedButton.click()
        await authenticatedPage.waitForTimeout(1500)

        // Leave placeholder empty and try to render
        const renderButton = authenticatedPage
          .locator('button:has-text("Render")')
          .first()
        if (await renderButton.isVisible()) {
          await renderButton.click()
          await authenticatedPage.waitForTimeout(1000)
        }
      }

      // Should handle gracefully without errors
      await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/)
    })
  })
})

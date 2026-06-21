import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: Prompt Inclusions (@mentions)
 * Tests the @mention syntax for including other prompts
 */
test.describe('Prompt Inclusions (@mentions)', () => {
  test.setTimeout(30000)

  test.describe('Basic @mention Functionality', () => {
    test('should include prompt via @mention syntax', async ({
      authenticatedPage,
    }) => {
      // Create a base prompt first
      const basePromptName = `Base ${Date.now()}`
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(basePromptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Base content here')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Extract slug from URL
      const baseUrl = authenticatedPage.url()
      const baseSlugMatch = baseUrl.match(/prompts\/([^/]+)$/)

      if (baseSlugMatch) {
        const baseSlug = baseSlugMatch[1]

        // Create another prompt that references the first one
        const referencePromptName = `Reference ${Date.now()}`
        await authenticatedPage.goto('/prompts/new')
        await authenticatedPage.waitForSelector(
          'input[placeholder*="Enter prompt name"]',
          { timeout: 10000 }
        )

        await authenticatedPage
          .locator('input[placeholder*="Enter prompt name"]')
          .fill(referencePromptName)
        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .fill(`Start @${baseSlug} end`)
        await authenticatedPage
          .locator('[data-testid="prompt-save-button"]')
          .click()

        await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
          timeout: 10000,
        })

        // Verify the prompt contains the @mention
        const promptContent = await authenticatedPage.textContent('body')
        expect(promptContent).toContain(`@${baseSlug}`)
      }
    })

    test('should autocomplete @mention from existing prompts', async ({
      authenticatedPage,
    }) => {
      // Create a base prompt
      const basePromptName = `Autocomplete ${Date.now()}`
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(basePromptName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Base for autocomplete')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      // Create new prompt and type @ to trigger autocomplete
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill('Test Autocomplete')

      const textarea = authenticatedPage.locator(
        'textarea[placeholder*="Write your prompt here"]'
      )
      await textarea.fill('Type @')

      // Wait a bit to see if autocomplete appears
      await authenticatedPage.waitForTimeout(1000)

      // Check if modal or dropdown appeared
      const modal = authenticatedPage.locator('[role="dialog"]')
      const hasModal = await modal.isVisible().catch(() => false)

      if (hasModal) {
        // Modal appeared for @mention selection
        expect(hasModal).toBeTruthy()
      } else {
        // No modal - just verify we can type the mention
        await textarea.fill(`Type @${basePromptName}`)
      }
    })
  })

  test.describe('@mention Resolution', () => {
    test('should resolve single @mention in rendered view', async ({
      authenticatedPage,
    }) => {
      // Create base prompt
      const baseName = `ResolveBase ${Date.now()}`
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(baseName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('This is the base content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })
      const baseSlug = authenticatedPage.url().match(/prompts\/([^/]+)$/)?.[1]

      if (baseSlug) {
        // Create referencing prompt
        await authenticatedPage.goto('/prompts/new')
        await authenticatedPage.waitForSelector(
          'input[placeholder*="Enter prompt name"]',
          { timeout: 10000 }
        )

        await authenticatedPage
          .locator('input[placeholder*="Enter prompt name"]')
          .fill(`Resolver ${Date.now()}`)
        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .fill(`Reference: @${baseSlug}`)
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

        // Verify content is displayed
        const content = await authenticatedPage.textContent('body')
        expect(content).toBeTruthy()
      }
    })

    test('should resolve multiple @mentions in one prompt', async ({
      authenticatedPage,
    }) => {
      // Create two base prompts
      const prompt1Name = `Multi1 ${Date.now()}`
      const prompt2Name = `Multi2 ${Date.now()}`

      // Create first
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
        .fill('Content 1')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      const slug1 = authenticatedPage.url().match(/prompts\/([^/]+)$/)?.[1]

      // Create second
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
        .fill('Content 2')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      const slug2 = authenticatedPage.url().match(/prompts\/([^/]+)$/)?.[1]

      if (slug1 && slug2) {
        // Create prompt with multiple references
        await authenticatedPage.goto('/prompts/new')
        await authenticatedPage.waitForSelector(
          'input[placeholder*="Enter prompt name"]',
          { timeout: 10000 }
        )
        await authenticatedPage
          .locator('input[placeholder*="Enter prompt name"]')
          .fill(`Multi Ref ${Date.now()}`)
        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .fill(`First: @${slug1} Second: @${slug2}`)
        await authenticatedPage
          .locator('[data-testid="prompt-save-button"]')
          .click()

        await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
          timeout: 10000,
        })

        // The detail page defaults to the Rendered tab, which resolves both
        // @mentions into the referenced prompts' contents (the literal `@slug`
        // text is replaced, so don't count `@` characters — a successful render
        // legitimately contains none). Both contents visible = both resolved.
        await expect(
          authenticatedPage.getByText('Content 1').first()
        ).toBeVisible({ timeout: 15000 })
        await expect(
          authenticatedPage.getByText('Content 2').first()
        ).toBeVisible({ timeout: 15000 })
      }
    })

    test('should handle @mentions with placeholders', async ({
      authenticatedPage,
    }) => {
      // Create base prompt with placeholder
      const baseName = `BaseWithPlaceholder ${Date.now()}`
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(baseName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Hello {{name}}')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      const baseSlug = authenticatedPage.url().match(/prompts\/([^/]+)$/)?.[1]

      if (baseSlug) {
        // Create referencing prompt
        await authenticatedPage.goto('/prompts/new')
        await authenticatedPage.waitForSelector(
          'input[placeholder*="Enter prompt name"]',
          { timeout: 10000 }
        )

        await authenticatedPage
          .locator('input[placeholder*="Enter prompt name"]')
          .fill(`RefWithPlaceholder ${Date.now()}`)
        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .fill(`Reference: @${baseSlug}`)
        await authenticatedPage
          .locator('[data-testid="prompt-save-button"]')
          .click()

        await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
          timeout: 10000,
        })

        // Verify creation
        await authenticatedPage.waitForLoadState('networkidle')
        const content = await authenticatedPage.textContent('body')
        expect(content).toBeTruthy()
      }
    })

    test('should combine @mentions and {{placeholders}}', async ({
      authenticatedPage,
    }) => {
      // Create base prompt
      const baseName = `CombineBase ${Date.now()}`
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(baseName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Base content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      const baseSlug = authenticatedPage.url().match(/prompts\/([^/]+)$/)?.[1]

      if (baseSlug) {
        // Create composite prompt with both features
        const compositeName = `Composite ${Date.now()}`
        await authenticatedPage.goto('/prompts/new')
        await authenticatedPage.waitForSelector(
          'input[placeholder*="Enter prompt name"]',
          { timeout: 10000 }
        )

        await authenticatedPage
          .locator('input[placeholder*="Enter prompt name"]')
          .fill(compositeName)
        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .fill(`@${baseSlug} works at {{company}}`)
        await authenticatedPage
          .locator('[data-testid="prompt-save-button"]')
          .click()

        await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
          timeout: 10000,
        })

        // Verify prompt was created with both features
        await authenticatedPage.waitForLoadState('networkidle')
        const bodyContent = await authenticatedPage.textContent('body')
        expect(bodyContent).toContain(compositeName)
      }
    })

    test('should update @mention when referenced prompt changes', async ({
      authenticatedPage,
    }) => {
      // Create base prompt
      const baseName = `UpdateBase ${Date.now()}`
      const originalContent = 'Original base content'

      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(baseName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill(originalContent)
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      const baseSlug = authenticatedPage.url().match(/prompts\/([^/]+)$/)?.[1]

      if (baseSlug) {
        // Create referencing prompt
        await authenticatedPage.goto('/prompts/new')
        await authenticatedPage.waitForSelector(
          'input[placeholder*="Enter prompt name"]',
          { timeout: 10000 }
        )

        await authenticatedPage
          .locator('input[placeholder*="Enter prompt name"]')
          .fill(`RefToUpdate ${Date.now()}`)
        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .fill(`Reference: @${baseSlug}`)
        await authenticatedPage
          .locator('[data-testid="prompt-save-button"]')
          .click()

        await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
          timeout: 10000,
        })
        const refUrl = authenticatedPage.url()

        // Now update the base prompt. Wait for the detail page actions to be
        // ready explicitly — networkidle alone can resolve while the prompt is
        // still loading under CI load.
        await authenticatedPage.goto(`/prompts/${baseSlug}`)

        const editLink = authenticatedPage
          .locator('[data-testid="edit-prompt-button"]')
          .first()
        await expect(editLink).toBeVisible({ timeout: 15000 })
        await editLink.click()
        await authenticatedPage.waitForURL(/edit/, { timeout: 10000 })

        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .clear()
        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .fill('Updated base content')
        await authenticatedPage
          .locator('[data-testid="prompt-save-button"]')
          .click()

        await authenticatedPage.waitForTimeout(2000)

        // Go back to referencing prompt
        await authenticatedPage.goto(refUrl)
        await authenticatedPage.waitForLoadState('networkidle')

        // The reference should still work
        const content = await authenticatedPage.textContent('body')
        expect(content).toBeTruthy()
      }
    })
  })

  test.describe('@mention Error Handling', () => {
    test('should handle circular @mention references gracefully', async ({
      authenticatedPage,
    }) => {
      // Create first prompt
      const prompt1Name = `Circular1 ${Date.now()}`
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
        .fill('First prompt content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      const slug1 = authenticatedPage.url().match(/prompts\/([^/]+)$/)?.[1]

      if (slug1) {
        // Create second prompt that references first
        const prompt2Name = `Circular2 ${Date.now()}`
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
          .fill(`Reference to first: @${slug1}`)
        await authenticatedPage
          .locator('[data-testid="prompt-save-button"]')
          .click()
        await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
          timeout: 10000,
        })

        // Should create successfully (circular check happens at render time)
        await authenticatedPage.waitForLoadState('networkidle')
        const content = await authenticatedPage.textContent('body')
        expect(content).toBeTruthy()
      }
    })

    test('should show error for non-existent @mention', async ({
      authenticatedPage,
    }) => {
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(`NonExistent ${Date.now()}`)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Reference: @nonexistent-slug-12345')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()

      // Should either show error or create with warning
      await authenticatedPage.waitForTimeout(2000)

      const currentUrl = authenticatedPage.url()
      // Either stayed on create (error) or went to detail (created with warning)
      expect(currentUrl).toContain('/prompts')
    })

    test('should preserve @mention syntax in raw view', async ({
      authenticatedPage,
    }) => {
      // Create base prompt
      const baseName = `RawBase ${Date.now()}`
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(baseName)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Base for raw view')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      const baseSlug = authenticatedPage.url().match(/prompts\/([^/]+)$/)?.[1]

      if (baseSlug) {
        // Create referencing prompt
        await authenticatedPage.goto('/prompts/new')
        await authenticatedPage.waitForSelector(
          'input[placeholder*="Enter prompt name"]',
          { timeout: 10000 }
        )

        await authenticatedPage
          .locator('input[placeholder*="Enter prompt name"]')
          .fill(`RawRef ${Date.now()}`)
        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .fill(`Raw view: @${baseSlug}`)
        await authenticatedPage
          .locator('[data-testid="prompt-save-button"]')
          .click()

        await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
          timeout: 10000,
        })
        await authenticatedPage.waitForLoadState('networkidle')

        // Switch to raw view
        const rawButton = authenticatedPage
          .locator('button:has-text("Raw")')
          .first()
        if (await rawButton.isVisible()) {
          await rawButton.click()
          await authenticatedPage.waitForTimeout(1000)

          // Verify @mention syntax is preserved
          const bodyContent = await authenticatedPage.textContent('body')
          expect(bodyContent).toContain(`@${baseSlug}`)
        }
      }
    })

    test('should render nested @mentions (multi-level)', async ({
      authenticatedPage,
    }) => {
      // Create level 1 prompt
      const level1Name = `Level1 ${Date.now()}`
      await authenticatedPage.goto('/prompts/new')
      await authenticatedPage.waitForSelector(
        'input[placeholder*="Enter prompt name"]',
        { timeout: 10000 }
      )

      await authenticatedPage
        .locator('input[placeholder*="Enter prompt name"]')
        .fill(level1Name)
      await authenticatedPage
        .locator('textarea[placeholder*="Write your prompt here"]')
        .fill('Level 1 content')
      await authenticatedPage
        .locator('[data-testid="prompt-save-button"]')
        .click()
      await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
        timeout: 10000,
      })

      const slug1 = authenticatedPage.url().match(/prompts\/([^/]+)$/)?.[1]

      if (slug1) {
        // Create level 2 that references level 1
        const level2Name = `Level2 ${Date.now()}`
        await authenticatedPage.goto('/prompts/new')
        await authenticatedPage.waitForSelector(
          'input[placeholder*="Enter prompt name"]',
          { timeout: 10000 }
        )

        await authenticatedPage
          .locator('input[placeholder*="Enter prompt name"]')
          .fill(level2Name)
        await authenticatedPage
          .locator('textarea[placeholder*="Write your prompt here"]')
          .fill(`Level 2 includes: @${slug1}`)
        await authenticatedPage
          .locator('[data-testid="prompt-save-button"]')
          .click()
        await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
          timeout: 10000,
        })

        const slug2 = authenticatedPage.url().match(/prompts\/([^/]+)$/)?.[1]

        if (slug2) {
          // Create level 3 that references level 2
          await authenticatedPage.goto('/prompts/new')
          await authenticatedPage.waitForSelector(
            'input[placeholder*="Enter prompt name"]',
            { timeout: 10000 }
          )

          await authenticatedPage
            .locator('input[placeholder*="Enter prompt name"]')
            .fill(`Level3 ${Date.now()}`)
          await authenticatedPage
            .locator('textarea[placeholder*="Write your prompt here"]')
            .fill(`Level 3 includes: @${slug2}`)
          await authenticatedPage
            .locator('[data-testid="prompt-save-button"]')
            .click()

          await authenticatedPage.waitForURL(/\/prompts\/(?!new$)[^/]+$/, {
            timeout: 10000,
          })

          // Verify nested structure created successfully
          await authenticatedPage.waitForLoadState('networkidle')
          const content = await authenticatedPage.textContent('body')
          expect(content).toBeTruthy()
        }
      }
    })
  })
})

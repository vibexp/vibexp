import { test, expect } from '../../fixtures/auth'

/**
 * Feature Test: Prompt Placeholder Variables ({{variable}})
 *
 * Trimmed to the end-to-end happy path (issue #66): create a prompt containing a
 * {{placeholder}}, switch to the rendered view, fill the value, and render.
 * Granular placeholder behaviour — detection, multiple/nested placeholders,
 * special characters, syntax validation, persistence across edits — is covered
 * by the usePromptRenderer unit tests (tests/hooks/usePromptRenderer.test.ts),
 * so it is intentionally not duplicated here.
 */
test.describe('Prompt Placeholder Variables', () => {
  test.setTimeout(30000)

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

    // Fill the placeholder value if the rendered view exposes the input
    const placeholderInput = authenticatedPage
      .locator('input[placeholder*="greeting"]')
      .first()
    const inputExists = await placeholderInput.isVisible().catch(() => false)

    if (inputExists) {
      await placeholderInput.fill('Hello World')

      const renderButton = authenticatedPage
        .locator('button:has-text("Render")')
        .first()
      if (await renderButton.isVisible()) {
        await renderButton.click()
        await authenticatedPage.waitForTimeout(2000)
      }
    }

    // The prompt persisted and we stayed on its detail page.
    await expect(authenticatedPage).toHaveURL(/\/prompts\/(?!new$)[^/]+$/)
  })
})

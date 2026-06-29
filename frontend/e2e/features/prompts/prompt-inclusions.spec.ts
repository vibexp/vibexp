import { test, expect } from '../../fixtures/auth'

/**
 * Feature Test: Prompt Inclusions (@mentions)
 *
 * Trimmed to the end-to-end happy path (issue #66): create a base prompt, then
 * reference it from another prompt via @slug and confirm the mention is stored.
 * Granular @mention behaviour — autocomplete, multi/nested resolution, mixing
 * with placeholders, circular references, missing targets — is covered by the
 * usePromptRenderer unit tests (tests/hooks/usePromptRenderer.test.ts), so it is
 * intentionally not duplicated here.
 */
test.describe('Prompt Inclusions (@mentions)', () => {
  test.setTimeout(30000)

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
    expect(baseSlugMatch).not.toBeNull()
    const baseSlug = baseSlugMatch![1]

    // Create another prompt that references the first one via @slug
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

    // The saved prompt carries the @mention.
    const promptContent = await authenticatedPage.textContent('body')
    expect(promptContent).toContain(`@${baseSlug}`)
  })
})

import { test, expect } from '../../fixtures/auth'

/**
 * Feature Test: Public Shared Prompt
 *
 * Closes the e2e gap on the public `/shared/prompts/:token` route (issue #66).
 * Happy path: create a prompt, make a public share link for it (via the real
 * share endpoint), then open that link in a CLEAN, unauthenticated browser
 * context and confirm the prompt renders. This proves the public route is
 * reachable outside the auth gate and that the share token resolves end-to-end.
 */
test.describe('Public Shared Prompt', () => {
  test('a public share link renders the prompt without authentication', async ({
    authenticatedPage: page,
    browser,
  }) => {
    // --- Create a prompt through the UI (mirrors the prompt-crud happy path) ---
    const promptName = `Shared Prompt ${Date.now()}`
    await page.goto('/prompts/new')
    await page.waitForSelector('input[placeholder*="Enter prompt name"]', {
      timeout: 10000,
    })
    await page
      .locator('input[placeholder*="Enter prompt name"]')
      .fill(promptName)
    await page
      .locator('textarea[placeholder*="Write your prompt here"]')
      .fill('Body of a prompt shared publicly for E2E.')
    await page.locator('[data-testid="prompt-save-button"]').click()

    // After save we land on the prompt detail page: /prompts/<slug>.
    await page.waitForURL(/\/prompts\/(?!new$)[^/]+$/, { timeout: 10000 })
    const slug = new URL(page.url()).pathname.split('/').pop()
    expect(slug).toBeTruthy()

    const teamId = await page.evaluate(() =>
      localStorage.getItem('vx_current_team_id')
    )
    expect(teamId).toBeTruthy()

    // --- Create a public share link via the real backend endpoint ---
    const shareResp = await page.request.post(
      `/api/v1/${teamId}/prompts/${slug}/share`,
      { data: { share_type: 'public' } }
    )
    expect(shareResp.ok()).toBeTruthy()
    const shareBody = (await shareResp.json()) as {
      share_token?: string
      data?: { share_token?: string }
    }
    const shareToken = shareBody.share_token ?? shareBody.data?.share_token
    expect(shareToken).toBeTruthy()

    // --- Open the public link in a fresh, unauthenticated context ---
    const origin = new URL(page.url()).origin
    const anonContext = await browser.newContext()
    try {
      const anonPage = await anonContext.newPage()
      await anonPage.goto(`${origin}/shared/prompts/${shareToken}`)
      await expect(
        anonPage.getByText(promptName, { exact: false })
      ).toBeVisible({ timeout: 10000 })
    } finally {
      await anonContext.close()
    }
  })
})

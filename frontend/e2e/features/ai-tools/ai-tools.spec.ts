import { mockAiToolsApi } from '../../fixtures/api-mocks'
import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: AI Tools pages
 *
 * Session/hook data is ingested from real Claude Code / Cursor IDE installs,
 * so the overview pages run on route mocks (mockAiToolsApi). The sessions and
 * setup routes are still DeferredToolPage placeholders ("Coming soon") — the
 * specs pin that state and will fail loudly when the pages are rebuilt.
 */
test.describe('AI Tools', () => {
  test('should render the AI tools overview with both tools', async ({
    authenticatedPage,
  }) => {
    await mockAiToolsApi(authenticatedPage)

    await authenticatedPage.goto('/ai-tools/overview')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'AI Tools', exact: true })
    ).toBeVisible({ timeout: 10000 })

    await expect(
      authenticatedPage.getByText('Available AI tools')
    ).toBeVisible()
    await expect(
      authenticatedPage.getByText('Claude Code').first()
    ).toBeVisible()
    await expect(
      authenticatedPage.getByText('Cursor IDE').first()
    ).toBeVisible()
  })

  test('should navigate from overview to the Claude Code tool page', async ({
    authenticatedPage,
  }) => {
    await mockAiToolsApi(authenticatedPage)

    await authenticatedPage.goto('/ai-tools/overview')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'AI Tools', exact: true })
    ).toBeVisible({ timeout: 10000 })

    await authenticatedPage
      .getByRole('button', { name: 'Manage' })
      .first()
      .click()

    await authenticatedPage.waitForURL(/\/ai-tools\/claude-code\/overview$/, {
      timeout: 10000,
    })
  })

  test('should render Claude Code overview stats and activity', async ({
    authenticatedPage,
  }) => {
    await mockAiToolsApi(authenticatedPage)

    await authenticatedPage.goto('/ai-tools/claude-code/overview')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Claude Code' })
    ).toBeVisible({ timeout: 10000 })

    await expect(
      authenticatedPage.getByText('Total sessions').first()
    ).toBeVisible({ timeout: 10000 })
    await expect(authenticatedPage.getByText('Recent activity')).toBeVisible()
    // Mocked activity row.
    await expect(authenticatedPage.getByText('Bash').first()).toBeVisible()
  })

  test('should render Cursor IDE overview stats and activity', async ({
    authenticatedPage,
  }) => {
    await mockAiToolsApi(authenticatedPage)

    await authenticatedPage.goto('/ai-tools/cursor-ide/overview')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Cursor IDE' })
    ).toBeVisible({ timeout: 10000 })

    await expect(
      authenticatedPage.getByText('Total sessions').first()
    ).toBeVisible({ timeout: 10000 })
    await expect(authenticatedPage.getByText('edit_file').first()).toBeVisible()
  })

  test('should show the deferred placeholder on sessions and setup pages', async ({
    authenticatedPage,
  }) => {
    for (const path of [
      '/ai-tools/claude-code/sessions',
      '/ai-tools/claude-code/setup',
      '/ai-tools/cursor-ide/sessions',
      '/ai-tools/cursor-ide/setup',
    ]) {
      await authenticatedPage.goto(path)
      await expect(authenticatedPage.getByText('Coming soon')).toBeVisible({
        timeout: 10000,
      })
    }
  })
})

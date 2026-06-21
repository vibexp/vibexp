import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: VibeXP MCP server page
 *
 * /mcp-servers/vibexp-mcp is a configuration/instructions page: OAuth connect
 * explainer, per-client setup sections, the MCP tools list, and the team
 * identifier (UUID/slug) rows sourced from the team context.
 */
test.describe('VibeXP MCP page', () => {
  test('should render the MCP integration page', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/mcp-servers/vibexp-mcp')
    await expect(authenticatedPage).toHaveURL(/mcp-servers\/vibexp-mcp$/)

    await expect(
      authenticatedPage.getByRole('heading', {
        name: 'VibeXP MCP Integration',
      })
    ).toBeVisible({ timeout: 15000 })

    // The guided-setup redesign (#1813) replaced the "How OAuth connect works"
    // explainer with a "Connect your client" section (client tabs + config).
    await expect(
      authenticatedPage.getByText('Connect your client')
    ).toBeVisible()
  })

  test('should show team identifiers for the current team', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/mcp-servers/vibexp-mcp')
    await expect(
      authenticatedPage.getByRole('heading', {
        name: 'VibeXP MCP Integration',
      })
    ).toBeVisible({ timeout: 15000 })

    // TeamIdentifiers renders UUID + Slug rows with copy buttons.
    await expect(authenticatedPage.getByText('UUID').first()).toBeVisible({
      timeout: 10000,
    })
    await expect(authenticatedPage.getByText('Slug').first()).toBeVisible()
  })
})

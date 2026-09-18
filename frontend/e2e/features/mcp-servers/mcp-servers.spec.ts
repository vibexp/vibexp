import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: VibeXP MCP server page
 *
 * /mcp-servers/vibexp-mcp is a configuration/instructions page: OAuth connect
 * explainer, per-client setup sections and the MCP tools list. Team
 * identifiers are no longer shown: the agent discovers teams itself (#1042).
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

  test('should not show a team identifiers section', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/mcp-servers/vibexp-mcp')
    await expect(
      authenticatedPage.getByRole('heading', {
        name: 'VibeXP MCP Integration',
      })
    ).toBeVisible({ timeout: 15000 })

    await expect(
      authenticatedPage.getByText('Your team identifiers')
    ).toHaveCount(0)
  })
})

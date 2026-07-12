import { test, expect } from '../../fixtures/auth'

/**
 * Feature Test: full A2A agent journey (no mocks, real end to end)
 *
 * Unlike agent-pages.spec.ts (which route-mocks agent data), this drives the
 * complete user journey against a REAL A2A agent — the `a2a-dummy-agent` served
 * by the e2e stack (backend/cmd/a2a-dummy-agent), which advertises a v0.3 card
 * and echoes each message. Nothing here is mocked: the backend resolves the
 * card, persists the agent, and proxies the chat over the A2A protocol.
 *
 * Journey: add the agent by URL → its card auto-resolves into a preview →
 * create it → chat and get a reply → open Conversations → resume → send again.
 *
 * The agent URL differs by environment (loopback for a local dev run, the
 * compose service name inside the e2e docker network), so it is injected via
 * E2E_A2A_AGENT_URL. The URL must be reachable by the BACKEND, not the browser.
 */
const AGENT_URL = process.env.E2E_A2A_AGENT_URL ?? 'http://127.0.0.1:9001'
const AGENT_NAME = 'A2A Dummy Test Agent'

test.describe('Agent journey (real A2A agent)', () => {
  // The full round-trip (card resolution + two chat turns with polling) needs
  // more than the default per-test budget.
  test.setTimeout(90_000)

  test('add → resolve → create → chat → conversations → resume', async ({
    authenticatedPage: page,
  }) => {
    // 1. Add the agent by base URL; the card resolves into a live preview.
    await page.goto('/agents/add')
    await expect(page.getByRole('heading', { name: 'Add agent' })).toBeVisible({
      timeout: 10_000,
    })

    await page.locator('#baseUrl').fill(AGENT_URL)

    // The preview renders the resolved card (name + v0.3 protocol) — proof the
    // backend fetched and parsed the real agent card.
    await expect(page.getByRole('heading', { name: AGENT_NAME })).toBeVisible({
      timeout: 15_000,
    })
    await expect(page.getByText('0.3', { exact: true })).toBeVisible()

    // 2. Create the agent; on success we land back on the agents list.
    await page.getByRole('button', { name: 'Create agent' }).click()
    await expect(page).toHaveURL(/\/agents$/, { timeout: 15_000 })
    await expect(page.getByText(AGENT_NAME).first()).toBeVisible({
      timeout: 10_000,
    })

    // 3. Open the agent's chat directly from the list's row action.
    await page.getByRole('button', { name: `Chat with ${AGENT_NAME}` }).click()
    await expect(
      page.getByRole('heading', { name: `Chat with ${AGENT_NAME}` })
    ).toBeVisible({ timeout: 10_000 })

    // Send the first message and assert the agent's echoed reply renders.
    await page.getByPlaceholder(/Type your message/).fill('ping')
    await page.getByRole('button', { name: 'Send message' }).click()
    await expect(page.getByText('ping', { exact: true })).toBeVisible({
      timeout: 10_000,
    })
    await expect(page.getByText('dummy response: you said "ping"')).toBeVisible(
      { timeout: 20_000 }
    )

    // 4. Go to Conversations; the conversation we just created is listed.
    await page.getByRole('button', { name: 'Back to agent' }).click()
    await page.getByRole('button', { name: 'Conversations' }).click()
    await expect(
      page.getByRole('heading', { name: `${AGENT_NAME} — Conversations` })
    ).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('ping').first()).toBeVisible({
      timeout: 10_000,
    })

    // 5. Resume the conversation and send another message; it replies again.
    await page.getByRole('button', { name: 'Resume' }).first().click()
    await expect(
      page.getByRole('heading', { name: `Chat with ${AGENT_NAME}` })
    ).toBeVisible({ timeout: 10_000 })
    // Prior turn is restored.
    await expect(page.getByText('dummy response: you said "ping"')).toBeVisible(
      { timeout: 10_000 }
    )

    await page.getByPlaceholder(/Type your message/).fill('hello again')
    await page.getByRole('button', { name: 'Send message' }).click()
    await expect(
      page.getByText('dummy response: you said "hello again"')
    ).toBeVisible({ timeout: 20_000 })
  })
})

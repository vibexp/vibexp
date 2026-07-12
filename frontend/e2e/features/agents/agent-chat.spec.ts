import {
  buildMockAgent,
  mockAgentChatHappyPath,
  mockAgentsApi,
} from '../../fixtures/api-mocks'
import { test, expect } from '../../fixtures/auth'

/**
 * Feature Test: Agent chat happy path
 *
 * Complements agent-pages.spec.ts (which mocks agent data but leaves the chat
 * execute/polling flow out of scope). Here we mock that flow too so the full
 * user journey is exercised end to end: open an agent's chat, send a message,
 * and see the agent's reply rendered.
 *
 * Agents can't be seeded through the UI (they register from a live A2A
 * agent-card URL), so both the agent and its execution are route-mocked. The
 * mock mirrors a real streaming agent that answers with a direct message reply
 * (accepted execution → poll finalizes success with no events → reply carried
 * on the finalized execution's artifacts).
 */
test.describe('Agent chat', () => {
  test('sends a message and renders the agent reply', async ({
    authenticatedPage,
  }) => {
    const agent = buildMockAgent({ id: 'agent-42', name: 'Chatty Agent' })
    const reply = 'Pong from the E2E agent'

    await mockAgentsApi(authenticatedPage, { agents: [agent] })
    // Registered after mockAgentsApi so these narrower routes win.
    await mockAgentChatHappyPath(authenticatedPage, {
      agentId: agent.id,
      reply,
    })

    await authenticatedPage.goto('/agents/agent-42/chat')

    await expect(
      authenticatedPage.getByRole('heading', { name: 'Chat with Chatty Agent' })
    ).toBeVisible({ timeout: 10000 })

    // Send a message.
    const input = authenticatedPage.getByPlaceholder(/Type your message/)
    await input.fill('Ping')
    await authenticatedPage
      .getByRole('button', { name: 'Send message' })
      .click()

    // The user's message is echoed into the transcript...
    await expect(
      authenticatedPage.getByText('Ping', { exact: true })
    ).toBeVisible({
      timeout: 10000,
    })
    // ...and the agent's reply is rendered once polling finalizes.
    await expect(authenticatedPage.getByText(reply)).toBeVisible({
      timeout: 10000,
    })

    // Capturing the conversation id flips the page into "continuing" mode.
    await expect(
      authenticatedPage.getByText('Continuing conversation.')
    ).toBeVisible({ timeout: 10000 })
  })
})

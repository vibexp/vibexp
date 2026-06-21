import {
  buildMockAgent,
  buildMockConversation,
  mockAgentsApi,
} from '../../fixtures/api-mocks'
import { test, expect } from '../../fixtures/auth'

/**
 * Feature Tests: Agents pages
 *
 * Agents are registered from live A2A agent-card URLs, which E2E can't seed
 * through the UI — all agent data comes from route mocks (mockAgentsApi).
 * The chat execute/polling flow is intentionally out of scope.
 */
test.describe('Agents', () => {
  test('should show empty state when there are no agents', async ({
    authenticatedPage,
  }) => {
    await mockAgentsApi(authenticatedPage, { agents: [] })

    await authenticatedPage.goto('/agents')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Agents', exact: true })
    ).toBeVisible({ timeout: 10000 })
    await expect(authenticatedPage.getByText('No agents yet')).toBeVisible({
      timeout: 10000,
    })
  })

  test('should list agents with stats', async ({ authenticatedPage }) => {
    const agent = buildMockAgent({ name: 'Mocked List Agent' })
    await mockAgentsApi(authenticatedPage, { agents: [agent] })

    await authenticatedPage.goto('/agents')
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Agents', exact: true })
    ).toBeVisible({ timeout: 10000 })

    await expect(
      authenticatedPage.getByText('Mocked List Agent').first()
    ).toBeVisible({ timeout: 10000 })
    // Stats cards render once the (non-empty) list loads.
    await expect(authenticatedPage.getByText('Total agents')).toBeVisible()
  })

  test('should render the add-agent form', async ({ authenticatedPage }) => {
    await authenticatedPage.goto('/agents/add')

    await expect(
      authenticatedPage.getByRole('heading', { name: 'Add agent' })
    ).toBeVisible({ timeout: 10000 })
    await expect(authenticatedPage.getByLabel('Agent base URL')).toBeVisible()
  })

  test('should show agent details with actions', async ({
    authenticatedPage,
  }) => {
    const agent = buildMockAgent({ id: 'agent-42', name: 'Detail Agent' })
    await mockAgentsApi(authenticatedPage, { agents: [agent] })

    await authenticatedPage.goto('/agents/agent-42')

    // The details page renders the agent name as both h1 and a section h2.
    await expect(
      authenticatedPage.getByRole('heading', { name: 'Detail Agent', level: 1 })
    ).toBeVisible({ timeout: 10000 })
    await expect(
      authenticatedPage.getByRole('button', { name: 'Chat' })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByRole('button', { name: 'Conversations' })
    ).toBeVisible()
    await expect(
      authenticatedPage.getByRole('button', { name: 'Edit' })
    ).toBeVisible()
  })

  test('should render the chat page for an agent', async ({
    authenticatedPage,
  }) => {
    const agent = buildMockAgent({ id: 'agent-42', name: 'Chatty Agent' })
    await mockAgentsApi(authenticatedPage, { agents: [agent] })

    await authenticatedPage.goto('/agents/agent-42/chat')

    await expect(
      authenticatedPage.getByRole('heading', { name: 'Chat with Chatty Agent' })
    ).toBeVisible({ timeout: 10000 })
  })

  test('should list agent conversations with resume action', async ({
    authenticatedPage,
  }) => {
    const agent = buildMockAgent({ id: 'agent-42', name: 'Convo Agent' })
    const conversation = buildMockConversation({
      agent_id: 'agent-42',
      first_message: 'Summarize the launch metrics',
    })
    await mockAgentsApi(authenticatedPage, {
      agents: [agent],
      conversations: [conversation],
    })

    await authenticatedPage.goto('/agents/agent-42/conversations')

    await expect(
      authenticatedPage.getByText('Summarize the launch metrics').first()
    ).toBeVisible({ timeout: 10000 })
    await expect(
      authenticatedPage.getByRole('button', { name: 'Resume' })
    ).toBeVisible()
  })

  test('should show empty conversations state', async ({
    authenticatedPage,
  }) => {
    const agent = buildMockAgent({ id: 'agent-42', name: 'Lonely Agent' })
    await mockAgentsApi(authenticatedPage, { agents: [agent] })

    await authenticatedPage.goto('/agents/agent-42/conversations')

    await expect(
      authenticatedPage.getByText('No conversations yet')
    ).toBeVisible({ timeout: 10000 })
  })

  test('should render the tasks page with empty executions', async ({
    authenticatedPage,
  }) => {
    const agent = buildMockAgent({ id: 'agent-42', name: 'Tasky Agent' })
    await mockAgentsApi(authenticatedPage, { agents: [agent] })

    await authenticatedPage.goto('/agents/agent-42/tasks')

    await expect(
      authenticatedPage.getByRole('heading', { name: 'Tasks: Tasky Agent' })
    ).toBeVisible({ timeout: 10000 })
    await expect(
      authenticatedPage.getByText('No executions found')
    ).toBeVisible({ timeout: 10000 })
  })
})

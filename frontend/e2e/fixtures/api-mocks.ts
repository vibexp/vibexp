/**
 * API Mock Utilities
 *
 * Provides utilities for mocking API responses in E2E tests.
 * Useful for testing error states, subscription flows, and external service interactions.
 */

import { Page } from '@playwright/test'

export interface SubscriptionProduct {
  id: string
  name: string
  price: number
  currency: string
  interval: 'month' | 'year'
  features: string[]
}

export interface SubscriptionStatus {
  isActive: boolean
  planId?: string
  planName?: string
  currentPeriodEnd?: string
  cancelAtPeriodEnd?: boolean
}

/**
 * Mock subscription products for testing pricing display
 *
 * @param page - Playwright page instance
 * @param products - Array of subscription products to mock
 */
export async function mockSubscriptionProducts(
  page: Page,
  products: SubscriptionProduct[]
): Promise<void> {
  await page.route('**/api/v1/subscription/products', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ products }),
    })
  })
}

/**
 * Mock subscription status for testing active/inactive states
 *
 * @param page - Playwright page instance
 * @param status - Subscription status to mock
 */
export async function mockSubscriptionStatus(
  page: Page,
  status: SubscriptionStatus
): Promise<void> {
  await page.route('**/api/v1/subscription/status', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(status),
    })
  })
}

/**
 * Mock Stripe checkout session creation
 *
 * @param page - Playwright page instance
 * @param successUrl - URL to redirect to after successful checkout
 */
export async function mockCheckoutSession(
  page: Page,
  successUrl: string
): Promise<void> {
  await page.route('**/api/v1/subscription/checkout', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        url: successUrl,
        sessionId: 'test_session_' + Date.now(),
      }),
    })
  })
}

/**
 * Mock API error response for testing error handling
 *
 * @param page - Playwright page instance
 * @param endpoint - API endpoint pattern to mock (supports wildcards)
 * @param statusCode - HTTP status code to return
 * @param errorMessage - Optional error message
 */
export async function mockApiError(
  page: Page,
  endpoint: string,
  statusCode: number,
  errorMessage?: string
): Promise<void> {
  await page.route(endpoint, async route => {
    await route.fulfill({
      status: statusCode,
      contentType: 'application/json',
      body: JSON.stringify({
        error: errorMessage || `Error ${statusCode}`,
        code: statusCode,
      }),
    })
  })
}

/**
 * Mock successful API response for testing happy paths
 *
 * @param page - Playwright page instance
 * @param endpoint - API endpoint pattern to mock
 * @param responseData - Data to return in the response
 */
export async function mockApiSuccess(
  page: Page,
  endpoint: string,
  responseData: Record<string, unknown>
): Promise<void> {
  await page.route(endpoint, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(responseData),
    })
  })
}

/**
 * Clear all route mocks for a page
 *
 * @param page - Playwright page instance
 */
export async function clearApiMocks(page: Page): Promise<void> {
  await page.unroute('**/*')
}

/**
 * Mock network delay for testing loading states
 *
 * @param page - Playwright page instance
 * @param endpoint - API endpoint pattern to mock
 * @param delayMs - Delay in milliseconds
 */
export async function mockApiDelay(
  page: Page,
  endpoint: string,
  delayMs: number
): Promise<void> {
  await page.route(endpoint, async route => {
    await new Promise(resolve => setTimeout(resolve, delayMs))
    await route.continue()
  })
}

// ---------------------------------------------------------------------------
// Agents (A2A) API mocks
//
// Agent data cannot be seeded through the UI (agents are registered from a
// live agent-card URL), so agent pages are tested against route mocks. All
// /agents endpoints are team-scoped: /api/v1/{teamId}/agents...
// ---------------------------------------------------------------------------

export interface MockAgent {
  id: string
  user_id: string
  team_id: string
  name: string
  description: string
  status: 'active' | 'paused' | 'error'
  config: Record<string, unknown>
  last_run: string | null
  total_runs: number
  success_rate: number
  created_at: string
  updated_at: string
}

/**
 * Build a complete Agent wire object with sensible defaults
 */
export function buildMockAgent(overrides?: Partial<MockAgent>): MockAgent {
  const now = new Date().toISOString()
  return {
    id: 'agent-e2e-1',
    user_id: 'user-e2e-1',
    team_id: 'team-e2e-1',
    name: 'E2E Test Agent',
    description: 'Agent used by Playwright route mocks',
    status: 'active',
    config: {},
    last_run: now,
    total_runs: 12,
    success_rate: 91.7,
    created_at: now,
    updated_at: now,
    ...overrides,
  }
}

export interface MockConversation {
  conversation_id: string
  agent_id: string
  message_count: number
  first_message: string
  last_message: string
  started_at: string
  last_activity_at: string
  last_status: string
}

/**
 * Build a conversation summary as returned by
 * GET /{teamId}/agents/{agentId}/conversations
 */
export function buildMockConversation(
  overrides?: Partial<MockConversation>
): MockConversation {
  const now = new Date().toISOString()
  return {
    conversation_id: 'conv-e2e-1',
    agent_id: 'agent-e2e-1',
    message_count: 4,
    first_message: 'Hello agent, summarize my week',
    last_message: 'Here is your weekly summary.',
    started_at: now,
    last_activity_at: now,
    last_status: 'completed',
    ...overrides,
  }
}

interface Envelope<T> {
  status: string
  message: string
  data: T
}

function envelope<T>(data: T): Envelope<T> {
  return { status: 'success', message: 'ok', data }
}

/**
 * Mock the team-scoped agents API surface.
 *
 * Registers a single route that dispatches on the URL path so the relative
 * registration order of sub-endpoints never matters:
 * - GET .../agents               → enveloped paginated list
 * - GET .../agents/stats         → enveloped stats derived from the list
 * - GET .../agents/{id}          → enveloped agent (matched by id)
 * - GET .../agents/{id}/executions    → bare executions page (empty)
 * - GET .../agents/{id}/conversations → bare conversations page
 *
 * Anything else (POST/PUT/DELETE, execution events) falls through to the
 * real backend.
 */
export async function mockAgentsApi(
  page: Page,
  options: {
    agents: MockAgent[]
    conversations?: MockConversation[]
  }
): Promise<void> {
  const { agents, conversations = [] } = options

  // RegExp, not a glob: route patterns match the FULL URL including the query
  // string, so a glob like "**/agents" silently misses "/agents?page=1".
  await page.route(/\/api\/v1\/[^/]+\/agents(\/|\?|$)/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    // path after ".../agents", e.g. "", "/stats", "/{id}", "/{id}/executions"
    const rest = url.pathname.replace(/^.*?\/agents/, '')

    if (request.method() !== 'GET') {
      await route.continue()
      return
    }

    if (rest === '' || rest === '/') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        // The backend returns the list bare (handleListAgents -> writeOK), not
        // enveloped — match that so the frontend's typed read works.
        body: JSON.stringify({
          agents,
          page: 1,
          per_page: 20,
          total_count: agents.length,
          total_pages: agents.length > 0 ? 1 : 0,
        }),
      })
      return
    }

    if (rest === '/stats') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        // Bare stats object (handleGetAgentStats -> writeOK), not enveloped.
        body: JSON.stringify({
          total_agents: agents.length,
          active_agents: agents.filter(a => a.status === 'active').length,
          paused_agents: agents.filter(a => a.status === 'paused').length,
          error_agents: agents.filter(a => a.status === 'error').length,
          total_runs: agents.reduce((sum, a) => sum + a.total_runs, 0),
          avg_success_rate: 91.7,
          runs_today: 2,
          runs_this_week: 8,
        }),
      })
      return
    }

    const executionsMatch = /^\/([^/]+)\/executions$/.exec(rest)
    if (executionsMatch) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          executions: [],
          total_count: 0,
          page: 1,
          per_page: 20,
          total_pages: 0,
        }),
      })
      return
    }

    const conversationsMatch = /^\/([^/]+)\/conversations$/.exec(rest)
    if (conversationsMatch) {
      const agentId = conversationsMatch[1]
      const agentConversations = conversations.filter(
        c => c.agent_id === agentId
      )
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          conversations: agentConversations,
          total_count: agentConversations.length,
          page: 1,
          per_page: 20,
          total_pages: agentConversations.length > 0 ? 1 : 0,
        }),
      })
      return
    }

    const detailMatch = /^\/([^/]+)$/.exec(rest)
    if (detailMatch) {
      const agent = agents.find(a => a.id === detailMatch[1])
      if (!agent) {
        await route.fulfill({
          status: 404,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'agent not found', code: 404 }),
        })
        return
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(agent),
      })
      return
    }

    await route.continue()
  })
}

// ---------------------------------------------------------------------------
// AI Tools API mocks
//
// Session/hook data is ingested from real Claude Code / Cursor IDE installs
// and cannot be seeded in E2E, so the overview pages run on route mocks.
// Endpoints are NOT team-scoped: /api/v1/ai-tools/{tool}/...
// ---------------------------------------------------------------------------

/**
 * Mock the ai-tools stats endpoints for both Claude Code and Cursor IDE:
 * session-counts, overview-stats, and recent-activities. Other ai-tools
 * endpoints fall through to the real backend.
 */
export async function mockAiToolsApi(
  page: Page,
  options?: { totalSessions?: number }
): Promise<void> {
  const totalSessions = options?.totalSessions ?? 5
  const now = new Date().toISOString()

  await page.route('**/api/v1/ai-tools/**', async route => {
    const url = new URL(route.request().url())
    const path = url.pathname

    if (path.endsWith('/session-counts')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(
          envelope({
            total_sessions: totalSessions,
            counts: [{ date: now.slice(0, 10), count: totalSessions }],
          })
        ),
      })
      return
    }

    if (path.endsWith('/overview-stats')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(
          envelope({
            total_sessions: totalSessions,
            sessions_this_week: 3,
            sessions_last_week: 2,
            weekly_trend_percent: 50,
            avg_user_prompts_per_session: 4.2,
            total_unique_tools: 6,
            top_tools: [{ tool_name: 'Bash', usage_count: 11 }],
            avg_session_duration_minutes: 17,
            total_memories: 2,
          })
        ),
      })
      return
    }

    if (path.endsWith('/recent-activities')) {
      const isCursor = path.includes('/cursor-ide/')
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(
          envelope({
            activities: [
              isCursor
                ? {
                    session_id: 'cursor-session-1',
                    tool_name: 'edit_file',
                    input: { target: 'src/index.ts' },
                    hook_event_name: 'beforeShellExecution',
                    created_at: now,
                  }
                : {
                    session_id: 'claude-session-1',
                    cwd: '/home/dev/project',
                    tool_name: 'Bash',
                    tool_input: { command: 'npm test' },
                    hook_event_name: 'PostToolUse',
                    created_at: now,
                  },
            ],
            page: 1,
            limit: 10,
            total: 1,
            total_pages: 1,
          })
        ),
      })
      return
    }

    await route.continue()
  })
}

// ---------------------------------------------------------------------------
// Notifications API mocks
//
// In-app notifications are produced by backend domain events that E2E tests
// can't reliably trigger, so notification flows run on a stateful route mock.
// Endpoints are user-scoped: /api/v1/notifications...
// ---------------------------------------------------------------------------

export interface MockNotification {
  id: string
  type: string
  category: 'high' | 'low'
  title: string
  body?: string
  action_url?: string
  read_at?: string
  created_at: string
}

/**
 * Build a Notification wire object with sensible defaults (unread)
 */
export function buildMockNotification(
  overrides?: Partial<MockNotification>
): MockNotification {
  return {
    id: `notif-${Math.random().toString(36).substring(2, 10)}`,
    type: 'feed.item.created',
    category: 'low',
    title: 'New feed item in E2E Feed',
    body: 'A new AI-generated item was posted to your feed.',
    created_at: new Date().toISOString(),
    ...overrides,
  }
}

/**
 * Mock the notifications API surface with mutable in-memory state so
 * mark-as-read flows behave like the real backend:
 * - GET   /notifications(?unread=true) → { notifications, count, limit, offset }
 * - GET   /notifications/unread-count  → { unread_count }
 * - PATCH /notifications/{id}/read     → 204, marks the item read
 * - PATCH /notifications/read-all      → 204, marks everything read
 */
export async function mockNotificationsApi(
  page: Page,
  initial: MockNotification[]
): Promise<void> {
  // Copy so each test run mutates its own state.
  const notifications = initial.map(n => ({ ...n }))

  // RegExp, not a glob — see mockAgentsApi: globs miss query strings.
  await page.route(/\/api\/v1\/notifications(\/|\?|$)/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const rest = url.pathname.replace(/^.*?\/notifications/, '')

    if (request.method() === 'GET' && rest === '/unread-count') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          unread_count: notifications.filter(n => !n.read_at).length,
        }),
      })
      return
    }

    if (request.method() === 'GET' && (rest === '' || rest === '/')) {
      const unreadOnly = url.searchParams.get('unread') === 'true'
      const limit = Number(url.searchParams.get('limit') ?? '20')
      const offset = Number(url.searchParams.get('offset') ?? '0')
      const filtered = unreadOnly
        ? notifications.filter(n => !n.read_at)
        : notifications
      const pageItems = filtered.slice(offset, offset + limit)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          notifications: pageItems,
          count: pageItems.length,
          limit,
          offset,
        }),
      })
      return
    }

    if (request.method() === 'PATCH' && rest === '/read-all') {
      const now = new Date().toISOString()
      for (const n of notifications) {
        n.read_at ??= now
      }
      await route.fulfill({ status: 204, body: '' })
      return
    }

    const readMatch = /^\/([^/]+)\/read$/.exec(rest)
    if (request.method() === 'PATCH' && readMatch) {
      const target = notifications.find(n => n.id === readMatch[1])
      if (target) {
        target.read_at ??= new Date().toISOString()
      }
      await route.fulfill({ status: 204, body: '' })
      return
    }

    await route.continue()
  })
}

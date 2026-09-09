import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mock } from 'vitest'

import type { Agent, AgentCard } from '@/services/agentService'

// recharts' ResponsiveContainer measures its parent, which has no layout in
// jsdom (the Access activity section renders a chart).
vi.mock('recharts', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: ReactNode }) => (
      <div style={{ width: 400, height: 110 }}>{children}</div>
    ),
  }
})

vi.mock('@/services/agentService', () => ({
  agentService: {
    getAgent: vi.fn(),
    listAgentExecutions: vi.fn(),
    deleteAgent: vi.fn(),
  },
}))

vi.mock('@/services/resourceAccessService', () => ({
  resourceAccessService: {
    getResourceAccessMetrics: vi.fn().mockResolvedValue({
      status: 'success',
      message: 'ok',
      data: { total_accesses: 0, range: '30d', counts: [] },
    }),
  },
}))

// usePermissions (#225) reads the signed-in user for own-vs-any delete gating.
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

// Mutable so each test chooses the server-granted permissions array — the page
// gates Delete on it through the real usePermissions hook (never mocked, #225).
const mockTeamState: {
  currentTeam: { id: string; name: string; permissions: string[] } | null
} = {
  currentTeam: { id: 'team-1', name: 'Test Team', permissions: [] },
}
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: mockTeamState.currentTeam,
    teams: mockTeamState.currentTeam ? [mockTeamState.currentTeam] : [],
    isLoading: false,
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn() as () => Promise<void>,
  }),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { agentService } from '@/services/agentService'

import { AgentDetails } from '../AgentDetails'

const AGENT_CARD: AgentCard = {
  version: '2.0.0',
  supportedInterfaces: [
    { protocolBinding: 'JSONRPC', protocolVersion: '1.0', url: 'https://a2a' },
  ],
  capabilities: { streaming: true },
  skills: [{ id: 'skill-1', name: 'Review', description: 'Reviews code' }],
  defaultInputModes: ['text'],
  defaultOutputModes: ['text'],
}

function buildAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: 'agent-1',
    user_id: 'user-1',
    team_id: 'team-1',
    name: 'Code Reviewer',
    description: 'Reviews pull requests',
    status: 'active',
    config: null,
    card_url: 'https://example.com/agent.json',
    agent_card: AGENT_CARD,
    last_run: '2026-01-03T00:00:00Z',
    last_synced_at: '2026-01-04T00:00:00Z',
    total_runs: 12,
    success_rate: 91.7,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    version: 1,
    ...overrides,
  }
}

function setTeamPermissions(permissions: string[]) {
  mockTeamState.currentTeam = { id: 'team-1', name: 'Test Team', permissions }
}

function renderAgentDetails(id = 'agent-1') {
  return render(
    <MemoryRouter initialEntries={[`/agents/${id}`]}>
      <Routes>
        <Route path="/agents/:id" element={<AgentDetails />} />
        <Route
          path="/agents"
          element={<div data-testid="list-probe">Agents list probe</div>}
        />
        <Route
          path="/agents/:id/chat"
          element={<div data-testid="chat-probe">Chat probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('AgentDetails page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setTeamPermissions([])
    ;(agentService.getAgent as Mock).mockResolvedValue(buildAgent())
    ;(agentService.listAgentExecutions as Mock).mockResolvedValue({
      executions: [
        {
          id: 'exec-1',
          status: 'success',
          input: { text: 'Review this diff' },
          started_at: '2026-01-02T10:00:00Z',
          duration: 1500,
        },
      ],
      total_count: 1,
    })
    ;(agentService.deleteAgent as Mock).mockResolvedValue(undefined)
  })

  it('renders through the reading shell rather than a page header', async () => {
    renderAgentDetails()

    expect(
      await screen.findByRole('heading', { name: 'Code Reviewer', level: 1 })
    ).toBeInTheDocument()
    expect(screen.getByTestId('reading-page')).toBeInTheDocument()
    expect(screen.getByTestId('details-column')).toBeInTheDocument()
    // The page-level ghost "Back to agents" button is gone: Back is a reading
    // action now, so it is inside the details column with the others.
    expect(screen.queryByText('Back to agents')).not.toBeInTheDocument()
  })

  it.each([
    ['active', 'Active', 'bg-success'],
    ['paused', 'Paused', 'bg-muted'],
    ['error', 'Error', 'bg-destructive'],
  ] as const)(
    'badges %s in the reading header from the shared agent status field',
    async (status, label, toneClass) => {
      // The agents list and this page read one `FieldSpec` (#907/#918). A
      // hardcoded label or tone here would re-fork them — same agent, different
      // colour on /agents and /agents/:id — while the descriptor's own tests
      // stayed green, so the guard has to live on the page. Every status is
      // exercised: a hardcoded map that happens to agree on `active` still
      // diverges on the other two.
      ;(agentService.getAgent as Mock).mockResolvedValue(buildAgent({ status }))
      renderAgentDetails()

      const header = await screen.findByTestId('resource-header-meta')
      // Both halves of the FieldSpec contract at once: the wording and the fill.
      expect(within(header).getByText(label)).toHaveClass(toneClass)
      expect(within(header).getByText(/Updated/)).toBeInTheDocument()
    }
  )

  it('renders the descriptor-generated metadata rows in the details column', async () => {
    renderAgentDetails()

    const metadata = await screen.findByTestId('metadata-panel')
    expect(within(metadata).getByText('Status')).toBeInTheDocument()
    expect(within(metadata).getByText('ID')).toBeInTheDocument()
    expect(within(metadata).getByText('Card version')).toBeInTheDocument()
    expect(within(metadata).getByText('Last synced')).toBeInTheDocument()
  })

  it('puts access activity in the details column, not in the article', async () => {
    renderAgentDetails()

    const activity = await screen.findByRole('region', {
      name: 'Access activity',
    })
    expect(
      within(screen.getByTestId('details-column')).getByRole('region', {
        name: 'Access activity',
      })
    ).toBe(activity)
    expect(screen.getByTestId('reading-page')).not.toContainElement(activity)
  })

  it('puts run stats in the details column', async () => {
    renderAgentDetails()

    const stats = await screen.findByTestId('agent-stats-panel')
    expect(within(stats).getByText('Success rate')).toBeInTheDocument()
    expect(within(stats).getByText('92%')).toBeInTheDocument()
    expect(within(stats).getByText('Total runs')).toBeInTheDocument()
    expect(screen.getByTestId('reading-page')).not.toContainElement(stats)
  })

  it('keeps recent executions in the article, linking on to the tasks view', async () => {
    renderAgentDetails()

    const article = await screen.findByTestId('reading-page')
    expect(within(article).getByText('Review this diff')).toBeInTheDocument()
    expect(
      within(article).getByRole('button', { name: /View all tasks/ })
    ).toBeInTheDocument()
  })

  it('carries Back, Chat, Conversations and Edit as reading actions', async () => {
    renderAgentDetails()

    const details = await screen.findByTestId('details-column')
    for (const label of ['Back', 'Chat', 'Conversations', 'Edit']) {
      expect(within(details).getByRole('button', { name: label })).toBeVisible()
    }
  })

  it('navigates to the chat page from the Chat action', async () => {
    const user = userEvent.setup()
    renderAgentDetails()

    await user.click(await screen.findByRole('button', { name: 'Chat' }))

    expect(await screen.findByTestId('chat-probe')).toBeInTheDocument()
  })

  it('withholds Delete when the team grants no delete permission', async () => {
    renderAgentDetails()

    await screen.findByTestId('details-column')
    expect(
      screen.queryByRole('button', { name: 'Delete' })
    ).not.toBeInTheDocument()
  })

  it('offers Delete as an outlined destructive action and confirms first', async () => {
    setTeamPermissions(['resource.delete.any'])
    const user = userEvent.setup()
    renderAgentDetails()

    const deleteButton = await screen.findByTestId('delete-agent-button')
    // The reading rail's outlined treatment (#890), not the solid red button
    // this page used to render. Compare whole class tokens: the outlined
    // variant carries `hover:bg-destructive/10`, so a substring check for
    // `bg-destructive` passes either way and proves nothing.
    const classes = deleteButton.className.split(' ')
    expect(classes).toContain('border-destructive')
    expect(classes).toContain('text-destructive')
    expect(classes).not.toContain('bg-destructive')

    await user.click(deleteButton)
    expect(await screen.findByText('Delete agent?')).toBeInTheDocument()
    expect(agentService.deleteAgent).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => {
      expect(agentService.deleteAgent).toHaveBeenCalledWith('team-1', 'agent-1')
    })
  })

  it('renders the not-found branch in the same shell, with Back', async () => {
    ;(agentService.getAgent as Mock).mockRejectedValue(new Error('boom'))
    renderAgentDetails()

    expect(
      await screen.findByRole('heading', {
        name: 'Agent not found',
        level: 1,
      })
    ).toBeInTheDocument()
    expect(screen.getByTestId('reading-page')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Back' })).toBeInTheDocument()
  })
})

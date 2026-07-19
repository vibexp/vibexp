import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type {
  Agent,
  AgentListResponse,
  AgentStatsResponse,
} from '@/services/agentService'

// Mock Radix Select (AgentFilters status dropdown) — it can loop in JSDOM
// (same approach as PromptEditor.test.tsx), but keep onValueChange wired so
// tests can still pick a status option as a plain button.
jest.mock('@/components/ui/select', () => {
  const ReactActual = jest.requireActual<typeof import('react')>('react')
  const SelectCtx = ReactActual.createContext<(value: string) => void>(() => {})
  return {
    Select: ({
      children,
      onValueChange,
    }: {
      children: React.ReactNode
      value: string
      onValueChange: (v: string) => void
    }) => (
      <SelectCtx.Provider value={onValueChange}>
        <div data-testid="select">{children}</div>
      </SelectCtx.Provider>
    ),
    SelectTrigger: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="select-trigger">{children}</div>
    ),
    SelectValue: ({ placeholder }: { placeholder?: string }) => (
      <span>{placeholder}</span>
    ),
    SelectContent: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="select-content">{children}</div>
    ),
    SelectItem: ({
      children,
      value,
    }: {
      children: React.ReactNode
      value: string
    }) => {
      const onValueChange = ReactActual.useContext(SelectCtx)
      return (
        <button
          type="button"
          data-value={value}
          onClick={() => {
            onValueChange(value)
          }}
        >
          {children}
        </button>
      )
    },
  }
})

jest.mock('@/services/agentService', () => ({
  agentService: {
    getAgents: jest.fn(),
    getAgentStats: jest.fn(),
    deleteAgent: jest.fn(),
  },
}))

// usePermissions (#225) reads the signed-in user for own-vs-any delete gating.
jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

// Mutable so each test chooses the server-granted permissions array — the page
// gates delete on it via the real usePermissions hook (never mocked, #225).
const mockTeamState: {
  currentTeam: { id: string; name: string; permissions: string[] } | null
  isLoading: boolean
} = {
  currentTeam: { id: 'team-1', name: 'Test Team', permissions: [] },
  isLoading: false,
}
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: mockTeamState.currentTeam,
    teams: mockTeamState.currentTeam ? [mockTeamState.currentTeam] : [],
    isLoading: mockTeamState.isLoading,
    setCurrentTeam: jest.fn(),
    refreshTeams: jest.fn() as () => Promise<void>,
  }),
}))

jest.mock('@/hooks/useErrorHandler', () => {
  const handleError = jest.fn()
  return {
    useErrorHandler: () => ({ handleError }),
  }
})

jest.mock('@/lib/toast', () => ({
  toast: {
    success: jest.fn(),
    error: jest.fn(),
    info: jest.fn(),
    warning: jest.fn(),
    message: jest.fn(),
  },
}))

import React from 'react'

import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import { agentService } from '@/services/agentService'

import { Agents } from '../Agents'

function buildAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: 'agent-1',
    user_id: 'user-1',
    team_id: 'team-1',
    name: 'Code Review Agent',
    description: 'Reviews pull requests automatically',
    status: 'active',
    card_url: 'https://agent.example.com/.well-known/agent-card.json',
    config: null,
    total_runs: 1500,
    success_rate: 95,
    last_run: '2026-01-02T00:00:00Z',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    version: 1,
    ...overrides,
  }
}

function buildListResponse(agents: Agent[]): AgentListResponse {
  return {
    agents,
    total_count: agents.length,
    page: 1,
    per_page: 20,
    total_pages: agents.length > 0 ? 1 : 0,
  }
}

function buildStats(
  overrides: Partial<AgentStatsResponse> = {}
): AgentStatsResponse {
  return {
    total_agents: 7,
    active_agents: 4,
    paused_agents: 2,
    error_agents: 1,
    total_runs: 1234,
    avg_success_rate: 87.4,
    runs_today: 3,
    runs_this_week: 9,
    recent_activities: null,
    ...overrides,
  }
}

function setTeamPermissions(permissions: string[]) {
  mockTeamState.currentTeam = {
    id: 'team-1',
    name: 'Test Team',
    permissions,
  }
}

/**
 * The stat card for `title`. Titles like "Total runs" also appear as table
 * column headers, so match the StatCard's <p> title and scope to its card.
 */
function statCard(title: string): HTMLElement {
  const titleEl = screen.getAllByText(title).find(el => el.tagName === 'P')
  expect(titleEl).toBeDefined()
  return titleEl!.parentElement!
}

function renderAgents() {
  return render(
    <MemoryRouter initialEntries={['/agents']}>
      <Routes>
        <Route path="/agents" element={<Agents />} />
        <Route
          path="/agents/add"
          element={<div data-testid="editor-probe">Agent editor probe</div>}
        />
        <Route
          path="/agents/:id"
          element={<div data-testid="detail-probe">Agent detail probe</div>}
        />
        <Route
          path="/agents/:id/edit"
          element={<div data-testid="edit-probe">Agent edit probe</div>}
        />
        <Route
          path="/agents/:id/chat"
          element={<div data-testid="chat-probe">Agent chat probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('Agents page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    setTeamPermissions([])
    ;(agentService.getAgents as jest.Mock).mockResolvedValue(
      buildListResponse([])
    )
    ;(agentService.getAgentStats as jest.Mock).mockResolvedValue(buildStats())
    ;(agentService.deleteAgent as jest.Mock).mockResolvedValue(undefined)
  })

  describe('data states', () => {
    it('renders agent rows returned by the service', async () => {
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildAgent(),
          buildAgent({
            id: 'agent-2',
            name: 'Deploy Agent',
            description: 'Ships releases',
            status: 'paused',
            total_runs: 42,
            success_rate: 63,
          }),
        ])
      )

      renderAgents()

      await waitFor(() => {
        expect(screen.getByText('Code Review Agent')).toBeInTheDocument()
      })
      expect(
        screen.getByText('Reviews pull requests automatically')
      ).toBeInTheDocument()
      expect(screen.getByText('Deploy Agent')).toBeInTheDocument()
      // Status badges scoped to their rows — the (mocked) status filter also
      // renders "Active"/"Paused" option labels.
      const reviewRow = screen
        .getByText('Code Review Agent')
        .closest('tr') as HTMLElement
      expect(within(reviewRow).getByText('Active')).toBeInTheDocument()
      expect(within(reviewRow).getByText('1,500')).toBeInTheDocument()
      expect(within(reviewRow).getByText('95%')).toBeInTheDocument()
      const deployRow = screen
        .getByText('Deploy Agent')
        .closest('tr') as HTMLElement
      expect(within(deployRow).getByText('Paused')).toBeInTheDocument()
      expect(within(deployRow).getByText('63%')).toBeInTheDocument()
      expect(agentService.getAgents).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ page: 1, limit: 20 })
      )
    })

    it('shows skeleton rows while the fetch is in flight', () => {
      ;(agentService.getAgents as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderAgents()

      expect(
        screen.getAllByTestId('list-page-skeleton-row').length
      ).toBeGreaterThan(0)
    })

    it('shows the error state when the fetch fails', async () => {
      ;(agentService.getAgents as jest.Mock).mockRejectedValue(
        new Error('network down')
      )

      renderAgents()

      await waitFor(() => {
        expect(screen.getByText('Failed to load agents')).toBeInTheDocument()
      })
      expect(screen.getByText('network down')).toBeInTheDocument()
      const { handleError } = useErrorHandler()
      expect(handleError).toHaveBeenCalled()
      // Stats are hidden on the error path.
      expect(screen.queryByText('Total agents')).not.toBeInTheDocument()
    })

    it('shows the empty state when there are no agents', async () => {
      renderAgents()

      await waitFor(() => {
        expect(screen.getByText('No agents yet')).toBeInTheDocument()
      })
      expect(
        screen.getByText('Create your first agent to start automating tasks.')
      ).toBeInTheDocument()
    })
  })

  describe('stats', () => {
    it('renders the stat cards from getAgentStats once agents are loaded', async () => {
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([buildAgent()])
      )

      renderAgents()

      expect(await screen.findByText('Total agents')).toBeInTheDocument()
      expect(agentService.getAgentStats).toHaveBeenCalledWith('team-1')
      expect(
        within(statCard('Total agents')).getByText('7')
      ).toBeInTheDocument()
      expect(
        within(statCard('Active agents')).getByText('4')
      ).toBeInTheDocument()
      expect(
        within(statCard('Total runs')).getByText('1,234')
      ).toBeInTheDocument()
      expect(
        within(statCard('Success rate')).getByText('87%')
      ).toBeInTheDocument()
    })

    it('falls back to stats computed from the loaded agents when the stats call fails', async () => {
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildAgent({ total_runs: 10, success_rate: 90, last_run: null }),
          buildAgent({
            id: 'agent-2',
            name: 'Deploy Agent',
            status: 'paused',
            total_runs: 20,
            success_rate: 70,
            last_run: null,
          }),
        ])
      )
      ;(agentService.getAgentStats as jest.Mock).mockRejectedValue(
        new Error('stats unavailable')
      )

      renderAgents()

      expect(await screen.findByText('Total agents')).toBeInTheDocument()
      // total runs 10 + 20, average success rate (90 + 70) / 2.
      expect(within(statCard('Total runs')).getByText('30')).toBeInTheDocument()
      expect(
        within(statCard('Success rate')).getByText('80%')
      ).toBeInTheDocument()
    })
  })

  describe('search filter', () => {
    it('re-fetches with the debounced search term', async () => {
      renderAgents()

      await waitFor(() => {
        expect(agentService.getAgents).toHaveBeenCalled()
      })

      const user = userEvent.setup()
      await user.type(
        screen.getByPlaceholderText('Search agents by name or description…'),
        'deploy'
      )

      await waitFor(
        () => {
          expect(agentService.getAgents).toHaveBeenCalledWith(
            'team-1',
            expect.objectContaining({ search: 'deploy', page: 1 })
          )
        },
        { timeout: 2000 }
      )
    })
  })

  describe('status filter', () => {
    it('re-fetches with the picked status and clears it on All statuses', async () => {
      renderAgents()

      await waitFor(() => {
        expect(agentService.getAgents).toHaveBeenCalled()
      })

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Paused' }))

      await waitFor(() => {
        expect(agentService.getAgents).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ status: 'paused', page: 1 })
        )
      })

      await user.click(screen.getByRole('button', { name: 'All statuses' }))

      await waitFor(() => {
        expect(agentService.getAgents).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ status: undefined, page: 1 })
        )
      })
    })
  })

  describe('pagination', () => {
    it('fetches the next page from the footer controls', async () => {
      ;(agentService.getAgents as jest.Mock).mockResolvedValue({
        ...buildListResponse([buildAgent()]),
        total_count: 25,
        total_pages: 2,
      })

      renderAgents()
      await screen.findByText('Code Review Agent')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Next' }))

      await waitFor(() => {
        expect(agentService.getAgents).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ page: 2 })
        )
      })
    })
  })

  describe('navigation', () => {
    it('navigates to the editor from the Add agent button', async () => {
      renderAgents()
      await screen.findByText('No agents yet')

      const user = userEvent.setup()
      const [headerButton] = screen.getAllByRole('button', {
        name: /Add agent/,
      })
      await user.click(headerButton)

      expect(screen.getByTestId('editor-probe')).toBeInTheDocument()
    })

    it('navigates to the editor from the empty-state Add agent button', async () => {
      renderAgents()
      await screen.findByText('No agents yet')

      const user = userEvent.setup()
      const addButtons = screen.getAllByRole('button', { name: /Add agent/ })
      expect(addButtons).toHaveLength(2)
      // The second one lives in the empty state.
      await user.click(addButtons[1])

      expect(screen.getByTestId('editor-probe')).toBeInTheDocument()
    })

    it('navigates to the agent detail when a row is clicked', async () => {
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([buildAgent()])
      )

      renderAgents()

      const user = userEvent.setup()
      await user.click(await screen.findByText('Code Review Agent'))

      expect(screen.getByTestId('detail-probe')).toBeInTheDocument()
    })

    it('navigates to edit from the row edit action', async () => {
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([buildAgent()])
      )

      renderAgents()

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Edit Code Review Agent'))

      expect(screen.getByTestId('edit-probe')).toBeInTheDocument()
    })

    it('navigates to chat from the row chat action', async () => {
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([buildAgent()])
      )

      renderAgents()

      const user = userEvent.setup()
      await user.click(
        await screen.findByLabelText('Chat with Code Review Agent')
      )

      expect(screen.getByTestId('chat-probe')).toBeInTheDocument()
    })
  })

  describe('delete gating via the server permissions array (#225)', () => {
    it('shows the delete action on any row when the team grants resource.delete.any', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildAgent({ name: 'Their Agent', user_id: 'user-2' }),
        ])
      )

      renderAgents()

      await screen.findByText('Their Agent')
      expect(screen.getByLabelText('Delete Their Agent')).toBeInTheDocument()
    })

    it('hides the delete action when the team grants no delete permission', async () => {
      setTeamPermissions([])
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildAgent({ name: 'Their Agent', user_id: 'user-2' }),
        ])
      )

      renderAgents()

      await screen.findByText('Their Agent')
      expect(
        screen.queryByLabelText('Delete Their Agent')
      ).not.toBeInTheDocument()
      // Non-gated row actions are still there — the row rendered fully.
      expect(screen.getByLabelText('Edit Their Agent')).toBeInTheDocument()
      expect(screen.getByLabelText('Chat with Their Agent')).toBeInTheDocument()
    })

    it('with only resource.delete.own, shows delete on own rows but not on others', async () => {
      setTeamPermissions(['resource.delete.own'])
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildAgent({ id: 'mine', name: 'My Agent', user_id: 'user-1' }),
          buildAgent({ id: 'theirs', name: 'Their Agent', user_id: 'user-2' }),
        ])
      )

      renderAgents()

      await screen.findByText('My Agent')
      // Exactly one delete button: the row owned by the signed-in user.
      expect(screen.getAllByLabelText(/^Delete /)).toHaveLength(1)
      const myRow = screen.getByText('My Agent').closest('tr')
      expect(myRow).not.toBeNull()
      expect(
        within(myRow as HTMLElement).getByLabelText('Delete My Agent')
      ).toBeInTheDocument()
      const theirRow = screen.getByText('Their Agent').closest('tr')
      expect(
        within(theirRow as HTMLElement).queryByLabelText('Delete Their Agent')
      ).not.toBeInTheDocument()
    })
  })

  describe('delete flow', () => {
    it('confirms and deletes via the service, then re-fetches and toasts', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([buildAgent()])
      )

      renderAgents()

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Delete Code Review Agent'))

      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText('Delete agent?')).toBeInTheDocument()
      const fetchCallsBefore = (agentService.getAgents as jest.Mock).mock.calls
        .length
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      await waitFor(() => {
        expect(agentService.deleteAgent).toHaveBeenCalledWith(
          'team-1',
          'agent-1'
        )
      })
      await waitFor(() => {
        expect(
          (agentService.getAgents as jest.Mock).mock.calls.length
        ).toBeGreaterThan(fetchCallsBefore)
      })
      expect(toast.success).toHaveBeenCalledWith('Agent deleted successfully')
    })

    it('cancelling the dialog closes it without deleting', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([buildAgent()])
      )

      renderAgents()

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Delete Code Review Agent'))
      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

      await waitFor(() => {
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      })
      expect(agentService.deleteAgent).not.toHaveBeenCalled()
    })

    it('keeps the page usable and reports the error when the delete fails', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(agentService.getAgents as jest.Mock).mockResolvedValue(
        buildListResponse([buildAgent()])
      )
      ;(agentService.deleteAgent as jest.Mock).mockRejectedValue(
        new Error('delete forbidden')
      )

      renderAgents()

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Delete Code Review Agent'))
      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      const { handleError } = useErrorHandler()
      await waitFor(() => {
        expect(handleError).toHaveBeenCalledWith(
          expect.any(Error),
          'Failed to delete agent'
        )
      })
      expect(toast.success).not.toHaveBeenCalled()
    })
  })
})

import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type {
  Agent,
  AgentExecution,
  AgentExecutionListResponse,
} from '@/services/agentService'

// Mock Radix Select (ExecutionFilters status dropdown) — it can loop in JSDOM.
// onValueChange stays wired so tests can pick a status as a plain button.
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
    getAgent: jest.fn(),
    listAgentExecutions: jest.fn(),
  },
}))

jest.mock('@/contexts/TeamContext', () => {
  const currentTeam = { id: 'team-1', name: 'Test Team' }
  return {
    useTeam: () => ({
      currentTeam,
      teams: [currentTeam],
      isLoading: false,
      setCurrentTeam: jest.fn(),
      refreshTeams: jest.fn() as () => Promise<void>,
    }),
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

import { toast } from '@/lib/toast'
import { agentService } from '@/services/agentService'

import { AgentTasks } from '../AgentTasks'

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

function buildExecution(
  overrides: Partial<AgentExecution> = {}
): AgentExecution {
  return {
    id: 'exec-1',
    agent_id: 'agent-1',
    user_id: 'user-1',
    status: 'success',
    input: { text: 'Review PR 42' },
    started_at: '2026-01-02T00:00:00Z',
    ended_at: '2026-01-02T00:00:05Z',
    duration: 2000,
    version: 1,
    ...overrides,
  }
}

function buildListResponse(
  executions: AgentExecution[],
  overrides: Partial<AgentExecutionListResponse> = {}
): AgentExecutionListResponse {
  return {
    executions,
    total_count: executions.length,
    page: 1,
    per_page: 20,
    total_pages: executions.length > 0 ? 1 : 0,
    ...overrides,
  }
}

function statCard(title: string): HTMLElement {
  const titleEl = screen.getByText(title)
  const card = titleEl.closest('[data-slot="card"], div.rounded-xl')
  // Card markup can change; fall back to walking up two levels.
  return (card ?? titleEl.parentElement?.parentElement) as HTMLElement
}

function renderAgentTasks() {
  return render(
    <MemoryRouter initialEntries={['/agents/agent-1/tasks']}>
      <Routes>
        <Route path="/agents/:id/tasks" element={<AgentTasks />} />
        <Route
          path="/agents"
          element={<div data-testid="agents-probe">Agents list probe</div>}
        />
        <Route
          path="/agents/:id"
          element={<div data-testid="detail-probe">Agent detail probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('AgentTasks page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    ;(agentService.getAgent as jest.Mock).mockResolvedValue(buildAgent())
    ;(agentService.listAgentExecutions as jest.Mock).mockResolvedValue(
      buildListResponse([])
    )
  })

  describe('agent loading states', () => {
    it('shows the loading header while the agent is being fetched', () => {
      ;(agentService.getAgent as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )
      // Keep the executions fetch pending too, so it cannot resolve after the
      // test finished (act warning).
      ;(agentService.listAgentExecutions as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderAgentTasks()

      expect(screen.getByText('Loading agent…')).toBeInTheDocument()
    })

    it('shows the not-found alert when the agent fails to load, with a way back', async () => {
      ;(agentService.getAgent as jest.Mock).mockRejectedValue(
        new Error('agent gone')
      )

      renderAgentTasks()

      expect(await screen.findByText('Agent not found')).toBeInTheDocument()
      expect(
        screen.getByText(
          'The agent you are looking for does not exist or has been removed.'
        )
      ).toBeInTheDocument()
      expect(toast.error).toHaveBeenCalledWith('agent gone')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /Back to agents/ }))
      expect(screen.getByTestId('agents-probe')).toBeInTheDocument()
    })
  })

  describe('executions rendering', () => {
    it('renders the header, stats computed from the executions, and the table rows', async () => {
      ;(agentService.listAgentExecutions as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildExecution(),
          buildExecution({
            id: 'exec-2',
            status: 'error',
            input: { text: 'Deploy the release' },
            duration: 4000,
          }),
          buildExecution({
            id: 'exec-3',
            status: 'running',
            input: { text: 'Summarize the changelog' },
            duration: null,
            ended_at: null,
          }),
        ])
      )

      renderAgentTasks()

      expect(
        await screen.findByText('Tasks: Code Review Agent')
      ).toBeInTheDocument()
      expect(agentService.listAgentExecutions).toHaveBeenCalledWith(
        'team-1',
        'agent-1',
        { status: undefined, page: 1, limit: 20 }
      )

      // Stats: 3 total, 1 success, 1 error; average of the two finished
      // durations (2000 + 4000) / 2 / 1000 = 3.00s.
      expect(
        within(statCard('Total executions')).getByText('3')
      ).toBeInTheDocument()
      expect(within(statCard('Successful')).getByText('1')).toBeInTheDocument()
      expect(within(statCard('Failed')).getByText('1')).toBeInTheDocument()
      expect(
        within(statCard('Avg duration')).getByText('3.00s')
      ).toBeInTheDocument()

      // Table rows with their input text, status badge, and duration.
      const successRow = screen
        .getByText('Review PR 42')
        .closest('tr') as HTMLElement
      expect(within(successRow).getByText('success')).toBeInTheDocument()
      expect(within(successRow).getByText('2.0s')).toBeInTheDocument()
      const runningRow = screen
        .getByText('Summarize the changelog')
        .closest('tr') as HTMLElement
      expect(within(runningRow).getByText('running')).toBeInTheDocument()
      expect(within(runningRow).getByText('—')).toBeInTheDocument()
      expect(screen.getByText('Deploy the release')).toBeInTheDocument()
    })

    it('reports a zero average duration when no execution has finished', async () => {
      ;(agentService.listAgentExecutions as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildExecution({ status: 'running', duration: null, ended_at: null }),
        ])
      )

      renderAgentTasks()

      expect(await screen.findByText('Tasks: Code Review Agent')).toBeVisible()
      expect(
        within(statCard('Avg duration')).getByText('0.00s')
      ).toBeInTheDocument()
    })

    it('shows the unfiltered empty state when the agent has no executions', async () => {
      renderAgentTasks()

      expect(await screen.findByText('No executions found')).toBeInTheDocument()
      expect(
        screen.getByText('No tasks have been executed for this agent yet.')
      ).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows the error alert with a working retry', async () => {
      ;(agentService.listAgentExecutions as jest.Mock).mockRejectedValueOnce(
        new Error('executions down')
      )

      renderAgentTasks()

      expect(
        await screen.findByText('Failed to load executions')
      ).toBeInTheDocument()
      expect(screen.getByText('executions down')).toBeInTheDocument()
      expect(toast.error).toHaveBeenCalledWith('executions down')

      // Retry succeeds (the rejection was once-only).
      ;(agentService.listAgentExecutions as jest.Mock).mockResolvedValue(
        buildListResponse([buildExecution()])
      )
      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Retry' }))

      expect(await screen.findByText('Review PR 42')).toBeInTheDocument()
      expect(
        screen.queryByText('Failed to load executions')
      ).not.toBeInTheDocument()
    })
  })

  describe('status filter', () => {
    it('re-fetches with the picked status, shows the filtered empty state, and clears on All statuses', async () => {
      renderAgentTasks()
      await screen.findByText('No executions found')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Error' }))

      await waitFor(() => {
        expect(agentService.listAgentExecutions).toHaveBeenCalledWith(
          'team-1',
          'agent-1',
          { status: 'error', page: 1, limit: 20 }
        )
      })
      expect(
        await screen.findByText('Try adjusting your filters.')
      ).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'All statuses' }))

      await waitFor(() => {
        expect(agentService.listAgentExecutions).toHaveBeenCalledWith(
          'team-1',
          'agent-1',
          { status: undefined, page: 1, limit: 20 }
        )
      })
    })
  })

  describe('pagination', () => {
    it('fetches the next page from the table controls', async () => {
      ;(agentService.listAgentExecutions as jest.Mock).mockResolvedValue(
        buildListResponse([buildExecution()], {
          total_count: 25,
          total_pages: 2,
        })
      )

      renderAgentTasks()
      await screen.findByText('Review PR 42')
      expect(screen.getByText('Page 1 of 2')).toBeInTheDocument()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Next' }))

      await waitFor(() => {
        expect(agentService.listAgentExecutions).toHaveBeenCalledWith(
          'team-1',
          'agent-1',
          { status: undefined, page: 2, limit: 20 }
        )
      })
    })
  })

  describe('navigation', () => {
    it('navigates back to the agent detail', async () => {
      renderAgentTasks()
      await screen.findByText('Tasks: Code Review Agent')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /Back to agent/ }))

      expect(screen.getByTestId('detail-probe')).toBeInTheDocument()
    })
  })
})

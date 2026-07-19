import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'

import type {
  Agent,
  ConversationListResponse,
  ConversationSummary,
} from '@/services/agentService'

// The shared lucide mock (tests/mocks/lucide-react.tsx) lists icons explicitly
// and misses some used here — serve any icon via a Proxy instead (same trick
// as PromptEditor.test.tsx).
jest.mock(
  'lucide-react',
  () =>
    new Proxy(
      {},
      {
        get: (_target, name) => {
          if (name === '__esModule') return true
          const Icon = (props: object) => (
            <svg
              data-testid={`${String(name).toLowerCase()}-icon`}
              {...props}
            />
          )
          return Icon
        },
      }
    )
)

// Pagination buttons call window.scrollTo, which jsdom does not implement.
beforeAll(() => {
  window.scrollTo = jest.fn()
})

jest.mock('@/services/agentService', () => ({
  agentService: {
    getAgent: jest.fn(),
    listAgentConversations: jest.fn(),
  },
}))

jest.mock('@/lib/toast', () => ({
  toast: { error: jest.fn(), success: jest.fn() },
}))

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

import { toast } from '@/lib/toast'
import { agentService } from '@/services/agentService'

import { AgentConversations } from '../AgentConversations'

function buildAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: 'agent-1',
    user_id: 'user-1',
    team_id: 'team-1',
    name: 'Support Bot',
    description: 'Answers support questions',
    status: 'active',
    agent_card: null,
    config: null,
    total_runs: 3,
    success_rate: 0.9,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    version: 1,
    ...overrides,
  }
}

function buildConversation(
  overrides: Partial<ConversationSummary> = {}
): ConversationSummary {
  return {
    conversation_id: 'conv-1',
    agent_id: 'agent-1',
    message_count: 3,
    first_message: 'How do I reset my password?',
    last_message: 'You can reset it from the settings page.',
    started_at: '2026-07-10T10:00:00Z',
    last_activity_at: '2026-07-11T10:00:00Z',
    last_status: 'completed',
    ...overrides,
  }
}

function buildListResponse(
  conversations: ConversationSummary[],
  overrides: Partial<ConversationListResponse> = {}
): ConversationListResponse {
  return {
    conversations,
    total_count: conversations.length,
    page: 1,
    per_page: 20,
    total_pages: conversations.length > 0 ? 1 : 0,
    ...overrides,
  }
}

// Probe that also exposes the query string, so tests can assert which
// conversation a "Resume" navigation carries.
function ChatProbe() {
  const location = useLocation()
  return <div data-testid="chat-probe">{location.search}</div>
}

function renderConversations() {
  return render(
    <MemoryRouter initialEntries={['/agents/agent-1/conversations']}>
      <Routes>
        <Route path="/agents" element={<div data-testid="agents-probe" />} />
        <Route
          path="/agents/:id"
          element={<div data-testid="agent-detail-probe" />}
        />
        <Route path="/agents/:id/chat" element={<ChatProbe />} />
        <Route
          path="/agents/:id/conversations"
          element={<AgentConversations />}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('AgentConversations page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockTeamState.currentTeam = {
      id: 'team-1',
      name: 'Test Team',
      permissions: [],
    }
    ;(agentService.getAgent as jest.Mock).mockResolvedValue(buildAgent())
    ;(agentService.listAgentConversations as jest.Mock).mockResolvedValue(
      buildListResponse([])
    )
  })

  describe('agent loading', () => {
    it('shows the loading skeleton while getAgent is in flight', () => {
      ;(agentService.getAgent as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderConversations()

      expect(screen.getByText('Loading…')).toBeInTheDocument()
    })

    it('shows the not-found alert when getAgent fails, with a way back', async () => {
      ;(agentService.getAgent as jest.Mock).mockRejectedValue(
        new Error('agent exploded')
      )

      renderConversations()

      expect(await screen.findByText('Agent not found')).toBeInTheDocument()
      expect(toast.error).toHaveBeenCalledWith('agent exploded')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /Back to agents/ }))
      expect(screen.getByTestId('agents-probe')).toBeInTheDocument()
    })

    it('redirects to the agents list when the URL carries no agent id', async () => {
      render(
        <MemoryRouter initialEntries={['/conversations']}>
          <Routes>
            <Route
              path="/agents"
              element={<div data-testid="agents-probe" />}
            />
            <Route path="/conversations" element={<AgentConversations />} />
          </Routes>
        </MemoryRouter>
      )

      expect(await screen.findByTestId('agents-probe')).toBeInTheDocument()
      expect(agentService.getAgent).not.toHaveBeenCalled()
    })

    it('navigates to the agent detail from the back button', async () => {
      renderConversations()
      await screen.findByText('Support Bot — Conversations')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /Back to agent$/ }))
      expect(screen.getByTestId('agent-detail-probe')).toBeInTheDocument()
    })
  })

  describe('conversation list', () => {
    it('renders the conversations returned by the service', async () => {
      ;(agentService.listAgentConversations as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildConversation(),
          buildConversation({
            conversation_id: 'conv-2',
            message_count: 1,
            first_message: 'Second conversation opener',
            last_message: '',
            last_status: 'failed',
          }),
        ])
      )

      renderConversations()

      expect(
        await screen.findByText('How do I reset my password?')
      ).toBeInTheDocument()
      expect(screen.getByText('Second conversation opener')).toBeInTheDocument()
      expect(screen.getByText('Total conversations: 2')).toBeInTheDocument()
      // Pluralization of the message count.
      expect(screen.getByText('3 messages')).toBeInTheDocument()
      expect(screen.getByText('1 message')).toBeInTheDocument()
      // Status badges.
      expect(screen.getByText('completed')).toBeInTheDocument()
      expect(screen.getByText('failed')).toBeInTheDocument()
      // Last message preview only where one exists.
      expect(
        screen.getByText(/You can reset it from the settings page\./)
      ).toBeInTheDocument()
      expect(screen.getAllByText(/Last message:/)).toHaveLength(1)
      expect(agentService.listAgentConversations).toHaveBeenCalledWith(
        'team-1',
        'agent-1',
        { page: 1, limit: 20 }
      )
    })

    it('falls back to an untitled label when the first message is empty', async () => {
      ;(agentService.listAgentConversations as jest.Mock).mockResolvedValue(
        buildListResponse([buildConversation({ first_message: '' })])
      )

      renderConversations()

      expect(
        await screen.findByText('Untitled conversation')
      ).toBeInTheDocument()
    })

    it('truncates a long first message', async () => {
      ;(agentService.listAgentConversations as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildConversation({ first_message: 'x'.repeat(150) }),
        ])
      )

      renderConversations()

      expect(await screen.findByText(`${'x'.repeat(100)}…`)).toBeInTheDocument()
    })

    it('renders in-progress and unknown statuses as badges too', async () => {
      ;(agentService.listAgentConversations as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildConversation({
            conversation_id: 'conv-running',
            first_message: 'Still going',
            last_status: 'running',
          }),
          buildConversation({
            conversation_id: 'conv-odd',
            first_message: 'Strange one',
            last_status: 'archived',
          }),
        ])
      )

      renderConversations()

      expect(await screen.findByText('running')).toBeInTheDocument()
      expect(screen.getByText('archived')).toBeInTheDocument()
    })

    it('shows the empty state and starts a new conversation from it', async () => {
      renderConversations()

      expect(
        await screen.findByText('No conversations yet')
      ).toBeInTheDocument()
      expect(
        screen.getByText(
          'Start a conversation with Support Bot to see it here.'
        )
      ).toBeInTheDocument()

      const user = userEvent.setup()
      await user.click(
        screen.getByRole('button', { name: /Start new conversation/ })
      )
      expect(screen.getByTestId('chat-probe')).toBeInTheDocument()
      expect(screen.getByTestId('chat-probe')).toHaveTextContent('')
    })

    it('shows the error alert with a retry that re-fetches', async () => {
      ;(agentService.listAgentConversations as jest.Mock)
        .mockRejectedValueOnce(new Error('server down'))
        .mockResolvedValue(buildListResponse([buildConversation()]))

      renderConversations()

      expect(
        await screen.findByText('Failed to load conversations')
      ).toBeInTheDocument()
      expect(screen.getByText('server down')).toBeInTheDocument()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Retry' }))

      expect(
        await screen.findByText('How do I reset my password?')
      ).toBeInTheDocument()
      expect(
        screen.queryByText('Failed to load conversations')
      ).not.toBeInTheDocument()
      expect(agentService.listAgentConversations).toHaveBeenCalledTimes(2)
    })
  })

  describe('navigation to chat', () => {
    it('resumes a conversation with its id in the query string', async () => {
      ;(agentService.listAgentConversations as jest.Mock).mockResolvedValue(
        buildListResponse([buildConversation({ conversation_id: 'conv-42' })])
      )

      renderConversations()
      await screen.findByText('How do I reset my password?')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /Resume/ }))

      expect(screen.getByTestId('chat-probe')).toHaveTextContent(
        '?conversation=conv-42'
      )
    })

    it('opens a fresh chat from the header action', async () => {
      renderConversations()
      await screen.findByText('Support Bot — Conversations')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /New chat/ }))

      const probe = screen.getByTestId('chat-probe')
      expect(probe).toBeInTheDocument()
      expect(probe.textContent).toBe('')
    })
  })

  describe('pagination', () => {
    it('pages through results and disables the boundary buttons', async () => {
      ;(agentService.listAgentConversations as jest.Mock).mockImplementation(
        (
          _teamId: string,
          _agentId: string,
          options: { page: number; limit: number }
        ) =>
          Promise.resolve(
            buildListResponse(
              [
                buildConversation({
                  conversation_id: `conv-p${options.page}`,
                  first_message: `Page ${options.page} conversation`,
                }),
              ],
              { total_count: 50, page: options.page, total_pages: 3 }
            )
          )
      )

      renderConversations()

      expect(await screen.findByText('Page 1 conversation')).toBeInTheDocument()
      expect(screen.getByText('Page 1 of 3')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Next' }))

      expect(await screen.findByText('Page 2 conversation')).toBeInTheDocument()
      expect(screen.getByText('Page 2 of 3')).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: 'Previous' })
      ).not.toBeDisabled()
      await waitFor(() => {
        expect(agentService.listAgentConversations).toHaveBeenCalledWith(
          'team-1',
          'agent-1',
          { page: 2, limit: 20 }
        )
      })
      expect(window.scrollTo).toHaveBeenCalled()

      await user.click(screen.getByRole('button', { name: 'Previous' }))

      expect(await screen.findByText('Page 1 conversation')).toBeInTheDocument()
      expect(screen.getByText('Page 1 of 3')).toBeInTheDocument()
    })
  })
})

import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Agent } from '@/services/agentService'

import type { ExecutionMetadata, Message } from '../chat/types'

// The shared lucide mock (tests/mocks/lucide-react.tsx) lists icons explicitly
// and misses some used by the chat components — serve any icon via a Proxy
// instead (same trick as PromptEditor.test.tsx).
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

// MessageList scrolls the transcript into view on every render; jsdom has no
// scrollIntoView implementation.
beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn()
})

jest.mock('@/services/agentService', () => ({
  agentService: {
    getAgent: jest.fn(),
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

// Controllable useChatMessages mock. The hook's own behavior is covered by
// agents/chat/__tests__/useChatMessages.test.tsx — here the page is exercised
// against a hand-set hook state, so tests stay focused on page behavior. Each
// test writes `mockChatHook.overrides` (before render, or inside a mocked
// action such as sendMessage — any state update in the page re-invokes the
// hook and picks the overrides up).
interface ChatHookResult {
  messages: Message[]
  isExecuting: boolean
  executionMetadata: ExecutionMetadata | null
  setExecutionMetadata: (metadata: ExecutionMetadata | null) => void
  currentState: string | null
  currentExecutionId: string | null
  hasEarlierMessages: boolean
  isLoadingEarlier: boolean
  totalMessageCount: number
  loadConversation: (conversationId: string, limit?: number) => Promise<void>
  loadEarlierMessages: () => Promise<void>
  sendMessage: (text: string) => Promise<void>
  cancelExecution: () => Promise<void>
  reset: () => void
}

const mockSendMessage = jest.fn()
const mockCancelExecution = jest.fn()
const mockLoadConversation = jest.fn()
const mockLoadEarlierMessages = jest.fn()
const mockReset = jest.fn()
const mockSetExecutionMetadata = jest.fn()
const mockCaptureHookArgs = jest.fn()
const mockChatHook: { overrides: Partial<ChatHookResult> } = { overrides: {} }

jest.mock('../chat/useChatMessages', () => ({
  useChatMessages: (args: unknown): ChatHookResult => {
    mockCaptureHookArgs(args)
    return {
      messages: [],
      isExecuting: false,
      executionMetadata: null,
      setExecutionMetadata: mockSetExecutionMetadata,
      currentState: null,
      currentExecutionId: null,
      hasEarlierMessages: false,
      isLoadingEarlier: false,
      totalMessageCount: 0,
      loadConversation: mockLoadConversation,
      loadEarlierMessages: mockLoadEarlierMessages,
      sendMessage: mockSendMessage,
      cancelExecution: mockCancelExecution,
      reset: mockReset,
      ...mockChatHook.overrides,
    }
  },
}))

import { toast } from '@/lib/toast'
import { agentService } from '@/services/agentService'

import { AgentChat } from '../AgentChat'
import { PLACEHOLDER_TEXT, STREAMING } from '../chat/types'

interface CapturedHookArgs {
  teamId: string | undefined
  agent: Agent | null
  conversationId: string | null
  onConversationCaptured: (conversationId: string) => void
}

function lastHookArgs(): CapturedHookArgs {
  const calls = mockCaptureHookArgs.mock.calls
  return calls[calls.length - 1][0] as CapturedHookArgs
}

function buildAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: 'agent-1',
    user_id: 'user-1',
    team_id: 'team-1',
    name: 'Support Bot',
    description: 'Answers support questions',
    status: 'active',
    // Spec-nullable field: the A2A card may be absent — the chat page (and its
    // message bubbles' icon fallback) must render safely with null.
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

const userMessage: Message = {
  role: 'user',
  text: 'Hello agent',
  timestamp: '2026-01-03T10:00:00Z',
}
const agentMessage: Message = {
  role: 'agent',
  text: 'Hi, how can I help?',
  timestamp: '2026-01-03T10:00:05Z',
}

function renderChat(initialEntry = '/agents/agent-1/chat') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/agents" element={<div data-testid="agents-probe" />} />
        <Route
          path="/agents/:id"
          element={<div data-testid="agent-detail-probe" />}
        />
        <Route path="/agents/:id/chat" element={<AgentChat />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('AgentChat page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockChatHook.overrides = {}
    mockTeamState.currentTeam = {
      id: 'team-1',
      name: 'Test Team',
      permissions: [],
    }
    ;(agentService.getAgent as jest.Mock).mockResolvedValue(buildAgent())
  })

  describe('agent loading', () => {
    it('shows the loading skeleton while getAgent is in flight', () => {
      ;(agentService.getAgent as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderChat()

      expect(screen.getByText('Loading agent…')).toBeInTheDocument()
    })

    it('renders the agent header and empty transcript from getAgent', async () => {
      renderChat()

      expect(
        await screen.findByText('Chat with Support Bot')
      ).toBeInTheDocument()
      expect(screen.getByText('Answers support questions')).toBeInTheDocument()
      // MessageList empty state (no messages, no conversation param).
      expect(screen.getByText('Start a conversation')).toBeInTheDocument()
      expect(
        screen.getByText('Send a message to Support Bot to begin')
      ).toBeInTheDocument()
      expect(agentService.getAgent).toHaveBeenCalledWith('team-1', 'agent-1')
      // The hook is wired with the loaded agent + team.
      await waitFor(() => {
        expect(lastHookArgs().agent).toMatchObject({ id: 'agent-1' })
      })
      expect(lastHookArgs().teamId).toBe('team-1')
      expect(lastHookArgs().conversationId).toBeNull()
    })

    it('shows the error alert and toasts when getAgent fails, with a way back', async () => {
      ;(agentService.getAgent as jest.Mock).mockRejectedValue(
        new Error('agent exploded')
      )

      renderChat()

      expect(await screen.findByText('Agent not found')).toBeInTheDocument()
      expect(screen.getByText('agent exploded')).toBeInTheDocument()
      expect(toast.error).toHaveBeenCalledWith('agent exploded')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /Back to agents/ }))
      expect(screen.getByTestId('agents-probe')).toBeInTheDocument()
    })

    it('redirects to the agents list when the URL carries no agent id', async () => {
      render(
        <MemoryRouter initialEntries={['/chat']}>
          <Routes>
            <Route
              path="/agents"
              element={<div data-testid="agents-probe" />}
            />
            <Route path="/chat" element={<AgentChat />} />
          </Routes>
        </MemoryRouter>
      )

      expect(await screen.findByTestId('agents-probe')).toBeInTheDocument()
      expect(agentService.getAgent).not.toHaveBeenCalled()
    })

    it('navigates back to the agent detail from the header action', async () => {
      renderChat()
      await screen.findByText('Chat with Support Bot')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /Back to agent$/ }))
      expect(screen.getByTestId('agent-detail-probe')).toBeInTheDocument()
    })
  })

  describe('message send flow', () => {
    it('sends the typed message and the transcript reflects it', async () => {
      // Mirror the real hook: sending appends the user message plus the
      // streaming placeholder and flips isExecuting.
      mockSendMessage.mockImplementation((text: string) => {
        mockChatHook.overrides = {
          messages: [
            { role: 'user', text, timestamp: '2026-01-03T10:00:00Z' },
            { role: 'agent', text: PLACEHOLDER_TEXT, timestamp: STREAMING },
          ],
          isExecuting: true,
        }
        return Promise.resolve()
      })

      renderChat()
      await screen.findByText('Chat with Support Bot')

      const user = userEvent.setup()
      const input = screen.getByPlaceholderText(/Type your message/)
      await user.type(input, 'Hello agent')
      await user.click(screen.getByRole('button', { name: 'Send message' }))

      expect(mockSendMessage).toHaveBeenCalledWith('Hello agent')
      // The user's message is now in the transcript, with the streaming
      // placeholder bubble behind it.
      expect(await screen.findByText('Hello agent')).toBeInTheDocument()
      expect(screen.getByText(PLACEHOLDER_TEXT)).toBeInTheDocument()
      expect(screen.getByText('Streaming response…')).toBeInTheDocument()
      // Input cleared and disabled while executing.
      expect(input).toHaveValue('')
      expect(input).toBeDisabled()
    })

    it('does not enable send for whitespace-only input', async () => {
      renderChat()
      await screen.findByText('Chat with Support Bot')

      const user = userEvent.setup()
      await user.type(screen.getByPlaceholderText(/Type your message/), '   ')

      expect(
        screen.getByRole('button', { name: 'Send message' })
      ).toBeDisabled()
    })
  })

  describe('execution progress states', () => {
    it('renders the in-progress state with progress text and a cancel control', async () => {
      mockChatHook.overrides = {
        messages: [
          userMessage,
          {
            role: 'agent',
            text: 'Partial answer…',
            timestamp: STREAMING,
            artifactId: 'a1',
          },
        ],
        isExecuting: true,
        currentState: 'working',
        currentExecutionId: 'exec-1',
      }

      renderChat()
      await screen.findByText('Chat with Support Bot')

      expect(screen.getByText('Agent is working…')).toBeInTheDocument()
      expect(screen.getByText('Partial answer…')).toBeInTheDocument()
      expect(screen.getByText('Streaming response…')).toBeInTheDocument()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(mockCancelExecution).toHaveBeenCalled()
    })

    it('renders a generic thinking indicator when no execution state is known', async () => {
      mockChatHook.overrides = {
        messages: [userMessage],
        isExecuting: true,
      }

      renderChat()
      await screen.findByText('Chat with Support Bot')

      expect(screen.getByText('Thinking…')).toBeInTheDocument()
      // No execution id — no cancel control.
      expect(
        screen.queryByRole('button', { name: 'Cancel' })
      ).not.toBeInTheDocument()
    })

    it('renders a failed execution as an error bubble in the transcript', async () => {
      mockChatHook.overrides = {
        messages: [
          userMessage,
          {
            role: 'agent',
            text: 'Error: model exploded',
            timestamp: '2026-01-03T10:00:05Z',
            isError: true,
          },
        ],
      }

      renderChat()
      await screen.findByText('Chat with Support Bot')

      expect(screen.getByText('Error: model exploded')).toBeInTheDocument()
    })
  })

  describe('execution metadata panel', () => {
    it('toggles the metadata panel and clears metadata on close', async () => {
      mockChatHook.overrides = {
        messages: [userMessage, agentMessage],
        executionMetadata: {
          taskId: 'exec-1',
          status: 'completed',
          started: '2026-01-03T10:00:00Z',
          duration: 2.5,
        },
      }

      renderChat()
      await screen.findByText('Chat with Support Bot')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /Show metadata/ }))

      expect(screen.getByText('Task ID')).toBeInTheDocument()
      expect(screen.getByText('exec-1')).toBeInTheDocument()
      expect(screen.getByText('completed')).toBeInTheDocument()
      expect(screen.getByText('2.50s')).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: /Hide metadata/ })
      ).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'Close metadata' }))
      expect(mockSetExecutionMetadata).toHaveBeenCalledWith(null)
      expect(screen.queryByText('Task ID')).not.toBeInTheDocument()
    })
  })

  describe('conversation mode', () => {
    it('loads the conversation from the URL when the transcript is empty', async () => {
      renderChat('/agents/agent-1/chat?conversation=conv-1')

      await screen.findByText('Chat with Support Bot')
      await waitFor(() => {
        expect(mockLoadConversation).toHaveBeenCalledWith('conv-1')
      })
      expect(mockLoadConversation).toHaveBeenCalledTimes(1)
      expect(lastHookArgs().conversationId).toBe('conv-1')
    })

    it('shows the continuation banner with the loaded/total counts and starts a new chat', async () => {
      mockChatHook.overrides = {
        messages: [userMessage, agentMessage],
        totalMessageCount: 5,
      }

      renderChat('/agents/agent-1/chat?conversation=conv-1')
      await screen.findByText('Chat with Support Bot')

      expect(screen.getByText('Continuing conversation.')).toBeInTheDocument()
      expect(screen.getByText('2 of 5 messages loaded')).toBeInTheDocument()
      // Messages already loaded — no re-fetch of the conversation.
      expect(mockLoadConversation).not.toHaveBeenCalled()

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Start new chat' }))

      expect(mockReset).toHaveBeenCalled()
      // Navigated back to the bare chat URL — banner gone.
      expect(
        screen.queryByText('Continuing conversation.')
      ).not.toBeInTheDocument()
    })

    it('offers to load earlier messages and delegates to the hook', async () => {
      mockChatHook.overrides = {
        messages: [userMessage, agentMessage],
        hasEarlierMessages: true,
        totalMessageCount: 10,
      }

      renderChat('/agents/agent-1/chat?conversation=conv-1')
      await screen.findByText('Chat with Support Bot')

      const user = userEvent.setup()
      await user.click(
        screen.getByRole('button', { name: /Load earlier messages/ })
      )
      expect(mockLoadEarlierMessages).toHaveBeenCalled()
    })

    it('switches the URL to the captured conversation after the first reply', async () => {
      renderChat()
      await screen.findByText('Chat with Support Bot')
      expect(
        screen.queryByText('Continuing conversation.')
      ).not.toBeInTheDocument()

      act(() => {
        lastHookArgs().onConversationCaptured('conv-9')
      })

      expect(
        await screen.findByText('Continuing conversation.')
      ).toBeInTheDocument()
      await waitFor(() => {
        expect(lastHookArgs().conversationId).toBe('conv-9')
      })
    })
  })
})

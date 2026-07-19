import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Agent, AgentCard } from '@/services/agentService'

const mockNavigate = jest.fn()
jest.mock('react-router-dom', () => ({
  ...jest.requireActual<typeof import('react-router-dom')>('react-router-dom'),
  useNavigate: () => mockNavigate,
}))

// Mock Radix Select — it can loop in JSDOM (same approach as
// PromptEditor.test.tsx), but keep onValueChange wired so tests can still pick
// the initial status as plain buttons.
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

// The preview pane has its own suite (editor/__tests__/AgentPreview.test.tsx);
// a probe keeps these tests focused on the page's fetch/debounce/retry logic.
jest.mock('../editor/AgentPreview', () => ({
  AgentPreview: ({
    loading,
    data,
    error,
    onRetry,
  }: {
    loading: boolean
    data: { name?: string } | null
    error: string | null
    onRetry: () => void
  }) => (
    <div data-testid="agent-preview" data-loading={String(loading)}>
      {data?.name && <span data-testid="preview-name">{data.name}</span>}
      {error && <span data-testid="preview-error">{error}</span>}
      <button type="button" data-testid="preview-retry" onClick={onRetry}>
        Retry preview
      </button>
    </div>
  ),
}))

// Already covered by editor/__tests__/AgentCredentialsEditor.test.tsx — here we
// only assert the page's show/hide rule and the props it hands down.
jest.mock('../editor/AgentCredentialsEditor', () => ({
  AgentCredentialsEditor: ({
    agentId,
    teamId,
  }: {
    agentId: string
    teamId: string
  }) => (
    <div
      data-testid="agent-credentials-editor"
      data-agent-id={agentId}
      data-team-id={teamId}
    />
  ),
}))

jest.mock('@/services/agentService', () => ({
  agentService: {
    getAgent: jest.fn(),
    previewAgentCard: jest.fn(),
    createAgent: jest.fn(),
    updateAgent: jest.fn(),
  },
}))

jest.mock('@/contexts/TeamContext', () => {
  const currentTeam = { id: 'team-1', name: 'Test Team' }
  return {
    useTeam: () => ({ currentTeam, teams: [currentTeam], isLoading: false }),
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

import { toast } from '@/lib/toast'
import { agentService } from '@/services/agentService'

import { AgentEditor } from '../AgentEditor'

const CARD_URL = 'https://agent.example.com/.well-known/agent-card.json'

function buildCard(overrides: Partial<AgentCard> = {}): AgentCard {
  return {
    name: 'Remote Agent',
    description: 'An A2A-compliant agent',
    version: '1.0.0',
    ...overrides,
  }
}

function buildAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: 'agent-1',
    user_id: 'user-1',
    team_id: 'team-1',
    name: 'Code Review Agent',
    description: 'Reviews pull requests automatically',
    status: 'active',
    card_url: CARD_URL,
    config: null,
    has_credentials: ['api_key'],
    total_runs: 42,
    success_rate: 95,
    last_run: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    version: 1,
    ...overrides,
  }
}

function renderEditor(initialEntry = '/agents/add') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/agents/add" element={<AgentEditor />} />
        <Route path="/agents/:id/edit" element={<AgentEditor />} />
      </Routes>
    </MemoryRouter>
  )
}

function baseUrlInput() {
  return screen.getByLabelText(/Agent base URL/)
}

/** Set the URL in one shot (paste-like) so the 800ms debounce fires once. */
function setBaseUrl(value: string) {
  fireEvent.change(baseUrlInput(), { target: { value } })
}

async function waitForPreviewName(name: string) {
  expect(
    await screen.findByTestId('preview-name', {}, { timeout: 3000 })
  ).toHaveTextContent(name)
}

beforeEach(() => {
  jest.clearAllMocks()
  ;(agentService.previewAgentCard as jest.Mock).mockResolvedValue(buildCard())
  ;(agentService.createAgent as jest.Mock).mockResolvedValue(buildAgent())
  ;(agentService.updateAgent as jest.Mock).mockResolvedValue(buildAgent())
})

describe('AgentEditor — create mode', () => {
  it('renders an empty form with a disabled submit and navigates back', async () => {
    const user = userEvent.setup()
    renderEditor()

    expect(screen.getByText('Add agent')).toBeInTheDocument()
    expect(baseUrlInput()).toHaveValue('')
    expect(baseUrlInput()).not.toBeDisabled()
    expect(screen.getByRole('button', { name: /Create agent/ })).toBeDisabled()
    expect(agentService.getAgent).not.toHaveBeenCalled()
    expect(
      screen.queryByTestId('agent-credentials-editor')
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Back to agents/ }))
    expect(mockNavigate).toHaveBeenCalledWith('/agents')
  })

  it('fetches the card preview for a valid base URL (trailing slash stripped) and enables save', async () => {
    renderEditor()

    setBaseUrl('https://agent.example.com/')

    await waitFor(
      () => {
        expect(agentService.previewAgentCard).toHaveBeenCalledWith(
          'team-1',
          CARD_URL
        )
      },
      { timeout: 3000 }
    )
    await waitForPreviewName('Remote Agent')
    expect(screen.getByRole('button', { name: /Create agent/ })).toBeEnabled()
  })

  it('flags an invalid URL without calling the preview service', async () => {
    renderEditor()

    setBaseUrl('not-a-url')

    expect(
      await screen.findByTestId('preview-error', {}, { timeout: 3000 })
    ).toHaveTextContent('Invalid URL format')
    expect(agentService.previewAgentCard).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /Create agent/ })).toBeDisabled()
  })

  it('resets the preview and disables save again when the URL is cleared', async () => {
    renderEditor()

    setBaseUrl('https://agent.example.com')
    await waitForPreviewName('Remote Agent')

    setBaseUrl('')

    await waitFor(
      () => {
        expect(screen.queryByTestId('preview-name')).not.toBeInTheDocument()
      },
      { timeout: 3000 }
    )
    expect(screen.queryByTestId('preview-error')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Create agent/ })).toBeDisabled()
    // The preview service was only hit for the valid URL, not the empty one.
    expect(agentService.previewAgentCard).toHaveBeenCalledTimes(1)
  })

  it('creates the agent with the exact payload and navigates to the list', async () => {
    const user = userEvent.setup()
    renderEditor()

    setBaseUrl('https://agent.example.com')
    await waitForPreviewName('Remote Agent')

    await user.click(screen.getByRole('button', { name: /Create agent/ }))

    await waitFor(() => {
      expect(agentService.createAgent).toHaveBeenCalledWith('team-1', {
        card_url: CARD_URL,
        status: 'active',
      })
    })
    expect(agentService.updateAgent).not.toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('Agent "Remote Agent" created')
    expect(mockNavigate).toHaveBeenCalledWith('/agents')
  })

  it('carries a changed initial status into the create payload', async () => {
    const user = userEvent.setup()
    renderEditor()

    setBaseUrl('https://agent.example.com')
    await waitForPreviewName('Remote Agent')

    await user.click(screen.getByRole('button', { name: 'Paused' }))
    await user.click(screen.getByRole('button', { name: /Create agent/ }))

    await waitFor(() => {
      expect(agentService.createAgent).toHaveBeenCalledWith('team-1', {
        card_url: CARD_URL,
        status: 'paused',
      })
    })
  })

  it('surfaces a preview failure and refetches via retry', async () => {
    ;(agentService.previewAgentCard as jest.Mock)
      .mockRejectedValueOnce(new Error('card unreachable'))
      .mockResolvedValueOnce(buildCard())
    const user = userEvent.setup()
    renderEditor()

    setBaseUrl('https://agent.example.com')

    expect(
      await screen.findByTestId('preview-error', {}, { timeout: 3000 })
    ).toHaveTextContent('card unreachable')
    expect(screen.getByRole('button', { name: /Create agent/ })).toBeDisabled()

    await user.click(screen.getByTestId('preview-retry'))

    await waitForPreviewName('Remote Agent')
    expect(agentService.previewAgentCard).toHaveBeenCalledTimes(2)
    expect(screen.getByRole('button', { name: /Create agent/ })).toBeEnabled()
  })

  it('surfaces a create failure without navigating away', async () => {
    ;(agentService.createAgent as jest.Mock).mockRejectedValue(
      new Error('could not save agent')
    )
    const user = userEvent.setup()
    renderEditor()

    setBaseUrl('https://agent.example.com')
    await waitForPreviewName('Remote Agent')

    await user.click(screen.getByRole('button', { name: /Create agent/ }))

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('could not save agent')
    })
    expect(mockNavigate).not.toHaveBeenCalledWith('/agents')
  })
})

describe('AgentEditor — edit mode', () => {
  it('shows a loading skeleton while the agent loads', () => {
    ;(agentService.getAgent as jest.Mock).mockImplementation(
      () => new Promise(() => undefined)
    )

    renderEditor('/agents/agent-1/edit')

    expect(screen.getByText('Edit agent')).toBeInTheDocument()
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  it('prefills from the loaded agent, disables the URL field, and updates on save', async () => {
    ;(agentService.getAgent as jest.Mock).mockResolvedValue(buildAgent())
    ;(agentService.previewAgentCard as jest.Mock).mockResolvedValue(
      buildCard({ securitySchemes: { api_key: { type: 'apiKey' } } })
    )
    const user = userEvent.setup()
    renderEditor('/agents/agent-1/edit')

    expect(
      await screen.findByText('Editing: Code Review Agent')
    ).toBeInTheDocument()
    expect(agentService.getAgent).toHaveBeenCalledWith('team-1', 'agent-1')
    // The stored card URL is shown as its base URL, and is immutable in edit.
    expect(baseUrlInput()).toHaveValue('https://agent.example.com')
    expect(baseUrlInput()).toBeDisabled()

    await waitForPreviewName('Remote Agent')

    // The card declares security schemes → the credentials editor is shown.
    const credentials = screen.getByTestId('agent-credentials-editor')
    expect(credentials).toHaveAttribute('data-agent-id', 'agent-1')
    expect(credentials).toHaveAttribute('data-team-id', 'team-1')

    const saveButton = screen.getByRole('button', { name: /Update agent/ })
    expect(saveButton).toBeEnabled()
    await user.click(saveButton)

    await waitFor(() => {
      expect(agentService.updateAgent).toHaveBeenCalledWith(
        'team-1',
        'agent-1',
        {
          card_url: CARD_URL,
          status: 'active',
        }
      )
    })
    expect(agentService.createAgent).not.toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('Agent updated successfully')
    expect(mockNavigate).toHaveBeenCalledWith('/agents')
  })

  it('maps an error-status agent to paused in the update payload', async () => {
    ;(agentService.getAgent as jest.Mock).mockResolvedValue(
      buildAgent({ status: 'error' })
    )
    const user = userEvent.setup()
    renderEditor('/agents/agent-1/edit')

    await screen.findByText('Editing: Code Review Agent')
    await waitForPreviewName('Remote Agent')

    await user.click(screen.getByRole('button', { name: /Update agent/ }))

    await waitFor(() => {
      expect(agentService.updateAgent).toHaveBeenCalledWith(
        'team-1',
        'agent-1',
        {
          card_url: CARD_URL,
          status: 'paused',
        }
      )
    })
  })

  it('toasts and navigates back when the agent fails to load', async () => {
    ;(agentService.getAgent as jest.Mock).mockRejectedValue(
      new Error('agent not found')
    )

    renderEditor('/agents/missing/edit')

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('agent not found')
    })
    expect(mockNavigate).toHaveBeenCalledWith('/agents')
  })
})

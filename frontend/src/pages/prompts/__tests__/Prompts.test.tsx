import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Prompt, PromptListResponse } from '@/services/promptService'

// Mock Radix Select — it can loop in JSDOM (same approach as Artifacts.test.tsx)
jest.mock('@/components/ui/select', () => ({
  Select: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select">{children}</div>
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
  }) => <div data-value={value}>{children}</div>,
}))

jest.mock('@/services/promptService', () => ({
  promptService: {
    getPrompts: jest.fn(),
    deletePrompt: jest.fn(),
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

jest.mock('@/contexts/ProjectContext', () => ({
  useProject: () => ({
    currentProject: null,
    setCurrentProject: jest.fn(),
    isLoading: false,
  }),
}))

jest.mock('@/hooks', () => {
  const showSuccess = jest.fn()
  const showError = jest.fn()
  const trackEvent = jest.fn()
  return {
    useAlerts: () => ({ showSuccess, showError }),
    useAnalytics: () => ({ trackEvent }),
  }
})

jest.mock('@/hooks/useErrorHandler', () => {
  const handleError = jest.fn()
  return {
    useErrorHandler: () => ({ handleError }),
  }
})

import React from 'react'

import { promptService } from '@/services/promptService'

import { Prompts } from '../Prompts'

function buildPrompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    id: 'prompt-1',
    name: 'Code Review Template',
    slug: 'code-review-template',
    description: 'Template for conducting code reviews',
    body: 'Please review this code for: {{criteria}}',
    user_id: 'user-1',
    team_id: 'team-1',
    project_id: 'proj-1',
    status: 'published',
    mcp_expose: true,
    is_shared: false,
    labels: ['code-review'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    version: 1,
    ...overrides,
  }
}

function buildListResponse(prompts: Prompt[]): PromptListResponse {
  return {
    prompts,
    total_count: prompts.length,
    page: 1,
    per_page: 20,
    total_pages: prompts.length > 0 ? 1 : 0,
  }
}

function setTeamPermissions(permissions: string[]) {
  mockTeamState.currentTeam = {
    id: 'team-1',
    name: 'Test Team',
    permissions,
  }
}

function renderPrompts() {
  return render(
    <MemoryRouter initialEntries={['/prompts']}>
      <Routes>
        <Route path="/prompts" element={<Prompts />} />
        <Route
          path="/prompts/new"
          element={<div data-testid="editor-probe">Prompt editor probe</div>}
        />
        <Route
          path="/prompts/:slug"
          element={<div data-testid="detail-probe">Prompt detail probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('Prompts page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    setTeamPermissions([])
    ;(promptService.getPrompts as jest.Mock).mockResolvedValue(
      buildListResponse([])
    )
  })

  describe('data states', () => {
    it('renders prompt rows returned by the service', async () => {
      ;(promptService.getPrompts as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildPrompt(),
          buildPrompt({
            id: 'prompt-2',
            name: 'Bug Triage Prompt',
            slug: 'bug-triage-prompt',
            status: 'draft',
            is_shared: true,
          }),
        ])
      )

      renderPrompts()

      await waitFor(() => {
        expect(screen.getByText('Code Review Template')).toBeInTheDocument()
      })
      expect(screen.getByText('Bug Triage Prompt')).toBeInTheDocument()
      expect(screen.getByText('published')).toBeInTheDocument()
      expect(screen.getByText('draft')).toBeInTheDocument()
      expect(promptService.getPrompts).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ page: 1, limit: 20 })
      )
    })

    it('shows skeleton rows while the fetch is in flight', () => {
      ;(promptService.getPrompts as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderPrompts()

      expect(
        screen.getAllByTestId('list-page-skeleton-row').length
      ).toBeGreaterThan(0)
    })

    it('shows the error state when the fetch fails', async () => {
      ;(promptService.getPrompts as jest.Mock).mockRejectedValue(
        new Error('network down')
      )

      renderPrompts()

      await waitFor(() => {
        expect(screen.getByText('Failed to load prompts')).toBeInTheDocument()
      })
      expect(screen.getByText('network down')).toBeInTheDocument()
    })

    it('shows the empty state when there are no prompts', async () => {
      renderPrompts()

      await waitFor(() => {
        expect(screen.getByText('No prompts yet')).toBeInTheDocument()
      })
      expect(
        screen.getByText(
          'Create your first prompt to build a reusable AI workflow.'
        )
      ).toBeInTheDocument()
    })
  })

  describe('search filter', () => {
    it('re-fetches with the debounced search term', async () => {
      renderPrompts()

      await waitFor(() => {
        expect(promptService.getPrompts).toHaveBeenCalled()
      })

      const user = userEvent.setup()
      await user.type(screen.getByPlaceholderText('Search prompts…'), 'review')

      await waitFor(
        () => {
          expect(promptService.getPrompts).toHaveBeenCalledWith(
            'team-1',
            expect.objectContaining({ search: 'review', page: 1 })
          )
        },
        { timeout: 2000 }
      )
    })
  })

  describe('sorting', () => {
    it('re-fetches sorted by name asc, then toggles to desc on a second click', async () => {
      ;(promptService.getPrompts as jest.Mock).mockResolvedValue(
        buildListResponse([buildPrompt()])
      )

      renderPrompts()
      await screen.findByText('Code Review Template')

      const user = userEvent.setup()
      const nameHeader = screen.getByRole('button', { name: /Name/ })
      await user.click(nameHeader)

      await waitFor(() => {
        expect(promptService.getPrompts).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ sort_by: 'name', sort_order: 'asc' })
        )
      })

      await user.click(screen.getByRole('button', { name: /Name/ }))

      await waitFor(() => {
        expect(promptService.getPrompts).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ sort_by: 'name', sort_order: 'desc' })
        )
      })
    })
  })

  describe('row navigation', () => {
    it('navigates to the editor from the New prompt button', async () => {
      renderPrompts()
      await screen.findByText('No prompts yet')

      const user = userEvent.setup()
      const [headerButton] = screen.getAllByRole('button', {
        name: /New prompt/,
      })
      await user.click(headerButton)

      expect(screen.getByTestId('editor-probe')).toBeInTheDocument()
    })

    it('navigates to the prompt detail when the name is clicked', async () => {
      ;(promptService.getPrompts as jest.Mock).mockResolvedValue(
        buildListResponse([buildPrompt()])
      )

      renderPrompts()

      const user = userEvent.setup()
      await user.click(await screen.findByText('Code Review Template'))

      expect(screen.getByTestId('detail-probe')).toBeInTheDocument()
    })
  })

  describe('delete gating via the server permissions array (#225)', () => {
    it('shows the delete action on any row when the team grants resource.delete.any', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(promptService.getPrompts as jest.Mock).mockResolvedValue(
        buildListResponse([buildPrompt({ user_id: 'user-2' })])
      )

      renderPrompts()

      await screen.findByText('Code Review Template')
      expect(screen.getByTestId('delete-prompt-button')).toBeInTheDocument()
    })

    it('hides the delete action when the team grants no delete permission', async () => {
      setTeamPermissions([])
      ;(promptService.getPrompts as jest.Mock).mockResolvedValue(
        buildListResponse([buildPrompt({ user_id: 'user-2' })])
      )

      renderPrompts()

      await screen.findByText('Code Review Template')
      expect(
        screen.queryByTestId('delete-prompt-button')
      ).not.toBeInTheDocument()
      // Non-gated row actions are still there — the row rendered fully.
      expect(screen.getByLabelText('Edit')).toBeInTheDocument()
    })

    it('with only resource.delete.own, shows delete on own rows but not on others', async () => {
      setTeamPermissions(['resource.delete.own'])
      ;(promptService.getPrompts as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildPrompt({
            id: 'mine',
            slug: 'mine',
            name: 'My Prompt',
            user_id: 'user-1',
          }),
          buildPrompt({
            id: 'theirs',
            slug: 'theirs',
            name: 'Their Prompt',
            user_id: 'user-2',
          }),
        ])
      )

      renderPrompts()

      await screen.findByText('My Prompt')
      // Exactly one delete button: the row owned by the signed-in user.
      const deleteButtons = screen.getAllByTestId('delete-prompt-button')
      expect(deleteButtons).toHaveLength(1)
      const myRow = screen.getByText('My Prompt').closest('tr')
      expect(myRow).not.toBeNull()
      expect(
        within(myRow as HTMLElement).getByTestId('delete-prompt-button')
      ).toBeInTheDocument()
    })
  })

  describe('delete flow', () => {
    it('confirms and deletes via the service, then re-fetches', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(promptService.getPrompts as jest.Mock).mockResolvedValue(
        buildListResponse([buildPrompt()])
      )
      ;(promptService.deletePrompt as jest.Mock).mockResolvedValue(undefined)

      renderPrompts()

      const user = userEvent.setup()
      await user.click(await screen.findByTestId('delete-prompt-button'))

      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText('Delete prompt?')).toBeInTheDocument()
      const fetchCallsBefore = (promptService.getPrompts as jest.Mock).mock
        .calls.length
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      await waitFor(() => {
        expect(promptService.deletePrompt).toHaveBeenCalledWith(
          'team-1',
          'code-review-template'
        )
      })
      await waitFor(() => {
        expect(
          (promptService.getPrompts as jest.Mock).mock.calls.length
        ).toBeGreaterThan(fetchCallsBefore)
      })
    })
  })
})

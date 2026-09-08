import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import type { Mock } from 'vitest'

import type { Project } from '@/services/projectService'
import type { Prompt, PromptListResponse } from '@/services/promptService'

// Mock Radix Select — it can loop in JSDOM (same approach as Artifacts.test.tsx)
vi.mock('@/components/ui/select', () => ({
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

vi.mock('@/services/promptService', () => ({
  promptService: {
    getPrompts: vi.fn(),
    deletePrompt: vi.fn(),
    // The taxonomy filter's catalog (#908); lazy, so most tests never hit it.
    getPromptLabels: vi.fn(),
  },
}))

// The labels filter is a Radix Popover + cmdk, which the Select mock above does
// not cover; these are the layout APIs jsdom lacks.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

// usePermissions (#225) reads the signed-in user for own-vs-any delete gating.
vi.mock('@/contexts/useAuth', () => ({
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
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: mockTeamState.currentTeam,
    teams: mockTeamState.currentTeam ? [mockTeamState.currentTeam] : [],
    isLoading: mockTeamState.isLoading,
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn() as () => Promise<void>,
  }),
}))

// Mutable so tests can drive the global project selector and its restore.
const projectContextValue: {
  currentProject: Project | null
  setCurrentProject: Mock
  isLoading: boolean
} = {
  currentProject: null,
  setCurrentProject: vi.fn(),
  isLoading: false,
}
vi.mock('@/contexts/ProjectContext', () => ({
  useProject: () => projectContextValue,
}))

vi.mock('@/hooks', () => {
  const showSuccess = vi.fn()
  const showError = vi.fn()
  const trackEvent = vi.fn()
  return {
    useAlerts: () => ({ showSuccess, showError }),
    useAnalytics: () => ({ trackEvent }),
  }
})

vi.mock('@/hooks/useErrorHandler', () => {
  const handleError = vi.fn()
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

const alpha: Project = {
  id: 'p1',
  user_id: 'user-1',
  team_id: 'team-1',
  name: 'Alpha Project',
  slug: 'alpha-project',
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  version: 1,
  github_connected: false,
}

let currentSearch = ''

function LocationProbe() {
  currentSearch = useLocation().search
  return null
}

/** The filter object of the most recent getPrompts call. */
const lastQuery = () => {
  const { calls } = (promptService.getPrompts as Mock).mock
  return calls[calls.length - 1][1] as Record<string, unknown>
}

function promptsTree(initialEntry: string) {
  return (
    <MemoryRouter initialEntries={[initialEntry]}>
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
      <LocationProbe />
    </MemoryRouter>
  )
}

function renderPrompts(initialEntry = '/prompts') {
  currentSearch = ''
  return render(promptsTree(initialEntry))
}

describe('Prompts page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setTeamPermissions([])
    projectContextValue.currentProject = null
    projectContextValue.isLoading = false
    ;(promptService.getPrompts as Mock).mockResolvedValue(buildListResponse([]))
  })

  describe('data states', () => {
    it('renders prompt rows returned by the service', async () => {
      ;(promptService.getPrompts as Mock).mockResolvedValue(
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
      // Humanised from the descriptor's `valueLabels` so the table and the
      // detail header read the same (#902). Scoped to the rows, since the
      // status filter's options carry the same words.
      const publishedRow = screen
        .getByText('Code Review Template')
        .closest('tr')
      const draftRow = screen.getByText('Bug Triage Prompt').closest('tr')
      expect(
        within(publishedRow as HTMLElement).getByText('Published')
      ).toBeInTheDocument()
      expect(
        within(draftRow as HTMLElement).getByText('Draft')
      ).toBeInTheDocument()
      expect(promptService.getPrompts).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ page: 1, limit: 20 })
      )
    })

    it('shows skeleton rows while the fetch is in flight', () => {
      ;(promptService.getPrompts as Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderPrompts()

      expect(
        screen.getAllByTestId('list-page-skeleton-row').length
      ).toBeGreaterThan(0)
    })

    it('shows the error state when the fetch fails', async () => {
      ;(promptService.getPrompts as Mock).mockRejectedValue(
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
      ;(promptService.getPrompts as Mock).mockResolvedValue(
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
      ;(promptService.getPrompts as Mock).mockResolvedValue(
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
      ;(promptService.getPrompts as Mock).mockResolvedValue(
        buildListResponse([buildPrompt({ user_id: 'user-2' })])
      )

      renderPrompts()

      await screen.findByText('Code Review Template')
      expect(screen.getByTestId('delete-prompt-button')).toBeInTheDocument()
    })

    it('hides the delete action when the team grants no delete permission', async () => {
      setTeamPermissions([])
      ;(promptService.getPrompts as Mock).mockResolvedValue(
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
      ;(promptService.getPrompts as Mock).mockResolvedValue(
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
      ;(promptService.getPrompts as Mock).mockResolvedValue(
        buildListResponse([buildPrompt()])
      )
      ;(promptService.deletePrompt as Mock).mockResolvedValue(undefined)

      renderPrompts()

      const user = userEvent.setup()
      await user.click(await screen.findByTestId('delete-prompt-button'))

      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText('Delete prompt?')).toBeInTheDocument()
      const fetchCallsBefore = (promptService.getPrompts as Mock).mock.calls
        .length
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      await waitFor(() => {
        expect(promptService.deletePrompt).toHaveBeenCalledWith(
          'team-1',
          'code-review-template'
        )
      })
      await waitFor(() => {
        expect(
          (promptService.getPrompts as Mock).mock.calls.length
        ).toBeGreaterThan(fetchCallsBefore)
      })
    })
  })
})

describe('Prompts page — stale badge (#738)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setTeamPermissions([])
    ;(promptService.getPrompts as Mock).mockResolvedValue(buildListResponse([]))
  })

  it('renders the stale badge on a flagged row', async () => {
    // End-to-end through the real columns and table, so dropping the badge
    // from promptsColumns fails here.
    ;(promptService.getPrompts as Mock).mockResolvedValue(
      buildListResponse([
        buildPrompt({
          freshness: {
            status: 'stale',
            since: '2026-08-01T00:00:00Z',
            matched_rule_ids: ['r1'],
            reason: 'rule_run',
          },
        }),
      ])
    )
    renderPrompts()

    await waitFor(() => {
      expect(screen.getByTestId('freshness-badge')).toBeInTheDocument()
    })
  })

  it('renders no badge for a fresh row', async () => {
    ;(promptService.getPrompts as Mock).mockResolvedValue(
      buildListResponse([buildPrompt()])
    )
    renderPrompts()

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    expect(screen.queryByTestId('freshness-badge')).not.toBeInTheDocument()
  })

  it('sends no freshness param by default', async () => {
    renderPrompts()

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    const { calls } = (promptService.getPrompts as Mock).mock
    const query = calls[calls.length - 1][1] as Record<string, unknown>
    expect(query.freshness).toBeUndefined()
  })
})

describe('Prompts page — URL-synced filters (#906)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setTeamPermissions([])
    projectContextValue.currentProject = null
    projectContextValue.isLoading = false
    ;(promptService.getPrompts as Mock).mockResolvedValue(buildListResponse([]))
  })

  it('keeps defaults out of the URL and sends no filter params', async () => {
    renderPrompts()

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    expect(lastQuery()).toEqual(
      expect.objectContaining({
        page: 1,
        limit: 20,
        search: undefined,
        status: undefined,
        shared: undefined,
        freshness: undefined,
        sort_by: 'updated_at',
        sort_order: 'desc',
      })
    )
    expect(currentSearch).toBe('')
  })

  it('rehydrates search, status, shared, sort and page from the URL on mount', async () => {
    renderPrompts(
      '/prompts?search=review&status=draft&shared=shared&sort_by=name&sort_order=asc&page=2'
    )

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    const query = lastQuery()
    expect(query.search).toBe('review')
    expect(query.status).toBe('draft')
    // The tri-state stays a string in the URL; only the request sees a boolean.
    expect(query.shared).toBe(true)
    expect(query.sort_by).toBe('name')
    expect(query.sort_order).toBe('asc')
    expect(query.page).toBe(2)
    expect(screen.getByPlaceholderText('Search prompts…')).toHaveValue('review')
  })

  it('maps ?shared=not_shared to shared: false', async () => {
    renderPrompts('/prompts?shared=not_shared')

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    expect(lastQuery().shared).toBe(false)
  })

  it('drops a status outside the enum rather than forwarding a 400', async () => {
    renderPrompts('/prompts?status=nonsense')

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    expect(lastQuery().status).toBeUndefined()
  })

  it('falls back to the default sort key for an unknown sort_by', async () => {
    // The API 400s on a sort field outside its enum.
    renderPrompts('/prompts?sort_by=whatever')

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    expect(lastQuery().sort_by).toBe('updated_at')
  })

  it('drops a junk freshness value rather than forwarding it', async () => {
    renderPrompts('/prompts?freshness=bogus')

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    expect(lastQuery().freshness).toBeUndefined()
  })

  it('writes the debounced search term to the URL in a single request', async () => {
    renderPrompts()
    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    const before = (promptService.getPrompts as Mock).mock.calls.length

    const user = userEvent.setup()
    await user.type(screen.getByPlaceholderText('Search prompts…'), 'api')

    await waitFor(
      () => {
        expect(lastQuery().search).toBe('api')
      },
      { timeout: 2000 }
    )
    expect((promptService.getPrompts as Mock).mock.calls).toHaveLength(
      before + 1
    )
    expect(currentSearch).toContain('search=api')
  })

  it('writes a sort change to the URL so it round-trips', async () => {
    ;(promptService.getPrompts as Mock).mockResolvedValue(
      buildListResponse([buildPrompt()])
    )
    renderPrompts()
    await screen.findByText('Code Review Template')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Name/ }))

    await waitFor(() => {
      expect(lastQuery().sort_by).toBe('name')
    })
    expect(currentSearch).toContain('sort_by=name')
    expect(currentSearch).toContain('sort_order=asc')
  })

  it('a restoring persisted project does not clobber a shared link’s page', async () => {
    projectContextValue.isLoading = true
    projectContextValue.currentProject = null

    const { rerender } = renderPrompts('/prompts?page=3')
    expect(promptService.getPrompts).not.toHaveBeenCalled()

    projectContextValue.isLoading = false
    projectContextValue.currentProject = alpha
    rerender(promptsTree('/prompts?page=3'))

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    expect(lastQuery().page).toBe(3)
    expect(lastQuery().project_id).toBe('p1')
  })
})

describe('Prompts page — Clear filters and the two-branch empty state (#906)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setTeamPermissions([])
    projectContextValue.currentProject = null
    projectContextValue.isLoading = false
    ;(promptService.getPrompts as Mock).mockResolvedValue(buildListResponse([]))
  })

  it('offers New prompt, and no Clear filters, when nothing is filtered', async () => {
    renderPrompts()

    await screen.findByText('No prompts yet')
    expect(
      screen.queryByRole('button', { name: 'Clear filters' })
    ).not.toBeInTheDocument()
    // Header button plus the empty-state action.
    expect(screen.getAllByRole('button', { name: /New prompt/ })).toHaveLength(
      2
    )
  })

  it('a selected project alone is NOT a page filter', async () => {
    // It comes from the global header selector, so counting it would promise a
    // `Clear filters` that cannot clear it.
    projectContextValue.currentProject = alpha
    renderPrompts()

    await screen.findByText('No prompts yet')
    expect(
      screen.queryByRole('button', { name: 'Clear filters' })
    ).not.toBeInTheDocument()
  })

  it('the status filter alone flips the empty state to the filtered branch', async () => {
    // The old empty state branched on `search` only, so filtering by status
    // alone showed "No prompts yet" with a create button.
    renderPrompts('/prompts?status=draft')

    await screen.findByText('No prompts match your filters')
    expect(
      screen.getAllByRole('button', { name: 'Clear filters' }).length
    ).toBeGreaterThan(0)
    // Only the page header's create button survives; the empty state offers
    // Clear filters instead.
    expect(screen.getAllByRole('button', { name: /New prompt/ })).toHaveLength(
      1
    )
  })

  it('the shared filter alone also counts as filtered', async () => {
    renderPrompts('/prompts?shared=not_shared')

    expect(
      await screen.findByText('No prompts match your filters')
    ).toBeInTheDocument()
  })

  it('Clear filters empties the URL, the search box and the request', async () => {
    renderPrompts('/prompts?search=nope&status=draft&shared=shared')
    await screen.findByText('No prompts match your filters')

    const user = userEvent.setup()
    const [clear] = screen.getAllByRole('button', { name: 'Clear filters' })
    await user.click(clear)

    await waitFor(() => {
      expect(lastQuery().search).toBeUndefined()
    })
    expect(lastQuery().status).toBeUndefined()
    expect(lastQuery().shared).toBeUndefined()
    expect(lastQuery().page).toBe(1)
    expect(currentSearch).toBe('')
    expect(screen.getByPlaceholderText('Search prompts…')).toHaveValue('')
    await screen.findByText('No prompts yet')
  })
})

describe('Prompts page — taxonomy (labels) filter (#908)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setTeamPermissions([])
    projectContextValue.currentProject = null
    projectContextValue.isLoading = false
    ;(promptService.getPrompts as Mock).mockResolvedValue(buildListResponse([]))
    ;(promptService.getPromptLabels as Mock).mockResolvedValue([
      'api',
      'review',
    ])
  })

  it('sends no labels param and shows no labels in the URL by default', async () => {
    renderPrompts()

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    expect(lastQuery().labels).toBeUndefined()
    expect(currentSearch).toBe('')
  })

  it('rehydrates a comma-separated list from the URL, into the request AND the bar', async () => {
    renderPrompts('/prompts?labels=api%2Creview')

    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    // The API takes the list verbatim (backend/paths/prompts.yaml `labels`).
    expect(lastQuery().labels).toBe('api,review')
    expect(screen.getByLabelText('Filter by labels')).toHaveTextContent(
      '2 labels'
    )
  })

  it('picking a label writes the URL and refetches from page 1', async () => {
    renderPrompts('/prompts?page=4')
    await waitFor(() => {
      expect(lastQuery().page).toBe(4)
    })

    const user = userEvent.setup()
    await user.click(screen.getByLabelText('Filter by labels'))
    await user.click(await screen.findByRole('option', { name: /review/ }))

    await waitFor(() => {
      expect(lastQuery().labels).toBe('review')
    })
    expect(lastQuery().page).toBe(1)
    expect(currentSearch).toContain('labels=review')
  })

  it('the labels filter alone flips the empty state to the filtered branch', async () => {
    renderPrompts('/prompts?labels=api')

    expect(
      await screen.findByText('No prompts match your filters')
    ).toBeInTheDocument()
  })

  it('Clear filters drops the labels param', async () => {
    renderPrompts('/prompts?labels=api')
    await screen.findByText('No prompts match your filters')

    const user = userEvent.setup()
    const [clear] = screen.getAllByRole('button', { name: 'Clear filters' })
    await user.click(clear)

    await waitFor(() => {
      expect(lastQuery().labels).toBeUndefined()
    })
    expect(currentSearch).toBe('')
  })

  it('fetches the label catalog only once the popover opens', async () => {
    renderPrompts()
    await waitFor(() => {
      expect(promptService.getPrompts).toHaveBeenCalled()
    })
    expect(promptService.getPromptLabels).not.toHaveBeenCalled()

    const user = userEvent.setup()
    await user.click(screen.getByLabelText('Filter by labels'))
    await waitFor(() => {
      expect(promptService.getPromptLabels).toHaveBeenCalledWith('team-1')
    })
  })
})

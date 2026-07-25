import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'

import type {
  Blueprint,
  BlueprintListResponse,
} from '@/services/blueprintService'
import type { Project } from '@/services/projectService'

// Mock Radix Select (BlueprintFilters type dropdown) — it can loop in JSDOM.
// onValueChange stays wired so tests can pick a type as a plain button.
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

jest.mock('@/services/blueprintService', () => ({
  blueprintService: {
    getBlueprints: jest.fn(),
    deleteBlueprint: jest.fn(),
  },
}))

// Only the catalog client is mocked; parseMetadataFilter/serializeMetadataFilter
// stay real, since the URL round-trip is exactly what these tests assert.
jest.mock('@/services/metadataService', () => {
  const actual = jest.requireActual<
    typeof import('@/services/metadataService')
  >('@/services/metadataService')
  return {
    ...actual,
    metadataService: { listKeys: jest.fn(), listValues: jest.fn() },
  }
})

// Radix Popover + cmdk (MetadataFilter) need layout APIs jsdom lacks.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
})

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

// Mutable so tests choose the globally selected project (header selector).
const projectContextValue: {
  currentProject: Project | null
  setCurrentProject: jest.Mock
  isLoading: boolean
} = {
  currentProject: null,
  setCurrentProject: jest.fn(),
  isLoading: false,
}
jest.mock('@/contexts/ProjectContext', () => ({
  useProject: () => projectContextValue,
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

import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { blueprintService } from '@/services/blueprintService'
import { metadataService } from '@/services/metadataService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

import { Blueprints } from '../Blueprints'

function buildBlueprint(overrides: Partial<Blueprint> = {}): Blueprint {
  return {
    id: 'blueprint-1',
    project_id: 'proj-1',
    slug: 'api-spec',
    path: 'api-spec.md',
    user_id: 'user-1',
    content: '# Blueprint content',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    status: 'active',
    title: 'API Specification',
    description: 'The canonical API spec',
    type: 'general',
    metadata: {},
    ...overrides,
  }
}

function buildListResponse(blueprints: Blueprint[]): BlueprintListResponse {
  return {
    blueprints,
    total_count: blueprints.length,
    page: 1,
    per_page: 20,
    total_pages: blueprints.length > 0 ? 1 : 0,
  }
}

const alphaProject: Project = {
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

function setTeamPermissions(permissions: string[]) {
  mockTeamState.currentTeam = {
    id: 'team-1',
    name: 'Test Team',
    permissions,
  }
}

function rowOf(title: string): HTMLElement {
  const row = screen.getByText(title).closest('tr')
  expect(row).not.toBeNull()
  return row as HTMLElement
}

let currentSearch = ''

function LocationProbe() {
  currentSearch = useLocation().search
  return null
}

/** The filter object of the most recent getBlueprints call. */
const lastQuery = () => {
  const { calls } = (blueprintService.getBlueprints as jest.Mock).mock
  return calls[calls.length - 1][1] as Record<string, unknown>
}

function renderBlueprints(initialEntry = '/blueprints') {
  currentSearch = ''
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/blueprints" element={<Blueprints />} />
        <Route
          path="/blueprints/new"
          element={<div data-testid="new-probe">New blueprint probe</div>}
        />
        <Route
          path="/blueprints/:project/:slug"
          element={<div data-testid="view-probe">Blueprint view probe</div>}
        />
        <Route
          path="/blueprints/:project/:slug/edit"
          element={<div data-testid="edit-probe">Blueprint edit probe</div>}
        />
      </Routes>
      <LocationProbe />
    </MemoryRouter>
  )
}

describe('Blueprints page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    setTeamPermissions([])
    projectContextValue.currentProject = null
    projectContextValue.isLoading = false
    ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
      buildListResponse([])
    )
    ;(blueprintService.deleteBlueprint as jest.Mock).mockResolvedValue(
      undefined
    )
    ;(metadataService.listKeys as jest.Mock).mockResolvedValue({
      keys: ['env', 'team'],
      truncated: false,
    })
    ;(metadataService.listValues as jest.Mock).mockResolvedValue({
      values: ['prod', 'staging'],
      truncated: false,
    })
  })

  describe('data states', () => {
    it('renders blueprint rows returned by the service and tracks the page view', async () => {
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildBlueprint(),
          buildBlueprint({
            id: 'blueprint-2',
            slug: 'cursor-rules',
            title: 'Cursor Rules',
            description: 'Editor rules',
            type: 'cursor',
            status: 'expired',
          }),
        ])
      )

      renderBlueprints()

      await waitFor(() => {
        expect(screen.getByText('API Specification')).toBeInTheDocument()
      })
      expect(screen.getByText('The canonical API spec')).toBeInTheDocument()
      expect(screen.getByText('Cursor Rules')).toBeInTheDocument()
      const specRow = rowOf('API Specification')
      expect(within(specRow).getByText('General')).toBeInTheDocument()
      expect(within(specRow).getByText('active')).toBeInTheDocument()
      const cursorRow = rowOf('Cursor Rules')
      expect(within(cursorRow).getByText('Cursor')).toBeInTheDocument()
      expect(within(cursorRow).getByText('expired')).toBeInTheDocument()
      expect(blueprintService.getBlueprints).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({
          page: 1,
          limit: 20,
          sort_by: 'updated_at',
          sort_order: 'desc',
        })
      )
      const { trackEvent } = useAnalytics()
      expect(trackEvent).toHaveBeenCalledWith({
        event: ANALYTICS_EVENTS.BLUEPRINT_PAGE_VIEW,
        properties: { action_context: 'view' },
      })
    })

    it('shows skeleton rows while the fetch is in flight', () => {
      ;(blueprintService.getBlueprints as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderBlueprints()

      expect(
        screen.getAllByTestId('list-page-skeleton-row').length
      ).toBeGreaterThan(0)
    })

    it('does not fetch while the persisted project selection is restoring', () => {
      projectContextValue.isLoading = true

      renderBlueprints()

      expect(blueprintService.getBlueprints).not.toHaveBeenCalled()
    })

    it('shows the error state when the fetch fails', async () => {
      ;(blueprintService.getBlueprints as jest.Mock).mockRejectedValue(
        new Error('network down')
      )

      renderBlueprints()

      await waitFor(() => {
        expect(
          screen.getByText('Failed to load blueprints')
        ).toBeInTheDocument()
      })
      expect(screen.getByText('network down')).toBeInTheDocument()
      const { handleError } = useErrorHandler()
      expect(handleError).toHaveBeenCalledWith(
        expect.any(Error),
        'Failed to load blueprints'
      )
    })

    it('shows the unfiltered empty state when there are no blueprints', async () => {
      renderBlueprints()

      await waitFor(() => {
        expect(screen.getByText('No blueprints yet')).toBeInTheDocument()
      })
      expect(
        screen.getByText(
          'Create your first blueprint to save AI-generated content.'
        )
      ).toBeInTheDocument()
    })

    it('a selected project alone is NOT a page filter', async () => {
      // The project comes from the global header selector, not this page's
      // filter bar, so counting it would promise a `Clear filters` that cannot
      // clear it (#522).
      projectContextValue.currentProject = alphaProject

      renderBlueprints()

      await waitFor(() => {
        expect(screen.getByText('No blueprints yet')).toBeInTheDocument()
      })
      expect(
        screen.queryByRole('button', { name: 'Clear filters' })
      ).not.toBeInTheDocument()
    })

    it('shows the filtered empty state when a page filter is applied', async () => {
      renderBlueprints('/blueprints?search=nope')

      await waitFor(() => {
        expect(
          screen.getByText('No blueprints match your filters')
        ).toBeInTheDocument()
      })
      expect(
        screen.getByText('Try different search, type or metadata settings.')
      ).toBeInTheDocument()
    })
  })

  describe('filters', () => {
    it('re-fetches with the debounced search term and shows the filtered empty state', async () => {
      renderBlueprints()

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalled()
      })

      const user = userEvent.setup()
      await user.type(
        screen.getByPlaceholderText('Search blueprints…'),
        'missing'
      )

      await waitFor(
        () => {
          expect(blueprintService.getBlueprints).toHaveBeenCalledWith(
            'team-1',
            expect.objectContaining({ search: 'missing', page: 1 })
          )
        },
        { timeout: 2000 }
      )
      expect(
        await screen.findByText('No blueprints match your filters')
      ).toBeInTheDocument()
    })

    it('re-fetches with the picked type and clears it on All types', async () => {
      renderBlueprints()

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalled()
      })

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Cursor' }))

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ type: 'cursor', page: 1 })
        )
      })

      await user.click(screen.getByRole('button', { name: 'All types' }))

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ type: undefined, page: 1 })
        )
      })
    })

    it('scopes the fetch to the globally selected project', async () => {
      projectContextValue.currentProject = alphaProject

      renderBlueprints()

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ project_id: 'p1' })
        )
      })
    })
  })

  describe('URL-synced filters (#522)', () => {
    it('keeps defaults out of the URL and sends no filter params', async () => {
      renderBlueprints()

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalled()
      })
      expect(lastQuery()).toEqual(
        expect.objectContaining({
          page: 1,
          limit: 20,
          search: undefined,
          type: undefined,
          metadata: undefined,
          sort_by: 'updated_at',
          sort_order: 'desc',
        })
      )
      expect(currentSearch).toBe('')
    })

    it('rehydrates search, type and metadata from the URL on mount', async () => {
      renderBlueprints(
        '/blueprints?search=api&type=cursor&metadata=%7B%22env%22%3A%5B%22prod%22%5D%7D&page=3'
      )

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalled()
      })
      const query = lastQuery()
      expect(query.search).toBe('api')
      expect(query.type).toBe('cursor')
      // The param goes on the wire as the raw JSON object, encoded exactly once.
      expect(query.metadata).toBe('{"env":["prod"]}')
      expect(query.page).toBe(3)
      // A shared link must reproduce the filter bar, not just the query.
      expect(
        screen.getByRole('textbox', { name: 'Search blueprints' })
      ).toHaveValue('api')
      expect(screen.getByTestId('metadata-chip-env')).toHaveTextContent(
        'env: prod'
      )
    })

    it('ignores a malformed metadata param instead of sending garbage', async () => {
      renderBlueprints('/blueprints?metadata=not-json')

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalled()
      })
      expect(lastQuery().metadata).toBeUndefined()
      expect(screen.queryByTestId('metadata-chip-env')).not.toBeInTheDocument()
      // Nothing filtered means the virgin empty state, not the filtered one.
      expect(await screen.findByText('No blueprints yet')).toBeInTheDocument()
    })

    it('ignores a metadata param whose values are not string arrays', async () => {
      renderBlueprints('/blueprints?metadata=%7B%22env%22%3A%22prod%22%7D')

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalled()
      })
      expect(lastQuery().metadata).toBeUndefined()
    })

    it('committing a metadata chip sends the JSON param and resets to page 1', async () => {
      renderBlueprints('/blueprints?page=4')
      await waitFor(() => {
        expect(lastQuery().page).toBe(4)
      })

      const user = userEvent.setup()
      await user.click(
        screen.getByRole('combobox', { name: 'Filter blueprints by metadata' })
      )
      await user.click(await screen.findByText('env'))
      await user.click(await screen.findByText('prod'))
      await user.click(screen.getByRole('button', { name: 'Apply env filter' }))

      await waitFor(() => {
        expect(lastQuery().metadata).toBe('{"env":["prod"]}')
      })
      // Page 4 of a narrowed result set is usually empty.
      expect(lastQuery().page).toBe(1)
      expect(currentSearch).toContain('metadata=')
    })

    it('removing the chip drops the param entirely', async () => {
      renderBlueprints(
        '/blueprints?metadata=%7B%22env%22%3A%5B%22prod%22%5D%7D'
      )
      await waitFor(() => {
        expect(lastQuery().metadata).toBe('{"env":["prod"]}')
      })

      const user = userEvent.setup()
      await user.click(
        screen.getByRole('button', { name: 'Remove env filter' })
      )

      await waitFor(() => {
        expect(lastQuery().metadata).toBeUndefined()
      })
      expect(currentSearch).not.toContain('metadata')
    })

    it('debounces the search box into a single request and one URL write', async () => {
      renderBlueprints()
      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalled()
      })
      const before = (blueprintService.getBlueprints as jest.Mock).mock.calls
        .length

      const user = userEvent.setup()
      await user.type(
        screen.getByRole('textbox', { name: 'Search blueprints' }),
        'api'
      )

      await waitFor(
        () => {
          expect(lastQuery().search).toBe('api')
        },
        { timeout: 2000 }
      )
      // Three keystrokes must not become three requests.
      expect(
        (blueprintService.getBlueprints as jest.Mock).mock.calls.length
      ).toBe(before + 1)
      expect(currentSearch).toContain('search=api')
    })

    it('falls back to the default sort when the URL names an unknown column', async () => {
      renderBlueprints('/blueprints?sort_by=owner_email')

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalled()
      })
      expect(lastQuery().sort_by).toBe('updated_at')
    })

    it('ignores a type outside the enum rather than forwarding it', async () => {
      renderBlueprints('/blueprints?type=not-a-type')

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalled()
      })
      expect(lastQuery().type).toBeUndefined()
    })

    it('Clear filters empties the URL, the search box and the chips', async () => {
      renderBlueprints(
        '/blueprints?search=nope&type=cursor&metadata=%7B%22env%22%3A%5B%22prod%22%5D%7D'
      )
      await screen.findByText('No blueprints match your filters')

      const user = userEvent.setup()
      const [clear] = screen.getAllByRole('button', { name: 'Clear filters' })
      await user.click(clear)

      await waitFor(() => {
        expect(lastQuery().search).toBeUndefined()
      })
      expect(lastQuery().type).toBeUndefined()
      expect(lastQuery().metadata).toBeUndefined()
      expect(currentSearch).toBe('')
      // Stale text would re-commit on the next debounce tick and undo the clear.
      expect(
        screen.getByRole('textbox', { name: 'Search blueprints' })
      ).toHaveValue('')
      expect(screen.queryByTestId('metadata-chip-env')).not.toBeInTheDocument()
    })
  })

  describe('sorting', () => {
    it('sorts by title ascending, toggles to descending, then switches to updated descending', async () => {
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([buildBlueprint()])
      )

      renderBlueprints()
      await screen.findByText('API Specification')

      const user = userEvent.setup()
      // Clicking the already-active default key (updated_at desc) toggles asc.
      await user.click(screen.getByRole('button', { name: 'Updated' }))
      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({
            sort_by: 'updated_at',
            sort_order: 'asc',
            page: 1,
          })
        )
      })

      await user.click(screen.getByRole('button', { name: 'Title' }))
      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({
            sort_by: 'title',
            sort_order: 'asc',
            page: 1,
          })
        )
      })

      await user.click(screen.getByRole('button', { name: 'Title' }))
      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ sort_by: 'title', sort_order: 'desc' })
        )
      })

      await user.click(screen.getByRole('button', { name: 'Updated' }))
      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ sort_by: 'updated_at', sort_order: 'desc' })
        )
      })
    })
  })

  describe('pagination', () => {
    it('fetches the next page from the footer controls', async () => {
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue({
        ...buildListResponse([buildBlueprint()]),
        total_count: 25,
        total_pages: 2,
      })

      renderBlueprints()
      await screen.findByText('API Specification')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Next' }))

      await waitFor(() => {
        expect(blueprintService.getBlueprints).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ page: 2 })
        )
      })
    })
  })

  describe('navigation', () => {
    it('navigates to the creation form from the header New blueprint button', async () => {
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([buildBlueprint()])
      )

      renderBlueprints()
      await screen.findByText('API Specification')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /New blueprint/ }))

      expect(screen.getByTestId('new-probe')).toBeInTheDocument()
    })

    it('navigates to the creation form from the empty-state button', async () => {
      renderBlueprints()
      await screen.findByText('No blueprints yet')

      const user = userEvent.setup()
      const newButtons = screen.getAllByRole('button', {
        name: /New blueprint/,
      })
      expect(newButtons).toHaveLength(2)
      // The second one lives in the empty state.
      await user.click(newButtons[1])

      expect(screen.getByTestId('new-probe')).toBeInTheDocument()
    })

    it('navigates to the blueprint view from the row title', async () => {
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([buildBlueprint()])
      )

      renderBlueprints()

      const user = userEvent.setup()
      await user.click(await screen.findByText('API Specification'))

      expect(screen.getByTestId('view-probe')).toBeInTheDocument()
    })

    it('navigates to edit from the row edit action', async () => {
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([buildBlueprint()])
      )

      renderBlueprints()
      await screen.findByText('API Specification')

      const user = userEvent.setup()
      await user.click(
        within(rowOf('API Specification')).getByLabelText('Edit')
      )

      expect(screen.getByTestId('edit-probe')).toBeInTheDocument()
    })
  })

  describe('delete gating via the server permissions array (#225)', () => {
    it('shows the delete action on any row when the team grants resource.delete.any', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildBlueprint({ title: 'Their Blueprint', user_id: 'user-2' }),
        ])
      )

      renderBlueprints()

      await screen.findByText('Their Blueprint')
      expect(
        within(rowOf('Their Blueprint')).getByLabelText('Delete')
      ).toBeInTheDocument()
    })

    it('hides the delete action when the team grants no delete permission', async () => {
      setTeamPermissions([])
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildBlueprint({ title: 'Their Blueprint', user_id: 'user-2' }),
        ])
      )

      renderBlueprints()

      await screen.findByText('Their Blueprint')
      const row = rowOf('Their Blueprint')
      expect(within(row).queryByLabelText('Delete')).not.toBeInTheDocument()
      // Non-gated row actions are still there — the row rendered fully.
      expect(within(row).getByLabelText('View')).toBeInTheDocument()
      expect(within(row).getByLabelText('Edit')).toBeInTheDocument()
    })

    it('with only resource.delete.own, shows delete on own rows but not on others', async () => {
      setTeamPermissions(['resource.delete.own'])
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildBlueprint({
            id: 'mine',
            slug: 'mine',
            title: 'My Blueprint',
            user_id: 'user-1',
          }),
          buildBlueprint({
            id: 'theirs',
            slug: 'theirs',
            title: 'Their Blueprint',
            user_id: 'user-2',
          }),
        ])
      )

      renderBlueprints()

      await screen.findByText('My Blueprint')
      expect(screen.getAllByLabelText('Delete')).toHaveLength(1)
      expect(
        within(rowOf('My Blueprint')).getByLabelText('Delete')
      ).toBeInTheDocument()
      expect(
        within(rowOf('Their Blueprint')).queryByLabelText('Delete')
      ).not.toBeInTheDocument()
    })
  })

  describe('delete flow', () => {
    it('confirms and deletes via the service, then re-fetches and toasts', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([buildBlueprint()])
      )

      renderBlueprints()
      await screen.findByText('API Specification')

      const user = userEvent.setup()
      await user.click(
        within(rowOf('API Specification')).getByLabelText('Delete')
      )

      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText('Delete blueprint?')).toBeInTheDocument()
      expect(within(dialog).getByText('API Specification')).toBeInTheDocument()
      const fetchCallsBefore = (blueprintService.getBlueprints as jest.Mock)
        .mock.calls.length
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      await waitFor(() => {
        expect(blueprintService.deleteBlueprint).toHaveBeenCalledWith(
          'team-1',
          'proj-1',
          'api-spec'
        )
      })
      await waitFor(() => {
        expect(
          (blueprintService.getBlueprints as jest.Mock).mock.calls.length
        ).toBeGreaterThan(fetchCallsBefore)
      })
      const { showSuccess } = useAlerts()
      expect(showSuccess).toHaveBeenCalledWith(
        'Blueprint deleted successfully',
        'Success'
      )
    })

    it('cancelling the dialog closes it without deleting', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([buildBlueprint()])
      )

      renderBlueprints()
      await screen.findByText('API Specification')

      const user = userEvent.setup()
      await user.click(
        within(rowOf('API Specification')).getByLabelText('Delete')
      )
      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

      await waitFor(() => {
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      })
      expect(blueprintService.deleteBlueprint).not.toHaveBeenCalled()
    })

    it('reports the error and closes the dialog when the delete fails', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(blueprintService.getBlueprints as jest.Mock).mockResolvedValue(
        buildListResponse([buildBlueprint()])
      )
      ;(blueprintService.deleteBlueprint as jest.Mock).mockRejectedValue(
        new Error('delete forbidden')
      )

      renderBlueprints()
      await screen.findByText('API Specification')

      const user = userEvent.setup()
      await user.click(
        within(rowOf('API Specification')).getByLabelText('Delete')
      )
      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      const { handleError } = useErrorHandler()
      await waitFor(() => {
        expect(handleError).toHaveBeenCalledWith(
          expect.any(Error),
          'Failed to delete blueprint'
        )
      })
      await waitFor(() => {
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      })
      const { showSuccess } = useAlerts()
      expect(showSuccess).not.toHaveBeenCalled()
    })
  })
})

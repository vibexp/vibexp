import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

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
import { ANALYTICS_EVENTS } from '@/types/analytics'

import { Blueprints } from '../Blueprints'

function buildBlueprint(overrides: Partial<Blueprint> = {}): Blueprint {
  return {
    id: 'blueprint-1',
    project_id: 'proj-1',
    slug: 'api-spec',
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

function renderBlueprints() {
  return render(
    <MemoryRouter initialEntries={['/blueprints']}>
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

    it('shows the filtered empty state when a project is selected', async () => {
      projectContextValue.currentProject = alphaProject

      renderBlueprints()

      await waitFor(() => {
        expect(
          screen.getByText('No blueprints match your filters')
        ).toBeInTheDocument()
      })
      expect(
        screen.getByText('Try different search or filter settings.')
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

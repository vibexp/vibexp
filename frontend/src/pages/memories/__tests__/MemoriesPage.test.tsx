import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Memory, MemoryListResponse } from '@/services/memoryService'
import type { Project } from '@/services/projectService'

// Mock Radix Select (MemoryFilters tag/status dropdowns) — it can loop in
// JSDOM (same approach as Agents.test.tsx), but keep onValueChange wired so
// tests can still pick an option as a plain button.
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

jest.mock('@/services/memoryService', () => ({
  memoryService: {
    getMemories: jest.fn(),
    deleteMemory: jest.fn(),
  },
}))

jest.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: jest.fn(),
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

// Mutable so tests can scope the list to a header-selected project — the page
// seeds filters.project_id from it and tailors the empty state.
const mockProjectState: {
  currentProject: { id: string; name: string } | null
  isLoading: boolean
} = {
  currentProject: null,
  isLoading: false,
}
jest.mock('@/contexts/ProjectContext', () => ({
  useProject: () => ({
    currentProject: mockProjectState.currentProject,
    setCurrentProject: jest.fn(),
    isLoading: mockProjectState.isLoading,
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

const mockHandleError = jest.fn()
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

import React from 'react'

import { memoryService } from '@/services/memoryService'
import { projectService } from '@/services/projectService'

import { Memories } from '../Memories'

function buildMemory(overrides: Partial<Memory> = {}): Memory {
  return {
    id: 'mem-1',
    user_id: 'user-1',
    team_id: 'team-1',
    project_id: 'proj-1',
    text: 'Remember the deploy checklist',
    status: 'active',
    metadata: {},
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    version: 1,
    ...overrides,
  }
}

function buildListResponse(memories: Memory[]): MemoryListResponse {
  return {
    memories,
    total_count: memories.length,
    page: 1,
    per_page: 20,
    total_pages: memories.length > 0 ? 1 : 0,
  }
}

function buildProject(overrides: Partial<Project> = {}): Project {
  return {
    id: 'proj-1',
    user_id: 'user-1',
    team_id: 'team-1',
    name: 'Alpha Project',
    slug: 'alpha-project',
    description: '',
    git_url: '',
    homepage: '',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    version: 1,
    github_connected: false,
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

function renderMemories() {
  return render(
    <MemoryRouter initialEntries={['/memories']}>
      <Routes>
        <Route path="/memories" element={<Memories />} />
        <Route
          path="/memories/new"
          element={<div data-testid="create-probe">Memory create probe</div>}
        />
        <Route
          path="/memories/:id"
          element={<div data-testid="detail-probe">Memory detail probe</div>}
        />
        <Route
          path="/memories/:id/edit"
          element={<div data-testid="edit-probe">Memory edit probe</div>}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('Memories page', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    setTeamPermissions([])
    mockProjectState.currentProject = null
    mockProjectState.isLoading = false
    ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
      buildListResponse([])
    )
    ;(projectService.getProjects as jest.Mock).mockResolvedValue({
      projects: [],
      total_count: 0,
      page: 1,
      per_page: 100,
      total_pages: 0,
    })
  })

  describe('data states', () => {
    it('renders memory rows returned by the service', async () => {
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildMemory(),
          buildMemory({
            id: 'mem-2',
            text: 'Draft note about pgvector',
            status: 'draft',
          }),
        ])
      )

      renderMemories()

      await waitFor(() => {
        expect(
          screen.getByText('Remember the deploy checklist')
        ).toBeInTheDocument()
      })
      expect(screen.getByText('Draft note about pgvector')).toBeInTheDocument()
      // Scope to the rows: the mocked status filter also renders these labels.
      const activeRow = screen
        .getByText('Remember the deploy checklist')
        .closest('tr') as HTMLElement
      expect(within(activeRow).getByText('Active')).toBeInTheDocument()
      const draftRow = screen
        .getByText('Draft note about pgvector')
        .closest('tr') as HTMLElement
      expect(within(draftRow).getByText('Draft')).toBeInTheDocument()
      expect(memoryService.getMemories).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ page: 1, limit: 20 })
      )
    })

    it('shows the project column when the team has projects', async () => {
      ;(projectService.getProjects as jest.Mock).mockResolvedValue({
        projects: [buildProject()],
        total_count: 1,
        page: 1,
        per_page: 100,
        total_pages: 1,
      })
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([buildMemory()])
      )

      renderMemories()

      await waitFor(() => {
        expect(screen.getByText('Alpha Project')).toBeInTheDocument()
      })
      expect(projectService.getProjects).toHaveBeenCalledWith('team-1', {
        limit: 100,
      })
    })

    it('shows skeleton rows while the fetch is in flight', () => {
      ;(memoryService.getMemories as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderMemories()

      expect(
        screen.getAllByTestId('list-page-skeleton-row').length
      ).toBeGreaterThan(0)
    })

    it('shows the error state when the fetch fails', async () => {
      ;(memoryService.getMemories as jest.Mock).mockRejectedValue(
        new Error('network down')
      )

      renderMemories()

      await waitFor(() => {
        expect(screen.getByText('Failed to load memories')).toBeInTheDocument()
      })
      expect(screen.getByText('network down')).toBeInTheDocument()
    })

    it('shows the empty state when there are no memories', async () => {
      renderMemories()

      await waitFor(() => {
        expect(screen.getByText('No memories yet')).toBeInTheDocument()
      })
      expect(
        screen.getByText(
          'Create your first memory to save insights, snippets, or notes.'
        )
      ).toBeInTheDocument()
    })

    it('scopes the fetch to the header-selected project and tailors the empty state', async () => {
      mockProjectState.currentProject = { id: 'proj-1', name: 'Alpha Project' }

      renderMemories()

      await waitFor(() => {
        expect(
          screen.getByText('No memories match your filters')
        ).toBeInTheDocument()
      })
      expect(
        screen.getByText(
          'No memories in Alpha Project. Create one to get started.'
        )
      ).toBeInTheDocument()
      expect(memoryService.getMemories).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ project_id: 'proj-1' })
      )
    })

    it('does not fetch while the persisted project selection is restoring', () => {
      mockProjectState.isLoading = true

      renderMemories()

      expect(memoryService.getMemories).not.toHaveBeenCalled()
    })

    it('suggests clearing the filters when search and project are both set', async () => {
      mockProjectState.currentProject = { id: 'proj-1', name: 'Alpha Project' }

      renderMemories()
      await screen.findByText('No memories match your filters')

      const user = userEvent.setup()
      await user.type(screen.getByPlaceholderText('Search memories…'), 'zzz')

      await waitFor(
        () => {
          expect(
            screen.getByText(
              'Try a different search term or clear the filters.'
            )
          ).toBeInTheDocument()
        },
        { timeout: 2000 }
      )
    })

    it('still renders the list when the projects fetch fails', async () => {
      ;(projectService.getProjects as jest.Mock).mockRejectedValue(
        new Error('projects down')
      )
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([buildMemory()])
      )

      renderMemories()

      await screen.findByText('Remember the deploy checklist')
      await waitFor(() => {
        expect(mockHandleError).toHaveBeenCalledWith(
          expect.any(Error),
          'Failed to load projects'
        )
      })
    })
  })

  describe('pagination', () => {
    it('re-fetches the next page when Next is clicked', async () => {
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue({
        ...buildListResponse([buildMemory()]),
        total_count: 30,
        total_pages: 2,
      })

      renderMemories()
      await screen.findByText('Remember the deploy checklist')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Next' }))

      await waitFor(() => {
        expect(memoryService.getMemories).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ page: 2 })
        )
      })
    })
  })

  describe('filters', () => {
    it('re-fetches with the debounced search term', async () => {
      renderMemories()

      await waitFor(() => {
        expect(memoryService.getMemories).toHaveBeenCalled()
      })

      const user = userEvent.setup()
      await user.type(screen.getByPlaceholderText('Search memories…'), 'deploy')

      await waitFor(
        () => {
          expect(memoryService.getMemories).toHaveBeenCalledWith(
            'team-1',
            expect.objectContaining({ search: 'deploy', page: 1 })
          )
        },
        { timeout: 2000 }
      )
    })

    it('re-fetches with the selected status', async () => {
      renderMemories()
      await screen.findByText('No memories yet')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Draft' }))

      await waitFor(() => {
        expect(memoryService.getMemories).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ status: 'draft', page: 1 })
        )
      })
    })

    it('filters the visible rows by the selected tag (client-side)', async () => {
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildMemory({ metadata: { tags: ['alpha'] } }),
          buildMemory({
            id: 'mem-2',
            text: 'Beta-tagged memory',
            metadata: { tags: ['beta'] },
          }),
        ])
      )

      renderMemories()
      await screen.findByText('Remember the deploy checklist')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'alpha' }))

      await waitFor(() => {
        expect(screen.queryByText('Beta-tagged memory')).not.toBeInTheDocument()
      })
      expect(
        screen.getByText('Remember the deploy checklist')
      ).toBeInTheDocument()
    })

    it('resets the selected tag when a re-fetch drops it from the list', async () => {
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildMemory({ metadata: { tags: ['alpha'] } }),
          buildMemory({
            id: 'mem-2',
            text: 'Beta-tagged memory',
            metadata: { tags: ['beta'] },
          }),
        ])
      )

      renderMemories()
      await screen.findByText('Remember the deploy checklist')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'alpha' }))
      await waitFor(() => {
        expect(screen.queryByText('Beta-tagged memory')).not.toBeInTheDocument()
      })

      // The next fetch returns rows without the selected tag — the stale tag
      // filter must clear itself instead of hiding everything.
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildMemory({ id: 'mem-3', text: 'Untagged memory' }),
        ])
      )
      await user.click(screen.getByRole('button', { name: 'Draft' }))

      await waitFor(() => {
        expect(screen.getByText('Untagged memory')).toBeInTheDocument()
      })
    })
  })

  describe('sorting', () => {
    it('toggles the updated_at sort order on header clicks', async () => {
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([buildMemory()])
      )

      renderMemories()
      await screen.findByText('Remember the deploy checklist')

      const user = userEvent.setup()
      // Initial sort is updated_at desc, so the first click toggles to asc.
      await user.click(screen.getByRole('button', { name: /Updated/ }))

      await waitFor(() => {
        expect(memoryService.getMemories).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ sort_by: 'updated_at', sort_order: 'asc' })
        )
      })

      await user.click(screen.getByRole('button', { name: /Updated/ }))

      await waitFor(() => {
        expect(memoryService.getMemories).toHaveBeenCalledWith(
          'team-1',
          expect.objectContaining({ sort_by: 'updated_at', sort_order: 'desc' })
        )
      })
    })
  })

  describe('navigation', () => {
    it('navigates to the create page from the New memory button', async () => {
      renderMemories()
      await screen.findByText('No memories yet')

      const user = userEvent.setup()
      const [headerButton] = screen.getAllByRole('button', {
        name: /New memory/,
      })
      await user.click(headerButton)

      expect(screen.getByTestId('create-probe')).toBeInTheDocument()
    })

    it('navigates to the create page from the empty-state action', async () => {
      renderMemories()
      await screen.findByText('No memories yet')

      const user = userEvent.setup()
      const newButtons = screen.getAllByRole('button', { name: /New memory/ })
      expect(newButtons).toHaveLength(2)
      await user.click(newButtons[1])

      expect(screen.getByTestId('create-probe')).toBeInTheDocument()
    })

    it('navigates to the memory detail from the row View action', async () => {
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([buildMemory()])
      )

      renderMemories()
      await screen.findByText('Remember the deploy checklist')

      const user = userEvent.setup()
      await user.click(screen.getByLabelText('View'))

      expect(screen.getByTestId('detail-probe')).toBeInTheDocument()
    })

    it('navigates to the memory editor from the row Edit action', async () => {
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([buildMemory()])
      )

      renderMemories()
      await screen.findByText('Remember the deploy checklist')

      const user = userEvent.setup()
      await user.click(screen.getByLabelText('Edit'))

      expect(screen.getByTestId('edit-probe')).toBeInTheDocument()
    })
  })

  describe('delete gating via the server permissions array (#225)', () => {
    it('shows the delete action on any row when the team grants resource.delete.any', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([buildMemory({ user_id: 'user-2' })])
      )

      renderMemories()

      await screen.findByText('Remember the deploy checklist')
      expect(screen.getByLabelText('Delete')).toBeInTheDocument()
    })

    it('hides the delete action when the team grants no delete permission', async () => {
      setTeamPermissions([])
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([buildMemory({ user_id: 'user-2' })])
      )

      renderMemories()

      await screen.findByText('Remember the deploy checklist')
      expect(screen.queryByLabelText('Delete')).not.toBeInTheDocument()
      // Non-gated row actions are still there — the row rendered fully.
      expect(screen.getByLabelText('Edit')).toBeInTheDocument()
    })

    it('with only resource.delete.own, shows delete on own rows but not on others', async () => {
      setTeamPermissions(['resource.delete.own'])
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([
          buildMemory({
            id: 'mine',
            text: 'My memory',
            user_id: 'user-1',
          }),
          buildMemory({
            id: 'theirs',
            text: 'Their memory',
            user_id: 'user-2',
          }),
        ])
      )

      renderMemories()

      await screen.findByText('My memory')
      // Exactly one delete button: the row owned by the signed-in user.
      expect(screen.getAllByLabelText('Delete')).toHaveLength(1)
      const myRow = screen.getByText('My memory').closest('tr')
      expect(myRow).not.toBeNull()
      expect(
        within(myRow as HTMLElement).getByLabelText('Delete')
      ).toBeInTheDocument()
    })
  })

  describe('delete flow', () => {
    it('confirms and deletes via the service, then re-fetches', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([buildMemory()])
      )
      ;(memoryService.deleteMemory as jest.Mock).mockResolvedValue(undefined)

      renderMemories()

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Delete'))

      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText('Delete memory?')).toBeInTheDocument()
      const fetchCallsBefore = (memoryService.getMemories as jest.Mock).mock
        .calls.length
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      await waitFor(() => {
        expect(memoryService.deleteMemory).toHaveBeenCalledWith(
          'team-1',
          'mem-1'
        )
      })
      await waitFor(() => {
        expect(
          (memoryService.getMemories as jest.Mock).mock.calls.length
        ).toBeGreaterThan(fetchCallsBefore)
      })
    })

    it('reports the error and closes the dialog when the delete fails', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([buildMemory()])
      )
      ;(memoryService.deleteMemory as jest.Mock).mockRejectedValue(
        new Error('delete failed')
      )

      renderMemories()

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Delete'))
      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

      await waitFor(() => {
        expect(mockHandleError).toHaveBeenCalledWith(
          expect.any(Error),
          'Failed to delete memory'
        )
      })
      await waitFor(() => {
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      })
      expect(
        screen.getByText('Remember the deploy checklist')
      ).toBeInTheDocument()
    })

    it('keeps the memory when the confirm dialog is cancelled', async () => {
      setTeamPermissions(['resource.delete.any'])
      ;(memoryService.getMemories as jest.Mock).mockResolvedValue(
        buildListResponse([buildMemory()])
      )

      renderMemories()

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Delete'))

      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

      await waitFor(() => {
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      })
      expect(memoryService.deleteMemory).not.toHaveBeenCalled()
      expect(
        screen.getByText('Remember the deploy checklist')
      ).toBeInTheDocument()
    })
  })
})

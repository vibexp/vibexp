import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mock } from 'vitest'

import type { Blueprint } from '@/services/blueprintService'

// Mock BlueprintForm to avoid complex form internals in unit tests
// Stub the generated form: these are TeamContext-lifecycle tests, and the form
// itself has its own suite (`patterns/resource/form`).
vi.mock('@/components/patterns/resource', async () => ({
  ...(await vi.importActual('@/components/patterns/resource')),
  ResourceFormReadingPage: vi.fn(() => <div data-testid="blueprint-form" />),
}))

// Mock TeamContext — stable references to prevent effect re-runs
const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/services/blueprintService', () => ({
  blueprintService: {
    getBlueprintVersions: vi.fn(),
    getBlueprint: vi.fn(),
  },
}))

vi.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: vi.fn().mockResolvedValue({ projects: [] }),
  },
}))

vi.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: vi.fn() }),
  useAnalytics: () => ({ trackEvent: vi.fn() }),
}))

vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: vi.fn() }),
}))

import { ResourceFormReadingPage } from '@/components/patterns/resource'
import { blueprintService } from '@/services/blueprintService'
import { projectService } from '@/services/projectService'

import { BlueprintEdit } from '../BlueprintEdit'

const mockBlueprint: Blueprint = {
  id: 'blueprint-1',
  project_id: 'my-project',
  project: null,
  slug: 'my-blueprint',
  path: 'my-blueprint.md',
  user_id: 'user-1',
  content: 'Blueprint content here',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  status: 'active',
  title: 'My Blueprint Title',
  description: 'A test blueprint',
  type: 'general' as const,
  metadata: {},
  labels: [],
}

function renderBlueprintEdit(project = 'my-project', slug = 'my-blueprint') {
  return render(
    <MemoryRouter initialEntries={[`/blueprints/${project}/${slug}/edit`]}>
      <Routes>
        <Route
          path="/blueprints/:project/:slug/edit"
          element={<BlueprintEdit />}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('BlueprintEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('when TeamContext is still loading (isLoadingTeam = true)', () => {
    it('shows loading spinner and does not call the service', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })

      renderBlueprintEdit()

      expect(screen.getByText('Loading blueprint…')).toBeInTheDocument()
      expect(blueprintService.getBlueprint).not.toHaveBeenCalled()
    })

    it('does not show "Blueprint not found" while team is loading', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })

      renderBlueprintEdit()

      expect(screen.queryByText('Blueprint not found')).not.toBeInTheDocument()
    })
  })

  describe('when TeamContext finishes loading with a team', () => {
    it('calls the service and renders the form', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(blueprintService.getBlueprint as Mock).mockResolvedValue(mockBlueprint)

      renderBlueprintEdit()

      await waitFor(() => {
        expect(screen.getByTestId('blueprint-form')).toBeInTheDocument()
      })
      expect(blueprintService.getBlueprint).toHaveBeenCalledWith(
        'team-1',
        'my-project',
        'my-blueprint'
      )
      // The page no longer fetches projects: the generated form's
      // `ProjectPicker` owns that (#915).
      expect(projectService.getProjects).not.toHaveBeenCalled()
    })
  })

  describe('when TeamContext finishes loading without a team', () => {
    it('shows "No team available" error message', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })

      renderBlueprintEdit()

      await waitFor(() => {
        expect(
          screen.getByText(
            'No team available. Please select or create a team first.'
          )
        ).toBeInTheDocument()
      })
      expect(blueprintService.getBlueprint).not.toHaveBeenCalled()
    })
  })

  describe('race condition — isLoadingTeam transitions true → false', () => {
    it('loads blueprint after team resolves without showing "Blueprint not found"', async () => {
      mockUseTeam.mockReturnValueOnce({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(blueprintService.getBlueprint as Mock).mockResolvedValue(mockBlueprint)

      const { rerender } = renderBlueprintEdit()

      expect(blueprintService.getBlueprint).not.toHaveBeenCalled()
      expect(screen.queryByText('Blueprint not found')).not.toBeInTheDocument()

      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      rerender(
        <MemoryRouter
          initialEntries={['/blueprints/my-project/my-blueprint/edit']}
        >
          <Routes>
            <Route
              path="/blueprints/:project/:slug/edit"
              element={<BlueprintEdit />}
            />
          </Routes>
        </MemoryRouter>
      )

      await waitFor(() => {
        expect(screen.getByTestId('blueprint-form')).toBeInTheDocument()
      })
      expect(screen.queryByText('Blueprint not found')).not.toBeInTheDocument()
    })
  })

  // #1180: the column's Attachments section and Version row need the page to
  // hand over the resource and its version history.
  it('passes the resource and its version history to the form', async () => {
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-1', name: 'Test Team' },
      teams: [{ id: 'team-1', name: 'Test Team' }],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn() as () => Promise<void>,
    })
    ;(blueprintService.getBlueprint as Mock).mockResolvedValue(mockBlueprint)
    ;(blueprintService.getBlueprintVersions as Mock).mockResolvedValue({
      versions: [{ version_number: 1 }, { version_number: 2 }],
    })

    renderBlueprintEdit()

    await waitFor(() => {
      expect(ResourceFormReadingPage).toHaveBeenLastCalledWith(
        expect.objectContaining({
          resource: {
            kind: 'blueprint',
            id: mockBlueprint.id,
            teamId: 'team-1',
          },
          versionHistory: expect.objectContaining({
            to: '/blueprints/my-project/my-blueprint/versions',
            count: 2,
            currentVersion: 3,
          }),
        }),
        undefined
      )
    })
    expect(blueprintService.getBlueprintVersions).toHaveBeenCalledWith(
      'team-1',
      'my-project',
      'my-blueprint'
    )
  })
})

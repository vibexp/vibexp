import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Blueprint } from '@/services/blueprintService'

// Mock BlueprintForm to avoid complex form internals in unit tests
jest.mock('@/pages/blueprints/BlueprintForm', () => ({
  BlueprintForm: jest.fn(() => <div data-testid="blueprint-form" />),
}))

// Mock TeamContext — stable references to prevent effect re-runs
const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

jest.mock('@/services/blueprintService', () => ({
  blueprintService: {
    getBlueprint: jest.fn(),
  },
}))

jest.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: jest.fn().mockResolvedValue({ projects: [] }),
  },
}))

jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: jest.fn() }),
  useAnalytics: () => ({ trackEvent: jest.fn() }),
}))

jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: jest.fn() }),
}))

import { blueprintService } from '@/services/blueprintService'
import { projectService } from '@/services/projectService'

import { BlueprintEdit } from '../BlueprintEdit'

const mockBlueprint: Blueprint = {
  id: 'blueprint-1',
  project_id: 'my-project',
  slug: 'my-blueprint',
  user_id: 'user-1',
  content: 'Blueprint content here',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  status: 'active',
  title: 'My Blueprint Title',
  description: 'A test blueprint',
  type: 'general' as const,
  metadata: {},
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
    jest.clearAllMocks()
  })

  describe('when TeamContext is still loading (isLoadingTeam = true)', () => {
    it('shows loading spinner and does not call the service', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
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
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
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
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(blueprintService.getBlueprint as jest.Mock).mockResolvedValue(
        mockBlueprint
      )

      renderBlueprintEdit()

      await waitFor(() => {
        expect(screen.getByTestId('blueprint-form')).toBeInTheDocument()
      })
      expect(blueprintService.getBlueprint).toHaveBeenCalledWith(
        'team-1',
        'my-project',
        'my-blueprint'
      )
      expect(projectService.getProjects).toHaveBeenCalledWith('team-1', {
        limit: 100,
      })
    })
  })

  describe('when TeamContext finishes loading without a team', () => {
    it('shows "No team available" error message', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
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
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(blueprintService.getBlueprint as jest.Mock).mockResolvedValue(
        mockBlueprint
      )

      const { rerender } = renderBlueprintEdit()

      expect(blueprintService.getBlueprint).not.toHaveBeenCalled()
      expect(screen.queryByText('Blueprint not found')).not.toBeInTheDocument()

      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
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
})

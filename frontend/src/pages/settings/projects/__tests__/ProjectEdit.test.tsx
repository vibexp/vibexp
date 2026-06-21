import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Project } from '@/types'

// Mock ProjectForm to avoid complex form internals in unit tests
jest.mock('@/pages/settings/projects/ProjectForm', () => ({
  ProjectForm: jest.fn(() => <div data-testid="project-form" />),
}))

// Mock TeamContext — stable references to prevent effect re-runs
const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

jest.mock('@/services/projectService', () => ({
  projectService: {
    getProject: jest.fn(),
    updateProject: jest.fn(),
  },
}))

jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: jest.fn() }),
}))

jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: jest.fn() }),
}))

import { projectService } from '@/services/projectService'

import { ProjectEdit } from '../ProjectEdit'

const mockProject: Project = {
  id: 'project-1',
  user_id: 'user-1',
  team_id: 'team-1',
  name: 'My Project',
  slug: 'my-project',
  description: 'A test project',
  git_url: '',
  homepage: '',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  version: 1,
}

function renderProjectEdit(slug = 'my-project') {
  return render(
    <MemoryRouter initialEntries={[`/settings/projects/${slug}/edit`]}>
      <Routes>
        <Route path="/settings/projects/:slug/edit" element={<ProjectEdit />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('ProjectEdit', () => {
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

      renderProjectEdit()

      expect(screen.getByText('Loading project…')).toBeInTheDocument()
      expect(projectService.getProject).not.toHaveBeenCalled()
    })

    it('does not show "Project not found" while team is loading', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })

      renderProjectEdit()

      expect(screen.queryByText('Project not found')).not.toBeInTheDocument()
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
      ;(projectService.getProject as jest.Mock).mockResolvedValue(mockProject)

      renderProjectEdit()

      await waitFor(() => {
        expect(screen.getByTestId('project-form')).toBeInTheDocument()
      })
      expect(projectService.getProject).toHaveBeenCalledWith(
        'team-1',
        'my-project'
      )
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

      renderProjectEdit()

      await waitFor(() => {
        expect(
          screen.getByText(
            'No team available. Please select or create a team first.'
          )
        ).toBeInTheDocument()
      })
      expect(projectService.getProject).not.toHaveBeenCalled()
    })
  })

  describe('race condition — isLoadingTeam transitions true → false', () => {
    it('loads project after team resolves without showing "Project not found"', async () => {
      mockUseTeam.mockReturnValueOnce({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(projectService.getProject as jest.Mock).mockResolvedValue(mockProject)

      const { rerender } = renderProjectEdit()

      expect(projectService.getProject).not.toHaveBeenCalled()
      expect(screen.queryByText('Project not found')).not.toBeInTheDocument()

      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      rerender(
        <MemoryRouter initialEntries={['/settings/projects/my-project/edit']}>
          <Routes>
            <Route
              path="/settings/projects/:slug/edit"
              element={<ProjectEdit />}
            />
          </Routes>
        </MemoryRouter>
      )

      await waitFor(() => {
        expect(screen.getByTestId('project-form')).toBeInTheDocument()
      })
      expect(screen.queryByText('Project not found')).not.toBeInTheDocument()
    })
  })
})

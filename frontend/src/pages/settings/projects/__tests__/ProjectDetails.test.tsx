import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Project, ProjectStatsResponse } from '@/services/projectService'

// Mock TeamContext — stable references to prevent effect re-runs
const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

jest.mock('@/services/projectService', () => ({
  projectService: {
    getProject: jest.fn(),
    getProjectStats: jest.fn(),
    deleteProject: jest.fn(),
  },
}))

const mockShowSuccess = jest.fn()
jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: mockShowSuccess }),
}))

const mockHandleError = jest.fn()
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

import { projectService } from '@/services/projectService'

import { ProjectDetails } from '../ProjectDetails'

const mockProject: Project = {
  id: 'project-1',
  user_id: 'user-1',
  team_id: 'team-1',
  name: 'My Project',
  slug: 'my-project',
  description: 'A test project description',
  git_url: 'https://github.com/example/repo',
  homepage: 'https://example.com',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  version: 1,
  github_connected: true,
}

const mockStats: ProjectStatsResponse = {
  total_prompts: 5,
  total_artifacts: 3,
  total_blueprints: 2,
  total_memories: 7,
  total_feed_items: 1,
}

const mockTeam = {
  currentTeam: { id: 'team-1', name: 'Test Team' },
  teams: [{ id: 'team-1', name: 'Test Team' }],
  isLoading: false,
  setCurrentTeam: jest.fn(),
  refreshTeams: jest.fn() as () => Promise<void>,
}

const mockTeamLoading = {
  currentTeam: null,
  teams: [],
  isLoading: true,
  setCurrentTeam: jest.fn(),
  refreshTeams: jest.fn() as () => Promise<void>,
}

function renderProjectDetails(slug = 'my-project') {
  return render(
    <MemoryRouter initialEntries={[`/settings/projects/${slug}`]}>
      <Routes>
        <Route path="/settings/projects/:slug" element={<ProjectDetails />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('ProjectDetails', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  describe('when TeamContext is still loading', () => {
    it('shows loading skeleton and does not call the service', () => {
      mockUseTeam.mockReturnValue(mockTeamLoading)

      renderProjectDetails()

      expect(projectService.getProject).not.toHaveBeenCalled()
      expect(projectService.getProjectStats).not.toHaveBeenCalled()
    })
  })

  describe('when project loads successfully', () => {
    beforeEach(() => {
      mockUseTeam.mockReturnValue(mockTeam)
      ;(projectService.getProject as jest.Mock).mockResolvedValue(mockProject)
      ;(projectService.getProjectStats as jest.Mock).mockResolvedValue(
        mockStats
      )
    })

    it('renders project name in page header', async () => {
      renderProjectDetails()

      await waitFor(() => {
        expect(screen.getByText('My Project')).toBeInTheDocument()
      })
    })

    it('renders project description', async () => {
      renderProjectDetails()

      await waitFor(() => {
        expect(
          screen.getByText('A test project description')
        ).toBeInTheDocument()
      })
    })

    it('renders project slug in code font', async () => {
      renderProjectDetails()

      await waitFor(() => {
        expect(screen.getByText('my-project')).toBeInTheDocument()
      })
    })

    it('renders git URL as clickable link', async () => {
      renderProjectDetails()

      await waitFor(() => {
        const link = screen.getByRole('link', {
          name: /https:\/\/github\.com\/example\/repo/i,
        })
        expect(link).toHaveAttribute('href', 'https://github.com/example/repo')
        expect(link).toHaveAttribute('target', '_blank')
      })
    })

    it('renders homepage as clickable link', async () => {
      renderProjectDetails()

      await waitFor(() => {
        const link = screen.getByRole('link', {
          name: /https:\/\/example\.com/i,
        })
        expect(link).toHaveAttribute('href', 'https://example.com')
        expect(link).toHaveAttribute('target', '_blank')
      })
    })

    it('renders stats counts correctly', async () => {
      renderProjectDetails()

      await waitFor(() => {
        expect(screen.getByText('5')).toBeInTheDocument()
        expect(screen.getByText('3')).toBeInTheDocument()
        expect(screen.getByText('2')).toBeInTheDocument()
        expect(screen.getByText('7')).toBeInTheDocument()
        expect(screen.getByText('1')).toBeInTheDocument()
      })
    })

    it('renders stats section labels', async () => {
      renderProjectDetails()

      await waitFor(() => {
        expect(screen.getByText('Prompts')).toBeInTheDocument()
        expect(screen.getByText('Artifacts')).toBeInTheDocument()
        expect(screen.getByText('Blueprints')).toBeInTheDocument()
        expect(screen.getByText('Memories')).toBeInTheDocument()
        expect(screen.getByText('Feed Items')).toBeInTheDocument()
      })
    })

    it('stats cards are not links (resource list pages do not support project filtering)', async () => {
      renderProjectDetails()

      await waitFor(() => {
        expect(screen.getByText('5')).toBeInTheDocument()
      })

      // Stat labels must be visible but must not be wrapped in an anchor element
      const promptsText = screen.getByText('Prompts')
      expect(promptsText.closest('a')).toBeNull()
    })

    it('Edit button is present and interactive', async () => {
      renderProjectDetails()

      await waitFor(() => {
        expect(screen.getByText('My Project')).toBeInTheDocument()
      })

      expect(screen.getByRole('button', { name: /edit/i })).toBeInTheDocument()
    })

    it('Migrate Resources button is present', async () => {
      renderProjectDetails()

      await waitFor(() => {
        expect(
          screen.getByRole('button', { name: /migrate resources/i })
        ).toBeInTheDocument()
      })
    })

    it('Delete button opens confirm dialog', async () => {
      const user = userEvent.setup()
      renderProjectDetails()

      await waitFor(() => {
        expect(screen.getByText('My Project')).toBeInTheDocument()
      })

      const deleteButton = screen.getByRole('button', { name: /delete/i })
      await user.click(deleteButton)

      await waitFor(() => {
        expect(screen.getByText('Delete project?')).toBeInTheDocument()
      })
    })

    it('confirms delete and calls deleteProject service', async () => {
      const user = userEvent.setup()
      ;(projectService.deleteProject as jest.Mock).mockResolvedValue(undefined)
      renderProjectDetails()

      await waitFor(() => {
        expect(screen.getByText('My Project')).toBeInTheDocument()
      })

      const deleteButton = screen.getByRole('button', { name: /delete/i })
      await user.click(deleteButton)

      await waitFor(() => {
        expect(screen.getByText('Delete project?')).toBeInTheDocument()
      })

      const confirmButton = screen.getByRole('button', { name: /^delete$/i })
      await user.click(confirmButton)

      await waitFor(() => {
        expect(projectService.deleteProject).toHaveBeenCalledWith(
          'team-1',
          'my-project'
        )
      })
    })
  })

  describe('when project load fails', () => {
    it('shows error state with message', async () => {
      mockUseTeam.mockReturnValue(mockTeam)
      ;(projectService.getProject as jest.Mock).mockRejectedValue(
        new Error('Not found')
      )
      ;(projectService.getProjectStats as jest.Mock).mockRejectedValue(
        new Error('Not found')
      )

      renderProjectDetails()

      await waitFor(() => {
        expect(screen.getByText('Could not load project')).toBeInTheDocument()
      })
    })

    it('shows Back to Projects button in error state', async () => {
      mockUseTeam.mockReturnValue(mockTeam)
      ;(projectService.getProject as jest.Mock).mockRejectedValue(
        new Error('Not found')
      )
      ;(projectService.getProjectStats as jest.Mock).mockRejectedValue(
        new Error('Not found')
      )

      renderProjectDetails()

      await waitFor(() => {
        expect(
          screen.getByRole('button', { name: /back to projects/i })
        ).toBeInTheDocument()
      })
    })
  })

  describe('when team is not available', () => {
    it('shows no team error after loading completes', async () => {
      mockUseTeam.mockReturnValue({
        ...mockTeamLoading,
        isLoading: false,
        currentTeam: null,
      })

      renderProjectDetails()

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
})

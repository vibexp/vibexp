import {
  act,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react'

import type { Project } from '../../types/project'
import { ProjectProvider, useProject } from '../ProjectContext'

// Mock the projectService
jest.mock('../../services/projectService', () => ({
  projectService: {
    getProjects: jest.fn(),
  },
}))

// Mock the centralized storage utilities
jest.mock('../../utils/storage', () => ({
  storage: {
    get: jest.fn(),
    set: jest.fn(),
    remove: jest.fn(),
    clear: jest.fn(),
  },
  sessionStore: {
    get: jest.fn(),
    set: jest.fn(),
    remove: jest.fn(),
    clear: jest.fn(),
  },
}))

// Mock TeamContext so tests control the current team directly
jest.mock('../TeamContext', () => ({
  useTeam: jest.fn(),
}))

// Import the mocked modules after the mock
import { projectService } from '../../services/projectService'
import { storage } from '../../utils/storage'
import { useTeam } from '../TeamContext'

const mockProjectService = projectService as jest.Mocked<typeof projectService>
const mockStorage = storage as jest.Mocked<typeof storage>
const mockUseTeam = useTeam as jest.Mock

function makeProject(id: string, name: string): Project {
  return {
    id,
    user_id: 'user-1',
    team_id: 'team-1',
    name,
    slug: name.toLowerCase().replace(/\s+/g, '-'),
    description: '',
    git_url: '',
    homepage: '',
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2023-01-01T00:00:00Z',
    version: 1,
  }
}

const mockProjects: Project[] = [
  makeProject('project-1', 'Project Alpha'),
  makeProject('project-2', 'Project Beta'),
]

function teamValue(teamId: string | null) {
  return {
    currentTeam: teamId ? { id: teamId, name: `Team ${teamId}` } : null,
    teams: [],
    setCurrentTeam: jest.fn(),
    refreshTeams: jest.fn(),
    isLoading: false,
  }
}

const TestComponent = () => {
  const { currentProject, setCurrentProject, isLoading } = useProject()
  return (
    <div>
      <div data-testid="current-project">
        {currentProject ? currentProject.name : 'null'}
      </div>
      <div data-testid="loading">{String(isLoading)}</div>
      <button
        onClick={() => {
          setCurrentProject(mockProjects[1])
        }}
        data-testid="select-project"
      >
        Select Project
      </button>
      <button
        onClick={() => {
          setCurrentProject(null)
        }}
        data-testid="clear-project"
      >
        All projects
      </button>
    </div>
  )
}

describe('ProjectContext', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockStorage.get.mockReturnValue(null)
    mockStorage.set.mockImplementation(() => {})
    mockStorage.remove.mockImplementation(() => {})
    mockUseTeam.mockReturnValue(teamValue('team-1'))
    mockProjectService.getProjects.mockResolvedValue({
      projects: mockProjects,
      total_count: mockProjects.length,
      page: 1,
      per_page: 100,
      total_pages: 1,
    })
  })

  it('defaults to "All projects" (null) when nothing is stored', async () => {
    render(
      <ProjectProvider>
        <TestComponent />
      </ProjectProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    })

    expect(screen.getByTestId('current-project')).toHaveTextContent('null')
    // No stored id — no need to fetch projects to validate anything
    expect(mockProjectService.getProjects).not.toHaveBeenCalled()
  })

  it('restores the stored project when it belongs to the current team', async () => {
    mockStorage.get.mockReturnValue('project-2')

    render(
      <ProjectProvider>
        <TestComponent />
      </ProjectProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    })

    expect(screen.getByTestId('current-project')).toHaveTextContent(
      'Project Beta'
    )
  })

  it('drops a stored project that is not in the current team', async () => {
    mockStorage.get.mockReturnValue('project-from-another-team')

    render(
      <ProjectProvider>
        <TestComponent />
      </ProjectProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    })

    expect(screen.getByTestId('current-project')).toHaveTextContent('null')
    expect(mockStorage.remove).toHaveBeenCalledWith(
      expect.any(String) // STORAGE_KEYS.CURRENT_PROJECT_ID
    )
  })

  it('persists the selection via setCurrentProject and clears it on "All projects"', async () => {
    render(
      <ProjectProvider>
        <TestComponent />
      </ProjectProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    })

    act(() => {
      screen.getByTestId('select-project').click()
    })

    expect(screen.getByTestId('current-project')).toHaveTextContent(
      'Project Beta'
    )
    expect(mockStorage.set).toHaveBeenCalledWith(
      expect.any(String), // STORAGE_KEYS.CURRENT_PROJECT_ID
      'project-2'
    )

    act(() => {
      screen.getByTestId('clear-project').click()
    })

    expect(screen.getByTestId('current-project')).toHaveTextContent('null')
    expect(mockStorage.remove).toHaveBeenCalled()
  })

  it('resets the selection when the team changes', async () => {
    const { rerender } = render(
      <ProjectProvider>
        <TestComponent />
      </ProjectProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    })

    act(() => {
      screen.getByTestId('select-project').click()
    })
    expect(screen.getByTestId('current-project')).toHaveTextContent(
      'Project Beta'
    )

    mockStorage.remove.mockClear()
    mockUseTeam.mockReturnValue(teamValue('team-2'))
    rerender(
      <ProjectProvider>
        <TestComponent />
      </ProjectProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('current-project')).toHaveTextContent('null')
    })
    expect(mockStorage.remove).toHaveBeenCalled()
  })

  it('keeps the selection when storage restore fails but logs the error', async () => {
    mockStorage.get.mockReturnValue('project-1')
    mockProjectService.getProjects.mockRejectedValue(new Error('network down'))
    const consoleErrorSpy = jest
      .spyOn(console, 'error')
      .mockImplementation(() => {})

    render(
      <ProjectProvider>
        <TestComponent />
      </ProjectProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    })

    expect(screen.getByTestId('current-project')).toHaveTextContent('null')
    expect(consoleErrorSpy).toHaveBeenCalled()

    consoleErrorSpy.mockRestore()
  })

  it('throws when useProject is used outside ProjectProvider', () => {
    const consoleErrorSpy = jest
      .spyOn(console, 'error')
      .mockImplementation(() => {})

    expect(() => {
      renderHook(() => useProject())
    }).toThrow('useProject must be used within ProjectProvider')

    consoleErrorSpy.mockRestore()
  })
})

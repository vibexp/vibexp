import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import type { Memory, Project } from '@/types'

const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

jest.mock('@/services/memoryService', () => ({
  memoryService: {
    createMemory: jest.fn(),
  },
}))

jest.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: jest.fn(),
  },
}))

jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
  useAnalytics: () => ({ trackEvent: jest.fn() }),
}))

jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: jest.fn() }),
}))

import { memoryService } from '@/services/memoryService'
import { projectService } from '@/services/projectService'

import { MemoryCreate } from '../MemoryCreate'

const mockProject: Project = {
  id: 'project-1',
  user_id: 'user-1',
  team_id: 'team-1',
  name: 'My Project',
  slug: 'my-project',
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  version: 1,
}

const mockCreatedMemory: Memory = {
  id: 'mem-new',
  user_id: 'user-1',
  team_id: 'team-1',
  project_id: 'project-1',
  text: 'My new memory',
  metadata: {},
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  version: 1,
}

function renderMemoryCreate() {
  return render(
    <MemoryRouter>
      <MemoryCreate />
    </MemoryRouter>
  )
}

describe('MemoryCreate', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-1', name: 'Test Team' },
      teams: [{ id: 'team-1', name: 'Test Team' }],
      isLoading: false,
      setCurrentTeam: jest.fn(),
      refreshTeams: jest.fn() as () => Promise<void>,
    })
  })

  it('shows loading spinner while projects are being fetched', () => {
    ;(projectService.getProjects as jest.Mock).mockImplementation(
      () => new Promise(() => undefined)
    )

    renderMemoryCreate()

    expect(screen.getByText('Create memory')).toBeInTheDocument()
    // Loading spinner should be present, form should not
    expect(
      screen.queryByPlaceholderText(/Enter your memory/)
    ).not.toBeInTheDocument()
  })

  it('renders form once projects are loaded', async () => {
    ;(projectService.getProjects as jest.Mock).mockResolvedValue({
      projects: [mockProject],
      total_count: 1,
      page: 1,
      per_page: 100,
      total_pages: 1,
    })

    renderMemoryCreate()

    await waitFor(() => {
      expect(
        screen.getByPlaceholderText(/Enter your memory content/)
      ).toBeInTheDocument()
    })
  })

  it('submit button is disabled when no projects are available', async () => {
    ;(projectService.getProjects as jest.Mock).mockResolvedValue({
      projects: [],
      total_count: 0,
      page: 1,
      per_page: 100,
      total_pages: 1,
    })

    renderMemoryCreate()

    await waitFor(() => {
      const createButton = screen.getByRole('button', {
        name: /create memory/i,
      })
      expect(createButton).toBeDisabled()
    })
  })

  it('submit is enabled when a project is available', async () => {
    ;(projectService.getProjects as jest.Mock).mockResolvedValue({
      projects: [mockProject],
      total_count: 1,
      page: 1,
      per_page: 100,
      total_pages: 1,
    })

    renderMemoryCreate()

    await waitFor(() => {
      const createButton = screen.getByRole('button', {
        name: /create memory/i,
      })
      expect(createButton).not.toBeDisabled()
    })
  })

  it('calls createMemory with project_id when submitted', async () => {
    ;(projectService.getProjects as jest.Mock).mockResolvedValue({
      projects: [mockProject],
      total_count: 1,
      page: 1,
      per_page: 100,
      total_pages: 1,
    })
    ;(memoryService.createMemory as jest.Mock).mockResolvedValue(
      mockCreatedMemory
    )

    renderMemoryCreate()

    // Wait for form to appear
    await waitFor(() => {
      expect(
        screen.getByPlaceholderText(/Enter your memory content/)
      ).toBeInTheDocument()
    })

    // Fill in the text
    const textarea = screen.getByPlaceholderText(/Enter your memory content/)
    fireEvent.change(textarea, { target: { value: 'My new memory' } })

    // Submit
    const createButton = screen.getByRole('button', { name: /create memory/i })
    fireEvent.click(createButton)

    await waitFor(() => {
      expect(memoryService.createMemory).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({
          project_id: 'project-1',
          text: 'My new memory',
        })
      )
    })
  })

  it('loads projects using the current team id', async () => {
    ;(projectService.getProjects as jest.Mock).mockResolvedValue({
      projects: [mockProject],
      total_count: 1,
      page: 1,
      per_page: 100,
      total_pages: 1,
    })

    renderMemoryCreate()

    await waitFor(() => {
      expect(projectService.getProjects).toHaveBeenCalledWith('team-1', {
        limit: 100,
      })
    })
  })
})

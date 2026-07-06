import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Project } from '@/services/projectService'
import type { Memory } from '@/types'

// Mock TeamContext — stable references to prevent effect re-runs
const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

jest.mock('@/services/memoryService', () => ({
  memoryService: {
    getMemory: jest.fn(),
    deleteMemory: jest.fn(),
  },
}))

jest.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: jest.fn(),
  },
}))

jest.mock('@/hooks', () => {
  const showSuccess = jest.fn()
  const trackEvent = jest.fn()
  return {
    useAlerts: () => ({ showSuccess }),
    useAnalytics: () => ({ trackEvent }),
  }
})

jest.mock('@/hooks/useErrorHandler', () => {
  const handleError = jest.fn()
  return {
    useErrorHandler: () => ({ handleError }),
  }
})

// Mock MarkdownRenderer to verify content is passed through it
jest.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-renderer">{content}</div>
  ),
}))

import { memoryService } from '@/services/memoryService'
import { projectService } from '@/services/projectService'

import { MemoryView } from '../MemoryView'

const mockMemory: Memory = {
  id: 'memory-1',
  user_id: 'user-1',
  team_id: 'team-1',
  project_id: 'project-1',
  text: 'This is memory text content',
  status: 'active',
  metadata: { type: 'note' },
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  version: 1,
}

const mockProject: Project = {
  id: 'project-1',
  user_id: 'user-1',
  team_id: 'team-1',
  name: 'Test Project',
  slug: 'test-project',
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  version: 1,
  github_connected: false,
}

function renderMemoryView(id = 'memory-1') {
  return render(
    <MemoryRouter initialEntries={[`/memories/${id}`]}>
      <Routes>
        <Route path="/memories/:id" element={<MemoryView />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('MemoryView', () => {
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

      renderMemoryView()

      expect(screen.getByText('Loading memory…')).toBeInTheDocument()
      expect(memoryService.getMemory).not.toHaveBeenCalled()
    })

    it('does not show "Memory not found" while team is loading', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })

      renderMemoryView()

      expect(screen.queryByText('Memory not found')).not.toBeInTheDocument()
    })
  })

  describe('when TeamContext finishes loading with a team', () => {
    it('loads and renders the memory text via MarkdownRenderer', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as jest.Mock).mockResolvedValue(mockMemory)
      ;(projectService.getProjects as jest.Mock).mockResolvedValue({
        projects: [mockProject],
        page: 1,
        per_page: 100,
        total_count: 1,
        total_pages: 1,
      })

      renderMemoryView()

      await waitFor(() => {
        const renderer = screen.getByTestId('markdown-renderer')
        expect(renderer).toBeInTheDocument()
        expect(renderer).toHaveTextContent('This is memory text content')
      })
      expect(memoryService.getMemory).toHaveBeenCalledWith('team-1', 'memory-1')
    })

    it('renders markdown content through MarkdownRenderer (not a <pre>)', async () => {
      const markdownContent =
        '# Heading\n\n```ts\nconst x = 1\n```\n\n- item1\n- item2'
      const memoryWithMarkdown: Memory = {
        ...mockMemory,
        text: markdownContent,
      }
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as jest.Mock).mockResolvedValue(
        memoryWithMarkdown
      )
      ;(projectService.getProjects as jest.Mock).mockResolvedValue({
        projects: [mockProject],
        page: 1,
        per_page: 100,
        total_count: 1,
        total_pages: 1,
      })

      renderMemoryView()

      await waitFor(() => {
        const renderer = screen.getByTestId('markdown-renderer')
        expect(renderer).toBeInTheDocument()
        // The mock renders the raw content as a child — check a distinctive marker
        expect(renderer.textContent).toContain('# Heading')
        expect(renderer.textContent).toContain('item1')
      })
      // The raw markdown source must NOT be in a <pre> element (content card)
      const preElements = document.querySelectorAll('pre')
      const preWithMarkdown = Array.from(preElements).find(el =>
        el.textContent?.includes('# Heading')
      )
      expect(preWithMarkdown).toBeUndefined()
    })

    it('shows loading spinner while fetch is in flight', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderMemoryView()

      expect(screen.getByText('Loading memory…')).toBeInTheDocument()
    })

    it('renders the memory heading with the correct id', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as jest.Mock).mockResolvedValue(mockMemory)
      ;(projectService.getProjects as jest.Mock).mockResolvedValue({
        projects: [mockProject],
        page: 1,
        per_page: 100,
        total_count: 1,
        total_pages: 1,
      })

      renderMemoryView()

      await waitFor(() => {
        expect(screen.getByText('Memory #memory-1')).toBeInTheDocument()
      })
    })

    it('shows the project name in the sidebar when project loads', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as jest.Mock).mockResolvedValue(mockMemory)
      ;(projectService.getProjects as jest.Mock).mockResolvedValue({
        projects: [mockProject],
        page: 1,
        per_page: 100,
        total_count: 1,
        total_pages: 1,
      })

      renderMemoryView()

      await waitFor(() => {
        expect(screen.getByText('Test Project')).toBeInTheDocument()
      })
      // getProjects is called with the team id; the component resolves by id from the list
      expect(projectService.getProjects).toHaveBeenCalledWith('team-1', {
        limit: 100,
      })
    })

    it('renders Copy content button', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as jest.Mock).mockResolvedValue(mockMemory)
      ;(projectService.getProjects as jest.Mock).mockResolvedValue({
        projects: [mockProject],
        page: 1,
        per_page: 100,
        total_count: 1,
        total_pages: 1,
      })

      renderMemoryView()

      await waitFor(() => {
        expect(
          screen.getByRole('button', { name: 'Copy content' })
        ).toBeInTheDocument()
      })
    })
  })

  describe('direct URL load (simulate isLoadingTeam flipping from true to false)', () => {
    it('loads memory after team finishes loading — no "Memory not found" flash', async () => {
      // Render once with isLoadingTeam: true, then rerender with isLoadingTeam: false
      // using the SAME router to test the in-component transition (not a remount).
      mockUseTeam.mockReturnValueOnce({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as jest.Mock).mockResolvedValue(mockMemory)
      ;(projectService.getProjects as jest.Mock).mockResolvedValue({
        projects: [mockProject],
        page: 1,
        per_page: 100,
        total_count: 1,
        total_pages: 1,
      })

      const { rerender } = renderMemoryView()

      // Team still loading — service must not be called
      expect(memoryService.getMemory).not.toHaveBeenCalled()
      expect(screen.queryByText('Memory not found')).not.toBeInTheDocument()

      // Team resolves: update the mock and rerender WITHOUT rebuilding the MemoryRouter
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      rerender(
        <MemoryRouter initialEntries={['/memories/memory-1']}>
          <Routes>
            <Route path="/memories/:id" element={<MemoryView />} />
          </Routes>
        </MemoryRouter>
      )

      await waitFor(() => {
        const renderer = screen.getByTestId('markdown-renderer')
        expect(renderer).toHaveTextContent('This is memory text content')
      })
      expect(screen.queryByText('Memory not found')).not.toBeInTheDocument()
    })
  })

  describe('error handling', () => {
    it('shows "Memory not found" when the service throws', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as jest.Mock).mockRejectedValue(
        new Error('Not found')
      )

      renderMemoryView()

      await waitFor(() => {
        // The not-found state renders both a page heading and an AlertTitle with this text
        const matches = screen.getAllByText('Memory not found')
        expect(matches.length).toBeGreaterThan(0)
      })
    })
  })

  describe('genuinely missing resource', () => {
    it('shows not-found state when the team loaded but the memory does not exist', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as jest.Mock).mockRejectedValue(
        new Error('Memory does not exist')
      )

      renderMemoryView('nonexistent-id')

      await waitFor(() => {
        const matches = screen.getAllByText('Memory not found')
        expect(matches.length).toBeGreaterThan(0)
      })
    })
  })
})

import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mock } from 'vitest'

import type { Memory } from '@/services/memoryService'
import type { Project } from '@/services/projectService'
import { storage } from '@/utils/storage'

// Mock TeamContext — stable references to prevent effect re-runs
const mockUseTeam = vi.hoisted(() => vi.fn())
// usePermissions (#225) reads the signed-in user for own-vs-any delete gating.
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/services/memoryService', () => ({
  memoryService: {
    getMemory: vi.fn(),
    getMemoryVersions: vi.fn().mockResolvedValue({ versions: [] }),
    deleteMemory: vi.fn(),
  },
}))

vi.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: vi.fn(),
  },
}))

vi.mock('@/hooks', () => {
  const showSuccess = vi.fn()
  const trackEvent = vi.fn()
  return {
    useAlerts: () => ({ showSuccess }),
    useAnalytics: () => ({ trackEvent }),
  }
})

vi.mock('@/hooks/useErrorHandler', () => {
  const handleError = vi.fn()
  return {
    useErrorHandler: () => ({ handleError }),
  }
})

// Mock MarkdownRenderer to verify content is passed through it
vi.mock('@/components/MarkdownRenderer', () => ({
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

      renderMemoryView()

      expect(screen.getByText('Loading memory…')).toBeInTheDocument()
      expect(memoryService.getMemory).not.toHaveBeenCalled()
    })

    it('does not show "Memory not found" while team is loading', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockResolvedValue(mockMemory)
      ;(projectService.getProjects as Mock).mockResolvedValue({
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockResolvedValue(memoryWithMarkdown)
      ;(projectService.getProjects as Mock).mockResolvedValue({
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderMemoryView()

      expect(screen.getByText('Loading memory…')).toBeInTheDocument()
    })

    it('renders a derived title instead of the raw id, with no static subtitle', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockResolvedValue(mockMemory)
      ;(projectService.getProjects as Mock).mockResolvedValue({
        projects: [mockProject],
        page: 1,
        per_page: 100,
        total_count: 1,
        total_pages: 1,
      })

      renderMemoryView()

      await waitFor(() => {
        expect(
          screen.getByRole('heading', {
            level: 1,
            name: 'This is memory text content',
          })
        ).toBeInTheDocument()
      })
      expect(screen.queryByText('Memory #memory-1')).not.toBeInTheDocument()
      expect(screen.queryByText('View memory details.')).not.toBeInTheDocument()
      // Scoped to the header: the descriptor-generated Metadata section
      // carries its own Status row (#903).
      const header = screen.getByTestId('resource-header-meta')
      expect(within(header).getByText('Active')).toBeInTheDocument()
      expect(within(header).getByText('Updated')).toBeInTheDocument()
    })

    it('prefers the memory body\u2019s first markdown heading as the title', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockResolvedValue({
        ...mockMemory,
        text: '# Deployment runbook\n\nDrain the node first.',
      })

      renderMemoryView()

      await waitFor(() => {
        expect(
          screen.getByRole('heading', { level: 1, name: 'Deployment runbook' })
        ).toBeInTheDocument()
      })
    })

    it('shows the project name in the sidebar when project loads', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockResolvedValue(mockMemory)
      ;(projectService.getProjects as Mock).mockResolvedValue({
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockResolvedValue(mockMemory)
      ;(projectService.getProjects as Mock).mockResolvedValue({
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockResolvedValue(mockMemory)
      ;(projectService.getProjects as Mock).mockResolvedValue({
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockRejectedValue(
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockRejectedValue(
        new Error('Memory does not exist')
      )

      renderMemoryView('nonexistent-id')

      await waitFor(() => {
        const matches = screen.getAllByText('Memory not found')
        expect(matches.length).toBeGreaterThan(0)
      })
    })
  })

  describe('body view switch (#901)', () => {
    beforeEach(() => {
      storage.clear()
    })

    it('renders the memory body through ResourceBody, with a Raw view of the source', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockResolvedValue(mockMemory)
      const user = userEvent.setup()
      renderMemoryView()

      // Rendered by default, through the shared body — not a bare renderer.
      const body = await screen.findByTestId('resource-body')
      expect(within(body).getByTestId('markdown-renderer')).toHaveTextContent(
        'This is memory text content'
      )

      await user.click(within(body).getByRole('tab', { name: 'Raw' }))

      expect(screen.getByTestId('resource-body-raw')).toHaveTextContent(
        'This is memory text content'
      )
      // Exactly one view is mounted, so the body is never in the DOM twice.
      expect(screen.queryByTestId('markdown-renderer')).not.toBeInTheDocument()
    })
  })
  // The Metadata panel's version-history affordance comes from
  // `useResourceVersions` (#905); these pin this page's own wiring — the route
  // it builds and the count it derives.
  describe('version history', () => {
    beforeEach(() => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(memoryService.getMemory as Mock).mockResolvedValue(mockMemory)
    })

    it('renders the version-history link with the total-version count chip', async () => {
      ;(memoryService.getMemoryVersions as Mock).mockResolvedValue({
        versions: [
          { id: 'v2', version_number: 2 },
          { id: 'v1', version_number: 1 },
        ],
      })

      renderMemoryView()

      const link = await screen.findByTestId('metadata-version-history-link')
      expect(link).toHaveTextContent('View version history')
      expect(link).toHaveTextContent('2')
      expect(link).toHaveAttribute('href', '/memories/memory-1/versions')
      expect(memoryService.getMemoryVersions).toHaveBeenCalledWith(
        'team-1',
        'memory-1'
      )
    })

    it('hides the version-history footer when there is no history yet', async () => {
      ;(memoryService.getMemoryVersions as Mock).mockResolvedValue({
        versions: [],
      })

      renderMemoryView()

      await screen.findByTestId('resource-body')
      expect(
        screen.queryByTestId('metadata-version-history-link')
      ).not.toBeInTheDocument()
    })

    it('keeps the page usable when the versions fetch fails', async () => {
      ;(memoryService.getMemoryVersions as Mock).mockRejectedValue(
        new Error('boom')
      )

      renderMemoryView()

      await screen.findByTestId('resource-body')
      expect(
        screen.queryByTestId('metadata-version-history-link')
      ).not.toBeInTheDocument()
    })
  })

  describe('unified taxonomy section (#904)', () => {
    it('lifts metadata.tags to chips and leaves the other keys as pairs', async () => {
      ;(memoryService.getMemory as Mock).mockResolvedValue({
        ...mockMemory,
        metadata: { tags: ['alpha', 'beta'], type: 'note' },
      })

      renderMemoryView()

      await screen.findByTestId('resource-body')
      expect(screen.getByText('Labels & metadata')).toBeInTheDocument()
      expect(screen.getByText('Tags')).toBeInTheDocument()
      expect(screen.getByText('alpha')).toBeInTheDocument()
      expect(screen.getByText('beta')).toBeInTheDocument()
      expect(screen.getByText('Type')).toBeInTheDocument()
      expect(screen.getByText('note')).toBeInTheDocument()
    })

    it('renders no section and no "No metadata." note when there is nothing to group by', async () => {
      ;(memoryService.getMemory as Mock).mockResolvedValue({
        ...mockMemory,
        metadata: {},
      })

      renderMemoryView()

      await screen.findByTestId('resource-body')
      // The bespoke "No metadata." fallback is gone: an empty taxonomy renders
      // nothing at all, like every other kind always did.
      expect(screen.queryByText('No metadata.')).not.toBeInTheDocument()
      expect(screen.queryByText('Labels & metadata')).not.toBeInTheDocument()
    })
  })
})

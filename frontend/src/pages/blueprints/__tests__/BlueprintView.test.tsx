import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Blueprint } from '@/types'

// Mock MarkdownRenderer to avoid marked/DOMPurify JSDOM issues
jest.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-renderer">{content}</div>
  ),
}))

// Mock TeamContext — stable references to prevent effect re-runs
const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

jest.mock('@/services/blueprintService', () => ({
  blueprintService: {
    getBlueprint: jest.fn(),
    deleteBlueprint: jest.fn(),
  },
}))

// BlueprintView renders ResourceAttachments, which loads attachments on mount.
jest.mock('@/services/attachmentService', () => ({
  attachmentService: {
    list: jest.fn().mockResolvedValue({
      attachments: [],
      total_count: 0,
      total_size_bytes: 0,
    }),
    upload: jest.fn(),
    remove: jest.fn(),
    download: jest.fn(),
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

import { attachmentService } from '@/services/attachmentService'
import { blueprintService } from '@/services/blueprintService'

import { BlueprintView } from '../BlueprintView'

const mockBlueprint: Blueprint = {
  id: 'blueprint-1',
  project_id: 'my-project',
  slug: 'my-blueprint',
  user_id: 'user-1',
  content: '# Blueprint content',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  status: 'active',
  title: 'My Blueprint Title',
  description: 'A test blueprint',
  type: 'general',
  metadata: {},
}

function renderBlueprintView(project = 'my-project', slug = 'my-blueprint') {
  return render(
    <MemoryRouter initialEntries={[`/blueprints/${project}/${slug}`]}>
      <Routes>
        <Route path="/blueprints/:project/:slug" element={<BlueprintView />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('BlueprintView', () => {
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

      renderBlueprintView()

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

      renderBlueprintView()

      expect(screen.queryByText('Blueprint not found')).not.toBeInTheDocument()
    })
  })

  describe('when TeamContext finishes loading with a team', () => {
    it('loads and renders the blueprint after team resolves', async () => {
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

      renderBlueprintView()

      await waitFor(() => {
        expect(screen.getByText('My Blueprint Title')).toBeInTheDocument()
      })
      expect(blueprintService.getBlueprint).toHaveBeenCalledWith(
        'team-1',
        'my-project',
        'my-blueprint'
      )
      // Attachments panel is wired with owner_type="blueprint" and the blueprint id.
      await waitFor(() => {
        expect(attachmentService.list).toHaveBeenCalledWith(
          'team-1',
          'blueprint',
          'blueprint-1'
        )
      })
    })

    it('shows loading spinner while fetch is in flight', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(blueprintService.getBlueprint as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderBlueprintView()

      expect(screen.getByText('Loading blueprint…')).toBeInTheDocument()
    })
  })

  describe('direct URL load (simulate isLoadingTeam flipping from true to false)', () => {
    it('loads blueprint after team finishes loading — no "Blueprint not found" flash', async () => {
      // Render once with isLoadingTeam: true, then rerender with isLoadingTeam: false
      // using the SAME router to test the in-component transition (not a remount).
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

      const { rerender } = renderBlueprintView()

      // Team still loading — service must not be called
      expect(blueprintService.getBlueprint).not.toHaveBeenCalled()
      expect(screen.queryByText('Blueprint not found')).not.toBeInTheDocument()

      // Team resolves: update the mock and rerender WITHOUT rebuilding the MemoryRouter
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      rerender(
        <MemoryRouter initialEntries={['/blueprints/my-project/my-blueprint']}>
          <Routes>
            <Route
              path="/blueprints/:project/:slug"
              element={<BlueprintView />}
            />
          </Routes>
        </MemoryRouter>
      )

      await waitFor(() => {
        expect(screen.getByText('My Blueprint Title')).toBeInTheDocument()
      })
      expect(screen.queryByText('Blueprint not found')).not.toBeInTheDocument()
    })
  })

  describe('error handling', () => {
    it('shows "Blueprint not found" when the service throws', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(blueprintService.getBlueprint as jest.Mock).mockRejectedValue(
        new Error('Not found')
      )

      renderBlueprintView()

      await waitFor(() => {
        // The not-found state renders both a page heading and an AlertTitle with this text
        const matches = screen.getAllByText('Blueprint not found')
        expect(matches.length).toBeGreaterThan(0)
      })
    })
  })

  describe('genuinely missing resource', () => {
    it('shows not-found state when the team loaded but the blueprint does not exist', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(blueprintService.getBlueprint as jest.Mock).mockRejectedValue(
        new Error('Blueprint does not exist')
      )

      renderBlueprintView('no-project', 'no-blueprint')

      await waitFor(() => {
        const matches = screen.getAllByText('Blueprint not found')
        expect(matches.length).toBeGreaterThan(0)
      })
    })
  })
})

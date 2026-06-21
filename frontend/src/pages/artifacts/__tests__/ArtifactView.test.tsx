import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Artifact } from '@/types'

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

jest.mock('@/services/artifactService', () => ({
  artifactService: {
    getArtifact: jest.fn(),
    getArtifactVersions: jest.fn().mockResolvedValue({ versions: [] }),
    deleteArtifact: jest.fn(),
  },
}))

// ArtifactView renders ResourceAttachments, which loads attachments on mount.
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
  const showError = jest.fn()
  const trackEvent = jest.fn()
  return {
    useAlerts: () => ({ showSuccess, showError }),
    useAnalytics: () => ({ trackEvent }),
  }
})

jest.mock('@/hooks/useErrorHandler', () => {
  const handleError = jest.fn()
  return {
    useErrorHandler: () => ({ handleError }),
  }
})

import { artifactService } from '@/services/artifactService'

import { ArtifactView } from '../ArtifactView'

const mockArtifact: Artifact = {
  id: 'artifact-1',
  project_id: 'my-project',
  slug: 'my-artifact',
  user_id: 'user-1',
  content: 'Hello world content',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  status: 'active',
  title: 'My Artifact Title',
  description: 'A test artifact',
  type: 'general',
  metadata: {},
}

function renderArtifactView(project = 'my-project', slug = 'my-artifact') {
  return render(
    <MemoryRouter initialEntries={[`/artifacts/${project}/${slug}`]}>
      <Routes>
        <Route path="/artifacts/:project/:slug" element={<ArtifactView />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('ArtifactView', () => {
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

      renderArtifactView()

      expect(screen.getByText('Loading artifact…')).toBeInTheDocument()
      expect(artifactService.getArtifact).not.toHaveBeenCalled()
    })

    it('does not show "Artifact not found" while team is loading', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })

      renderArtifactView()

      expect(screen.queryByText('Artifact not found')).not.toBeInTheDocument()
    })
  })

  describe('when TeamContext finishes loading with a team', () => {
    it('loads and renders the artifact after team resolves', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as jest.Mock).mockResolvedValue(
        mockArtifact
      )

      renderArtifactView()

      await waitFor(() => {
        expect(screen.getByText('My Artifact Title')).toBeInTheDocument()
      })
      expect(artifactService.getArtifact).toHaveBeenCalledWith(
        'team-1',
        'my-project',
        'my-artifact'
      )
    })

    it('shows loading spinner while fetch is in flight', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as jest.Mock).mockImplementation(
        () => new Promise(() => undefined)
      )

      renderArtifactView()

      expect(screen.getByText('Loading artifact…')).toBeInTheDocument()
    })
  })

  describe('direct URL load (simulate isLoadingTeam flipping from true to false)', () => {
    it('loads artifact after team finishes loading — no "Artifact not found" flash', async () => {
      // Render once with isLoadingTeam: true, then rerender with isLoadingTeam: false
      // using the SAME router to test the in-component transition (not a remount).
      mockUseTeam.mockReturnValueOnce({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as jest.Mock).mockResolvedValue(
        mockArtifact
      )

      const { rerender } = renderArtifactView()

      // Team still loading — service must not be called
      expect(artifactService.getArtifact).not.toHaveBeenCalled()
      expect(screen.queryByText('Artifact not found')).not.toBeInTheDocument()

      // Team resolves: update the mock and rerender WITHOUT rebuilding the MemoryRouter
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      rerender(
        <MemoryRouter initialEntries={['/artifacts/my-project/my-artifact']}>
          <Routes>
            <Route
              path="/artifacts/:project/:slug"
              element={<ArtifactView />}
            />
          </Routes>
        </MemoryRouter>
      )

      await waitFor(() => {
        expect(screen.getByText('My Artifact Title')).toBeInTheDocument()
      })
      expect(screen.queryByText('Artifact not found')).not.toBeInTheDocument()
    })
  })

  describe('version history footer', () => {
    beforeEach(() => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as jest.Mock).mockResolvedValue(
        mockArtifact
      )
    })

    it('renders the version-history link with the total-version count chip', async () => {
      ;(artifactService.getArtifactVersions as jest.Mock).mockResolvedValue({
        versions: [
          { id: 'v2', version_number: 2 },
          { id: 'v1', version_number: 1 },
        ],
      })

      renderArtifactView()

      const link = await screen.findByTestId('metadata-version-history-link')
      expect(link).toHaveTextContent('View version history')
      expect(link).toHaveTextContent('2')
      expect(link).toHaveAttribute(
        'href',
        '/artifacts/my-project/my-artifact/versions'
      )
    })

    it('no longer renders the old header version-history button', async () => {
      renderArtifactView()

      await waitFor(() => {
        expect(screen.getByText('My Artifact Title')).toBeInTheDocument()
      })
      expect(
        screen.queryByTestId('artifact-versions-button')
      ).not.toBeInTheDocument()
    })

    it('hides the version-history footer when there is no history yet', async () => {
      ;(artifactService.getArtifactVersions as jest.Mock).mockResolvedValue({
        versions: [],
      })

      renderArtifactView()

      await waitFor(() => {
        expect(screen.getByText('My Artifact Title')).toBeInTheDocument()
      })
      expect(
        screen.queryByTestId('metadata-version-history-link')
      ).not.toBeInTheDocument()
      expect(screen.queryByText('Version')).not.toBeInTheDocument()
    })

    it('keeps the version row resilient to a versions-fetch failure', async () => {
      ;(artifactService.getArtifactVersions as jest.Mock).mockRejectedValue(
        new Error('boom')
      )

      renderArtifactView()

      // Artifact still renders; footer simply stays hidden (empty history).
      await waitFor(() => {
        expect(screen.getByText('My Artifact Title')).toBeInTheDocument()
      })
      expect(
        screen.queryByTestId('metadata-version-history-link')
      ).not.toBeInTheDocument()
    })
  })

  describe('error handling', () => {
    it('shows "Artifact not found" when the service throws', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as jest.Mock).mockRejectedValue(
        new Error('Not found')
      )

      renderArtifactView()

      await waitFor(() => {
        // The not-found state renders both a page heading and an AlertTitle with this text
        const matches = screen.getAllByText('Artifact not found')
        expect(matches.length).toBeGreaterThan(0)
      })
    })
  })

  describe('genuinely missing resource', () => {
    it('shows not-found state when the team loaded but the artifact does not exist', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as jest.Mock).mockRejectedValue(
        new Error('Artifact does not exist')
      )

      renderArtifactView('no-project', 'no-artifact')

      await waitFor(() => {
        const matches = screen.getAllByText('Artifact not found')
        expect(matches.length).toBeGreaterThan(0)
      })
    })
  })
})

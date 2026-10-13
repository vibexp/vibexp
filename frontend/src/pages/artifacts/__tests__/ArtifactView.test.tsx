import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mock } from 'vitest'

import { openMoreActions } from '@/components/patterns/reading-page/testing'
import type { Artifact } from '@/services/artifactService'
import { storage } from '@/utils/storage'

// The team's status catalog (#1263), settable per test. Empty and answered by
// default — a team with no custom statuses.
const statusCatalog = vi.hoisted(() => ({
  statuses: [] as unknown[],
  isLoading: false,
}))
vi.mock('@/hooks/useStatusCatalog', () => ({
  useStatusCatalog: () => statusCatalog,
}))
afterEach(() => {
  statusCatalog.statuses = []
})

// Mock MarkdownRenderer to avoid marked/DOMPurify JSDOM issues
vi.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-renderer">{content}</div>
  ),
}))

// Mock TeamContext — stable references to prevent effect re-runs
const mockUseTeam = vi.hoisted(() => vi.fn())
// usePermissions (#225) reads the signed-in user for own-vs-any delete gating.
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/services/artifactService', () => ({
  artifactService: {
    getArtifact: vi.fn(),
    getArtifactVersions: vi.fn().mockResolvedValue({ versions: [] }),
    deleteArtifact: vi.fn(),
    lockArtifact: vi.fn(),
    unlockArtifact: vi.fn(),
    getArtifactLockHistory: vi.fn().mockResolvedValue([]),
  },
}))

// ArtifactView renders ResourceAttachments, which loads attachments on mount.
vi.mock('@/services/attachmentService', () => ({
  attachmentService: {
    list: vi.fn().mockResolvedValue({
      attachments: [],
      total_count: 0,
      total_size_bytes: 0,
    }),
    upload: vi.fn(),
    remove: vi.fn(),
    download: vi.fn(),
  },
}))

// Mocked only to prove it is never called: the Project row reads the payload's
// `project` summary (#956), where a list lookup was capped at 100 projects.
vi.mock('@/services/projectService', () => ({
  projectService: {
    getProjects: vi.fn(),
  },
}))

vi.mock('@/hooks', () => {
  const showSuccess = vi.fn()
  const showError = vi.fn()
  const trackEvent = vi.fn()
  return {
    useAlerts: () => ({ showSuccess, showError }),
    useAnalytics: () => ({ trackEvent }),
  }
})

vi.mock('@/hooks/useErrorHandler', () => {
  const handleError = vi.fn()
  return {
    useErrorHandler: () => ({ handleError }),
  }
})

import { artifactService } from '@/services/artifactService'
import { projectService } from '@/services/projectService'

import { ArtifactView } from '../ArtifactView'

const mockArtifact: Artifact = vi.hoisted(() => ({
  last_write_source: null,
  last_write_assistant_name: null,
  lock: null,
  id: 'artifact-1',
  project_id: 'my-project',
  project: { id: 'my-project', name: 'My Project', slug: 'my-project-slug' },
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
  labels: [],
}))

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

      renderArtifactView()

      expect(screen.getByText('Loading artifact…')).toBeInTheDocument()
      expect(artifactService.getArtifact).not.toHaveBeenCalled()
    })

    it('does not show "Artifact not found" while team is loading', () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })

      renderArtifactView()

      expect(screen.queryByText('Artifact not found')).not.toBeInTheDocument()
    })
  })

  describe('when TeamContext finishes loading with a team', () => {
    it('shows a custom status by its display name, once, in the header (#1263)', async () => {
      statusCatalog.statuses = [
        { slug: 'in-review', name: 'In review', is_system: false },
      ]
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue({
        ...mockArtifact,
        status: 'in-review',
      })
      renderArtifactView()

      const header = await screen.findByTestId('resource-header-meta')
      expect(within(header).getByText('In review')).toHaveClass('bg-muted')
      // Once: the rail's details no longer repeat what the header shows.
      expect(screen.getAllByText('In review')).toHaveLength(1)
      expect(screen.queryByText('in-review')).not.toBeInTheDocument()
    })

    it('shows the raw slug for a status the catalog no longer has (#1263)', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue({
        ...mockArtifact,
        status: 'in-review',
      })
      renderArtifactView()

      const header = await screen.findByTestId('resource-header-meta')
      expect(within(header).getByText('in-review')).toHaveClass('bg-muted')
      expect(screen.getAllByText('in-review')).toHaveLength(1)
    })

    it('renders the standard header: status and updated time, with the slug left to the rail (#902)', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue(mockArtifact)

      renderArtifactView()

      const header = await screen.findByTestId('resource-header-meta')
      // Scoped to the header: the descriptor-generated Metadata section
      // carries its own Status and Slug rows (#903).
      expect(within(header).getByText('Active')).toBeInTheDocument()
      // The slug is a detail: it lives in the rail, not twice on the page.
      expect(
        within(header).queryByRole('button', { name: /Copy slug/ })
      ).not.toBeInTheDocument()
      expect(screen.getAllByText('my-artifact')).toHaveLength(1)
      expect(within(header).getByText('Updated')).toBeInTheDocument()
      // The description is the lead paragraph beneath the meta line.
      expect(screen.getByTestId('resource-header-summary')).toHaveTextContent(
        'A test artifact'
      )
    })

    it('loads and renders the artifact after team resolves', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue(mockArtifact)

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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockImplementation(
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue(mockArtifact)

      const { rerender } = renderArtifactView()

      // Team still loading — service must not be called
      expect(artifactService.getArtifact).not.toHaveBeenCalled()
      expect(screen.queryByText('Artifact not found')).not.toBeInTheDocument()

      // Team resolves: update the mock and rerender WITHOUT rebuilding the MemoryRouter
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
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

    it('links the Metadata Project row from the payload summary without listing projects (#903, #956)', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue(mockArtifact)

      renderArtifactView()

      const link = await screen.findByRole('link', { name: /My Project/ })
      expect(link).toHaveAttribute(
        'href',
        '/teams/team-1/projects/my-project-slug/edit'
      )
      expect(projectService.getProjects).not.toHaveBeenCalled()
    })
  })

  describe('version history footer', () => {
    beforeEach(() => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue(mockArtifact)
    })

    it('links the version row to the history, naming the version count', async () => {
      ;(artifactService.getArtifactVersions as Mock).mockResolvedValue({
        versions: [
          { id: 'v2', version_number: 2 },
          { id: 'v1', version_number: 1 },
        ],
      })

      renderArtifactView()

      const link = await screen.findByTestId('metadata-version-history-link')
      // In the rail the version row itself is the link to the history: one
      // row, named for where it goes and how many versions are there.
      expect(link).toHaveAccessibleName('View version history, 2 versions')
      expect(link).toHaveTextContent(/^v\d+/)
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
      ;(artifactService.getArtifactVersions as Mock).mockResolvedValue({
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
      ;(artifactService.getArtifactVersions as Mock).mockRejectedValue(
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockRejectedValue(
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
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockRejectedValue(
        new Error('Artifact does not exist')
      )

      renderArtifactView('no-project', 'no-artifact')

      await waitFor(() => {
        const matches = screen.getAllByText('Artifact not found')
        expect(matches.length).toBeGreaterThan(0)
      })
    })
  })

  describe('body view switch (#901)', () => {
    beforeEach(() => {
      storage.clear()
    })

    it('renders the artifact body through ResourceBody, with a Raw view of the source', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue(mockArtifact)
      const user = userEvent.setup()
      renderArtifactView()

      // Rendered by default, through the shared body — not a bare renderer.
      const body = await screen.findByTestId('resource-body')
      expect(within(body).getByTestId('markdown-renderer')).toHaveTextContent(
        'Hello world content'
      )

      await user.click(screen.getByRole('tab', { name: 'Raw' }))

      expect(screen.getByTestId('resource-body-raw')).toHaveTextContent(
        'Hello world content'
      )
      // Exactly one view is mounted, so the body is never in the DOM twice.
      expect(screen.queryByTestId('markdown-renderer')).not.toBeInTheDocument()
    })
  })
  describe('unified taxonomy section (#904)', () => {
    it('renders the metadata bag under the one unified heading', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue({
        ...mockArtifact,
        metadata: { author: 'ada' },
      })

      renderArtifactView()

      await screen.findByTestId('resource-body')
      expect(screen.getByText('Labels & metadata')).toBeInTheDocument()
      expect(screen.getByText('Author')).toBeInTheDocument()
      expect(screen.getByText('ada')).toBeInTheDocument()
      // The bespoke "Additional data" panel is gone — one heading now.
      expect(screen.queryByText('Additional data')).not.toBeInTheDocument()
    })

    it('renders no section when there is nothing to group by', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue(mockArtifact)

      renderArtifactView()

      await screen.findByTestId('resource-body')
      expect(screen.queryByText('Labels & metadata')).not.toBeInTheDocument()
    })
  })

  describe('resource lock (#1291)', () => {
    const setTeam = (permissions: string[]) => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team', permissions },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
    }
    const mockGet = (value: Artifact) => {
      ;(artifactService.getArtifact as Mock).mockResolvedValue(value)
    }
    beforeEach(() => {
      ;(artifactService.getArtifactLockHistory as Mock).mockResolvedValue([])
    })

    const lockedBy = (userId: string) => ({
      locked_by: { user_id: userId, display_name: 'Ada Lovelace' },
      comment: 'Used by the nightly release agent',
      locked_at: '2026-10-11T10:15:30Z',
    })

    it('offers Lock and Delete on an unlocked artifact, with no lock row', async () => {
      setTeam(['resource.delete.own'])
      mockGet(mockArtifact)

      renderArtifactView()

      await screen.findByTestId('resource-body')
      await openMoreActions()
      expect(screen.getByTestId('lock-artifact-button')).toBeInTheDocument()
      expect(screen.getByTestId('delete-artifact-button')).toBeInTheDocument()
      expect(
        screen.queryByTestId('unlock-artifact-button')
      ).not.toBeInTheDocument()
      expect(screen.queryByText('Locked by')).not.toBeInTheDocument()
    })

    it('shows who locked it and hides Lock, Unlock and Delete from another member', async () => {
      // The reader created the artifact and may delete their own, so only the
      // lock is hiding Delete here.
      setTeam(['resource.delete.own'])
      mockGet({ ...mockArtifact, lock: lockedBy('user-2') })

      renderArtifactView()

      expect(await screen.findByTestId('locked-by')).toHaveTextContent(
        'Ada Lovelace'
      )
      expect(screen.getByText('Locked by')).toBeInTheDocument()
      // The menu is opened first: an action missing from a closed menu proves
      // nothing. (No menu at all means no such action either.)
      await openMoreActions()
      expect(
        screen.queryByTestId('lock-artifact-button')
      ).not.toBeInTheDocument()
      expect(
        screen.queryByTestId('unlock-artifact-button')
      ).not.toBeInTheDocument()
      expect(
        screen.queryByTestId('delete-artifact-button')
      ).not.toBeInTheDocument()
    })

    it('offers Unlock to the locker, still without Delete', async () => {
      setTeam(['resource.delete.own'])
      mockGet({ ...mockArtifact, lock: lockedBy('user-1') })

      renderArtifactView()

      await screen.findByTestId('locked-by')
      await openMoreActions()
      expect(screen.getByTestId('unlock-artifact-button')).toBeInTheDocument()
      expect(
        screen.queryByTestId('delete-artifact-button')
      ).not.toBeInTheDocument()
    })

    it("offers Unlock on someone else's lock to a holder of resource.unlock.any, read from permissions", async () => {
      setTeam(['resource.unlock.any', 'resource.delete.any'])
      mockGet({ ...mockArtifact, lock: lockedBy('user-2') })

      renderArtifactView()

      await screen.findByTestId('locked-by')
      await openMoreActions()
      expect(screen.getByTestId('unlock-artifact-button')).toBeInTheDocument()
      // Not even resource.delete.any deletes a locked artifact.
      expect(
        screen.queryByTestId('delete-artifact-button')
      ).not.toBeInTheDocument()
    })

    it('locks from the dialog and shows the lock without refetching; Delete returns after unlock', async () => {
      const user = userEvent.setup()
      setTeam(['resource.delete.own'])
      mockGet(mockArtifact)
      ;(artifactService.lockArtifact as Mock).mockResolvedValue({
        ...mockArtifact,
        lock: lockedBy('user-1'),
      })
      ;(artifactService.unlockArtifact as Mock).mockResolvedValue(mockArtifact)

      renderArtifactView()

      await screen.findByTestId('resource-body')
      await openMoreActions()
      await user.click(screen.getByTestId('lock-artifact-button'))
      // The dialog does not submit without a comment.
      expect(screen.getByTestId('confirm-lock-button')).toBeDisabled()
      await user.type(
        screen.getByTestId('lock-comment-input'),
        'Used by the nightly release agent'
      )
      await user.click(screen.getByTestId('confirm-lock-button'))

      expect(await screen.findByTestId('locked-by')).toHaveTextContent(
        'Ada Lovelace'
      )
      expect(artifactService.lockArtifact).toHaveBeenCalledWith(
        'team-1',
        'my-project',
        'my-artifact',
        'Used by the nightly release agent'
      )
      await openMoreActions()
      expect(
        screen.queryByTestId('delete-artifact-button')
      ).not.toBeInTheDocument()
      expect(
        screen.queryByTestId('lock-artifact-button')
      ).not.toBeInTheDocument()
      expect(artifactService.getArtifact).toHaveBeenCalledTimes(1)

      await user.click(screen.getByTestId('unlock-artifact-button'))

      await waitFor(() => {
        expect(screen.queryByTestId('locked-by')).not.toBeInTheDocument()
      })
      await openMoreActions()
      expect(screen.getByTestId('delete-artifact-button')).toBeInTheDocument()
      expect(artifactService.unlockArtifact).toHaveBeenCalledWith(
        'team-1',
        'my-project',
        'my-artifact'
      )
      expect(screen.queryByText('Locked by')).not.toBeInTheDocument()
      expect(artifactService.getArtifact).toHaveBeenCalledTimes(1)
    })

    it('lists the lock history in its own section', async () => {
      setTeam([])
      mockGet({ ...mockArtifact, lock: lockedBy('user-2') })
      ;(artifactService.getArtifactLockHistory as Mock).mockResolvedValue([
        {
          id: 'e-2',
          action: 'lock',
          actor: { user_id: 'user-2', display_name: 'Ada Lovelace' },
          comment: 'Used by the nightly release agent',
          created_at: '2026-10-11T10:15:30Z',
        },
        {
          id: 'e-1',
          action: 'unlock',
          actor: null,
          comment: null,
          created_at: '2026-10-10T10:15:30Z',
        },
      ])

      renderArtifactView()

      const events = await screen.findAllByTestId('lock-history-event')
      expect(artifactService.getArtifactLockHistory).toHaveBeenCalledWith(
        'team-1',
        'my-project',
        'my-artifact'
      )
      expect(events.map(event => event.dataset.action)).toEqual([
        'lock',
        'unlock',
      ])
      expect(events[0]).toHaveTextContent(/Ada Lovelace\s*locked/)
      expect(events[1]).toHaveTextContent(/Deleted user\s*unlocked/)
      expect(
        within(screen.getByTestId('lock-history')).getByText('Lock history')
      ).toBeInTheDocument()
    })
  })
})

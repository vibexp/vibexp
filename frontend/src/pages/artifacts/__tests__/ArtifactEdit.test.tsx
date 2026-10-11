import { act, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import type { Mock } from 'vitest'

import type { Artifact } from '@/services/artifactService'

// Stub the generated form: these are TeamContext-lifecycle tests, and the form
// itself has its own suite (`patterns/resource/form`).
vi.mock('@/components/patterns/resource', async () => ({
  ...(await vi.importActual('@/components/patterns/resource')),
  ResourceFormReadingPage: vi.fn(() => <div data-testid="artifact-form" />),
}))

// Mock TeamContext — stable references to prevent effect re-runs
const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/services/artifactService', () => ({
  artifactService: {
    getArtifactVersions: vi.fn(),
    getArtifact: vi.fn(),
    updateArtifact: vi.fn(),
  },
}))

vi.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: vi.fn() }),
  useAnalytics: () => ({ trackEvent: vi.fn() }),
}))

const mockHandleError = vi.hoisted(() => vi.fn())
vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

import type { ResourceFormReadingPageProps } from '@/components/patterns/resource'
import { ResourceFormReadingPage } from '@/components/patterns/resource'
import { lockedWriteError, TEST_LOCK } from '@/lib/testing/resourceLock'
import { artifactService } from '@/services/artifactService'

import { ArtifactEdit } from '../ArtifactEdit'

const mockArtifact: Artifact = {
  lock: null,
  id: 'artifact-1',
  project_id: 'my-project',
  project: null,
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
}

function renderArtifactEdit(project = 'my-project', slug = 'my-artifact') {
  return render(
    <MemoryRouter initialEntries={[`/artifacts/${project}/${slug}/edit`]}>
      <Routes>
        <Route
          path="/artifacts/:project/:slug/edit"
          element={<ArtifactEdit />}
        />
      </Routes>
    </MemoryRouter>
  )
}

describe('ArtifactEdit', () => {
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

      renderArtifactEdit()

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

      renderArtifactEdit()

      expect(screen.queryByText('Artifact not found')).not.toBeInTheDocument()
    })
  })

  describe('when TeamContext finishes loading with a team', () => {
    it('calls the service and renders the form', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue(mockArtifact)

      renderArtifactEdit()

      await waitFor(() => {
        expect(screen.getByTestId('artifact-form')).toBeInTheDocument()
      })
      expect(artifactService.getArtifact).toHaveBeenCalledWith(
        'team-1',
        'my-project',
        'my-artifact'
      )
    })
  })

  describe('when TeamContext finishes loading without a team', () => {
    it('shows "No team available" error message', async () => {
      mockUseTeam.mockReturnValue({
        currentTeam: null,
        teams: [],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })

      renderArtifactEdit()

      await waitFor(() => {
        expect(
          screen.getByText(
            'No team available. Please select or create a team first.'
          )
        ).toBeInTheDocument()
      })
      expect(artifactService.getArtifact).not.toHaveBeenCalled()
    })
  })

  describe('race condition — isLoadingTeam transitions true → false', () => {
    it('loads artifact after team resolves without showing "Artifact not found"', async () => {
      mockUseTeam.mockReturnValueOnce({
        currentTeam: null,
        teams: [],
        isLoading: true,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValue(mockArtifact)

      const { rerender } = renderArtifactEdit()

      expect(artifactService.getArtifact).not.toHaveBeenCalled()
      expect(screen.queryByText('Artifact not found')).not.toBeInTheDocument()

      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      rerender(
        <MemoryRouter
          initialEntries={['/artifacts/my-project/my-artifact/edit']}
        >
          <Routes>
            <Route
              path="/artifacts/:project/:slug/edit"
              element={<ArtifactEdit />}
            />
          </Routes>
        </MemoryRouter>
      )

      await waitFor(() => {
        expect(screen.getByTestId('artifact-form')).toBeInTheDocument()
      })
      expect(screen.queryByText('Artifact not found')).not.toBeInTheDocument()
    })
  })

  // #1180: the column's Attachments section and Version row need the page to
  // hand over the resource and its version history.
  it('passes the resource and its version history to the form', async () => {
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-1', name: 'Test Team' },
      teams: [{ id: 'team-1', name: 'Test Team' }],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn() as () => Promise<void>,
    })
    ;(artifactService.getArtifact as Mock).mockResolvedValue(mockArtifact)
    ;(artifactService.getArtifactVersions as Mock).mockResolvedValue({
      versions: [{ version_number: 1 }, { version_number: 2 }],
    })

    renderArtifactEdit()

    await waitFor(() => {
      expect(ResourceFormReadingPage).toHaveBeenLastCalledWith(
        expect.objectContaining({
          resource: {
            kind: 'artifact',
            id: mockArtifact.id,
            teamId: 'team-1',
          },
          versionHistory: expect.objectContaining({
            to: '/artifacts/my-project/my-artifact/versions',
            count: 2,
            currentVersion: 3,
          }),
        }),
        undefined
      )
    })
    expect(artifactService.getArtifactVersions).toHaveBeenCalledWith(
      'team-1',
      'my-project',
      'my-artifact'
    )
  })

  // #1285: the fields a lock freezes are the form's business; the page's is to
  // hand it the lock, and to find out about one taken after the page loaded.
  describe('a locked artifact', () => {
    const lockedArtifact: Artifact = {
      ...mockArtifact,
      status: 'archived',
      lock: TEST_LOCK,
    }

    /** The props of the form's latest render. */
    function formProps(): ResourceFormReadingPageProps {
      const call = (ResourceFormReadingPage as Mock).mock.lastCall
      return (call as [ResourceFormReadingPageProps])[0]
    }

    async function renderLoaded(loaded: Artifact) {
      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: vi.fn(),
        refreshTeams: vi.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as Mock).mockResolvedValueOnce(loaded)
      ;(artifactService.getArtifactVersions as Mock).mockResolvedValue({
        versions: [],
      })
      renderArtifactEdit()
      await screen.findByTestId('artifact-form')
    }

    it('hands the loaded lock to the form', async () => {
      await renderLoaded(lockedArtifact)

      expect(formProps().lock).toEqual(TEST_LOCK)
      // Nothing was refetched, so the loaded resource is the only record.
      expect(formProps().record).toBeUndefined()
    })

    it('hands the form no lock for an unlocked artifact', async () => {
      await renderLoaded(mockArtifact)

      expect(formProps().lock).toBeNull()
    })

    it('refetches the lock when a save is refused, without re-seeding the form', async () => {
      await renderLoaded(mockArtifact)
      const refused = lockedWriteError()
      ;(artifactService.updateArtifact as Mock).mockRejectedValue(refused)
      ;(artifactService.getArtifact as Mock).mockResolvedValueOnce(
        lockedArtifact
      )

      await act(async () => {
        await formProps().onSubmit({ title: 'Typed by hand' })
      })

      expect(mockHandleError).toHaveBeenCalledWith(
        refused,
        'Failed to update artifact'
      )
      expect(artifactService.getArtifact).toHaveBeenCalledTimes(2)
      expect(artifactService.getArtifact).toHaveBeenLastCalledWith(
        'team-1',
        'my-project',
        'my-artifact'
      )
      expect(formProps().lock).toEqual(TEST_LOCK)
      // The refetch arrives as `record`, which only the frozen fields read…
      expect(formProps().record).toBe(lockedArtifact)
      // …while the seed is still the object the page loaded: a new seed would
      // reset the form and discard what the reader typed.
      expect(formProps().initialValues).toBe(mockArtifact)
      // The page never went back to its loading state, which would have
      // unmounted the form.
      expect(screen.getByTestId('artifact-form')).toBeInTheDocument()
      expect(formProps().isLoading).toBe(false)
    })

    it('does not refetch when a save fails for any other reason', async () => {
      await renderLoaded(mockArtifact)
      ;(artifactService.updateArtifact as Mock).mockRejectedValue(
        new Error('offline')
      )

      await act(async () => {
        await formProps().onSubmit({ title: 'Typed by hand' })
      })

      expect(mockHandleError).toHaveBeenCalled()
      expect(artifactService.getArtifact).toHaveBeenCalledTimes(1)
      expect(formProps().lock).toBeNull()
      expect(formProps().record).toBeUndefined()
    })

    it('keeps the page as it was when the refetch itself fails', async () => {
      await renderLoaded(mockArtifact)
      ;(artifactService.updateArtifact as Mock).mockRejectedValue(
        lockedWriteError()
      )
      ;(artifactService.getArtifact as Mock).mockRejectedValueOnce(
        new Error('offline')
      )

      await act(async () => {
        await formProps().onSubmit({ title: 'Typed by hand' })
      })

      expect(artifactService.getArtifact).toHaveBeenCalledTimes(2)
      expect(screen.getByTestId('artifact-form')).toBeInTheDocument()
      expect(formProps().lock).toBeNull()
    })
  })
})

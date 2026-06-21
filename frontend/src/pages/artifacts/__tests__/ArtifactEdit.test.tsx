import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { Artifact } from '@/types'

// Mock ArtifactForm to avoid complex form internals in unit tests
jest.mock('@/pages/artifacts/ArtifactForm', () => ({
  ArtifactForm: jest.fn(() => <div data-testid="artifact-form" />),
}))

// Mock TeamContext — stable references to prevent effect re-runs
const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

jest.mock('@/services/artifactService', () => ({
  artifactService: {
    getArtifact: jest.fn(),
  },
}))

jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: jest.fn() }),
  useAnalytics: () => ({ trackEvent: jest.fn() }),
}))

jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: jest.fn() }),
}))

import { artifactService } from '@/services/artifactService'

import { ArtifactEdit } from '../ArtifactEdit'

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

      renderArtifactEdit()

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
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as jest.Mock).mockResolvedValue(
        mockArtifact
      )

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
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
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
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
      })
      ;(artifactService.getArtifact as jest.Mock).mockResolvedValue(
        mockArtifact
      )

      const { rerender } = renderArtifactEdit()

      expect(artifactService.getArtifact).not.toHaveBeenCalled()
      expect(screen.queryByText('Artifact not found')).not.toBeInTheDocument()

      mockUseTeam.mockReturnValue({
        currentTeam: { id: 'team-1', name: 'Test Team' },
        teams: [{ id: 'team-1', name: 'Test Team' }],
        isLoading: false,
        setCurrentTeam: jest.fn(),
        refreshTeams: jest.fn() as () => Promise<void>,
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
})

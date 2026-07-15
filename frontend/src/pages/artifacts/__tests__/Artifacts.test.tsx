import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import type { Project } from '@/services/projectService'

// Mock Radix Select — it can loop in JSDOM (same approach as Feeds.test.tsx)
jest.mock('@/components/ui/select', () => ({
  Select: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select">{children}</div>
  ),
  SelectTrigger: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-trigger">{children}</div>
  ),
  SelectValue: ({ placeholder }: { placeholder?: string }) => (
    <span>{placeholder}</span>
  ),
  SelectContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="select-content">{children}</div>
  ),
  SelectItem: ({
    children,
    value,
  }: {
    children: React.ReactNode
    value: string
  }) => <div data-value={value}>{children}</div>,
}))

jest.mock('@/services/artifactService', () => ({
  artifactService: {
    getArtifacts: jest.fn(),
    deleteArtifact: jest.fn(),
  },
}))

jest.mock('@/hooks/useTypes', () => ({
  useTypes: () => ({ types: [], loading: false }),
}))

// Mock TeamContext — stable references
// usePermissions (#225) reads the signed-in user for own-vs-any delete gating.
jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

jest.mock('@/contexts/TeamContext', () => {
  const currentTeam = { id: 'team-1', name: 'Test Team' }
  return {
    useTeam: () => ({ currentTeam, teams: [currentTeam], isLoading: false }),
  }
})

// Mock ProjectContext — mutable so tests choose the global selection
const projectContextValue: {
  currentProject: Project | null
  setCurrentProject: jest.Mock
  isLoading: boolean
} = {
  currentProject: null,
  setCurrentProject: jest.fn(),
  isLoading: false,
}
jest.mock('@/contexts/ProjectContext', () => ({
  useProject: () => projectContextValue,
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

import React from 'react'

import { artifactService } from '@/services/artifactService'

import { Artifacts } from '../Artifacts'

const emptyResponse = {
  artifacts: [],
  total_count: 0,
  page: 1,
  per_page: 20,
  total_pages: 0,
}

const alpha: Project = {
  id: 'p1',
  user_id: 'user-1',
  team_id: 'team-1',
  name: 'Alpha Project',
  slug: 'alpha-project',
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  version: 1,
  github_connected: false,
}

function renderArtifacts() {
  return render(
    <MemoryRouter>
      <Artifacts />
    </MemoryRouter>
  )
}

describe('Artifacts page — global project filter', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    projectContextValue.currentProject = null
    projectContextValue.isLoading = false
    ;(artifactService.getArtifacts as jest.Mock).mockResolvedValue(
      emptyResponse
    )
  })

  it('fetches without project_id under "All projects"', async () => {
    renderArtifacts()

    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ project_id: undefined })
      )
    })
  })

  it('fetches scoped to the globally selected project', async () => {
    projectContextValue.currentProject = alpha
    renderArtifacts()

    await waitFor(() => {
      expect(artifactService.getArtifacts).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ project_id: 'p1' })
      )
    })
  })

  it('does not fetch while the persisted project selection is restoring', async () => {
    projectContextValue.isLoading = true
    renderArtifacts()

    // Flush pending effects/microtasks, then assert no fetch happened
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(artifactService.getArtifacts).not.toHaveBeenCalled()
  })

  it('shows the filtered empty state when a project is selected', async () => {
    projectContextValue.currentProject = alpha
    renderArtifacts()

    await waitFor(() => {
      expect(
        screen.getByText('No artifacts match your filters')
      ).toBeInTheDocument()
    })
  })
})

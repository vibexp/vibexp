import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'

import type { Team } from '@/services/teamService'
import { ApiError } from '@/types/errors'

// --------------------------------------------------------------------------
// TeamContext is driven per test through this mutable object. It must be a
// module-stable reference: a fresh object per render loops any effect keyed on
// its identity ("Maximum update depth exceeded").
// --------------------------------------------------------------------------
const mockSetCurrentTeam = vi.hoisted(() => vi.fn())
const mockRefreshTeams = vi.hoisted(() => vi.fn())
const teamContext: {
  teams: Team[]
  currentTeam: Team | null
  isLoading: boolean
} = { teams: [], currentTeam: null, isLoading: false }

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    teams: teamContext.teams,
    currentTeam: teamContext.currentTeam,
    isLoading: teamContext.isLoading,
    setCurrentTeam: mockSetCurrentTeam,
    refreshTeams: mockRefreshTeams,
  }),
}))

const mockGetTeamDetails = vi.hoisted(() => vi.fn())
vi.mock('@/services/teamService', () => ({
  teamService: {
    getTeamDetails: (...args: unknown[]) => mockGetTeamDetails(...args),
  },
}))

// The nested routes and the scope header are exercised by their own suites;
// stub them so this file tests the layout's resolution/gating logic only.
vi.mock('@/pages/teams/TeamRoutes', () => ({
  TeamRoutes: ({ team }: { team: Team }) => (
    <div data-testid="team-routes">{team.name}</div>
  ),
}))

vi.mock('@/pages/teams/TeamScopeHeader', () => ({
  TeamScopeHeader: ({ team }: { team: Team }) => (
    <div data-testid="team-scope-header">{team.name}</div>
  ),
}))

import { TeamScopeLayout } from '../TeamScopeLayout'

/** ApiError wraps an RFC 9457 problem-details body, not (message, status). */
const apiError = (status: number, title: string) =>
  new ApiError({
    type: `https://api.vibexp.io/errors/${String(status)}`,
    title,
    status,
    detail: title,
    code: title.toUpperCase().replace(' ', '_'),
    request_id: 'req-test',
    timestamp: '2026-01-01T00:00:00Z',
  })

const makeTeam = (overrides: Partial<Team> = {}): Team => ({
  id: 'team-a',
  owner_id: 'owner-1',
  name: 'Engineering',
  slug: 'engineering',
  description: '',
  is_personal: false,
  permissions: [],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides,
})

function renderAt(id: string) {
  return render(
    <MemoryRouter initialEntries={[`/teams/${id}`]}>
      <Routes>
        <Route path="/teams/:id/*" element={<TeamScopeLayout />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('TeamScopeLayout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    teamContext.teams = []
    teamContext.currentTeam = null
    teamContext.isLoading = false
    mockGetTeamDetails.mockReset()
  })

  it('renders the scoped routes for a team the user belongs to', async () => {
    teamContext.teams = [makeTeam()]

    renderAt('team-a')

    expect(await screen.findByTestId('team-routes')).toHaveTextContent(
      'Engineering'
    )
    // Resolving from the already-loaded list must cost no request - the list
    // only contains teams you belong to, so it *is* the membership check.
    expect(mockGetTeamDetails).not.toHaveBeenCalled()
  })

  it('renders a skeleton, NOT a not-found, while the team list is hydrating', () => {
    teamContext.isLoading = true
    teamContext.teams = []

    renderAt('team-a')

    expect(screen.getByTestId('team-scope-loading')).toBeInTheDocument()
    expect(screen.queryByText(/team unavailable/i)).not.toBeInTheDocument()
    // A premature fetch during hydration would also be wrong: the list may yet
    // contain this team.
    expect(mockGetTeamDetails).not.toHaveBeenCalled()
  })

  it('falls back to fetching when a settled list has no match (stale list)', async () => {
    teamContext.teams = [makeTeam({ id: 'other-team', name: 'Other' })]
    mockGetTeamDetails.mockResolvedValue(makeTeam({ id: 'team-a' }))

    renderAt('team-a')

    expect(await screen.findByTestId('team-routes')).toHaveTextContent(
      'Engineering'
    )
    expect(mockGetTeamDetails).toHaveBeenCalledWith('team-a')
  })

  it('renders the unavailable state when the user is not a member (403)', async () => {
    teamContext.teams = [makeTeam({ id: 'other-team' })]
    mockGetTeamDetails.mockRejectedValue(apiError(403, 'Forbidden'))

    renderAt('team-a')

    expect(await screen.findByText(/team unavailable/i)).toBeInTheDocument()
    expect(screen.queryByTestId('team-routes')).not.toBeInTheDocument()
  })

  it('renders the unavailable state for an unknown team id (404)', async () => {
    teamContext.teams = [makeTeam({ id: 'other-team' })]
    mockGetTeamDetails.mockRejectedValue(apiError(404, 'Not Found'))

    renderAt('nope')

    expect(await screen.findByText(/team unavailable/i)).toBeInTheDocument()
    expect(screen.queryByTestId('team-routes')).not.toBeInTheDocument()
  })

  it('logs an unexpected failure instead of silently reading as "no access"', async () => {
    // A 5xx or network error is also unrenderable, but it is NOT a permission
    // problem - swallowing it would make an outage indistinguishable from being
    // removed from a team.
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => undefined)
    teamContext.teams = [makeTeam({ id: 'other-team' })]
    mockGetTeamDetails.mockRejectedValue(apiError(500, 'Server Error'))

    renderAt('team-a')

    expect(await screen.findByText(/team unavailable/i)).toBeInTheDocument()
    expect(consoleError).toHaveBeenCalledWith(
      'Failed to resolve team for /teams/:id',
      expect.anything()
    )
    consoleError.mockRestore()
  })

  it('does not log an expected 403 as an error', async () => {
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => undefined)
    teamContext.teams = [makeTeam({ id: 'other-team' })]
    mockGetTeamDetails.mockRejectedValue(apiError(403, 'Forbidden'))

    renderAt('team-a')

    expect(await screen.findByText(/team unavailable/i)).toBeInTheDocument()
    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
  })

  it('syncs the ambient team to the URL team when they differ', async () => {
    const urlTeam = makeTeam({ id: 'team-b', name: 'Design' })
    teamContext.teams = [makeTeam({ id: 'team-a' }), urlTeam]
    teamContext.currentTeam = makeTeam({ id: 'team-a' })

    renderAt('team-b')

    await waitFor(() => {
      expect(mockSetCurrentTeam).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'team-b' })
      )
    })
  })

  it('does not re-set the ambient team when it already matches the URL', async () => {
    const team = makeTeam({ id: 'team-a' })
    teamContext.teams = [team]
    teamContext.currentTeam = team

    renderAt('team-a')

    expect(await screen.findByTestId('team-routes')).toBeInTheDocument()
    // Guarding on the id is what prevents an effect -> context change -> effect
    // loop. Without it this assertion fails by a large margin, not by one.
    expect(mockSetCurrentTeam).not.toHaveBeenCalled()
  })
})

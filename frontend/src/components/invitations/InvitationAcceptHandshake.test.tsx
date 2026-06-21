import { render, waitFor } from '@testing-library/react'

import type { Team } from '@/types/team'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const sessionState = new Map<string, string>()
const mockSessionStore = {
  get: jest.fn((key: string) => sessionState.get(key) ?? null),
  getJSON: jest.fn((key: string): unknown => {
    const raw = sessionState.get(key)
    if (raw === undefined) return null
    try {
      return JSON.parse(raw) as unknown
    } catch {
      return null
    }
  }),
  set: jest.fn((key: string, value: unknown) => {
    sessionState.set(
      key,
      typeof value === 'string' ? value : JSON.stringify(value)
    )
  }),
  remove: jest.fn((key: string) => {
    sessionState.delete(key)
  }),
}

jest.mock('@/utils/storage', () => ({
  sessionStore: mockSessionStore,
}))

const mockToastSuccess = jest.fn()
jest.mock('@/lib/toast', () => ({
  toast: {
    success: (...args: unknown[]) => mockToastSuccess(...args),
  },
}))

const mockSetCurrentTeam = jest.fn()
const mockRefreshTeams = jest.fn()
let mockTeams: Team[] = []

jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: null,
    teams: mockTeams,
    setCurrentTeam: mockSetCurrentTeam,
    refreshTeams: mockRefreshTeams,
    isLoading: false,
  }),
}))

const mockGetTeamDetails = jest.fn()
jest.mock('@/services/teamService', () => ({
  teamService: {
    getTeamDetails: (...args: unknown[]) => mockGetTeamDetails(...args),
  },
}))

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------

import { InvitationAcceptHandshake } from './InvitationAcceptHandshake'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const buildTeam = (overrides: Partial<Team> = {}): Team => ({
  id: 'team-1',
  name: 'Engineering',
  slug: 'engineering',
  description: '',
  role: 'member',
  member_count: 3,
  is_personal: false,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides,
})

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('InvitationAcceptHandshake', () => {
  let consoleErrorSpy: jest.SpyInstance

  beforeEach(() => {
    jest.clearAllMocks()
    sessionState.clear()
    mockTeams = []
    consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleErrorSpy.mockRestore()
  })

  it('does nothing when no stash is present', () => {
    render(<InvitationAcceptHandshake />)

    expect(mockSetCurrentTeam).not.toHaveBeenCalled()
    expect(mockToastSuccess).not.toHaveBeenCalled()
  })

  it('switches to the joined team when it is already in the list', async () => {
    const team = buildTeam({ id: 'team-1', name: 'Engineering' })
    mockTeams = [team]
    sessionState.set(
      'vx_invitation_just_accepted',
      JSON.stringify({ team_id: 'team-1', team_name: 'Engineering' })
    )

    render(<InvitationAcceptHandshake />)

    await waitFor(() => {
      expect(mockSetCurrentTeam).toHaveBeenCalledWith(team)
    })
    expect(mockToastSuccess).toHaveBeenCalledWith('Welcome to Engineering!')
    expect(mockSessionStore.remove).toHaveBeenCalledWith(
      'vx_invitation_just_accepted'
    )
  })

  it('refreshes teams when the joined team is missing', async () => {
    const team = buildTeam({ id: 'team-2', name: 'Marketing' })
    mockRefreshTeams.mockResolvedValueOnce([team])
    sessionState.set(
      'vx_invitation_just_accepted',
      JSON.stringify({ team_id: 'team-2', team_name: 'Marketing' })
    )

    render(<InvitationAcceptHandshake />)

    await waitFor(() => {
      expect(mockRefreshTeams).toHaveBeenCalled()
    })
    expect(mockSetCurrentTeam).toHaveBeenCalledWith(team)
    expect(mockToastSuccess).toHaveBeenCalledWith('Welcome to Marketing!')
  })

  it('falls back to getTeamDetails if refresh does not include the team', async () => {
    const team = buildTeam({ id: 'team-3', name: 'Ops' })
    mockRefreshTeams.mockResolvedValueOnce([])
    mockGetTeamDetails.mockResolvedValueOnce(team)
    sessionState.set(
      'vx_invitation_just_accepted',
      JSON.stringify({ team_id: 'team-3', team_name: 'Ops' })
    )

    render(<InvitationAcceptHandshake />)

    await waitFor(() => {
      expect(mockGetTeamDetails).toHaveBeenCalledWith('team-3')
    })
    expect(mockSetCurrentTeam).toHaveBeenCalledWith(team)
  })

  it('still shows a toast when no team can be resolved', async () => {
    mockRefreshTeams.mockResolvedValueOnce([])
    mockGetTeamDetails.mockRejectedValueOnce(new Error('not found'))
    sessionState.set(
      'vx_invitation_just_accepted',
      JSON.stringify({ team_id: 'team-x', team_name: 'X' })
    )

    render(<InvitationAcceptHandshake />)

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith('Welcome to X!')
    })
    expect(mockSetCurrentTeam).not.toHaveBeenCalled()
  })

  it('ignores a malformed stash without throwing', () => {
    sessionState.set('vx_invitation_just_accepted', 'not-json')

    render(<InvitationAcceptHandshake />)

    expect(mockSetCurrentTeam).not.toHaveBeenCalled()
    expect(mockToastSuccess).not.toHaveBeenCalled()
  })
})

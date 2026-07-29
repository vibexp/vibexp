import { render, waitFor } from '@testing-library/react'
import type { MockInstance } from 'vitest'

import type { Team } from '@/services/teamService'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const sessionState = new Map<string, string>()
const mockSessionStore = vi.hoisted(() => ({
  get: vi.fn((key: string) => sessionState.get(key) ?? null),
  getJSON: vi.fn((key: string): unknown => {
    const raw = sessionState.get(key)
    if (raw === undefined) return null
    try {
      return JSON.parse(raw) as unknown
    } catch {
      return null
    }
  }),
  set: vi.fn((key: string, value: unknown) => {
    sessionState.set(
      key,
      typeof value === 'string' ? value : JSON.stringify(value)
    )
  }),
  remove: vi.fn((key: string) => {
    sessionState.delete(key)
  }),
}))

vi.mock('@/utils/storage', () => ({
  sessionStore: mockSessionStore,
}))

const mockToastSuccess = vi.hoisted(() => vi.fn())
vi.mock('@/lib/toast', () => ({
  toast: {
    success: (...args: unknown[]) => mockToastSuccess(...args),
  },
}))

const mockSetCurrentTeam = vi.hoisted(() => vi.fn())
const mockRefreshTeams = vi.hoisted(() => vi.fn())
let mockTeams: Team[] = []

vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    currentTeam: null,
    teams: mockTeams,
    setCurrentTeam: mockSetCurrentTeam,
    refreshTeams: mockRefreshTeams,
    isLoading: false,
  }),
}))

const mockGetTeamDetails = vi.hoisted(() => vi.fn())
vi.mock('@/services/teamService', () => ({
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
  owner_id: 'owner-1',
  name: 'Engineering',
  slug: 'engineering',
  description: '',
  role: 'member',
  // Required since #224; not exercised here.
  permissions: [],
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
  let consoleErrorSpy: MockInstance

  beforeEach(() => {
    vi.clearAllMocks()
    sessionState.clear()
    mockTeams = []
    consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
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

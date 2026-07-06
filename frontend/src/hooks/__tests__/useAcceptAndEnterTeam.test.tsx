import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'

import type { Team } from '@/services/teamService'

// ---------------------------------------------------------------------------
// Mocks (set up before importing the hook under test)
// ---------------------------------------------------------------------------

const mockNavigate = jest.fn()

jest.mock('react-router-dom', () => {
  const actual =
    jest.requireActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

const mockAcceptInvitation = jest.fn()
const mockGetTeamDetails = jest.fn()

jest.mock('@/services/teamService', () => ({
  teamService: {
    acceptInvitation: (...args: unknown[]) => mockAcceptInvitation(...args),
    getTeamDetails: (...args: unknown[]) => mockGetTeamDetails(...args),
  },
}))

const mockToastSuccess = jest.fn()
const mockToastError = jest.fn()

jest.mock('@/lib/toast', () => ({
  toast: {
    success: (...args: unknown[]) => mockToastSuccess(...args),
    error: (...args: unknown[]) => mockToastError(...args),
  },
}))

const mockRefreshTeams = jest.fn()
const mockSetCurrentTeam = jest.fn()

jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => ({
    refreshTeams: mockRefreshTeams,
    setCurrentTeam: mockSetCurrentTeam,
    currentTeam: null,
    teams: [],
    isLoading: false,
  }),
}))

const mockSessionStoreRemove = jest.fn()

jest.mock('@/utils/storage', () => ({
  sessionStore: {
    remove: (...args: unknown[]) => mockSessionStoreRemove(...args),
  },
}))

// ---------------------------------------------------------------------------
// Imports after mocks
// ---------------------------------------------------------------------------

import { useAcceptAndEnterTeam } from '../useAcceptAndEnterTeam'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const buildTeam = (overrides: Partial<Team> = {}): Team => ({
  id: 'team-123',
  owner_id: 'owner-1',
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

const wrapper = ({ children }: { children: ReactNode }) => (
  <MemoryRouter>{children}</MemoryRouter>
)

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useAcceptAndEnterTeam', () => {
  let consoleErrorSpy: jest.SpyInstance

  beforeEach(() => {
    jest.clearAllMocks()
    consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleErrorSpy.mockRestore()
  })

  it('accepts the invitation, switches team, navigates, and toasts', async () => {
    const joinedTeam = buildTeam()
    mockAcceptInvitation.mockResolvedValueOnce({
      team_id: 'team-123',
      team_name: 'Engineering',
      message: 'ok',
    })
    mockRefreshTeams.mockResolvedValueOnce([joinedTeam])

    const { result } = renderHook(() => useAcceptAndEnterTeam(), { wrapper })

    let outcome: Awaited<ReturnType<typeof result.current>> | undefined
    await act(async () => {
      outcome = await result.current('token-abc')
    })

    expect(mockAcceptInvitation).toHaveBeenCalledWith('token-abc')
    expect(mockRefreshTeams).toHaveBeenCalled()
    expect(mockSetCurrentTeam).toHaveBeenCalledWith(joinedTeam)
    expect(mockNavigate).toHaveBeenCalledWith('/settings/teams/team-123')
    expect(mockToastSuccess).toHaveBeenCalledWith('Welcome to Engineering!')
    expect(mockSessionStoreRemove).toHaveBeenCalled()

    expect(outcome).toEqual({
      ok: true,
      team: joinedTeam,
      teamId: 'team-123',
      teamName: 'Engineering',
    })
  })

  it('falls back to getTeamDetails when refreshed list is missing the team', async () => {
    const joinedTeam = buildTeam({ id: 'team-999', name: 'Late Team' })
    mockAcceptInvitation.mockResolvedValueOnce({
      team_id: 'team-999',
      team_name: 'Late Team',
      message: 'ok',
    })
    mockRefreshTeams.mockResolvedValueOnce([buildTeam({ id: 'other' })])
    mockGetTeamDetails.mockResolvedValueOnce(joinedTeam)

    const { result } = renderHook(() => useAcceptAndEnterTeam(), { wrapper })

    await act(async () => {
      await result.current('token-xyz')
    })

    expect(mockGetTeamDetails).toHaveBeenCalledWith('team-999')
    expect(mockSetCurrentTeam).toHaveBeenCalledWith(joinedTeam)
    expect(mockNavigate).toHaveBeenCalledWith('/settings/teams/team-999')
  })

  it('still navigates and toasts when the team lookup fails', async () => {
    mockAcceptInvitation.mockResolvedValueOnce({
      team_id: 'team-555',
      team_name: 'Lost Team',
      message: 'ok',
    })
    mockRefreshTeams.mockResolvedValueOnce([])
    mockGetTeamDetails.mockRejectedValueOnce(new Error('404'))

    const { result } = renderHook(() => useAcceptAndEnterTeam(), { wrapper })

    let outcome: Awaited<ReturnType<typeof result.current>> | undefined
    await act(async () => {
      outcome = await result.current('token-555')
    })

    expect(mockSetCurrentTeam).not.toHaveBeenCalled()
    expect(mockNavigate).toHaveBeenCalledWith('/settings/teams/team-555')
    expect(mockToastSuccess).toHaveBeenCalledWith('Welcome to Lost Team!')
    expect(outcome).toEqual({
      ok: true,
      team: null,
      teamId: 'team-555',
      teamName: 'Lost Team',
    })
  })

  it('returns ok:false and shows an error toast when accept fails', async () => {
    const failure = new Error('boom')
    mockAcceptInvitation.mockRejectedValueOnce(failure)

    const { result } = renderHook(() => useAcceptAndEnterTeam(), { wrapper })

    let outcome: Awaited<ReturnType<typeof result.current>> | undefined
    await act(async () => {
      outcome = await result.current('token-fail')
    })

    expect(mockToastError).toHaveBeenCalled()
    expect(mockNavigate).not.toHaveBeenCalled()
    expect(mockSetCurrentTeam).not.toHaveBeenCalled()
    expect(outcome).toEqual({ ok: false, error: failure })
  })
})

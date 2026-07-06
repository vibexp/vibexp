import type { TeamInvitation } from '../teamService'

const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
  put: jest.fn(),
  delete: jest.fn(),
}

jest.mock('../../lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

// Import the real service after mocking apiClient.
import { teamService } from '../teamService'

const buildInvitation = (
  overrides: Partial<TeamInvitation> = {}
): TeamInvitation => ({
  id: 'inv-1',
  token: 'token-abc',
  team_id: 'team-123',
  team_name: 'Engineering',
  invitee_email: 'invited@example.com',
  role: 'member',
  status: 'pending',
  created_at: '2024-01-01T00:00:00Z',
  expires_at: '2024-12-31T23:59:59Z',
  ...overrides,
})

describe('TeamService.getTeamInvitations', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('calls GET /teams/{id}/invitations and returns the array as-is', async () => {
    const invitations: TeamInvitation[] = [
      buildInvitation(),
      buildInvitation({ id: 'inv-2', invitee_email: 'second@example.com' }),
    ]
    mockApiClient.get.mockResolvedValueOnce(invitations)

    const result = await teamService.getTeamInvitations('team-123')

    expect(mockApiClient.get).toHaveBeenCalledWith(
      '/teams/team-123/invitations'
    )
    expect(result).toEqual(invitations)
    expect(result).toHaveLength(2)
  })

  it('returns an empty array when the backend returns []', async () => {
    mockApiClient.get.mockResolvedValueOnce([])

    const result = await teamService.getTeamInvitations('team-123')

    expect(result).toEqual([])
    expect(Array.isArray(result)).toBe(true)
  })

  it('does not unwrap an envelope — the endpoint already returns a raw array', async () => {
    // Regression guard: a previous bug shape would have called `response.invitations`.
    const invitations = [buildInvitation()]
    mockApiClient.get.mockResolvedValueOnce(invitations)

    const result = await teamService.getTeamInvitations('team-xyz')

    expect(result).toBe(invitations)
  })

  it('passes through pending invitations that include all status values from the backend', async () => {
    const invitations: TeamInvitation[] = [
      buildInvitation({ id: 'p-1', status: 'pending' }),
      buildInvitation({ id: 'a-1', status: 'accepted' }),
      buildInvitation({ id: 'r-1', status: 'rejected' }),
      buildInvitation({ id: 'e-1', status: 'revoked' }),
    ]
    mockApiClient.get.mockResolvedValueOnce(invitations)

    const result = await teamService.getTeamInvitations('team-123')

    expect(result).toHaveLength(4)
    expect(result.map(i => i.status)).toEqual([
      'pending',
      'accepted',
      'rejected',
      'revoked',
    ])
  })

  it('propagates errors from the API client', async () => {
    const error = new Error('forbidden')
    mockApiClient.get.mockRejectedValueOnce(error)

    await expect(teamService.getTeamInvitations('team-123')).rejects.toThrow(
      'forbidden'
    )
  })

  it('encodes the team id verbatim in the URL', async () => {
    mockApiClient.get.mockResolvedValueOnce([])

    await teamService.getTeamInvitations('team-with-dashes')

    expect(mockApiClient.get).toHaveBeenCalledWith(
      '/teams/team-with-dashes/invitations'
    )
  })
})

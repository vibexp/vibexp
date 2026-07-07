import type { TeamInvitation } from '../teamService'

const mockGeneratedClient = { GET: jest.fn() }

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return { ...actual, generatedClient: mockGeneratedClient }
})

import { teamService } from '../teamService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

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

const PATH = '/api/v1/teams/{id}/invitations'

describe('TeamService.getTeamInvitations', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('GETs the team invitations endpoint and returns the array', async () => {
    const invitations: TeamInvitation[] = [
      buildInvitation(),
      buildInvitation({ id: 'inv-2', invitee_email: 'second@example.com' }),
    ]
    mockGeneratedClient.GET.mockReturnValue(success(invitations))

    const result = await teamService.getTeamInvitations('team-123')

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(PATH, {
      params: { path: { id: 'team-123' } },
    })
    expect(result).toEqual(invitations)
    expect(result).toHaveLength(2)
  })

  it('returns an empty array when the backend returns []', async () => {
    mockGeneratedClient.GET.mockReturnValue(success([]))

    const result = await teamService.getTeamInvitations('team-123')

    expect(result).toEqual([])
    expect(Array.isArray(result)).toBe(true)
  })

  it('does not unwrap an envelope — the endpoint returns a raw array', async () => {
    const invitations = [buildInvitation()]
    mockGeneratedClient.GET.mockReturnValue(success(invitations))

    const result = await teamService.getTeamInvitations('team-xyz')

    expect(result).toBe(invitations)
  })

  it('passes through all invitation status values from the backend', async () => {
    const invitations: TeamInvitation[] = [
      buildInvitation({ id: 'p-1', status: 'pending' }),
      buildInvitation({ id: 'a-1', status: 'accepted' }),
      buildInvitation({ id: 'r-1', status: 'rejected' }),
      buildInvitation({ id: 'e-1', status: 'revoked' }),
    ]
    mockGeneratedClient.GET.mockReturnValue(success(invitations))

    const result = await teamService.getTeamInvitations('team-123')

    expect(result.map(i => i.status)).toEqual([
      'pending',
      'accepted',
      'rejected',
      'revoked',
    ])
  })

  it('throws ApiError on an RFC 9457 error', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      Promise.resolve({
        error: {
          type: 'https://api.vibexp.io/errors/FORBIDDEN',
          title: 'Forbidden',
          status: 403,
          detail: 'forbidden',
          code: 'FORBIDDEN',
          request_id: 'req-1',
          timestamp: '2024-01-01T00:00:00Z',
        },
        response: {
          ok: false,
          status: 403,
          statusText: 'Forbidden',
        } as Response,
      })
    )

    await expect(teamService.getTeamInvitations('team-123')).rejects.toThrow(
      'forbidden'
    )
  })
})

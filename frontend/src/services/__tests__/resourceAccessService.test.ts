import type { ResourceAccessMetricsResponse } from '../resourceAccessService'

// Mock the generated client; unwrap stays real so the test exercises the same
// success/error resolution production uses.
const mockGeneratedClient = { GET: jest.fn() }

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return { ...actual, generatedClient: mockGeneratedClient }
})

import { resourceAccessService } from '../resourceAccessService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response
const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

const PATH = '/api/v1/{team_id}/resource-access-metrics'

describe('ResourceAccessService', () => {
  const teamId = 'team-123'
  const resourceType = 'prompt'
  const resourceId = '7c9e6679-7425-40de-944b-e07fc1f90ae7'

  const response: ResourceAccessMetricsResponse = {
    status: 'success',
    message: 'Resource access metrics retrieved successfully',
    data: {
      total_accesses: 4,
      range: '30d',
      counts: [
        { date: '2026-05-01', web: 3, cli: 1, mcp: 0, api: 0, total: 4 },
      ],
    },
  }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('calls the team-scoped endpoint with path + query params', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(response))

    const result = await resourceAccessService.getResourceAccessMetrics(
      teamId,
      resourceType,
      resourceId,
      '7d'
    )

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(PATH, {
      params: {
        path: { team_id: teamId },
        query: {
          resource_type: 'prompt',
          resource_id: resourceId,
          range: '7d',
        },
      },
      signal: undefined,
    })
    expect(result).toEqual(response)
  })

  it('defaults the range to 30d when omitted', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(response))

    await resourceAccessService.getResourceAccessMetrics(
      teamId,
      resourceType,
      resourceId
    )

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      PATH,
      expect.objectContaining({
        params: expect.objectContaining({
          query: expect.objectContaining({ range: '30d' }),
        }),
      })
    )
  })

  it('forwards an abort signal to the generated client', async () => {
    mockGeneratedClient.GET.mockReturnValue(success(response))
    const controller = new AbortController()

    await resourceAccessService.getResourceAccessMetrics(
      teamId,
      resourceType,
      resourceId,
      '30d',
      controller.signal
    )

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      PATH,
      expect.objectContaining({ signal: controller.signal })
    )
  })

  it('throws ApiError on an RFC 9457 error response', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      Promise.resolve({
        error: {
          type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
          title: 'Internal Server Error',
          status: 500,
          detail: 'Failed to compute metrics',
          code: 'INTERNAL_ERROR',
          request_id: 'req-1',
          timestamp: '2026-05-01T00:00:00Z',
        },
        response: { ok: false, status: 500, statusText: 'Error' } as Response,
      })
    )

    await expect(
      resourceAccessService.getResourceAccessMetrics(
        teamId,
        resourceType,
        resourceId
      )
    ).rejects.toThrow('Failed to compute metrics')
  })
})

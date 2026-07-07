import type { ResourceAccessMetricsResponse } from '../resourceAccessService'
import type {
  TeamResourceCreationMetricsResponse,
  TeamStats,
} from '../teamService'

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

describe('teamService analytics methods', () => {
  const teamId = 'team-123'

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('getTeamStats GETs the bare /stats endpoint and returns the stats object', async () => {
    const stats: TeamStats = {
      total_projects: 4,
      total_prompts: 25,
      total_artifacts: 13,
      total_blueprints: 6,
      total_memories: 40,
      total_feed_items: 52,
    }
    mockGeneratedClient.GET.mockReturnValue(success(stats))

    const result = await teamService.getTeamStats(teamId)

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/teams/{id}/stats',
      { params: { path: { id: teamId } } }
    )
    expect(result).toEqual(stats)
  })

  it('getTeamResourceCreationMetrics passes range + forwards the signal', async () => {
    const response: TeamResourceCreationMetricsResponse = {
      status: 'success',
      message: 'ok',
      data: {
        total_created: 7,
        range: '7d',
        counts: [
          {
            date: '2026-05-01',
            prompts: 3,
            artifacts: 1,
            blueprints: 0,
            memories: 2,
            projects: 1,
            total: 7,
          },
        ],
      },
    }
    mockGeneratedClient.GET.mockReturnValue(success(response))
    const controller = new AbortController()

    const result = await teamService.getTeamResourceCreationMetrics(
      teamId,
      '7d',
      controller.signal
    )

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/teams/{id}/resource-creation-metrics',
      {
        params: { path: { id: teamId }, query: { range: '7d' } },
        signal: controller.signal,
      }
    )
    expect(result).toEqual(response)
  })

  it('getTeamResourceCreationMetrics defaults the range to 30d', async () => {
    mockGeneratedClient.GET.mockReturnValue(success({ data: { counts: [] } }))

    await teamService.getTeamResourceCreationMetrics(teamId)

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/teams/{id}/resource-creation-metrics',
      expect.objectContaining({
        params: expect.objectContaining({ query: { range: '30d' } }),
      })
    )
  })

  it('getTeamResourceAccessMetrics passes range', async () => {
    const response: ResourceAccessMetricsResponse = {
      status: 'success',
      message: 'ok',
      data: {
        total_accesses: 4,
        range: '14d',
        counts: [
          { date: '2026-05-01', web: 3, cli: 1, mcp: 0, api: 0, total: 4 },
        ],
      },
    }
    mockGeneratedClient.GET.mockReturnValue(success(response))

    const result = await teamService.getTeamResourceAccessMetrics(teamId, '14d')

    expect(mockGeneratedClient.GET).toHaveBeenCalledWith(
      '/api/v1/teams/{id}/resource-access-metrics',
      {
        params: { path: { id: teamId }, query: { range: '14d' } },
        signal: undefined,
      }
    )
    expect(result).toEqual(response)
  })

  it('propagates errors thrown by the generated client', async () => {
    mockGeneratedClient.GET.mockReturnValue(
      Promise.resolve({
        error: {
          type: 'https://api.vibexp.io/errors/INTERNAL_ERROR',
          title: 'Internal Server Error',
          status: 500,
          detail: 'boom',
          code: 'INTERNAL_ERROR',
          request_id: 'req-1',
          timestamp: '2026-05-01T00:00:00Z',
        },
        response: { ok: false, status: 500, statusText: 'Error' } as Response,
      })
    )

    await expect(teamService.getTeamStats(teamId)).rejects.toThrow('boom')
  })
})

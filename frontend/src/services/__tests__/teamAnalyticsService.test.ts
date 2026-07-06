import type { TeamStats } from '../../types/team'
import type { ResourceAccessMetricsResponse } from '../resourceAccessService'
import type { TeamResourceCreationMetricsResponse } from '../teamService'

const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
  put: jest.fn(),
  delete: jest.fn(),
}

jest.mock('../../lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

import { teamService } from '../teamService'

describe('teamService analytics methods', () => {
  const teamId = 'team-123'

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('getTeamStats hits the bare /stats endpoint and returns the stats object', async () => {
    const stats: TeamStats = {
      total_projects: 4,
      total_prompts: 25,
      total_artifacts: 13,
      total_blueprints: 6,
      total_memories: 40,
      total_feed_items: 52,
    }
    mockApiClient.get.mockResolvedValue(stats)

    const result = await teamService.getTeamStats(teamId)

    expect(mockApiClient.get).toHaveBeenCalledWith(`/teams/${teamId}/stats`)
    expect(result).toEqual(stats)
  })

  it('getTeamResourceCreationMetrics builds the endpoint with range + forwards the signal', async () => {
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
    mockApiClient.get.mockResolvedValue(response)
    const controller = new AbortController()

    const result = await teamService.getTeamResourceCreationMetrics(
      teamId,
      '7d',
      controller.signal
    )

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/teams/${teamId}/resource-creation-metrics?range=7d`,
      { signal: controller.signal }
    )
    expect(result).toEqual(response)
  })

  it('getTeamResourceCreationMetrics defaults the range to 30d', async () => {
    mockApiClient.get.mockResolvedValue({ data: { counts: [] } })

    await teamService.getTeamResourceCreationMetrics(teamId)

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/teams/${teamId}/resource-creation-metrics?range=30d`,
      { signal: undefined }
    )
  })

  it('getTeamResourceAccessMetrics builds the endpoint with range', async () => {
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
    mockApiClient.get.mockResolvedValue(response)

    const result = await teamService.getTeamResourceAccessMetrics(teamId, '14d')

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/teams/${teamId}/resource-access-metrics?range=14d`,
      { signal: undefined }
    )
    expect(result).toEqual(response)
  })

  it('url-encodes the team id in the analytics endpoints', async () => {
    mockApiClient.get.mockResolvedValue({})

    await teamService.getTeamStats('team/with space')

    expect(mockApiClient.get).toHaveBeenCalledWith(
      '/teams/team%2Fwith%20space/stats'
    )
  })

  it('propagates errors thrown by the api client', async () => {
    mockApiClient.get.mockRejectedValue(new Error('boom'))

    await expect(teamService.getTeamStats(teamId)).rejects.toThrow('boom')
  })
})

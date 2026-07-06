import type { ResourceAccessMetricsResponse } from '../resourceAccessService'

const mockApiClient = {
  get: jest.fn(),
  post: jest.fn(),
  put: jest.fn(),
  delete: jest.fn(),
}

jest.mock('../../lib/apiClient', () => ({
  apiClient: mockApiClient,
}))

import { resourceAccessService } from '../resourceAccessService'

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

  it('builds the team-scoped endpoint with query params and returns the response', async () => {
    mockApiClient.get.mockResolvedValue(response)

    const result = await resourceAccessService.getResourceAccessMetrics(
      teamId,
      resourceType,
      resourceId,
      '7d'
    )

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/${teamId}/resource-access-metrics?resource_type=prompt&resource_id=${resourceId}&range=7d`,
      { signal: undefined }
    )
    expect(result).toEqual(response)
  })

  it('defaults the range to 30d when omitted', async () => {
    mockApiClient.get.mockResolvedValue(response)

    await resourceAccessService.getResourceAccessMetrics(
      teamId,
      resourceType,
      resourceId
    )

    expect(mockApiClient.get).toHaveBeenCalledWith(
      `/${teamId}/resource-access-metrics?resource_type=prompt&resource_id=${resourceId}&range=30d`,
      { signal: undefined }
    )
  })

  it('url-encodes the team id in the endpoint path', async () => {
    mockApiClient.get.mockResolvedValue(response)

    await resourceAccessService.getResourceAccessMetrics(
      'team/with space',
      resourceType,
      resourceId
    )

    expect(mockApiClient.get).toHaveBeenCalledWith(
      expect.stringContaining('/team%2Fwith%20space/resource-access-metrics?'),
      { signal: undefined }
    )
  })

  it('forwards an abort signal to the api client', async () => {
    mockApiClient.get.mockResolvedValue(response)
    const controller = new AbortController()

    await resourceAccessService.getResourceAccessMetrics(
      teamId,
      resourceType,
      resourceId,
      '30d',
      controller.signal
    )

    expect(mockApiClient.get).toHaveBeenCalledWith(expect.any(String), {
      signal: controller.signal,
    })
  })

  it('propagates errors thrown by the api client', async () => {
    mockApiClient.get.mockRejectedValue(new Error('boom'))

    await expect(
      resourceAccessService.getResourceAccessMetrics(
        teamId,
        resourceType,
        resourceId
      )
    ).rejects.toThrow('boom')
  })
})

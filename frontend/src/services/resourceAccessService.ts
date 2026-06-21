import { apiClient } from '../lib/apiClient'
import type {
  ResourceAccessMetricsResponse,
  ResourceAccessType,
} from '../types'

/**
 * Service for fetching per-resource access metrics (web/cli/mcp/api breakdown
 * over a time range). Backed by the team-scoped resource-access-metrics endpoint.
 */
class ResourceAccessService {
  async getResourceAccessMetrics(
    teamId: string,
    resourceType: ResourceAccessType,
    resourceId: string,
    range = '30d',
    signal?: AbortSignal
  ): Promise<ResourceAccessMetricsResponse> {
    const params = new URLSearchParams({
      resource_type: resourceType,
      resource_id: resourceId,
      range,
    })
    return apiClient.get<ResourceAccessMetricsResponse>(
      `/${encodeURIComponent(teamId)}/resource-access-metrics?${params.toString()}`,
      { signal }
    )
  }
}

export const resourceAccessService = new ResourceAccessService()

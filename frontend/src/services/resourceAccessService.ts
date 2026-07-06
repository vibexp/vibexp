import type { components, operations } from '@vibexp/api-client'

import { apiClient } from '../lib/apiClient'

// Generated wire types for the resource-access metrics domain (issue #92 hooks
// slice) — the OpenAPI spec is the single source of truth. `ResourceAccessType`
// is the endpoint's `resource_type` query enum; `AccessCountByDate` keeps its
// historical name as an alias of the renamed `ResourceAccessDailyCount` row so
// chart consumers don't churn.
export type ResourceAccessType =
  operations['getResourceAccessMetrics']['parameters']['query']['resource_type']
export type AccessCountByDate =
  components['schemas']['ResourceAccessDailyCount']
export type ResourceAccessMetricsResponse =
  components['schemas']['ResourceAccessMetricsResponse']

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

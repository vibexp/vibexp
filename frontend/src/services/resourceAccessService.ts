import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

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
// The selectable reporting windows the metrics endpoints accept.
export type MetricsRange = NonNullable<
  operations['getResourceAccessMetrics']['parameters']['query']['range']
>

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
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/resource-access-metrics', {
        params: {
          path: { team_id: teamId },
          // `range` originates from a fixed selector, so it is always one of the
          // accepted window values.
          query: {
            resource_type: resourceType,
            resource_id: resourceId,
            range: range as MetricsRange,
          },
        },
        signal,
      })
    )
  }
}

export const resourceAccessService = new ResourceAccessService()

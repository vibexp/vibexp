import type { components } from '@vibexp/api-client'

import { apiClient } from '../lib/apiClient'

// Generated wire types for the per-project resource-creation metrics domain
// (issue #92 hooks slice). `CreationCountByDate` keeps its historical name as an
// alias of the renamed `ProjectResourceCreationDailyCount` row so chart
// consumers don't churn.
export type CreationCountByDate =
  components['schemas']['ProjectResourceCreationDailyCount']
export type ResourceCreationMetricsResponse =
  components['schemas']['ProjectResourceCreationMetricsResponse']

/**
 * Service for fetching per-project resource-creation metrics (daily counts of
 * prompts/artifacts/blueprints/memories created over a time range). Backed by
 * the project-scoped resource-creation-metrics endpoint. Mirrors
 * resourceAccessService — both use the plain apiClient (the project metrics
 * domain is not yet on the generated typed client).
 */
class ResourceCreationService {
  async getResourceCreationMetrics(
    teamId: string,
    slug: string,
    range = '30d',
    signal?: AbortSignal
  ): Promise<ResourceCreationMetricsResponse> {
    const params = new URLSearchParams({ range })
    return apiClient.get<ResourceCreationMetricsResponse>(
      `/${encodeURIComponent(teamId)}/projects/${encodeURIComponent(slug)}/resource-creation-metrics?${params.toString()}`,
      { signal }
    )
  }
}

export const resourceCreationService = new ResourceCreationService()

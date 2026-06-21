import { apiClient } from '../lib/apiClient'
import type { ResourceCreationMetricsResponse } from '../types'

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

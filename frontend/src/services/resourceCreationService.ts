import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import type { MetricsRange } from './resourceAccessService'

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
 * the project-scoped resource-creation-metrics endpoint.
 */
class ResourceCreationService {
  async getResourceCreationMetrics(
    teamId: string,
    slug: string,
    range = '30d',
    signal?: AbortSignal
  ): Promise<ResourceCreationMetricsResponse> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/projects/{slug}/resource-creation-metrics',
        {
          params: {
            path: { team_id: teamId, slug },
            query: { range: range as MetricsRange },
          },
          signal,
        }
      )
    )
  }
}

export const resourceCreationService = new ResourceCreationService()

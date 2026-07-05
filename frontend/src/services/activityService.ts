import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the activities domain — the OpenAPI spec is the
// single source of truth; do not hand-write request/response shapes here.
export type Activity = components['schemas']['Activity']
export type ActivityListResponse = components['schemas']['ActivityListResponse']
export type ActivityStatsResponse =
  components['schemas']['ActivityStatsResponse']
export type ActivityTypesResponse =
  components['schemas']['ActivityTypesResponse']
export type ActivityFilters = NonNullable<
  operations['listActivities']['parameters']['query']
>
export type ActivitiesResponse = components['schemas']['ActivityListEnvelope']
export type ActivityStatsApiResponse =
  components['schemas']['ActivityStatsEnvelope']
export type ActivityTypesApiResponse =
  components['schemas']['ActivityTypesEnvelope']

class ActivityService {
  async getActivities(
    filters: ActivityFilters = {}
  ): Promise<ActivitiesResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/activities', {
        params: { query: filters },
      })
    )
  }

  async getActivityStats(): Promise<ActivityStatsApiResponse> {
    return unwrap(generatedClient.GET('/api/v1/activities/stats'))
  }

  async getActivityAndEntityTypes(): Promise<ActivityTypesApiResponse> {
    return unwrap(generatedClient.GET('/api/v1/activities/types'))
  }
}

export const activityService = new ActivityService()

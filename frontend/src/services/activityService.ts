import { apiClient } from '../lib/apiClient'
import type {
  ActivitiesResponse,
  ActivityFilters,
  ActivityStatsApiResponse,
  ActivityTypesApiResponse,
} from '../types'

class ActivityService {
  async getActivities(
    filters: ActivityFilters = {}
  ): Promise<ActivitiesResponse> {
    const params = new URLSearchParams()

    if (filters.activity_type)
      params.append('activity_type', filters.activity_type)
    if (filters.entity_type) params.append('entity_type', filters.entity_type)
    if (filters.entity_id) params.append('entity_id', filters.entity_id)
    if (filters.session_id) params.append('session_id', filters.session_id)
    if (filters.search) params.append('search', filters.search)
    if (filters.date_from) params.append('date_from', filters.date_from)
    if (filters.date_to) params.append('date_to', filters.date_to)
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())

    const queryString = params.toString()
    const endpoint = `/activities${queryString ? `?${queryString}` : ''}`

    return apiClient.get<ActivitiesResponse>(endpoint)
  }

  async getActivityStats(): Promise<ActivityStatsApiResponse> {
    return apiClient.get<ActivityStatsApiResponse>('/activities/stats')
  }

  async getActivityAndEntityTypes(): Promise<ActivityTypesApiResponse> {
    return apiClient.get<ActivityTypesApiResponse>('/activities/types')
  }
}

export const activityService = new ActivityService()

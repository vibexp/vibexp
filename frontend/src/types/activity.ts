import type { ApiResponse } from './api'

// Activity types - matching backend schema
export interface Activity {
  id: string
  user_id: string
  activity_type: string
  entity_type: string
  entity_id?: string | null
  entity_name?: string | null
  actor_name?: string | null
  session_id?: string | null
  description: string
  metadata: Record<string, unknown>
  source_ip?: string | null
  user_agent?: string | null
  created_at: string
}

export interface ActivityFilters {
  activity_type?: string
  entity_type?: string
  entity_id?: string
  session_id?: string
  search?: string
  date_from?: string
  date_to?: string
  page?: number
  limit?: number
}

export interface ActivityListResponse {
  activities: Activity[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}

export interface ActivityStatsResponse {
  total_activities: number
  activities_today: number
  activities_this_week: number
  top_activity_types: {
    activity_type: string
    count: number
  }[]
  top_entity_types: {
    entity_type: string
    count: number
  }[]
  recent_activities: Activity[]
  activities_by_date_week: {
    date: string
    count: number
  }[]
}

export interface ActivityTypesResponse {
  activity_types: string[]
  entity_types: string[]
}

export interface EntityTypesResponse {
  entity_types: string[]
}

export type ActivitiesResponse = ApiResponse<ActivityListResponse>
export type ActivityStatsApiResponse = ApiResponse<ActivityStatsResponse>
export type ActivityTypesApiResponse = ApiResponse<ActivityTypesResponse>
export type EntityTypesApiResponse = ApiResponse<EntityTypesResponse>

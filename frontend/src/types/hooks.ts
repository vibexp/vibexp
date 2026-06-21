import type { PaginatedData } from './api'
import type { ApiResponse } from './api'

// Claude Code Hook types
export interface ClaudeCodeHookPayload {
  id: number
  session_id: string
  transcript_path?: string | null
  cwd?: string | null
  hook_event_name: string
  tool_name?: string | null
  tool_input?: Record<string, unknown> | null
  tool_response?: Record<string, unknown> | null
  prompt?: string | null
  message?: string | null
  payload: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface SessionSummary {
  session_id: string
  first_seen: string
  last_seen: string
  hook_count: number
  latest_cwd?: string | null
  unique_tools: number
}

// Cursor IDE Hook types
export interface CursorIDEHookPayload {
  id: number
  user_id?: string | null
  session_id: string
  conversation_id?: string | null
  generation_id?: string | null
  hook_event_name: string
  tool_name?: string | null
  workspace_roots?: string[]
  configuration?: Record<string, unknown> | null
  reference?: Record<string, unknown> | null
  context?: Record<string, unknown> | null
  input?: Record<string, unknown> | null
  output?: Record<string, unknown> | null
  induced_failure?: Record<string, unknown> | null
  payload: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface CursorSessionSummary {
  session_id: string
  first_seen: string
  last_seen: string
  hook_count: number
  unique_tools: number
}

export type SessionsResponse = ApiResponse<PaginatedData<SessionSummary>>
export type HooksResponse = ApiResponse<PaginatedData<ClaudeCodeHookPayload>>

// Cursor IDE Response types
export type CursorSessionsResponse = ApiResponse<
  PaginatedData<CursorSessionSummary>
>
export type CursorHooksResponse = ApiResponse<
  PaginatedData<CursorIDEHookPayload>
>

// Session Counts types
export interface SessionCountByDate {
  date: string
  count: number
}

export interface SessionCountsData {
  total_sessions: number
  counts: SessionCountByDate[]
}

export type SessionCountsResponse = ApiResponse<SessionCountsData>

// Resource Access Metrics types
export type ResourceAccessType =
  | 'prompt'
  | 'artifact'
  | 'blueprint'
  | 'memory'
  | 'project'
  | 'agent'

export interface AccessCountByDate {
  date: string
  web: number
  cli: number
  mcp: number
  api: number
  total: number
  // Index signature so the row is assignable to TimeSeriesBarChart's generic
  // TimeSeriesDatum (the shell reads series values by arbitrary key).
  [key: string]: number | string
}

export interface ResourceAccessMetrics {
  total_accesses: number
  range: string
  counts: AccessCountByDate[]
}

export type ResourceAccessMetricsResponse = ApiResponse<ResourceAccessMetrics>

// Resource Creation Metrics types — daily per-type creation counts for a
// project, mirroring the access-metrics shape so both feed the shared
// TimeSeriesBarChart shell.
export interface CreationCountByDate {
  date: string
  prompts: number
  artifacts: number
  blueprints: number
  memories: number
  total: number
  [key: string]: number | string
}

export interface ResourceCreationMetrics {
  total_created: number
  range: string
  counts: CreationCountByDate[]
}

export type ResourceCreationMetricsResponse =
  ApiResponse<ResourceCreationMetrics>

// Team Resource Creation Metrics types — daily per-type creation counts for a
// whole team. Adds a `projects` series on top of the project-level shape, since
// projects belong to a team.
export interface TeamCreationCountByDate {
  date: string
  prompts: number
  artifacts: number
  blueprints: number
  memories: number
  projects: number
  total: number
  [key: string]: number | string
}

export interface TeamResourceCreationMetrics {
  total_created: number
  range: string
  counts: TeamCreationCountByDate[]
}

export type TeamResourceCreationMetricsResponse =
  ApiResponse<TeamResourceCreationMetrics>

// Team Feed Creation Metrics types — daily counts of feeds and feed items
// created across a whole team, mirroring the creation-metrics shape so it feeds
// the shared TimeSeriesBarChart shell.
export interface TeamFeedCreationCountByDate {
  date: string
  feeds: number
  feed_items: number
  total: number
  [key: string]: number | string
}

export interface TeamFeedCreationMetrics {
  total_created: number
  range: string
  counts: TeamFeedCreationCountByDate[]
}

export type TeamFeedCreationMetricsResponse =
  ApiResponse<TeamFeedCreationMetrics>

// Team Top Accessed Resources types — a ranked list of the most-accessed
// resources across a team over a range. `name` may be empty when the resource
// type has no name table or the resource no longer exists.
export interface TopAccessedResource {
  resource_type: string
  resource_id: string
  name: string
  access_count: number
}

export interface TeamTopAccessedResources {
  range: string
  items: TopAccessedResource[]
}

export type TeamTopAccessedResourcesResponse =
  ApiResponse<TeamTopAccessedResources>

// Overview Stats types
export interface ToolUsageCount {
  tool_name: string
  count: number
}

export interface OverviewStats {
  total_sessions: number
  sessions_this_week: number
  sessions_last_week: number
  weekly_trend_percent: number
  avg_user_prompts_per_session: number
  total_unique_tools: number
  top_tools: ToolUsageCount[]
  avg_session_duration_minutes: number
  total_memories: number
}

export type OverviewStatsResponse = ApiResponse<OverviewStats>

// Cursor IDE Overview Stats types
export interface CursorOverviewStats {
  total_sessions: number
  sessions_this_week: number
  sessions_last_week: number
  weekly_trend_percent: number
  avg_user_prompts_per_session: number
  total_unique_tools: number
  top_tools: ToolUsageCount[]
  avg_session_duration_minutes: number
}

export type CursorOverviewStatsResponse = ApiResponse<CursorOverviewStats>

// Recent Activity types
export interface RecentActivity {
  session_id: string
  cwd?: string | null
  tool_name?: string | null
  tool_input?: Record<string, unknown> | null
  hook_event_name: string
  created_at: string
}

export interface RecentActivitiesData {
  activities: RecentActivity[]
  page: number
  limit: number
  total: number
  total_pages: number
}

export type RecentActivitiesResponse = ApiResponse<RecentActivitiesData>

// Cursor IDE Recent Activity types
export interface CursorRecentActivity {
  session_id: string
  tool_name?: string | null
  input?: Record<string, unknown> | null
  hook_event_name: string
  created_at: string
}

export interface CursorRecentActivitiesData {
  activities: CursorRecentActivity[]
  page: number
  limit: number
  total: number
  total_pages: number
}

export type CursorRecentActivitiesResponse =
  ApiResponse<CursorRecentActivitiesData>

// Cursor IDE Session Counts types
export type CursorSessionCountsResponse = ApiResponse<SessionCountsData>

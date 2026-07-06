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

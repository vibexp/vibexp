import type { ApiResponse } from './api'
import type { ResourceVersion, ResourceVersionListResponse } from './version'

// Centralized type and subtype definitions
export type BlueprintType =
  | 'general'
  | 'claude-code'
  | 'claude'
  | 'cursor'
  | 'codex'
export type BlueprintSubtype =
  | 'sub-agents'
  | 'skills'
  | 'slash-commands'
  | 'others'
  | 'claude-md'
  | 'agents'
  | 'commands'
  | 'rules'
  | 'cursor-md'
  | 'agents-md'

// Blueprint types - matching backend OpenAPI schema
export interface Blueprint {
  id: string
  project_id: string
  slug: string
  user_id: string
  content: string
  created_at: string
  updated_at: string
  status: 'active' | 'expired'
  title: string
  description: string
  type: BlueprintType
  subtype?: BlueprintSubtype
  metadata: Record<string, unknown>
}

export interface CreateBlueprintRequest {
  project_id: string // Required, must be a valid UUID
  slug: string // Required
  content: string // Required
  title: string // Required
  description?: string // Optional, max 500 chars
  type?: BlueprintType // Optional, defaults to "general"
  subtype?: BlueprintSubtype // Optional
  status?: 'active' | 'expired' // Optional, defaults to "active"
  metadata?: Record<string, unknown> // Optional
}

export interface UpdateBlueprintRequest {
  project_id?: string // Optional, must be a valid UUID
  slug?: string // Optional, max 255 chars
  content?: string // Optional
  title?: string // Optional, max 255 chars
  description?: string // Optional, max 500 chars
  type?: BlueprintType // Optional
  subtype?: BlueprintSubtype // Optional
  status?: 'active' | 'expired' // Optional
  metadata?: Record<string, unknown> // Optional
}

export interface BlueprintFilters {
  project_id?: string
  search?: string
  status?: 'active' | 'expired'
  type?: BlueprintType
  subtype?: BlueprintSubtype
  sort_by?: 'created_at' | 'updated_at' | 'title'
  sort_order?: 'asc' | 'desc'
  page?: number
  limit?: number // Max 100
  // Metadata filtering support - dynamic keys with "metadata_" prefix
  [key: `metadata_${string}`]: string | undefined
}

export interface BlueprintListResponse {
  blueprints: Blueprint[]
  page: number
  per_page: number
  total_count: number
  total_pages: number
}

export interface BlueprintStatsResponse {
  total_projects: number
  total_blueprints: number
  added_this_week: number
  total_by_type: Record<string, number>
  total_by_status: Record<string, number>
}

// Blueprint versions are just the generic resource version, with `resource_type`
// === "blueprint". Kept as aliases so blueprint call sites read naturally while the
// version-history module works against the resource-agnostic types.
export type BlueprintVersion = ResourceVersion
export type BlueprintVersionListResponse = ResourceVersionListResponse

export type BlueprintsResponse = ApiResponse<BlueprintListResponse>
export type BlueprintResponse = ApiResponse<Blueprint>
export type BlueprintStatsApiResponse = ApiResponse<BlueprintStatsResponse>

// Type-to-subtype mapping for blueprint types
export const BLUEPRINT_TYPE_SUBTYPES: Record<string, string[]> = {
  general: [],
  'claude-code': ['sub-agents', 'skills', 'slash-commands', 'others'],
  claude: ['claude-md'],
  cursor: ['skills', 'agents', 'commands', 'rules', 'cursor-md'],
  codex: ['rules', 'skills', 'agents-md'],
}

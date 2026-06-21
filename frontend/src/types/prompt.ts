import type { ApiResponse } from './api'
import type { ResourceVersion, ResourceVersionListResponse } from './version'

// Prompt versions are the generic resource version with `resource_type` === "prompt".
// The versioned content is the raw prompt body template (placeholders / @slug refs),
// not any rendered output. Kept as aliases so prompt call sites read naturally while the
// version-history module works against the resource-agnostic types.
export type PromptVersion = ResourceVersion
export type PromptVersionListResponse = ResourceVersionListResponse

// Prompt Management types
export interface Prompt {
  id: string
  name: string
  slug: string
  description: string
  body: string
  user_id: string
  project_id: string
  status: 'draft' | 'published'
  mcp_expose: boolean
  is_shared: boolean
  labels: string[]
  created_at: string
  updated_at: string
  version: number
}

export interface CreatePromptRequest {
  name: string
  slug: string
  description: string
  body: string
  project_id: string
  status?: 'draft' | 'published'
  mcp_expose?: boolean
  labels?: string[]
}

export interface UpdatePromptRequest {
  name?: string
  slug?: string
  description?: string
  body?: string
  project_id?: string
  status?: 'draft' | 'published'
  mcp_expose?: boolean
  labels?: string[]
}

export interface PromptFilters {
  status?: 'draft' | 'published'
  search?: string
  shared?: boolean
  labels?: string[]
  project_id?: string
  sort_by?: 'name' | 'status' | 'updated_at' | 'created_at'
  sort_order?: 'asc' | 'desc'
  page?: number
  limit?: number
}

export interface PromptListResponse {
  prompts: Prompt[]
  page: number
  per_page: number
  total_count: number
  total_pages: number
}

export interface RenderPromptRequest {
  placeholders: Record<string, string>
}

export interface RenderPromptResponse {
  rendered_body: string
  placeholders_missing?: string[]
  references_used?: string[]
}

export type PromptsResponse = ApiResponse<PromptListResponse>

// Prompt Dependencies types
export interface PromptDependencyInfo {
  id: string
  slug: string
  name: string
}

export interface PromptDependenciesResponse {
  used_by: PromptDependencyInfo[] // Prompts that reference this prompt
  uses: PromptDependencyInfo[] // Prompts that this prompt references
}

export type PromptDependenciesApiResponse =
  ApiResponse<PromptDependenciesResponse>

import type { ResourceVersion, ResourceVersionListResponse } from './version'

// Memory Management types - matches the bare-object response from the backend
// (GET /api/v1/{team_id}/memories/{id}). The backend does not wrap memory
// payloads in an `ApiResponse` envelope.
export interface Memory {
  id: string
  user_id: string
  team_id: string
  project_id: string
  text: string
  metadata: Record<string, unknown>
  created_at: string
  updated_at: string
  version: number
}

export interface CreateMemoryRequest {
  project_id: string
  text: string
  metadata?: Record<string, unknown>
}

export interface UpdateMemoryRequest {
  project_id?: string
  text?: string
  metadata?: Record<string, unknown>
}

export interface MemoryFilters {
  project_id?: string
  search?: string
  metadata_key?: string
  metadata_value?: string
  sort_by?: 'updated_at'
  sort_order?: 'asc' | 'desc'
  page?: number
  limit?: number
}

export interface MemoryListResponse {
  memories: Memory[]
  page: number
  per_page: number
  total_count: number
  total_pages: number
}

export type MemoriesResponse = MemoryListResponse
export type MemoryResponse = Memory

// Memory versions are just the generic resource version, with `resource_type`
// === "memory". Kept as aliases so memory call sites read naturally while the
// version-history module works against the resource-agnostic types.
export type MemoryVersion = ResourceVersion
export type MemoryVersionListResponse = ResourceVersionListResponse

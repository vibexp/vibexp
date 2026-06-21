import type { ApiResponse } from './api'
import type { ResourceVersion, ResourceVersionListResponse } from './version'

// Artifact lifecycle status. `active` is the default (surfaced in lists and
// search), `draft` is a work-in-progress visible to its owner but excluded from
// search, and `archived` is retired (hidden from default lists and search,
// reachable via an explicit status filter).
export type ArtifactStatus = 'active' | 'draft' | 'archived'

// Artifact types - matching current OpenAPI schema
export interface Artifact {
  id: string
  project_id: string
  slug: string
  user_id: string
  content?: string
  created_at: string
  updated_at: string
  status: ArtifactStatus
  title: string
  description: string
  // Team type slug (system default or custom). Validated against the team's
  // types server-side; a free-form string here so custom types are supported.
  type: string
  metadata: Record<string, unknown>
}

export interface CreateArtifactRequest {
  project_id: string // Required, must be a valid UUID
  slug: string // Required
  content: string // Required
  title: string // Required
  description?: string // Optional, max 500 chars
  type?: string // Optional team type slug, defaults to "general"
  status?: ArtifactStatus // Optional, defaults to "active"
  metadata?: Record<string, unknown> // Optional
}

export interface UpdateArtifactRequest {
  project_id?: string // Optional, must be a valid UUID
  slug?: string // Optional, max 255 chars
  content?: string // Optional
  title?: string // Optional, max 255 chars
  description?: string // Optional, max 500 chars
  type?: string // Optional team type slug
  status?: ArtifactStatus // Optional
  metadata?: Record<string, unknown> // Optional
}

export interface ArtifactFilters {
  project_id?: string
  search?: string
  status?: ArtifactStatus
  type?: string
  sort_by?: 'created_at' | 'updated_at'
  sort_order?: 'asc' | 'desc'
  page?: number
  limit?: number // Max 100
  // Metadata filtering support - dynamic keys with "metadata_" prefix
  [key: `metadata_${string}`]: string | undefined
}

export interface ArtifactListResponse {
  artifacts: Artifact[]
  page: number
  per_page: number
  total_count: number
  total_pages: number
}

export interface ArtifactStatsResponse {
  total_projects: number
  total_artifacts: number
  added_this_week: number
  total_by_type: Record<string, number>
  total_by_status: Record<string, number>
}

// Artifact versions are just the generic resource version, with `resource_type`
// === "artifact". Kept as aliases so existing artifact call sites stay valid
// while the version-history module works against the resource-agnostic types.
export type ArtifactVersion = ResourceVersion
export type ArtifactVersionListResponse = ResourceVersionListResponse

export type ArtifactsResponse = ApiResponse<ArtifactListResponse>
export type ArtifactResponse = ApiResponse<Artifact>
export type ArtifactStatsApiResponse = ApiResponse<ArtifactStatsResponse>

import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import type {
  ResourceVersion,
  ResourceVersionListResponse,
} from '../types/version'

// Generated wire types for the memory domain — the OpenAPI spec is the single
// source of truth; do not hand-write request/response shapes.
export type Memory = components['schemas']['Memory']
export type MemoryListResponse = components['schemas']['MemoryListResponse']

// Lifecycle-status union, derived from the generated `Memory` schema.
export type MemoryStatus = Memory['status']

// Request shapes are the generated bodies — the spec now carries `project_id`
// (required on create, optional on update).
export type CreateMemoryRequest = components['schemas']['CreateMemoryRequest']
export type UpdateMemoryRequest = components['schemas']['UpdateMemoryRequest']

// List query params, sourced from the generated `listMemories` operation (the
// spec now carries the project-scoping `project_id` query param).
export type MemoryFilters = NonNullable<
  operations['listMemories']['parameters']['query']
>

// Memory versions are the generic resource version, with `resource_type` ===
// "memory". Kept as aliases so call sites read naturally while the version-history
// module works against the resource-agnostic types.
export type MemoryVersion = ResourceVersion
export type MemoryVersionListResponse = ResourceVersionListResponse

class MemoryService {
  async getMemories(
    teamId: string,
    filters: MemoryFilters = {}
  ): Promise<MemoryListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/memories', {
        params: { path: { team_id: teamId }, query: filters },
      })
    )
  }

  async getMemory(teamId: string, id: string): Promise<Memory> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/memories/{id}', {
        params: { path: { team_id: teamId, id } },
      })
    )
  }

  async createMemory(
    teamId: string,
    data: CreateMemoryRequest
  ): Promise<Memory> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/memories', {
        params: { path: { team_id: teamId } },
        body: data,
      })
    )
  }

  async updateMemory(
    teamId: string,
    id: string,
    data: UpdateMemoryRequest
  ): Promise<Memory> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/memories/{id}', {
        params: { path: { team_id: teamId, id } },
        body: data,
      })
    )
  }

  async deleteMemory(teamId: string, id: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/memories/{id}', {
        params: { path: { team_id: teamId, id } },
      })
    )
  }

  async getMemoryVersions(
    teamId: string,
    id: string
  ): Promise<MemoryVersionListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/memories/{id}/versions', {
        params: { path: { team_id: teamId, id } },
      })
    )
  }

  async getMemoryVersion(
    teamId: string,
    id: string,
    versionNumber: number
  ): Promise<MemoryVersion> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/memories/{id}/versions/{version_number}',
        {
          params: {
            path: { team_id: teamId, id, version_number: versionNumber },
          },
        }
      )
    )
  }

  async restoreMemoryVersion(
    teamId: string,
    id: string,
    versionNumber: number
  ): Promise<Memory> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/memories/{id}/versions/{version_number}/restore',
        {
          params: {
            path: { team_id: teamId, id, version_number: versionNumber },
          },
        }
      )
    )
  }

  async searchMemoriesByMetadata(
    teamId: string,
    filters: MemoryFilters
  ): Promise<MemoryListResponse> {
    if (!filters.metadata_key || !filters.metadata_value) {
      throw new Error(
        'metadata_key and metadata_value are required for metadata search'
      )
    }

    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/memories/search', {
        params: {
          path: { team_id: teamId },
          query: {
            metadata_key: filters.metadata_key,
            metadata_value: filters.metadata_value,
            search: filters.search,
            page: filters.page,
            limit: filters.limit,
          },
        },
      })
    )
  }
}

export const memoryService = new MemoryService()

import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import type {
  ResourceVersion,
  ResourceVersionListResponse,
} from '../types/version'

// Generated wire types for the artifact domain — the OpenAPI spec is the single
// source of truth; do not hand-write request/response shapes. `type` is an open
// string in the spec (validated at runtime against the team's system + custom
// types), so custom types type-check directly with no widening shim.
export type Artifact = components['schemas']['Artifact']
export type ArtifactListResponse = components['schemas']['ArtifactListResponse']
export type ArtifactStatsResponse =
  components['schemas']['ArtifactStatsResponse']
export type CreateArtifactRequest =
  components['schemas']['CreateArtifactRequest']
export type UpdateArtifactRequest =
  components['schemas']['UpdateArtifactRequest']

// Status union, derived from the generated `Artifact` schema; `type` is the open
// string described above, surfaced dynamically via `useTypes()`.
export type ArtifactStatus = Artifact['status']

// List query params, sourced from the generated operations (the by-project variant
// carries `project_id` in the path, so its query omits it).
export type ArtifactFilters = NonNullable<
  operations['listArtifacts']['parameters']['query']
>
export type ArtifactByProjectFilters = NonNullable<
  operations['listArtifactsByProject']['parameters']['query']
>

// Artifact versions are the generic resource version, with `resource_type` ===
// "artifact". Kept as aliases so call sites read naturally while the
// version-history module works against the resource-agnostic types.
export type ArtifactVersion = ResourceVersion
export type ArtifactVersionListResponse = ResourceVersionListResponse

class ArtifactService {
  async getArtifacts(
    teamId: string,
    filters: ArtifactFilters = {}
  ): Promise<ArtifactListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/artifacts', {
        params: {
          path: { team_id: teamId },
          query: filters,
        },
      })
    )
  }

  async getArtifactsByProject(
    teamId: string,
    projectId: string,
    filters: ArtifactByProjectFilters = {}
  ): Promise<ArtifactListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/artifacts/{project_id}', {
        params: {
          path: { team_id: teamId, project_id: projectId },
          query: filters,
        },
      })
    )
  }

  async getArtifact(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<Artifact> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/artifacts/{project_id}/{slug}', {
        params: { path: { team_id: teamId, project_id: projectId, slug } },
      })
    )
  }

  async createArtifact(
    teamId: string,
    data: CreateArtifactRequest
  ): Promise<Artifact> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/artifacts', {
        params: { path: { team_id: teamId } },
        body: data,
      })
    )
  }

  async updateArtifact(
    teamId: string,
    projectId: string,
    slug: string,
    data: UpdateArtifactRequest
  ): Promise<Artifact> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/artifacts/{project_id}/{slug}', {
        params: { path: { team_id: teamId, project_id: projectId, slug } },
        body: data,
      })
    )
  }

  async deleteArtifact(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<void> {
    await unwrap(
      generatedClient.DELETE(
        '/api/v1/{team_id}/artifacts/{project_id}/{slug}',
        {
          params: { path: { team_id: teamId, project_id: projectId, slug } },
        }
      )
    )
  }

  async getArtifactVersions(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<ArtifactVersionListResponse> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/artifacts/{project_id}/{slug}/versions',
        {
          params: { path: { team_id: teamId, project_id: projectId, slug } },
        }
      )
    )
  }

  async getArtifactVersion(
    teamId: string,
    projectId: string,
    slug: string,
    versionNumber: number
  ): Promise<ArtifactVersion> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/artifacts/{project_id}/{slug}/versions/{version_number}',
        {
          params: {
            path: {
              team_id: teamId,
              project_id: projectId,
              slug,
              version_number: versionNumber,
            },
          },
        }
      )
    )
  }

  async restoreArtifactVersion(
    teamId: string,
    projectId: string,
    slug: string,
    versionNumber: number
  ): Promise<Artifact> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/artifacts/{project_id}/{slug}/versions/{version_number}/restore',
        {
          params: {
            path: {
              team_id: teamId,
              project_id: projectId,
              slug,
              version_number: versionNumber,
            },
          },
        }
      )
    )
  }

  async getArtifactStats(teamId: string): Promise<ArtifactStatsResponse> {
    try {
      return await unwrap(
        generatedClient.GET('/api/v1/{team_id}/artifacts/stats', {
          params: { path: { team_id: teamId } },
        })
      )
    } catch (error) {
      console.error('Failed to fetch artifact stats:', error)
      return {
        total_projects: 0,
        total_artifacts: 0,
        added_this_week: 0,
        total_by_type: {},
        total_by_status: {},
      }
    }
  }
}

export const artifactService = new ArtifactService()

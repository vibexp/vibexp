import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import type {
  ResourceVersion,
  ResourceVersionListResponse,
} from '../types/version'

// Generated wire types for the artifact domain — the OpenAPI spec is the single
// source of truth; do not hand-write request/response shapes.
//
// DRIFT: the spec enumerates `type` as [work_reports, static_contexts, general]
// (the system-default types), but the backend validates it against the team's
// types — system defaults PLUS custom types (backend/internal/models/artifact.go)
// — so it is an open string, and the type dropdowns are populated dynamically via
// `useTypes()`. Widen `type` to `string` on the request/response/filter shapes so
// custom types type-check (deferred spec-gap follow-up); every other field stays
// generated. Widening is a loosening, so unwrapped responses remain assignable.
type WireArtifact = components['schemas']['Artifact']
export type Artifact = Omit<WireArtifact, 'type'> & { type: string }
export type ArtifactListResponse = Omit<
  components['schemas']['ArtifactListResponse'],
  'artifacts'
> & { artifacts: Artifact[] }
export type ArtifactStatsResponse =
  components['schemas']['ArtifactStatsResponse']
export type CreateArtifactRequest = Omit<
  components['schemas']['CreateArtifactRequest'],
  'type'
> & { type?: string }
export type UpdateArtifactRequest = Omit<
  components['schemas']['UpdateArtifactRequest'],
  'type'
> & { type?: string }

// Status union, derived from the generated `Artifact` schema; `type` is the open
// string above.
export type ArtifactStatus = Artifact['status']
export type ArtifactType = string

// List query params, sourced from the generated operations (the by-project variant
// carries `project_id` in the path, so its query omits it); `type` widened to match.
export type ArtifactFilters = Omit<
  NonNullable<operations['listArtifacts']['parameters']['query']>,
  'type'
> & { type?: string }
export type ArtifactByProjectFilters = Omit<
  NonNullable<operations['listArtifactsByProject']['parameters']['query']>,
  'type'
> & { type?: string }

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
          // `type` is widened to string (see the DRIFT note); the generated query
          // narrows it to the enum, so cast back at the boundary.
          query: filters as NonNullable<
            operations['listArtifacts']['parameters']['query']
          >,
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
          query: filters as NonNullable<
            operations['listArtifactsByProject']['parameters']['query']
          >,
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
        // `type` is widened to string (see the DRIFT note); cast back to the
        // generated body shape at the boundary.
        body: data as components['schemas']['CreateArtifactRequest'],
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
        body: data as components['schemas']['UpdateArtifactRequest'],
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

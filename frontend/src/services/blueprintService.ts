import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import type {
  ResourceVersion,
  ResourceVersionListResponse,
} from '../types/version'

// Generated wire types for the spec-library ("blueprint") domain — the OpenAPI
// spec is the single source of truth; do not hand-write request/response shapes.
export type Blueprint = components['schemas']['Blueprint']
export type CreateBlueprintRequest =
  components['schemas']['CreateBlueprintRequest']
export type UpdateBlueprintRequest =
  components['schemas']['UpdateBlueprintRequest']
export type BlueprintListResponse =
  components['schemas']['BlueprintListResponse']
export type BlueprintStatsResponse =
  components['schemas']['BlueprintStatsResponse']

// Type/subtype unions, derived from the generated `Blueprint` schema.
export type BlueprintType = Blueprint['type']
export type BlueprintSubtype = NonNullable<Blueprint['subtype']>

// Blueprint versions are just the generic resource version, with `resource_type`
// === "blueprint". Kept as aliases so blueprint call sites read naturally while
// the version-history module works against the resource-agnostic types.
export type BlueprintVersion = ResourceVersion
export type BlueprintVersionListResponse = ResourceVersionListResponse

// The generated list query is narrower than what the backend accepts (it omits
// `project_id`, `subtype`, `metadata_*`, and only documents `type: "general"`),
// so the richer hand-written filter shape is kept and serialized directly. The
// generated query serializer forwards every non-undefined key regardless.
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

// Type-to-subtype mapping for blueprint types
export const BLUEPRINT_TYPE_SUBTYPES: Record<string, string[]> = {
  general: [],
  'claude-code': ['sub-agents', 'skills', 'slash-commands', 'others'],
  claude: ['claude-md'],
  cursor: ['skills', 'agents', 'commands', 'rules', 'cursor-md'],
  codex: ['rules', 'skills', 'agents-md'],
}

type ListQuery = NonNullable<
  operations['listSpecLibraries']['parameters']['query']
>
type ListByProjectQuery = NonNullable<
  operations['listSpecLibrariesByProject']['parameters']['query']
>

class BlueprintService {
  async getBlueprints(
    teamId: string,
    filters: BlueprintFilters = {}
  ): Promise<BlueprintListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/blueprints', {
        params: {
          path: { team_id: teamId },
          query: filters as unknown as ListQuery,
        },
      })
    )
  }

  async getBlueprintsByProject(
    teamId: string,
    projectId: string,
    filters: Omit<BlueprintFilters, 'project_id'> = {}
  ): Promise<BlueprintListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/blueprints/{project_id}', {
        params: {
          path: { team_id: teamId, project_id: projectId },
          query: filters as unknown as ListByProjectQuery,
        },
      })
    )
  }

  async getBlueprint(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<Blueprint> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/blueprints/{project_id}/{slug}', {
        params: { path: { team_id: teamId, project_id: projectId, slug } },
      })
    )
  }

  async getBlueprintVersions(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<BlueprintVersionListResponse> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/blueprints/{project_id}/{slug}/versions',
        { params: { path: { team_id: teamId, project_id: projectId, slug } } }
      )
    )
  }

  async getBlueprintVersion(
    teamId: string,
    projectId: string,
    slug: string,
    versionNumber: number
  ): Promise<BlueprintVersion> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/blueprints/{project_id}/{slug}/versions/{version_number}',
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

  async restoreBlueprintVersion(
    teamId: string,
    projectId: string,
    slug: string,
    versionNumber: number
  ): Promise<Blueprint> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/blueprints/{project_id}/{slug}/versions/{version_number}/restore',
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

  async createBlueprint(
    teamId: string,
    data: CreateBlueprintRequest
  ): Promise<Blueprint> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/blueprints', {
        params: { path: { team_id: teamId } },
        body: data,
      })
    )
  }

  async updateBlueprint(
    teamId: string,
    projectId: string,
    slug: string,
    data: UpdateBlueprintRequest
  ): Promise<Blueprint> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/blueprints/{project_id}/{slug}', {
        params: { path: { team_id: teamId, project_id: projectId, slug } },
        body: data,
      })
    )
  }

  async deleteBlueprint(
    teamId: string,
    projectId: string,
    slug: string
  ): Promise<void> {
    await unwrap(
      generatedClient.DELETE(
        '/api/v1/{team_id}/blueprints/{project_id}/{slug}',
        { params: { path: { team_id: teamId, project_id: projectId, slug } } }
      )
    )
  }

  async getBlueprintStats(teamId: string): Promise<BlueprintStatsResponse> {
    try {
      return await unwrap(
        generatedClient.GET('/api/v1/{team_id}/blueprints/stats', {
          params: { path: { team_id: teamId } },
        })
      )
    } catch (error) {
      console.error('Failed to fetch blueprint stats:', error)
      return {
        total_projects: 0,
        total_spec_libraries: 0,
        added_this_week: 0,
        total_by_type: {},
        total_by_status: {},
      }
    }
  }
}

export const blueprintService = new BlueprintService()

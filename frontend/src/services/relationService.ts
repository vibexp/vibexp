import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the relations domain — the OpenAPI spec is the single
// source of truth; do not hand-write request/response shapes here.
export type Relation = components['schemas']['Relation']
export type RelatedResource = components['schemas']['RelatedResource']
export type CreateRelationRequest =
  components['schemas']['CreateRelationRequest']
export type RelationListResponse = components['schemas']['RelationListResponse']

/** The four resource kinds a relation can connect. */
export type RelationResourceType =
  | 'artifact'
  | 'memory'
  | 'prompt'
  | 'blueprint'

/** The four stored relation types. */
export type RelationType =
  | 'governed-by'
  | 'supersedes'
  | 'built-from'
  | 'explained-by'

/**
 * Thin wrapper over the generated client for typed resource relations
 * (commentService-shaped). Every method resolves through `unwrap` so failures
 * throw the same `ApiError` the rest of the SPA handles.
 */
class RelationService {
  async list(
    teamId: string,
    resourceType: RelationResourceType,
    resourceId: string,
    page?: number,
    limit?: number
  ): Promise<RelationListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/relations', {
        params: {
          path: { team_id: teamId },
          query: {
            resource_type: resourceType,
            resource_id: resourceId,
            page,
            limit,
          },
        },
      })
    )
  }

  /** Creates a human-authored edge (idempotent server-side; 201 new / 200 existing). */
  async create(teamId: string, body: CreateRelationRequest): Promise<Relation> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/relations', {
        params: { path: { team_id: teamId } },
        body,
      })
    )
  }

  /** Promotes a suggested edge to confirmed. */
  async confirm(teamId: string, relationId: string): Promise<Relation> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/relations/{relation_id}/confirm',
        {
          params: { path: { team_id: teamId, relation_id: relationId } },
        }
      )
    )
  }

  async remove(teamId: string, relationId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/relations/{relation_id}', {
        params: { path: { team_id: teamId, relation_id: relationId } },
      })
    )
  }
}

export const relationService = new RelationService()

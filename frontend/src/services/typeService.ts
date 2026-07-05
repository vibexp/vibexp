import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the resource-type domain — the OpenAPI spec is the
// single source of truth; do not hand-write request/response shapes here.
export type Type = components['schemas']['Type']
export type CreateTypeRequest = components['schemas']['CreateTypeRequest']
export type TypeListResponse = components['schemas']['TypeListResponse']

// typeService talks to the resource-type-agnostic /types endpoints (#1846).
// Every call is team-scoped (teamId first), mirroring artifactService.
class TypeService {
  async getTypes(teamId: string, resourceType: string): Promise<Type[]> {
    const response = await unwrap<TypeListResponse>(
      generatedClient.GET('/api/v1/{team_id}/types', {
        params: {
          path: { team_id: teamId },
          query: { resource_type: resourceType },
        },
      })
    )
    return response.types
  }

  async createType(teamId: string, data: CreateTypeRequest): Promise<Type> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/types', {
        params: { path: { team_id: teamId } },
        body: data,
      })
    )
  }

  async deleteType(teamId: string, id: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/types/{id}', {
        params: { path: { team_id: teamId, id } },
      })
    )
  }
}

export const typeService = new TypeService()

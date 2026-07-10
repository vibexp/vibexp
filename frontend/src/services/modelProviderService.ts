import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the model-provider domain — the OpenAPI spec is the
// single source of truth; do not hand-write request/response shapes here.
export type ModelProvider = components['schemas']['ModelProvider']
export type ModelProviderResponse =
  components['schemas']['ModelProviderResponse']
export type CreateModelProviderRequest =
  components['schemas']['CreateModelProviderRequest']
export type UpdateModelProviderRequest =
  components['schemas']['UpdateModelProviderRequest']
export type ModelProviderListResponse =
  components['schemas']['ModelProviderListResponse']
export type ValidateModelProviderRequest =
  components['schemas']['ValidateModelProviderRequest']
export type ValidateModelProviderResponse =
  components['schemas']['ValidateModelProviderResponse']

/**
 * Model-provider settings service backed by
 * `/api/v1/{team_id}/settings/model-providers`.
 *
 * Model providers are team-scoped (epic #109): every call is nested under the
 * current team's id. Authentication is the httpOnly session cookie sent by
 * `generatedClient` (`credentials: 'include'`); no `Authorization` header is
 * attached. The list route returns a bare array (no pagination envelope).
 */
class ModelProviderService {
  async createModelProvider(
    teamId: string,
    request: CreateModelProviderRequest
  ): Promise<ModelProviderResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/settings/model-providers', {
        params: { path: { team_id: teamId } },
        body: request,
      })
    )
  }

  async getModelProviders(teamId: string): Promise<ModelProviderResponse[]> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/settings/model-providers', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  async getModelProvider(
    teamId: string,
    id: string
  ): Promise<ModelProviderResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/settings/model-providers/{id}', {
        params: { path: { team_id: teamId, id } },
      })
    )
  }

  async updateModelProvider(
    teamId: string,
    id: string,
    request: UpdateModelProviderRequest
  ): Promise<ModelProviderResponse> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/settings/model-providers/{id}', {
        params: { path: { team_id: teamId, id } },
        body: request,
      })
    )
  }

  async deleteModelProvider(teamId: string, id: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE(
        '/api/v1/{team_id}/settings/model-providers/{id}',
        { params: { path: { team_id: teamId, id } } }
      )
    )
  }

  async validateModelProvider(
    teamId: string,
    request: ValidateModelProviderRequest
  ): Promise<ValidateModelProviderResponse> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/settings/model-providers/validate',
        { params: { path: { team_id: teamId } }, body: request }
      )
    )
  }
}

export const modelProviderService = new ModelProviderService()

import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the embedding-provider domain — the OpenAPI spec is
// the single source of truth; do not hand-write request/response shapes here.
export type EmbeddingProvider = components['schemas']['EmbeddingProvider']
export type EmbeddingProviderResponse =
  components['schemas']['EmbeddingProviderResponse']
export type CreateEmbeddingProviderRequest =
  components['schemas']['CreateEmbeddingProviderRequest']
export type UpdateEmbeddingProviderRequest =
  components['schemas']['UpdateEmbeddingProviderRequest']
export type EmbeddingProviderListResponse =
  components['schemas']['EmbeddingProviderListResponse']
export type ValidateEmbeddingProviderRequest =
  components['schemas']['ValidateEmbeddingProviderRequest']
export type ValidateEmbeddingProviderResponse =
  components['schemas']['ValidateEmbeddingProviderResponse']

// EMBEDDING_VECTOR_DIMENSIONS is the fixed vector width VibeXP stores (locked to
// the vector(1024) column). It is displayed read-only; a provider is accepted
// only if it returns embeddings of this width.
export const EMBEDDING_VECTOR_DIMENSIONS = 1024

/**
 * Embedding-provider settings service backed by
 * `/api/v1/{team_id}/settings/embedding-providers`.
 *
 * Embedding providers are team-scoped (issue #79): every call is nested under
 * the current team's id. Authentication is the httpOnly session cookie sent by
 * `generatedClient` (`credentials: 'include'`); no `Authorization` header is
 * attached. The list route returns a bare array (no pagination envelope).
 */
class EmbeddingProviderService {
  async createEmbeddingProvider(
    teamId: string,
    request: CreateEmbeddingProviderRequest
  ): Promise<EmbeddingProviderResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/settings/embedding-providers', {
        params: { path: { team_id: teamId } },
        body: request,
      })
    )
  }

  async getEmbeddingProviders(
    teamId: string
  ): Promise<EmbeddingProviderResponse[]> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/settings/embedding-providers', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  async getEmbeddingProvider(
    teamId: string,
    id: string
  ): Promise<EmbeddingProviderResponse> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/settings/embedding-providers/{id}',
        { params: { path: { team_id: teamId, id } } }
      )
    )
  }

  async updateEmbeddingProvider(
    teamId: string,
    id: string,
    request: UpdateEmbeddingProviderRequest
  ): Promise<EmbeddingProviderResponse> {
    return unwrap(
      generatedClient.PUT(
        '/api/v1/{team_id}/settings/embedding-providers/{id}',
        { params: { path: { team_id: teamId, id } }, body: request }
      )
    )
  }

  async deleteEmbeddingProvider(teamId: string, id: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE(
        '/api/v1/{team_id}/settings/embedding-providers/{id}',
        { params: { path: { team_id: teamId, id } } }
      )
    )
  }

  async validateEmbeddingProvider(
    teamId: string,
    request: ValidateEmbeddingProviderRequest
  ): Promise<ValidateEmbeddingProviderResponse> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/settings/embedding-providers/validate',
        { params: { path: { team_id: teamId } }, body: request }
      )
    )
  }
}

export const embeddingProviderService = new EmbeddingProviderService()

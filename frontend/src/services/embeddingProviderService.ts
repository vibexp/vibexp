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
// Renamed in the spec from `EmbeddingProviderListResponse`; the alias tracks
// the schema name so it stays greppable against the spec. Nothing consumes it
// today — the list call infers its own type — but every other wire type in this
// file is aliased here, so dropping it would be the odd one out.
export type EmbeddingProviderArrayResponse =
  components['schemas']['EmbeddingProviderArrayResponse']
export type ValidateEmbeddingProviderRequest =
  components['schemas']['ValidateEmbeddingProviderRequest']
export type ValidateEmbeddingProviderResponse =
  components['schemas']['ValidateEmbeddingProviderResponse']
export type EmbeddingCoverageResponse =
  components['schemas']['EmbeddingCoverageResponse']
export type EmbeddingCoverageItem =
  components['schemas']['EmbeddingCoverageItem']
export type ClearEmbeddingsResponse =
  components['schemas']['ClearEmbeddingsResponse']
export type CopyEmbeddingProviderRequest =
  components['schemas']['CopyEmbeddingProviderRequest']
export type CopyEmbeddingProviderResponse =
  components['schemas']['CopyEmbeddingProviderResponse']
export type EmbeddingProviderCopyActivation =
  components['schemas']['EmbeddingProviderCopyActivation']

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

  /**
   * Copies one embedding provider out of another team into `teamId`,
   * credential included (#831).
   *
   * The API key is deliberately absent from the request: it is write-only over
   * the API, so the server re-reads the source row's ciphertext and writes it
   * to the copy without ever decrypting it. Everything beyond the two source
   * identifiers and `reprocess` is an optional override of the source value.
   *
   * Unlike the other create-shaped calls this returns an envelope, not the bare
   * provider: `activation` is the server's verdict on what the copy did to the
   * destination team's SEARCH — a copy always lands non-default, but the active
   * provider resolves to "the default one, else the most recently updated one",
   * so it can still take over by recency alone. Callers must surface that
   * verdict rather than re-deriving it from `is_default`.
   */
  async copyEmbeddingProviderFromTeam(
    teamId: string,
    request: CopyEmbeddingProviderRequest
  ): Promise<CopyEmbeddingProviderResponse> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/settings/embedding-providers/copy',
        { params: { path: { team_id: teamId } }, body: request }
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

  /**
   * Derived, team-scoped embedding coverage (total / embedded / pending / %
   * per entity type) measured against the team's active provider model. Counts
   * come from existing rows — a non-decreasing pending count is the signal that
   * embedding is stuck.
   */
  async getEmbeddingCoverage(
    teamId: string
  ): Promise<EmbeddingCoverageResponse> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/settings/embedding-providers/coverage',
        { params: { path: { team_id: teamId } } }
      )
    )
  }

  /**
   * Enqueue a background re-drive of missing embeddings. Reprocess is
   * team-scoped (a provider is per-team), so `id` identifies the team's active
   * provider; the backend responds 202 and regenerates in the background.
   */
  async reprocessEmbeddingProvider(teamId: string, id: string): Promise<void> {
    await unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/settings/embedding-providers/{id}/reprocess',
        { params: { path: { team_id: teamId, id } } }
      )
    )
  }

  /**
   * Permanently delete (truncate) every stored embedding for the team, returning
   * how many rows were removed. This is a destructive maintenance action: unlike
   * reprocess it does not regenerate anything, so the team's content stays
   * unembedded — and semantic search returns nothing for it — until a reprocess
   * or an identity-changing provider update re-embeds the team.
   */
  async clearEmbeddings(teamId: string): Promise<ClearEmbeddingsResponse> {
    return unwrap(
      generatedClient.DELETE(
        '/api/v1/{team_id}/settings/embedding-providers/embeddings',
        { params: { path: { team_id: teamId } } }
      )
    )
  }
}

export const embeddingProviderService = new EmbeddingProviderService()

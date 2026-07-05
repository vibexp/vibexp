import { apiClient } from '../lib/apiClient'
import type {
  CreateEmbeddingProviderRequest,
  EmbeddingProviderResponse,
  UpdateEmbeddingProviderRequest,
  ValidateEmbeddingProviderRequest,
  ValidateEmbeddingProviderResponse,
} from '../types'

// Embedding providers are team-scoped (issue #79): every call is nested under
// the current team's id.
const base = (teamId: string) =>
  `/${encodeURIComponent(teamId)}/settings/embedding-providers`

class EmbeddingProviderService {
  async createEmbeddingProvider(
    teamId: string,
    request: CreateEmbeddingProviderRequest
  ): Promise<EmbeddingProviderResponse> {
    return apiClient.post<EmbeddingProviderResponse>(base(teamId), request)
  }

  async getEmbeddingProviders(
    teamId: string
  ): Promise<EmbeddingProviderResponse[]> {
    return apiClient.get<EmbeddingProviderResponse[]>(base(teamId))
  }

  async getEmbeddingProvider(
    teamId: string,
    id: string
  ): Promise<EmbeddingProviderResponse> {
    return apiClient.get<EmbeddingProviderResponse>(`${base(teamId)}/${id}`)
  }

  async updateEmbeddingProvider(
    teamId: string,
    id: string,
    request: UpdateEmbeddingProviderRequest
  ): Promise<EmbeddingProviderResponse> {
    return apiClient.put<EmbeddingProviderResponse>(
      `${base(teamId)}/${id}`,
      request
    )
  }

  async deleteEmbeddingProvider(teamId: string, id: string): Promise<void> {
    await apiClient.delete<Record<string, never>>(`${base(teamId)}/${id}`)
  }

  async validateEmbeddingProvider(
    teamId: string,
    request: ValidateEmbeddingProviderRequest
  ): Promise<ValidateEmbeddingProviderResponse> {
    return apiClient.post<ValidateEmbeddingProviderResponse>(
      `${base(teamId)}/validate`,
      request
    )
  }
}

export const embeddingProviderService = new EmbeddingProviderService()

import { apiClient } from '../lib/apiClient'
import type {
  CreateEmbeddingProviderRequest,
  EmbeddingProviderResponse,
  UpdateEmbeddingProviderRequest,
  ValidateEmbeddingProviderRequest,
  ValidateEmbeddingProviderResponse,
} from '../types'

class EmbeddingProviderService {
  async createEmbeddingProvider(
    request: CreateEmbeddingProviderRequest
  ): Promise<EmbeddingProviderResponse> {
    return apiClient.post<EmbeddingProviderResponse>(
      '/settings/embedding-providers',
      request
    )
  }

  async getEmbeddingProviders(): Promise<EmbeddingProviderResponse[]> {
    return apiClient.get<EmbeddingProviderResponse[]>(
      '/settings/embedding-providers'
    )
  }

  async getEmbeddingProvider(id: string): Promise<EmbeddingProviderResponse> {
    return apiClient.get<EmbeddingProviderResponse>(
      `/settings/embedding-providers/${id}`
    )
  }

  async updateEmbeddingProvider(
    id: string,
    request: UpdateEmbeddingProviderRequest
  ): Promise<EmbeddingProviderResponse> {
    return apiClient.put<EmbeddingProviderResponse>(
      `/settings/embedding-providers/${id}`,
      request
    )
  }

  async deleteEmbeddingProvider(id: string): Promise<void> {
    await apiClient.delete<Record<string, never>>(
      `/settings/embedding-providers/${id}`
    )
  }

  async validateEmbeddingProvider(
    request: ValidateEmbeddingProviderRequest
  ): Promise<ValidateEmbeddingProviderResponse> {
    return apiClient.post<ValidateEmbeddingProviderResponse>(
      '/settings/embedding-providers/validate',
      request
    )
  }
}

export const embeddingProviderService = new EmbeddingProviderService()

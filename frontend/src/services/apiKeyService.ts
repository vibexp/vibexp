import { apiClient } from '../lib/apiClient'
import type {
  APIKey,
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
} from '../types'

class APIKeyService {
  async createAPIKey(
    request: CreateAPIKeyRequest
  ): Promise<CreateAPIKeyResponse> {
    return apiClient.post<CreateAPIKeyResponse>('/settings/api-keys', request)
  }

  async getAPIKeys(): Promise<APIKey[]> {
    return apiClient.get<APIKey[]>('/settings/api-keys')
  }

  async deleteAPIKey(apiKeyId: string): Promise<void> {
    await apiClient.delete<Record<string, never>>(
      `/settings/api-keys/${apiKeyId}`
    )
  }
}

export const apiKeyService = new APIKeyService()

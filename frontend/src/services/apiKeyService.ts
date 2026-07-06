import type { components } from '@vibexp/api-client'

import { apiClient } from '../lib/apiClient'

// Generated wire types for the API-key domain — the OpenAPI spec is the single
// source of truth. `integrations` / `integration_codes` are the closed union
// `("ai_tools" | "cli" | "mcp_server")[]`; `IntegrationCode` names one member
// for the settings UI (form + badges).
export type APIKey = components['schemas']['APIKey']
export type CreateAPIKeyRequest = components['schemas']['CreateAPIKeyRequest']
export type CreateAPIKeyResponse = components['schemas']['CreateAPIKeyResponse']
export type IntegrationCode = CreateAPIKeyRequest['integration_codes'][number]

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

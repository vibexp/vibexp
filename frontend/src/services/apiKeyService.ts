import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

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
    return unwrap(
      generatedClient.POST('/api/v1/settings/api-keys', { body: request })
    )
  }

  async getAPIKeys(): Promise<APIKey[]> {
    // Spec drift: `listAPIKeysSettings` documents an APIKeyListResponse envelope,
    // but the backend (handleListAPIKeys → writeOK) returns a bare APIKey[]
    // array. Cast through the real runtime shape until the spec/handler are
    // reconciled (tracked as a backend response-contract follow-up).
    const result = await unwrap(
      generatedClient.GET('/api/v1/settings/api-keys', {})
    )
    return result as unknown as APIKey[]
  }

  async deleteAPIKey(apiKeyId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/settings/api-keys/{id}', {
        params: { path: { id: apiKeyId } },
      })
    )
  }
}

export const apiKeyService = new APIKeyService()

// Embedding Provider types
export interface EmbeddingProvider {
  id: string
  user_id: string
  name: string
  provider_type: string
  is_default: boolean
  base_url?: string
  configuration: string
  created_at: string
  updated_at: string
}

export interface EmbeddingProviderResponse extends EmbeddingProvider {
  has_api_key: boolean
}

export interface CreateEmbeddingProviderRequest {
  name: string
  provider_type: string
  is_default?: boolean
  base_url?: string
  api_key?: string
  configuration?: Record<string, unknown>
}

export interface UpdateEmbeddingProviderRequest {
  name?: string
  provider_type?: string
  is_default?: boolean
  base_url?: string
  api_key?: string
  configuration?: Record<string, unknown>
}

export interface EmbeddingProviderListResponse {
  embedding_providers: EmbeddingProviderResponse[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}

export interface ValidateEmbeddingProviderRequest {
  provider_type: string
  base_url: string
  api_key?: string
  configuration?: Record<string, unknown>
}

export interface ValidateEmbeddingProviderResponse {
  is_valid: boolean
  message: string
  details?: {
    response_time_ms?: number
    status_code?: number
    error_details?: string
  }
}

// Embedding Provider types
export interface EmbeddingProvider {
  id: string
  user_id: string
  team_id?: string
  name: string
  provider_type: string
  model: string
  chunk_size: number
  chunk_overlap: number
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
  model: string
  chunk_size?: number
  chunk_overlap?: number
  is_default?: boolean
  base_url?: string
  api_key?: string
  configuration?: Record<string, unknown>
}

export interface UpdateEmbeddingProviderRequest {
  name?: string
  provider_type?: string
  model?: string
  chunk_size?: number
  chunk_overlap?: number
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
  model: string
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
    dimension?: number
    error_details?: string
  }
}

// EMBEDDING_VECTOR_DIMENSIONS is the fixed vector width VibeXP stores (locked to
// the vector(1024) column). It is displayed read-only; a provider is accepted
// only if it returns embeddings of this width.
export const EMBEDDING_VECTOR_DIMENSIONS = 1024

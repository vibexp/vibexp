import type { ApiResponse } from './api'
import type { Prompt } from './prompt'

// Prompt Gallery types
export interface PromptGalleryTemplate {
  id: string
  title: string
  description: string
  content: string
  category: string
  tags: string[]
  metadata: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface PromptGalleryCategory {
  category: string
  count: number
}

export interface PromptGalleryListResponse {
  prompts: PromptGalleryTemplate[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}

export interface PromptGalleryFilters {
  category?: string
  search?: string
  tags?: string[] // Array of tags to filter by
  page?: number
  limit?: number
}

export interface PromptGalleryUsageRequest {
  prompt_id: string
}

export type PromptGalleryCategoriesResponse = ApiResponse<
  PromptGalleryCategory[]
>
export type PromptGalleryPromptsResponse =
  ApiResponse<PromptGalleryListResponse>
export type PromptGalleryTemplateResponse = ApiResponse<PromptGalleryTemplate>

// Prompt Sharing types
export interface CreateShareRequest {
  share_type: 'public' | 'restricted'
  emails?: string[]
  expires_at?: string
}

export interface ShareResponse {
  share_token: string
  share_url: string
  share_type: 'public' | 'restricted'
  emails?: string[]
  expires_at?: string
  created_at: string
}

export interface SharedPromptResponse {
  prompt: Prompt
  share_type: 'public' | 'restricted'
  rendered_body: string
  expires_at?: string
}

export type CreateShareApiResponse = ShareResponse
export type GetShareApiResponse = ShareResponse
export type SharedPromptApiResponse = ApiResponse<SharedPromptResponse>

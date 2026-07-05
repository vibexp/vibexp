import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the prompt-gallery domain — the OpenAPI spec is the
// single source of truth; do not hand-write request/response shapes here.
export type PromptGalleryTemplate =
  components['schemas']['PromptGalleryTemplate']
export type PromptGalleryCategory =
  components['schemas']['PromptGalleryCategory']
export type PromptGalleryListResponse =
  components['schemas']['PromptGalleryListResponse']
export type PromptGalleryUsageRequest =
  components['schemas']['PromptGalleryUsageRequest']

// The generated list query has no `tags` field, so the richer hand-written
// filter shape is kept; `tags` is serialized as a comma-separated string.
export interface PromptGalleryFilters {
  category?: string
  search?: string
  tags?: string[] // Array of tags to filter by
  page?: number
  limit?: number
}

type ListQuery = NonNullable<
  operations['listPromptGalleryPrompts']['parameters']['query']
>

class PromptGalleryService {
  async getCategories(): Promise<PromptGalleryCategory[]> {
    return unwrap(generatedClient.GET('/api/v1/prompt-gallery/categories'))
  }

  async getPrompts(
    filters: PromptGalleryFilters = {}
  ): Promise<PromptGalleryListResponse> {
    const { tags, ...rest } = filters
    const query: ListQuery = {
      ...rest,
      ...(tags && tags.length > 0 ? { tags: tags.join(',') } : {}),
    }
    return unwrap(
      generatedClient.GET('/api/v1/prompt-gallery/prompts', {
        params: { query },
      })
    )
  }

  async getPromptById(id: string): Promise<PromptGalleryTemplate> {
    return unwrap(
      generatedClient.GET('/api/v1/prompt-gallery/prompts/{id}', {
        params: { path: { id } },
      })
    )
  }

  async trackPromptUsage(promptId: string): Promise<void> {
    await unwrap(
      generatedClient.POST('/api/v1/prompt-gallery/prompts/{id}/use', {
        params: { path: { id: promptId } },
      })
    )
  }
}

export const promptGalleryService = new PromptGalleryService()

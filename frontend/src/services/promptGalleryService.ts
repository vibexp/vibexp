import { apiClient } from '../lib/apiClient'
import type {
  PromptGalleryCategory,
  PromptGalleryFilters,
  PromptGalleryListResponse,
  PromptGalleryTemplate,
  PromptGalleryUsageRequest,
} from '../types'

class PromptGalleryService {
  async getCategories(): Promise<PromptGalleryCategory[]> {
    const response = await apiClient.get<PromptGalleryCategory[]>(
      '/prompt-gallery/categories'
    )
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
    return response || []
  }

  async getPrompts(
    filters: PromptGalleryFilters = {}
  ): Promise<PromptGalleryListResponse> {
    const params = new URLSearchParams()

    if (filters.category) params.append('category', filters.category)
    if (filters.search) params.append('search', filters.search)
    if (filters.tags && filters.tags.length > 0) {
      params.append('tags', filters.tags.join(','))
    }
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())

    const queryString = params.toString()
    const endpoint = `/prompt-gallery/prompts${queryString ? `?${queryString}` : ''}`

    const response = await apiClient.get<PromptGalleryListResponse>(endpoint)
    return (
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
      response || {
        prompts: [],
        total_count: 0,
        page: 1,
        per_page: 10,
        total_pages: 0,
      }
    )
  }

  async getPromptById(id: string): Promise<PromptGalleryTemplate> {
    const response = await apiClient.get<PromptGalleryTemplate>(
      `/prompt-gallery/prompts/${id}`
    )
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
    if (!response) {
      throw new Error('Prompt not found')
    }
    return response
  }

  async trackPromptUsage(promptId: string): Promise<void> {
    const data: PromptGalleryUsageRequest = { prompt_id: promptId }
    // eslint-disable-next-line @typescript-eslint/no-invalid-void-type
    await apiClient.post<void>(`/prompt-gallery/prompts/${promptId}/use`, data)
  }
}

export const promptGalleryService = new PromptGalleryService()

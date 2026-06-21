import { apiClient } from '../lib/apiClient'
import type {
  CreatePromptRequest,
  Prompt,
  PromptDependenciesApiResponse,
  PromptDependenciesResponse,
  PromptFilters,
  PromptsResponse,
  PromptVersion,
  PromptVersionListResponse,
  RenderPromptResponse,
  UpdatePromptRequest,
} from '../types'

/**
 * Normalize a single-prompt API response into a bare {@link Prompt}.
 *
 * The backend returns the prompt object directly for single-prompt
 * endpoints, but may also be wrapped in a {status, message, data}
 * envelope. This unwraps both shapes so callers never read `.data`.
 */
function unwrapPrompt(response: unknown): Prompt {
  // A `Prompt` has no `data` field, so the presence of a top-level `data`
  // key reliably distinguishes the envelope from a bare prompt. If `Prompt`
  // ever gains a `data` field, this discriminator must be revisited.
  const isEnveloped =
    typeof response === 'object' && response !== null && 'data' in response
  const data = isEnveloped
    ? (response as { data?: Prompt }).data
    : (response as Prompt | undefined)

  if (!data || typeof data !== 'object') {
    throw new Error('No prompt data received from server')
  }

  return data
}

class PromptService {
  async getPrompts(
    teamId: string,
    filters: PromptFilters = {}
  ): Promise<PromptsResponse> {
    const params = new URLSearchParams()

    // Remove team_id from query params - it's now in the URL path
    if (filters.status) params.append('status', filters.status)
    if (filters.search) params.append('search', filters.search)
    if (filters.shared !== undefined)
      params.append('shared', filters.shared.toString())
    if (filters.labels && filters.labels.length > 0) {
      params.append('labels', filters.labels.join(','))
    }
    if (filters.project_id) params.append('project_id', filters.project_id)
    if (filters.sort_by) params.append('sort_by', filters.sort_by)
    if (filters.sort_order) params.append('sort_order', filters.sort_order)
    if (filters.page) params.append('page', filters.page.toString())
    if (filters.limit) params.append('limit', filters.limit.toString())

    const queryString = params.toString()
    const endpoint = `/${teamId}/prompts${queryString ? `?${queryString}` : ''}`

    return apiClient.get<PromptsResponse>(endpoint)
  }

  async getPrompt(teamId: string, slug: string): Promise<Prompt> {
    const response = await apiClient.get<unknown>(`/${teamId}/prompts/${slug}`)
    return unwrapPrompt(response)
  }

  async createPrompt(
    teamId: string,
    data: CreatePromptRequest
  ): Promise<Prompt> {
    const response = await apiClient.post<unknown>(`/${teamId}/prompts`, data)
    return unwrapPrompt(response)
  }

  async updatePrompt(
    teamId: string,
    slug: string,
    data: UpdatePromptRequest
  ): Promise<Prompt> {
    const response = await apiClient.put<unknown>(
      `/${teamId}/prompts/${slug}`,
      data
    )
    return unwrapPrompt(response)
  }

  async deletePrompt(teamId: string, slug: string): Promise<void> {
    await apiClient.delete(`/${teamId}/prompts/${slug}`)
  }

  async getPromptPlaceholders(teamId: string, slug: string): Promise<string[]> {
    const response = await apiClient.get<{ placeholders: string[] }>(
      `/${teamId}/prompts/${slug}/placeholders`
    )
    return (
      (response as { placeholders?: string[] } | undefined)?.placeholders ?? []
    )
  }

  async renderPrompt(
    teamId: string,
    slug: string,
    placeholders: Record<string, string>
  ): Promise<RenderPromptResponse> {
    return apiClient.post<RenderPromptResponse>(
      `/${teamId}/prompts/${slug}/render`,
      {
        placeholders,
      }
    )
  }

  async getPromptDependencies(
    teamId: string,
    slug: string
  ): Promise<PromptDependenciesResponse> {
    const response = await apiClient.get<PromptDependenciesApiResponse>(
      `/${teamId}/prompts/${slug}/dependencies`
    )
    // Handle both wrapped {data: {...}} and unwrapped {...} responses

    const responseData: PromptDependenciesResponse =
      (response as { data?: PromptDependenciesResponse } | undefined)?.data ??
      (response as unknown as PromptDependenciesResponse)

    // Ensure we always return arrays, never null
    const data = responseData as PromptDependenciesResponse | undefined
    if (!data) {
      return { used_by: [], uses: [] }
    }

    return {
      used_by:
        (data as { used_by?: PromptDependenciesResponse['used_by'] }).used_by ??
        [],
      uses: (data as { uses?: PromptDependenciesResponse['uses'] }).uses ?? [],
    }
  }

  // Content version history. Slug-addressed within a team (no project_id), mirroring
  // the prompt CRUD endpoints. The versioned content is the raw body template.
  async getPromptVersions(
    teamId: string,
    slug: string
  ): Promise<PromptVersionListResponse> {
    return apiClient.get<PromptVersionListResponse>(
      `/${encodeURIComponent(teamId)}/prompts/${encodeURIComponent(slug)}/versions`
    )
  }

  async getPromptVersion(
    teamId: string,
    slug: string,
    versionNumber: number
  ): Promise<PromptVersion> {
    return apiClient.get<PromptVersion>(
      `/${encodeURIComponent(teamId)}/prompts/${encodeURIComponent(slug)}/versions/${encodeURIComponent(versionNumber)}`
    )
  }

  async restorePromptVersion(
    teamId: string,
    slug: string,
    versionNumber: number
  ): Promise<Prompt> {
    const response = await apiClient.post<unknown>(
      `/${encodeURIComponent(teamId)}/prompts/${encodeURIComponent(slug)}/versions/${encodeURIComponent(versionNumber)}/restore`
    )
    return unwrapPrompt(response)
  }

  async getPromptLabels(teamId: string): Promise<string[]> {
    // Remove team_id from query params - it's now in the URL path
    const response = await apiClient.get<{ data: { labels: string[] } }>(
      `/${teamId}/prompts/labels`
    )
    return (
      (response as { data?: { labels?: string[] } } | undefined)?.data
        ?.labels ?? []
    )
  }
}

export const promptService = new PromptService()

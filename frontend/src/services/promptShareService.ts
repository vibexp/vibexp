import { apiClient } from '../lib/apiClient'
import type {
  CreateShareApiResponse,
  CreateShareRequest,
  GetShareApiResponse,
  SharedPromptApiResponse,
} from '../types'

class PromptShareService {
  async createShare(
    teamId: string,
    slug: string,
    data: CreateShareRequest
  ): Promise<CreateShareApiResponse> {
    return apiClient.post<CreateShareApiResponse>(
      `/${teamId}/prompts/${slug}/share`,
      data
    )
  }

  async getShare(teamId: string, slug: string): Promise<GetShareApiResponse> {
    return apiClient.get<GetShareApiResponse>(
      `/${teamId}/prompts/${slug}/share`
    )
  }

  async deleteShare(teamId: string, slug: string): Promise<void> {
    await apiClient.delete(`/${teamId}/prompts/${slug}/share`)
  }

  async getSharedPrompt(token: string): Promise<SharedPromptApiResponse> {
    return apiClient.get<SharedPromptApiResponse>(`/shared/prompts/${token}`)
  }
}

export const promptShareService = new PromptShareService()

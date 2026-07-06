import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the prompt-sharing domain — the OpenAPI spec is the
// single source of truth; do not hand-write request/response shapes here.
export type CreateShareRequest = components['schemas']['CreateShareRequest']
export type ShareResponse = components['schemas']['ShareResponse']
export type SharedPromptResponse = components['schemas']['SharedPromptResponse']

class PromptShareService {
  async createShare(
    teamId: string,
    slug: string,
    data: CreateShareRequest
  ): Promise<ShareResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/prompts/{slug}/share', {
        params: { path: { team_id: teamId, slug } },
        body: data,
      })
    )
  }

  async getShare(teamId: string, slug: string): Promise<ShareResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/prompts/{slug}/share', {
        params: { path: { team_id: teamId, slug } },
      })
    )
  }

  async deleteShare(teamId: string, slug: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/prompts/{slug}/share', {
        params: { path: { team_id: teamId, slug } },
      })
    )
  }

  async getSharedPrompt(token: string): Promise<SharedPromptResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/shared/prompts/{token}', {
        params: { path: { token } },
      })
    )
  }
}

export const promptShareService = new PromptShareService()

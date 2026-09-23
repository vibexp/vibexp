import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the team AI-summary-settings domain — the OpenAPI
// spec is the single source of truth; do not hand-write request/response shapes.
export type TeamAISummarySettings =
  components['schemas']['TeamAISummarySettings']
export type TeamAISummarySettingsValues =
  components['schemas']['TeamAISummarySettingsValues']
export type UpdateTeamAISummarySettingsRequest =
  components['schemas']['UpdateTeamAISummarySettingsRequest']

/**
 * Team AI-summary-settings service backed by
 * `/api/v1/{team_id}/settings/ai-summary` (epic #1068).
 *
 * Like search settings, the profile is a per-team SINGLETON: a team either
 * stores a complete profile or inherits the instance defaults, so an update
 * replaces the whole profile and a reset drops it.
 *
 * The GET response carries everything the settings card needs in one call:
 * the effective values, their provenance (`source`), the `instance_defaults`
 * to preview a reset against, the instance-owned `max_top_n` ceiling, and
 * whether the team has a model provider at all (`available`).
 */
class AISummarySettingsService {
  async getAISummarySettings(teamId: string): Promise<TeamAISummarySettings> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/settings/ai-summary', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  async updateAISummarySettings(
    teamId: string,
    request: UpdateTeamAISummarySettingsRequest
  ): Promise<TeamAISummarySettings> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/settings/ai-summary', {
        params: { path: { team_id: teamId } },
        body: request,
      })
    )
  }

  async resetAISummarySettings(teamId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/settings/ai-summary', {
        params: { path: { team_id: teamId } },
      })
    )
  }
}

export const aiSummarySettingsService = new AISummarySettingsService()

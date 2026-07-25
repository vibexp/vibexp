import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the team search-settings domain — the OpenAPI spec
// is the single source of truth; do not hand-write request/response shapes.
export type TeamSearchSettings = components['schemas']['TeamSearchSettings']
export type TeamSearchSettingsValues =
  components['schemas']['TeamSearchSettingsValues']
export type UpdateTeamSearchSettingsRequest =
  components['schemas']['UpdateTeamSearchSettingsRequest']

/**
 * Team search-settings service backed by `/api/v1/{team_id}/settings/search`.
 *
 * The settings are a per-team SINGLETON (epic #487): a team either stores a
 * complete ranking profile or stores nothing and inherits the instance
 * defaults from `config.yaml`. That is why there is no partial update —
 * `updateSearchSettings` replaces the whole profile, and `resetSearchSettings`
 * drops it so the team inherits again.
 *
 * The GET response carries everything the settings page needs in one call:
 * the effective values, their provenance (`source`), the `instance_defaults`
 * to preview a reset against, and the instance-owned `rank_candidate_cap`.
 */
class SearchSettingsService {
  async getSearchSettings(teamId: string): Promise<TeamSearchSettings> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/settings/search', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  async updateSearchSettings(
    teamId: string,
    request: UpdateTeamSearchSettingsRequest
  ): Promise<TeamSearchSettings> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/settings/search', {
        params: { path: { team_id: teamId } },
        body: request,
      })
    )
  }

  async resetSearchSettings(teamId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/settings/search', {
        params: { path: { team_id: teamId } },
      })
    )
  }
}

export const searchSettingsService = new SearchSettingsService()

import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the freshness domain — the OpenAPI spec is the
// single source of truth; do not hand-write request/response shapes.
export type FreshnessRule = components['schemas']['FreshnessRule']
export type FreshnessRuleResourceType =
  components['schemas']['FreshnessRuleResourceType']
export type FreshnessRuleMedium = components['schemas']['FreshnessRuleMedium']
export type CreateFreshnessRuleRequest =
  components['schemas']['CreateFreshnessRuleRequest']
export type UpdateFreshnessRuleRequest =
  components['schemas']['UpdateFreshnessRuleRequest']
export type TeamFreshnessSettings =
  components['schemas']['TeamFreshnessSettings']
export type UpdateTeamFreshnessSettingsRequest =
  components['schemas']['UpdateTeamFreshnessSettingsRequest']

/**
 * Resource Freshness service backed by `/api/v1/{team_id}/freshness/**` and
 * `/api/v1/{team_id}/settings/freshness` (epic #726).
 *
 * Two surfaces with different shapes, deliberately:
 *
 *  - **Rules** are a collection. Reads are open to any member; writes require
 *    `team.settings.update`. `updateRule` is a full replacement (PUT), because
 *    an omitted field would otherwise silently reset that dimension of the
 *    rule — the server requires every mutable field.
 *  - **Settings** are a per-team SINGLETON with the same inherit-or-override
 *    model as search settings (#487): a team either stores its own values or
 *    stores nothing and reports `source: instance`. Hence no partial update —
 *    `updateSettings` replaces both values and `resetSettings` drops the
 *    override so the team inherits again. The GET carries `defaults` on every
 *    read so a reset can be previewed without a second call.
 */
class FreshnessService {
  async getRules(teamId: string): Promise<FreshnessRule[]> {
    const response = await unwrap(
      generatedClient.GET('/api/v1/{team_id}/freshness/rules', {
        params: { path: { team_id: teamId } },
      })
    )
    return response.rules
  }

  async createRule(
    teamId: string,
    request: CreateFreshnessRuleRequest
  ): Promise<FreshnessRule> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/freshness/rules', {
        params: { path: { team_id: teamId } },
        body: request,
      })
    )
  }

  async updateRule(
    teamId: string,
    ruleId: string,
    request: UpdateFreshnessRuleRequest
  ): Promise<FreshnessRule> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/freshness/rules/{rule_id}', {
        params: { path: { team_id: teamId, rule_id: ruleId } },
        body: request,
      })
    )
  }

  async deleteRule(teamId: string, ruleId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/freshness/rules/{rule_id}', {
        params: { path: { team_id: teamId, rule_id: ruleId } },
      })
    )
  }

  async getSettings(teamId: string): Promise<TeamFreshnessSettings> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/settings/freshness', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  async updateSettings(
    teamId: string,
    request: UpdateTeamFreshnessSettingsRequest
  ): Promise<TeamFreshnessSettings> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/settings/freshness', {
        params: { path: { team_id: teamId } },
        body: request,
      })
    )
  }

  async resetSettings(teamId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/settings/freshness', {
        params: { path: { team_id: teamId } },
      })
    )
  }
}

export const freshnessService = new FreshnessService()

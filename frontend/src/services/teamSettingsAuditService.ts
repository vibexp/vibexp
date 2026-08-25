import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the team settings audit domain (#832) — the OpenAPI
// spec is the single source of truth; do not hand-write these shapes.
export type TeamSettingsAuditEntry =
  components['schemas']['TeamSettingsAuditEntry']
export type TeamSettingsAuditSurface =
  components['schemas']['TeamSettingsAuditSurface']
export type TeamSettingsAuditListResponse =
  components['schemas']['TeamSettingsAuditListResponse']

/**
 * The team settings audit log, backed by `GET /api/v1/{team_id}/settings/audit`
 * (#832, epic #827).
 *
 * Read-only by construction: the log is written by the cross-team copy
 * endpoints and has no client-facing mutations. Every entry is a snapshot taken
 * at copy time — the actor and source-team *names* are the only two fields the
 * server resolves live, which is why both are nullable (the account may be
 * deleted, and the source team carries no foreign key by design).
 */
class TeamSettingsAuditService {
  async getAudit(
    teamId: string,
    page: number,
    limit: number,
    signal?: AbortSignal
  ): Promise<TeamSettingsAuditListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/settings/audit', {
        params: {
          path: { team_id: teamId },
          query: { page, limit },
        },
        signal,
      })
    )
  }
}

export const teamSettingsAuditService = new TeamSettingsAuditService()

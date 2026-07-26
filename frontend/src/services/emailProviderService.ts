import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the team email-provider domain — the OpenAPI spec
// is the single source of truth; do not hand-write request/response shapes.
export type TeamEmailProviderResponse =
  components['schemas']['TeamEmailProviderResponse']
export type UpsertTeamEmailProviderRequest =
  components['schemas']['UpsertTeamEmailProviderRequest']
export type TeamEmailProviderTestResponse =
  components['schemas']['TeamEmailProviderTestResponse']
export type TeamEmailProviderSettings =
  components['schemas']['TeamEmailProviderSettings']

/** The four provider types a team may send through. */
export type EmailProviderType = NonNullable<
  TeamEmailProviderResponse['provider_type']
>

/**
 * Team email-provider service backed by `/api/v1/{team_id}/settings/email-provider`.
 *
 * The provider is a per-team SINGLETON (epic #499): a team either stores its own
 * provider or stores nothing and inherits the instance provider configured in
 * `config.yaml`. The absence of a row IS the fallback, which is why `get` never
 * 404s — it always describes the configuration in force, reporting
 * `configured: false` / `source: "instance"` when the team inherits.
 *
 * Two consequences worth knowing before calling this:
 *
 *  * `secret` is write-only. No response type can carry it, so a configured
 *    provider reports `has_credential` instead. Omit `secret` from an upsert to
 *    keep the stored one; an empty string is rejected server-side rather than
 *    treated as "clear", because a provider that cannot send would silently
 *    disable the team's mail.
 *  * `test` sends with the configuration in the REQUEST BODY, not the stored
 *    one, so credentials can be checked before they are saved — which means it
 *    always requires `secret`, even for an already-configured team. The
 *    recipient is always the acting user's own account email; it cannot be
 *    supplied by the caller.
 */
class EmailProviderService {
  async getEmailProvider(teamId: string): Promise<TeamEmailProviderResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/settings/email-provider', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  async upsertEmailProvider(
    teamId: string,
    request: UpsertTeamEmailProviderRequest
  ): Promise<TeamEmailProviderResponse> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/settings/email-provider', {
        params: { path: { team_id: teamId } },
        body: request,
      })
    )
  }

  /** Reverts the team to the instance provider. 409 when it has none of its own. */
  async deleteEmailProvider(teamId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/settings/email-provider', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  /**
   * Sends a test message with `request` — a failed send comes back as
   * `is_valid: false`, not as a thrown error, so callers must read the body.
   */
  async testEmailProvider(
    teamId: string,
    request: UpsertTeamEmailProviderRequest
  ): Promise<TeamEmailProviderTestResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/settings/email-provider/test', {
        params: { path: { team_id: teamId } },
        body: request,
      })
    )
  }
}

export const emailProviderService = new EmailProviderService()

import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import { ApiError } from '../types/errors'

// Generated wire types for the GitHub App configuration domain — the OpenAPI
// spec is the single source of truth; do not hand-write request/response
// shapes here.
export type GitHubAppConfigResponse =
  components['schemas']['GitHubAppConfigResponse']
export type CreateGitHubAppConfigRequest =
  components['schemas']['CreateGitHubAppConfigRequest']
export type CreateGitHubAppConfigResponse =
  components['schemas']['CreateGitHubAppConfigResponse']
export type UpdateGitHubAppConfigRequest =
  components['schemas']['UpdateGitHubAppConfigRequest']
export type ValidateGitHubAppConfigResponse =
  components['schemas']['ValidateGitHubAppConfigResponse']
export type ValidateGitHubAppConfigDetails =
  components['schemas']['ValidateGitHubAppConfigDetails']

/** The fixed error categories the validate probe reports (#464: never an oracle). */
export type GitHubAppValidationErrorCode = NonNullable<
  ValidateGitHubAppConfigDetails['error_details']
>

/**
 * Per-team GitHub App configuration, backed by
 * `/api/v1/{team_id}/settings/github-app` (epic #476).
 *
 * Each team registers its own GitHub App; there is no instance-wide App any
 * more. Every call is nested under the current team's id, and authentication is
 * the httpOnly session cookie sent by `generatedClient`.
 *
 * Secrets travel in one direction only. `private_key` and `client_secret` exist
 * on requests and are never returned; the webhook secret is disclosed exactly
 * once, by `createAppConfig`, and afterwards a read reports only
 * `has_webhook_secret`.
 */
class GitHubAppConfigService {
  /**
   * Returns the team's App configuration.
   *
   * Throws an `ApiError` with status 409 when the team has no App registered —
   * that is the documented "not configured yet" signal, not a failure. Callers
   * distinguish it with {@link isNotConfigured}.
   */
  async getAppConfig(teamId: string): Promise<GitHubAppConfigResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/settings/github-app', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  async createAppConfig(
    teamId: string,
    request: CreateGitHubAppConfigRequest
  ): Promise<CreateGitHubAppConfigResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/settings/github-app', {
        params: { path: { team_id: teamId } },
        body: request,
      })
    )
  }

  /**
   * Updates the registration. Omitted secret fields keep the stored value —
   * the UI never receives the current secrets, so it cannot resend them. An
   * explicitly empty secret is a validation error server-side, never a silent
   * clear, so callers must send `undefined` rather than `''`.
   */
  async updateAppConfig(
    teamId: string,
    request: UpdateGitHubAppConfigRequest
  ): Promise<GitHubAppConfigResponse> {
    return unwrap(
      generatedClient.PUT('/api/v1/{team_id}/settings/github-app', {
        params: { path: { team_id: teamId } },
        body: request,
      })
    )
  }

  async deleteAppConfig(teamId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/{team_id}/settings/github-app', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  /**
   * Proves the stored credentials work by calling GitHub's `GET /app`.
   *
   * A failed probe comes back with `is_valid: false` in the body, not as a
   * thrown error — bad credentials are a user-correctable condition, not a
   * server fault.
   */
  async validateAppConfig(
    teamId: string
  ): Promise<ValidateGitHubAppConfigResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/settings/github-app/validate', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  /**
   * Mints a new routing token, which changes the webhook URL. Deliveries stop
   * until the new URL is pasted into the App's settings on GitHub.
   */
  async rotateWebhookToken(teamId: string): Promise<GitHubAppConfigResponse> {
    return unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/settings/github-app/rotate-webhook-token',
        { params: { path: { team_id: teamId } } }
      )
    )
  }
}

export const githubAppConfigService = new GitHubAppConfigService()

/**
 * Whether an error means "this team has not registered an App yet".
 *
 * The backend answers 409 rather than 404 here: the endpoint exists and the
 * team is addressable, it is simply not in a state that can serve the request.
 * That makes it a normal empty state for the settings page, not an error to
 * report — so it is worth a named predicate rather than a bare status check
 * scattered across components.
 */
export function isGitHubAppNotConfigured(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 409 &&
    error.code === 'GITHUB_APP_NOT_CONFIGURED'
  )
}

import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the GitHub integration domain — the OpenAPI spec is the
// single source of truth; do not hand-write request/response shapes.
export type GitHubRepository = components['schemas']['GitHubRepository']
export type GitHubRepositoriesResponse =
  components['schemas']['GitHubRepositoriesResponse']
export type GitHubInstallUrlResponse = components['schemas']['GitHubInstallURL']
export type GitHubCallbackResponse =
  components['schemas']['GitHubCallbackResponse']
export type BlueprintImportSuccess =
  components['schemas']['BlueprintImportSuccess']
export type BlueprintImportFailed =
  components['schemas']['BlueprintImportFailed']
export type BlueprintImportSkipped =
  components['schemas']['BlueprintImportSkipped']

// DRIFT: the backend serializes the successful items as `successful_items`
// (backend/internal/models/github_installation.go), but the spec's
// `BlueprintImportReport` names the field `success_items`. Correct it locally so
// the import-report view reads the real field (deferred spec-gap follow-up).
export type BlueprintImportReport = Omit<
  components['schemas']['BlueprintImportReport'],
  'success_items'
> & { successful_items: BlueprintImportSuccess[] }

// UI-only repository visibility filter — not a wire type. Kept here now that the
// hand-written `src/types/github.ts` is gone.
export type VisibilityFilter = 'all' | 'private' | 'public'

// DRIFT: `handleGitHubStatus` returns `models.GitHubInstallationStatus`
// ({ installed, account_login, installation_id, suspended, installed_at } —
// backend/internal/models/github_installation.go), but the spec's
// `GitHubInstallationStatus` documents a different shape (suspended_at,
// account_type, permissions, …). Model the real backend response locally until the
// spec is corrected (deferred spec-gap follow-up).
export interface GitHubInstallationStatus {
  installed: boolean
  account_login?: string
  installation_id?: number
  suspended?: boolean
  installed_at?: string
}

// DRIFT: the spec's callback body omits `setup_action`; the backend struct carries
// it but reads only `installation_id` + `state` (github_handlers.go). Keep it
// optional so the callback page can forward the GitHub redirect's query param
// unchanged — it is serialized but ignored server-side.
export type GitHubInstallCallbackRequest =
  operations['handleGitHubCallback']['requestBody']['content']['application/json'] & {
    setup_action?: string
  }

// DRIFT: the spec types `project` and `created` optional on the import response,
// but the backend always returns them on a 2xx (github_handlers.go). Narrow to the
// real success shape so callers read project fields without guards.
export interface ImportProjectResponse {
  project: components['schemas']['Project']
  created: boolean
  message?: string
}

class GitHubIntegrationService {
  /**
   * Get GitHub integration status for a team
   */
  async getStatus(teamId: string): Promise<GitHubInstallationStatus> {
    // See the GitHubInstallationStatus DRIFT note: the runtime payload matches the
    // local shape (it adds the optional `suspended`/`installed_at` the generated
    // type lacks), so the generated response type is assignable to it directly.
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/integrations/github/status', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  /**
   * Get GitHub App installation URL
   */
  async getInstallUrl(teamId: string): Promise<GitHubInstallUrlResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/integrations/github/install-url', {
        params: { path: { team_id: teamId } },
      })
    )
  }

  /**
   * Handle GitHub App installation callback
   */
  async handleCallback(
    teamId: string,
    data: GitHubInstallCallbackRequest
  ): Promise<GitHubCallbackResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/{team_id}/integrations/github/callback', {
        params: { path: { team_id: teamId } },
        body: data,
      })
    )
  }

  /**
   * Get accessible repositories
   */
  async getRepositories(
    teamId: string,
    page = 1,
    signal?: AbortSignal
  ): Promise<GitHubRepositoriesResponse> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/{team_id}/integrations/github/repositories',
        {
          params: { path: { team_id: teamId }, query: { page } },
          signal,
        }
      )
    )
  }

  /**
   * Disconnect GitHub integration
   */
  async disconnect(teamId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE(
        '/api/v1/{team_id}/integrations/github/disconnect',
        {
          params: { path: { team_id: teamId } },
        }
      )
    )
  }

  /**
   * Import repository as a project
   */
  async importProject(
    teamId: string,
    repoId: string
  ): Promise<ImportProjectResponse> {
    const result = await unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/integrations/github/repositories/{repo_id}/import-project',
        {
          params: { path: { team_id: teamId, repo_id: Number(repoId) } },
        }
      )
    )
    // See the ImportProjectResponse DRIFT note: `project`/`created` are always
    // present on a successful response.
    return result as unknown as ImportProjectResponse
  }

  /**
   * Import blueprints from a repository
   * Project is automatically discovered by matching the repository URL
   */
  async importBlueprints(
    teamId: string,
    repositoryId: number
  ): Promise<BlueprintImportReport> {
    const report = await unwrap(
      generatedClient.POST(
        '/api/v1/{team_id}/integrations/github/import-blueprints',
        {
          params: { path: { team_id: teamId } },
          body: { repository_id: repositoryId },
        }
      )
    )
    // See the BlueprintImportReport DRIFT note: the runtime payload uses
    // `successful_items`, not the spec's `success_items`.
    return report as unknown as BlueprintImportReport
  }
}

export const githubIntegrationService = new GitHubIntegrationService()

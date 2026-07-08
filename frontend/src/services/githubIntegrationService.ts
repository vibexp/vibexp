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

export type BlueprintImportReport =
  components['schemas']['BlueprintImportReport']

// UI-only repository visibility filter — not a wire type. Kept here now that the
// hand-written `src/types/github.ts` is gone.
export type VisibilityFilter = 'all' | 'private' | 'public'

export type GitHubInstallationStatus =
  components['schemas']['GitHubInstallationStatus']

// The callback body carries `setup_action` (accepted-and-ignored server-side); it
// is part of the generated request body now.
export type GitHubInstallCallbackRequest =
  operations['handleGitHubCallback']['requestBody']['content']['application/json']

// The import-project 2xx response always carries `project` + `created` (the spec
// marks them required); this is the generated success shape.
export type ImportProjectResponse =
  operations['importGitHubProject']['responses'][200]['content']['application/json']

class GitHubIntegrationService {
  /**
   * Get GitHub integration status for a team
   */
  async getStatus(teamId: string): Promise<GitHubInstallationStatus> {
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
    return result
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
    return report
  }
}

export const githubIntegrationService = new GitHubIntegrationService()

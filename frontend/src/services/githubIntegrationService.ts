import { apiClient } from '../lib/apiClient'
import type {
  BlueprintImportReport,
  GitHubInstallationStatus,
  GitHubInstallCallbackRequest,
  GitHubInstallUrlResponse,
  GitHubRepositoriesResponse,
  ImportProjectResponse,
} from '../types/github'

class GitHubIntegrationService {
  /**
   * Get GitHub integration status for a team
   */
  async getStatus(teamId: string): Promise<GitHubInstallationStatus> {
    return apiClient.get<GitHubInstallationStatus>(
      `/${teamId}/integrations/github/status`
    )
  }

  /**
   * Get GitHub App installation URL
   */
  async getInstallUrl(teamId: string): Promise<GitHubInstallUrlResponse> {
    return apiClient.get<GitHubInstallUrlResponse>(
      `/${teamId}/integrations/github/install-url`
    )
  }

  /**
   * Handle GitHub App installation callback
   */
  async handleCallback(
    teamId: string,
    data: GitHubInstallCallbackRequest
  ): Promise<{ reconnected: boolean }> {
    return apiClient.post<{ reconnected: boolean }>(
      `/${teamId}/integrations/github/callback`,
      data
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
    const params = new URLSearchParams()
    params.append('page', page.toString())

    return apiClient.get<GitHubRepositoriesResponse>(
      `/${teamId}/integrations/github/repositories?${params.toString()}`,
      { signal }
    )
  }

  /**
   * Disconnect GitHub integration
   */
  async disconnect(teamId: string): Promise<void> {
    await apiClient.delete<Record<string, never>>(
      `/${teamId}/integrations/github/disconnect`
    )
  }

  /**
   * Import repository as a project
   */
  async importProject(
    teamId: string,
    repoId: string
  ): Promise<ImportProjectResponse> {
    return apiClient.post<ImportProjectResponse>(
      `/${teamId}/integrations/github/repositories/${repoId}/import-project`
    )
  }

  /**
   * Import blueprints from a repository
   * Project is automatically discovered by matching the repository URL
   */
  async importBlueprints(
    teamId: string,
    repositoryId: number
  ): Promise<BlueprintImportReport> {
    return apiClient.post<BlueprintImportReport>(
      `/${teamId}/integrations/github/import-blueprints`,
      {
        repository_id: repositoryId,
      }
    )
  }
}

export const githubIntegrationService = new GitHubIntegrationService()

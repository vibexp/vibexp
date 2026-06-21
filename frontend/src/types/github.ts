/**
 * GitHub Integration Type Definitions
 *
 * Types for GitHub App installation and repository management
 * Matches backend models from backend/internal/domain/integration_github.go
 */

// GitHub installation status response
export interface GitHubInstallationStatus {
  installed: boolean
  account_login?: string
  installation_id?: number
  suspended?: boolean
  installed_at?: string
}

// GitHub repository information
export interface GitHubRepository {
  id: number
  name: string
  full_name: string
  private: boolean
  html_url: string
  description: string | null
  imported_project_slug?: string
  owner: {
    login: string
    type: string
  }
}

// GitHub repositories response (paginated)
export interface GitHubRepositoriesResponse {
  repositories: GitHubRepository[]
  total_count: number
}

// Request for GitHub App installation callback
export interface GitHubInstallCallbackRequest {
  installation_id: number
  setup_action: string
  state: string
}

// Repository visibility filter type
export type VisibilityFilter = 'all' | 'private' | 'public'

// Response containing GitHub App install URL
export interface GitHubInstallUrlResponse {
  install_url: string
}

// Import project from repository response
export interface ImportProjectResponse {
  project: {
    id: string
    user_id: string
    team_id: string
    name: string
    slug: string
    description: string
    git_url: string
    homepage: string
    created_at: string
    updated_at: string
    version: number
  }
  created: boolean
  message?: string
}

// Blueprint import request
export interface BlueprintImportRequest {
  repository_id: number
}

// Blueprint import success item
export interface BlueprintImportSuccess {
  file_path: string
  blueprint_id: string
  title: string
  type: string
  subtype: string
}

// Blueprint import failed item
export interface BlueprintImportFailed {
  file_path: string
  error: string
}

// Blueprint import skipped item
export interface BlueprintImportSkipped {
  file_path: string
  reason: string
}

// Blueprint import report
export interface BlueprintImportReport {
  total_scanned: number
  total_successful: number
  total_failed: number
  total_skipped: number
  successful_items: BlueprintImportSuccess[]
  failed_items: BlueprintImportFailed[]
  skipped_items: BlueprintImportSkipped[]
}

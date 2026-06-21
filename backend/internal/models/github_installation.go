package models

import "time"

// GitHubInstallation represents a GitHub App installation for a team
type GitHubInstallation struct {
	ID                   string                 `json:"id" db:"id"`
	TeamID               string                 `json:"team_id" db:"team_id"`
	InstallationID       int64                  `json:"installation_id" db:"installation_id"`
	AccountLogin         string                 `json:"account_login" db:"account_login"`
	AccountType          string                 `json:"account_type" db:"account_type"`
	TargetType           string                 `json:"target_type" db:"target_type"`
	EncryptedAccessToken string                 `json:"-" db:"encrypted_access_token"`
	TokenExpiresAt       time.Time              `json:"token_expires_at" db:"token_expires_at"`
	Permissions          map[string]interface{} `json:"permissions" db:"permissions"`
	Events               []string               `json:"events" db:"events"`
	SuspendedAt          *time.Time             `json:"suspended_at,omitempty" db:"suspended_at"`
	CreatedAt            time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at" db:"updated_at"`
}

// GitHubRepository represents a repository accessible through the installation
type GitHubRepository struct {
	ID          int64                 `json:"id"`
	Name        string                `json:"name"`
	FullName    string                `json:"full_name"`
	Description *string               `json:"description"`
	Private     bool                  `json:"private"`
	HTMLURL     string                `json:"html_url"`
	Owner       GitHubRepositoryOwner `json:"owner"`
	// ImportedProjectSlug is the slug of an existing VibeXP project whose git_url
	// matches this repo's HTMLURL within the same team. It is populated by the
	// repositories list endpoint so the UI can link to the project instead of
	// offering to import it again. Empty when no matching project exists.
	ImportedProjectSlug string `json:"imported_project_slug,omitempty"`
}

type GitHubRepositoryOwner struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

// GitHubInstallationStatus represents the status response for the GitHub App installation
type GitHubInstallationStatus struct {
	Installed      bool      `json:"installed"`
	AccountLogin   string    `json:"account_login,omitempty"`
	InstallationID int64     `json:"installation_id,omitempty"`
	Suspended      bool      `json:"suspended,omitempty"`
	InstalledAt    time.Time `json:"installed_at,omitempty"`
}

// GitHubInstallCallbackRequest represents the callback request from GitHub OAuth flow
type GitHubInstallCallbackRequest struct {
	InstallationID int64  `json:"installation_id"`
	SetupAction    string `json:"setup_action"`
	State          string `json:"state"` // HMAC-signed state for CSRF protection
}

// GitHubRepositoriesResponse represents the response for listing repositories
type GitHubRepositoriesResponse struct {
	Repositories []GitHubRepository `json:"repositories"`
	TotalCount   int                `json:"total_count"`
}

// BlueprintImportRequest represents the request to import blueprints from a repository
type BlueprintImportRequest struct {
	RepositoryID int64 `json:"repository_id"`
	// ProjectID is automatically discovered by matching the repository URL
	// No need to provide it in the request
}

// BlueprintImportReport represents the result of importing blueprints from a repository
type BlueprintImportReport struct {
	TotalScanned    int                      `json:"total_scanned"`
	TotalSuccessful int                      `json:"total_successful"`
	TotalFailed     int                      `json:"total_failed"`
	TotalSkipped    int                      `json:"total_skipped"`
	SuccessfulItems []BlueprintImportSuccess `json:"successful_items"`
	FailedItems     []BlueprintImportFailed  `json:"failed_items"`
	SkippedItems    []BlueprintImportSkipped `json:"skipped_items"`
}

// BlueprintImportSuccess represents a successfully imported blueprint
type BlueprintImportSuccess struct {
	FilePath    string `json:"file_path"`
	BlueprintID string `json:"blueprint_id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	Subtype     string `json:"subtype,omitempty"`
}

// BlueprintImportFailed represents a failed blueprint import
type BlueprintImportFailed struct {
	FilePath string `json:"file_path"`
	Error    string `json:"error"`
}

// BlueprintImportSkipped represents a skipped blueprint
type BlueprintImportSkipped struct {
	FilePath string `json:"file_path"`
	Reason   string `json:"reason"`
}

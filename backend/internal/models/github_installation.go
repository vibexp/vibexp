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
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	FullName    string  `json:"full_name"`
	Description *string `json:"description"`
	Private     bool    `json:"private"`
	HTMLURL     string  `json:"html_url"`
	// DefaultBranch is the repository's default branch (e.g. "main"), used to
	// resolve the head commit SHA once per blueprint import run (#337).
	DefaultBranch string                `json:"default_branch,omitempty"`
	Owner         GitHubRepositoryOwner `json:"owner"`
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
	// Code is the OAuth authorization code GitHub appends to the post-install
	// redirect. It is exchanged for a user access token to establish that the
	// caller has access to InstallationID — state alone only proves the caller
	// started an install flow for their own team (#463).
	Code string `json:"code"`
}

// GitHubRepositoriesResponse represents the response for listing repositories
type GitHubRepositoriesResponse struct {
	Repositories JSONArray[GitHubRepository] `json:"repositories"`
	TotalCount   int                         `json:"total_count"`
}

// BlueprintImportRequest represents the request to import blueprints from a repository
type BlueprintImportRequest struct {
	RepositoryID int64 `json:"repository_id"`
	// ProjectID is automatically discovered by matching the repository URL
	// No need to provide it in the request
}

// BlueprintImportReport represents the result of importing blueprints from a repository
type BlueprintImportReport struct {
	TotalScanned    int                               `json:"total_scanned"`
	TotalSuccessful int                               `json:"total_successful"`
	TotalFailed     int                               `json:"total_failed"`
	TotalSkipped    int                               `json:"total_skipped"`
	SuccessfulItems JSONArray[BlueprintImportSuccess] `json:"successful_items"`
	FailedItems     JSONArray[BlueprintImportFailed]  `json:"failed_items"`
	SkippedItems    JSONArray[BlueprintImportSkipped] `json:"skipped_items"`
	// Update-aware re-import outcomes (#341): TotalUpdated re-imports whose repo
	// file changed while the blueprint was unedited in VibeXP; TotalConflicts
	// VibeXP-edited blueprints left untouched; TotalUpToDate unchanged repo files.
	TotalUpdated   int                                `json:"total_updated"`
	TotalConflicts int                                `json:"total_conflicts"`
	TotalUpToDate  int                                `json:"total_up_to_date"`
	UpdatedItems   JSONArray[BlueprintImportUpdated]  `json:"updated_items"`
	ConflictItems  JSONArray[BlueprintImportConflict] `json:"conflict_items"`
	UpToDateItems  JSONArray[BlueprintImportUpToDate] `json:"up_to_date_items"`
	// Multi-file Agent Skill companion outcomes (#342), reported separately from
	// blueprint (SKILL.md) outcomes above. A skill directory imports its SKILL.md
	// as a blueprint and every sibling file as a blueprint-owned attachment;
	// TotalCompanionsImported counts companions stored (new or replaced),
	// TotalCompanionsRemoved counts companions deleted during re-import
	// reconciliation, and TotalCompanionsSkipped counts companions the attachment
	// service rejected (oversized, over the per-owner budget, disallowed type, or
	// storage unconfigured) — the SKILL.md still imports regardless.
	TotalCompanionsImported int                                 `json:"total_companions_imported"`
	TotalCompanionsRemoved  int                                 `json:"total_companions_removed"`
	TotalCompanionsSkipped  int                                 `json:"total_companions_skipped"`
	CompanionItems          JSONArray[BlueprintImportCompanion] `json:"companion_items"`
}

// BlueprintImportCompanion is the per-file outcome of importing one Agent Skill
// companion file (a sibling of a SKILL.md) as a blueprint-owned attachment
// (#342). Outcome is one of "imported" (newly stored), "updated" (replaced an
// existing companion at the same relative_path during re-import), "removed"
// (deleted because it is absent from the re-imported skill), or "skipped"
// (rejected by the attachment service — Reason carries why).
type BlueprintImportCompanion struct {
	BlueprintID  string `json:"blueprint_id"`
	RelativePath string `json:"relative_path"`
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason,omitempty"`
}

// BlueprintImportUpdated represents a blueprint refreshed from a changed repo
// file during re-import (the blueprint was unedited in VibeXP).
type BlueprintImportUpdated struct {
	FilePath    string `json:"file_path"`
	BlueprintID string `json:"blueprint_id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	Subtype     string `json:"subtype,omitempty"`
}

// BlueprintImportConflict represents a re-import skipped because the blueprint was
// edited in VibeXP (its raw no longer matches the imported bytes); resolving the
// conflict is out of scope (report-only, PRD 2).
type BlueprintImportConflict struct {
	FilePath    string `json:"file_path"`
	BlueprintID string `json:"blueprint_id"`
	Reason      string `json:"reason"`
}

// BlueprintImportUpToDate represents a re-import no-op: the repo file is unchanged
// since it was imported, so nothing was mutated.
type BlueprintImportUpToDate struct {
	FilePath    string `json:"file_path"`
	BlueprintID string `json:"blueprint_id"`
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

package external

import (
	"context"
	"errors"
	"time"

	"github.com/darkrockmountain/gomail"

	"github.com/vibexp/vibexp/internal/models"
)

// ErrGitHubInstallationGone signals that GitHub reported the App installation
// no longer exists (HTTP 404 on token refresh) — the user uninstalled the app,
// so the stored installation record must be treated as dead, not retried.
var ErrGitHubInstallationGone = errors.New("github app installation no longer exists")

// EmailProvider defines the interface for email delivery providers
type EmailProvider interface {
	SendEmail(ctx context.Context, message *gomail.EmailMessage) error
}

// EmailSender defines the interface for SMTP operations (DEPRECATED: Use EmailProvider instead)
type EmailSender interface {
	SendEmail(ctx context.Context, req *EmailRequest) error
}

// EmailRequest represents an email to be sent (DEPRECATED: Use gomail.EmailMessage instead)
type EmailRequest struct {
	From        string
	To          []string
	Subject     string
	HTMLBody    string
	TextBody    string
	ReplyTo     string
	Attachments []EmailAttachment
}

// EmailAttachment represents an email attachment (DEPRECATED: Use gomail attachments instead)
type EmailAttachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// GitHubInstallationInfo represents GitHub App installation information
type GitHubInstallationInfo struct {
	AccountLogin string
	AccountType  string
	TargetType   string
	Permissions  map[string]string
	Events       []string
	SuspendedAt  *time.Time
}

// GitHubFile represents a file fetched from GitHub
type GitHubFile struct {
	Path    string
	Content string
	// BlobSHA is the Git blob SHA of the file as reported by the Contents API.
	// It uniquely identifies the file's bytes and is used for cheap
	// change-detection on re-import (#341). Empty when GitHub omits it — callers
	// must treat an empty value as "unknown", never fail the fetch.
	BlobSHA string
}

// GitHubAppClient defines the interface for GitHub App operations
type GitHubAppClient interface {
	GetInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error)
	GetInstallationRepositories(
		ctx context.Context, installationID int64, page int,
	) ([]models.GitHubRepository, int, error)
	GetInstallation(ctx context.Context, installationID int64) (*GitHubInstallationInfo, error)
	GetRepository(ctx context.Context, installationID int64, repoID int64) (*models.GitHubRepository, error)
	GetFileContent(
		ctx context.Context, installationID int64, owner, repoName, path string,
	) (*GitHubFile, error)
	GetDirectoryContentsRecursive(
		ctx context.Context, installationID int64, owner, repoName, dirPath string,
	) ([]*GitHubFile, error)
	// GetBranchHeadSHA resolves the head commit SHA of a branch in one API call.
	// Blueprint import calls it once per run to record provenance
	// (source_commit_sha, #341). branch is the plain branch name (e.g. the
	// repository's default branch).
	GetBranchHeadSHA(
		ctx context.Context, installationID int64, owner, repoName, branch string,
	) (string, error)
	// EvictCachedClient removes the cached GitHub client for the given installationID.
	// Call this when an installation is disconnected to prevent stale entries.
	EvictCachedClient(installationID int64)
}

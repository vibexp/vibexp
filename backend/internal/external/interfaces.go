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

// SMTPClient defines the interface for SMTP operations (DEPRECATED: Use EmailProvider instead)
type SMTPClient interface {
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
	// EvictCachedClient removes the cached GitHub client for the given installationID.
	// Call this when an installation is disconnected to prevent stale entries.
	EvictCachedClient(installationID int64)
}

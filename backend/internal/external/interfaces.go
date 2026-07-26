package external

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/darkrockmountain/gomail"

	"github.com/vibexp/vibexp/internal/models"
)

// ErrGitHubInstallationGone signals that GitHub reported the App installation
// no longer exists (HTTP 404 on token refresh) — the user uninstalled the app,
// so the stored installation record must be treated as dead, not retried.
var ErrGitHubInstallationGone = errors.New("github app installation no longer exists")

// ErrGitHubUserAuthNotConfigured signals that the GitHub App's OAuth
// credentials (client id / secret) are absent, so the install callback cannot
// establish that the caller has access to the installation. Callers must fail
// closed on this error rather than falling back to the app-JWT-only path (#463).
var ErrGitHubUserAuthNotConfigured = errors.New("github app user authorization is not configured")

// ErrGitHubUserCodeInvalid signals that GitHub rejected the authorization code
// submitted with an install callback (expired, replayed, or minted for a
// different app).
var ErrGitHubUserCodeInvalid = errors.New("github authorization code is invalid or expired")

// OutgoingMessage is one message on its way to a provider: the gomail message
// plus the sender metadata gomail's own struct has no field for.
//
// It exists because gomail.EmailMessage carries the From address only, and
// validates it with a plain email regex (GetFrom returns "" for anything that
// is not a bare address). Folding a display name into that field therefore
// produces a message with NO From header at all. Anything the providers need
// beyond a bare address travels here instead.
type OutgoingMessage struct {
	// Message is the message body and addressing. Its From stays a bare,
	// gomail-valid address on every path.
	Message *gomail.EmailMessage
	// FromName is the display name to show alongside the From address. Empty
	// means send a bare address, exactly as before this field existed.
	FromName string
}

// FromHeader returns the value for the message's From header: the bare address
// when there is no display name, otherwise the RFC 5322 `"Name" <addr>` form.
//
// Encoding goes through net/mail.Address, never string concatenation. That is
// what makes a team-controlled display name safe: Address.String quotes
// specials, RFC 2047-encodes non-ASCII, and base64-encodes any CR/LF, so a name
// cannot inject a second header. Callers that hand the name to a provider SDK
// with its own name field (SendGrid) should use SanitizedFromName instead and
// let the SDK encode.
//
// The empty-name result is deliberately the bare address rather than
// mail.Address{Name: ""}.String(), which yields "<addr>" — a different byte
// sequence from what every provider received before this existed.
func (m *OutgoingMessage) FromHeader() string {
	address := m.Message.GetFrom()

	name := m.SanitizedFromName()
	if name == "" || address == "" {
		return address
	}

	return (&mail.Address{Name: name, Address: address}).String()
}

// SanitizedFromName returns the display name trimmed of surrounding whitespace,
// which is the form both FromHeader and the name-aware provider SDKs use.
func (m *OutgoingMessage) SanitizedFromName() string {
	return strings.TrimSpace(m.FromName)
}

// EmailProvider defines the interface for email delivery providers
type EmailProvider interface {
	SendEmail(ctx context.Context, message *OutgoingMessage) error
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

	// ExchangeUserCode exchanges the authorization code GitHub appends to the
	// post-install redirect for a *user* access token. Unlike every other method
	// here it authenticates as the installing human, not as the App, which is
	// what makes it usable as a proof of authority. Returns
	// ErrGitHubUserAuthNotConfigured when no OAuth credentials are configured
	// and ErrGitHubUserCodeInvalid when GitHub rejects the code.
	ExchangeUserCode(ctx context.Context, code string) (string, error)

	// UserCanAccessInstallation reports whether installationID appears in the
	// installation list of the user behind userToken (GET /user/installations).
	//
	// Scope of the guarantee, precisely: that endpoint lists installations
	// *accessible to* the authenticated user — not only ones they administer.
	// For an org installation that includes ordinary members with repository
	// access. So a true result proves the caller is an insider of that
	// installation, which is what the install callback needs (#463): it reduces
	// "any authenticated VibeXP user may claim any installation" to "only
	// someone who already has access to it may". Do not treat it as proof of
	// admin rights.
	UserCanAccessInstallation(ctx context.Context, userToken string, installationID int64) (bool, error)
}

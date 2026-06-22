package implementations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-github/v57/github"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
)

const (
	// githubAPITimeout is the timeout for large GitHub API operations
	githubAPITimeout = 30 * time.Second
	// githubAPIFastTimeout is the timeout for simple GitHub API lookups
	githubAPIFastTimeout = 15 * time.Second
)

// githubTracer is the OTel tracer used by the GitHub App client.
// Using a package-level tracer follows the standard OTel Go convention:
// the global tracer provider is set during server initialisation.
var githubTracer = otel.Tracer("vibexp-api/github")

// GitHubAppClient implements external.GitHubAppClient using go-github
type GitHubAppClient struct {
	cfg           *config.GitHubAppConfig
	logger        *slog.Logger
	httpClient    *http.Client
	clientCache   map[int64]*github.Client
	clientCacheMu sync.RWMutex
}

// NewGitHubAppClient creates a new GitHub App client
func NewGitHubAppClient(cfg *config.GitHubAppConfig, logger *slog.Logger) external.GitHubAppClient {
	// Return stub client for test/dev environments without GitHub App config
	if cfg == nil || cfg.AppID == "" || cfg.PrivateKey == nil {
		logger.Warn("GitHub App config missing, returning stub client")
		return &stubGitHubAppClient{}
	}

	// Create shared HTTP client with connection pooling
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &GitHubAppClient{
		cfg:         cfg,
		logger:      logger,
		httpClient:  httpClient,
		clientCache: make(map[int64]*github.Client),
	}
}

// GetInstallationToken retrieves an installation access token
func (c *GitHubAppClient) GetInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, githubAPIFastTimeout)
	defer cancel()

	jwtToken, err := c.generateJWT()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate JWT: %w", err)
	}

	// IMPORTANT: Use nil (fresh http.Client) instead of c.httpClient.
	// go-github's WithAuthToken shallow-copies the *Client but shares the same
	// *http.Client pointer, then mutates its Transport. Passing a shared client
	// permanently corrupts its Transport for all subsequent callers, causing
	// ghinstallation's installation-token auth to be overwritten by the JWT wrapper.
	client := github.NewClient(nil).WithAuthToken(jwtToken)
	token, resp, err := client.Apps.CreateInstallationToken(ctx, installationID, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create installation token: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return "", time.Time{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return token.GetToken(), token.GetExpiresAt().Time, nil
}

// createInstallationTransport returns a GitHub client for the given installation.
// Clients are cached per installationID to avoid re-parsing the PEM key and
// re-creating the HTTP transport on every call.
func (c *GitHubAppClient) createInstallationTransport(installationID int64) (*github.Client, error) {
	// Fast path: check the cache under a read lock.
	c.clientCacheMu.RLock()
	if cached, ok := c.clientCache[installationID]; ok {
		c.clientCacheMu.RUnlock()
		return cached, nil
	}
	c.clientCacheMu.RUnlock()

	// Slow path: build the client then store it.
	// Parse App ID to int64
	appID, err := strconv.ParseInt(c.cfg.AppID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub App ID: %w", err)
	}

	c.logger.Debug(
		"Creating GitHub installation transport",
		"app_id", appID,
		"installation_id", installationID,
		"pem_length", len(c.cfg.PrivateKeyPEM),
	)

	// Use http.DefaultTransport directly as the shared base for TCP connection reuse.
	// Do NOT use c.httpClient.Transport — it may have been mutated by a previous
	// WithAuthToken call (see comment in GetInstallationToken).
	itr, err := ghinstallation.New(
		http.DefaultTransport,
		appID,
		installationID,
		c.cfg.PrivateKeyPEM, // Use PEM-encoded private key bytes
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create installation transport: %w", err)
	}

	client := github.NewClient(&http.Client{Transport: itr})

	// Store in cache under write lock. Use double-checked locking so we don't
	// overwrite an entry another goroutine may have written while we were building.
	c.clientCacheMu.Lock()
	if _, ok := c.clientCache[installationID]; !ok {
		c.clientCache[installationID] = client
	} else {
		// Another goroutine beat us — return their entry for consistency.
		client = c.clientCache[installationID]
	}
	c.clientCacheMu.Unlock()

	return client, nil
}

// convertToModelRepository converts a GitHub API repository to the internal model.
// Returns nil if the repository owner is nil.
func (c *GitHubAppClient) convertToModelRepository(repo *github.Repository) *models.GitHubRepository {
	owner := repo.GetOwner()
	if owner == nil {
		c.logger.Warn("Skipping repository with nil owner", "repo_id", repo.GetID())
		return nil
	}

	return &models.GitHubRepository{
		ID:          repo.GetID(),
		Name:        repo.GetName(),
		FullName:    repo.GetFullName(),
		Description: repo.Description,
		Private:     repo.GetPrivate(),
		HTMLURL:     repo.GetHTMLURL(),
		Owner: models.GitHubRepositoryOwner{
			Login: owner.GetLogin(),
			Type:  owner.GetType(),
		},
	}
}

// isInstallationGone reports whether err is ghinstallation's token-refresh
// failure with HTTP 404 — GitHub's definitive signal that the App installation
// was uninstalled. 401/403 are deliberately excluded: those indicate app
// credential or suspension problems, not installation removal.
func isInstallationGone(err error) bool {
	var httpErr *ghinstallation.HTTPError
	return errors.As(err, &httpErr) &&
		httpErr.Response != nil &&
		httpErr.Response.StatusCode == http.StatusNotFound
}

// respStatusCode safely extracts the HTTP status code from a GitHub API response.
func respStatusCode(resp *github.Response) int {
	if resp != nil {
		return resp.StatusCode
	}

	return 0
}

// GetInstallationRepositories retrieves repositories accessible by the installation
func (c *GitHubAppClient) GetInstallationRepositories(
	ctx context.Context,
	installationID int64,
	page int,
) ([]models.GitHubRepository, int, error) {
	ctx, span := githubTracer.Start(ctx, "github.list_repositories",
		trace.WithAttributes(
			attribute.Int64("github.installation_id", installationID),
			attribute.Int("github.page", page),
		),
	)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, githubAPITimeout)
	defer cancel()

	client, err := c.createInstallationTransport(installationID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, 0, err
	}

	c.logger.Debug(
		"Calling GitHub API to list repositories",
		"installation_id", installationID,
		"page", page,
	)

	repos, resp, err := client.Apps.ListRepos(ctx, &github.ListOptions{Page: page, PerPage: 100})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		if isInstallationGone(err) {
			c.logger.Warn(
				"GitHub App installation no longer exists",
				"error", err.Error(),
				"installation_id", installationID,
			)
			return nil, 0, fmt.Errorf("failed to list repositories: %w: %w", external.ErrGitHubInstallationGone, err)
		}
		c.logger.Error(
			"GitHub API error when listing repositories",
			"error", err.Error(),
			"installation_id", installationID,
			"status_code", respStatusCode(resp),
		)
		return nil, 0, fmt.Errorf("failed to list repositories: %w", err)
	}

	result := make([]models.GitHubRepository, 0, len(repos.Repositories))
	for _, repo := range repos.Repositories {
		if m := c.convertToModelRepository(repo); m != nil {
			result = append(result, *m)
		}
	}

	span.SetAttributes(attribute.Int("github.total_count", repos.GetTotalCount()))
	return result, repos.GetTotalCount(), nil
}

// GetRepository retrieves a single repository by ID
func (c *GitHubAppClient) GetRepository(
	ctx context.Context,
	installationID int64,
	repoID int64,
) (*models.GitHubRepository, error) {
	ctx, span := githubTracer.Start(ctx, "github.get_repository",
		trace.WithAttributes(
			attribute.Int64("github.installation_id", installationID),
			attribute.Int64("github.repo_id", repoID),
		),
	)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, githubAPIFastTimeout)
	defer cancel()

	client, err := c.createInstallationTransport(installationID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	c.logger.Debug(
		"Calling GitHub API to get repository",
		"installation_id", installationID,
		"repo_id", repoID,
	)

	repo, resp, err := client.Repositories.GetByID(ctx, repoID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		c.logger.Error(
			"GitHub API error when getting repository",
			"error", err.Error(),
			"installation_id", installationID,
			"repo_id", repoID,
			"status_code", respStatusCode(resp),
		)
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	result := c.convertToModelRepository(repo)
	if result == nil {
		c.logger.Error("Repository has nil owner", "repo_id", repo.GetID())
		return nil, fmt.Errorf("repository has no owner")
	}

	return result, nil
}

// GetInstallation retrieves installation information
func (c *GitHubAppClient) GetInstallation(
	ctx context.Context,
	installationID int64,
) (*external.GitHubInstallationInfo, error) {
	ctx, span := githubTracer.Start(ctx, "github.get_installation",
		trace.WithAttributes(
			attribute.Int64("github.installation_id", installationID),
		),
	)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, githubAPIFastTimeout)
	defer cancel()

	jwtToken, err := c.generateJWT()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	// IMPORTANT: Use nil — see comment in GetInstallationToken.
	client := github.NewClient(nil).WithAuthToken(jwtToken)
	installation, _, err := client.Apps.GetInstallation(ctx, installationID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to get installation: %w", err)
	}

	permissions := make(map[string]string)
	if installation.Permissions != nil {
		if installation.Permissions.Contents != nil {
			permissions["contents"] = *installation.Permissions.Contents
		}
		if installation.Permissions.Metadata != nil {
			permissions["metadata"] = *installation.Permissions.Metadata
		}
		if installation.Permissions.PullRequests != nil {
			permissions["pull_requests"] = *installation.Permissions.PullRequests
		}
		if installation.Permissions.Issues != nil {
			permissions["issues"] = *installation.Permissions.Issues
		}
	}

	var suspendedAt *time.Time
	if installation.SuspendedAt != nil {
		t := installation.SuspendedAt.Time
		suspendedAt = &t
	}

	return &external.GitHubInstallationInfo{
		AccountLogin: installation.Account.GetLogin(),
		AccountType:  installation.Account.GetType(),
		TargetType:   installation.GetTargetType(),
		Permissions:  permissions,
		Events:       installation.Events,
		SuspendedAt:  suspendedAt,
	}, nil
}

// GetFileContent retrieves the content of a single file from a repository
func (c *GitHubAppClient) GetFileContent(
	ctx context.Context,
	installationID int64,
	owner, repoName, path string,
) (*external.GitHubFile, error) {
	ctx, cancel := context.WithTimeout(ctx, githubAPIFastTimeout)
	defer cancel()

	// Create GitHub client with installation transport
	client, err := c.createInstallationTransport(installationID)
	if err != nil {
		return nil, err
	}

	c.logger.Debug(
		"Fetching file content from GitHub",
		"installation_id", installationID,
		"owner", owner,
		"repo", repoName,
		"path", path,
	)

	// Get file content
	fileContent, _, _, err := client.Repositories.GetContents(ctx, owner, repoName, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get file content: %w", err)
	}

	if fileContent == nil {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	content, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("failed to decode file content: %w", err)
	}

	return &external.GitHubFile{
		Path:    path,
		Content: content,
	}, nil
}

// GetDirectoryContentsRecursive recursively fetches all files in a directory
func (c *GitHubAppClient) GetDirectoryContentsRecursive(
	ctx context.Context,
	installationID int64,
	owner, repoName, dirPath string,
) ([]*external.GitHubFile, error) {
	ctx, cancel := context.WithTimeout(ctx, githubAPITimeout)
	defer cancel()

	// Create GitHub client with installation transport
	client, err := c.createInstallationTransport(installationID)
	if err != nil {
		return nil, err
	}

	c.logger.Debug(
		"Recursively fetching directory contents from GitHub",
		"installation_id", installationID,
		"owner", owner,
		"repo", repoName,
		"dir_path", dirPath,
	)

	var allFiles []*external.GitHubFile
	err = c.fetchDirectoryRecursive(ctx, client, owner, repoName, dirPath, &allFiles, 10, 500)
	if err != nil {
		return nil, err
	}

	return allFiles, nil
}

// fetchDirectoryRecursive is a helper function to recursively fetch directory contents
//
//nolint:gocognit // Recursive directory traversal requires sequential safety checks
func (c *GitHubAppClient) fetchDirectoryRecursive(
	ctx context.Context,
	client *github.Client,
	owner, repo, path string,
	allFiles *[]*external.GitHubFile,
	depth, maxFiles int,
) error {
	if depth <= 0 {
		c.logger.Warn("Maximum directory depth reached, skipping deeper directories", "path", path)
		return nil
	}
	if len(*allFiles) >= maxFiles {
		c.logger.Warn("Maximum file count reached, stopping collection")
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	_, dirContents, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return fmt.Errorf("failed to get directory contents for %s: %w", path, err)
	}

	for _, item := range dirContents {
		switch item.GetType() {
		case "file":
			c.fetchFileContent(ctx, client, owner, repo, item, allFiles)
		case "dir":
			subErr := c.fetchDirectoryRecursive(
				ctx, client, owner, repo, item.GetPath(),
				allFiles, depth-1, maxFiles,
			)
			if subErr != nil {
				c.logger.Warn(
					"Failed to fetch subdirectory",
					"error", subErr,
					"path", item.GetPath(),
				)
			}
		}
	}

	return nil
}

// fetchFileContent fetches and appends a single file's content
func (c *GitHubAppClient) fetchFileContent(
	ctx context.Context,
	client *github.Client,
	owner, repo string,
	item *github.RepositoryContent,
	allFiles *[]*external.GitHubFile,
) {
	fileContent, _, _, err := client.Repositories.GetContents(ctx, owner, repo, item.GetPath(), nil)
	if err != nil {
		c.logger.Warn(
			"Failed to fetch file content",
			"error", err,
			"path", item.GetPath(),
		)
		return
	}

	content, err := fileContent.GetContent()
	if err != nil {
		c.logger.Warn(
			"Failed to decode file content",
			"error", err,
			"path", item.GetPath(),
		)
		return
	}

	*allFiles = append(*allFiles, &external.GitHubFile{
		Path:    item.GetPath(),
		Content: content,
	})
}

// generateJWT creates a JWT for authenticating as the GitHub App
func (c *GitHubAppClient) generateJWT() (string, error) {
	now := time.Now()
	appIDInt, err := strconv.ParseInt(c.cfg.AppID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid GitHub App ID: %w", err)
	}

	c.logger.Debug(
		"Generating GitHub App JWT",
		"app_id", c.cfg.AppID,
		"app_id_int", appIDInt,
		"has_private_key", c.cfg.PrivateKey != nil,
	)

	// Set IssuedAt 60 seconds in the past to account for clock skew (per GitHub docs)
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		Issuer:    strconv.FormatInt(appIDInt, 10),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(c.cfg.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return signedToken, nil
}

// stubGitHubAppClient is a no-op implementation for test/dev environments
type stubGitHubAppClient struct{}

func (s *stubGitHubAppClient) GetInstallationToken(
	ctx context.Context,
	installationID int64,
) (string, time.Time, error) {
	return "", time.Time{}, fmt.Errorf("GitHub App not configured")
}

func (s *stubGitHubAppClient) GetInstallationRepositories(
	ctx context.Context,
	installationID int64,
	page int,
) ([]models.GitHubRepository, int, error) {
	return nil, 0, fmt.Errorf("GitHub App not configured")
}

func (s *stubGitHubAppClient) GetInstallation(
	ctx context.Context,
	installationID int64,
) (*external.GitHubInstallationInfo, error) {
	return nil, fmt.Errorf("GitHub App not configured")
}

func (s *stubGitHubAppClient) GetRepository(
	ctx context.Context,
	installationID int64,
	repoID int64,
) (*models.GitHubRepository, error) {
	return nil, fmt.Errorf("GitHub App not configured")
}

func (s *stubGitHubAppClient) GetFileContent(
	ctx context.Context,
	installationID int64,
	owner, repoName, path string,
) (*external.GitHubFile, error) {
	return nil, fmt.Errorf("GitHub App not configured")
}

func (s *stubGitHubAppClient) GetDirectoryContentsRecursive(
	ctx context.Context,
	installationID int64,
	owner, repoName, dirPath string,
) ([]*external.GitHubFile, error) {
	return nil, fmt.Errorf("GitHub App not configured")
}

func (s *stubGitHubAppClient) EvictCachedClient(_ int64) {}

// EvictCachedClient removes the cached GitHub client for the given installationID.
// Call this when an installation is disconnected to free resources and prevent
// stale cached clients from being served after revocation.
func (c *GitHubAppClient) EvictCachedClient(installationID int64) {
	c.clientCacheMu.Lock()
	delete(c.clientCache, installationID)
	c.clientCacheMu.Unlock()
}

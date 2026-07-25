package services

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/vibexp/vibexp/internal/authz"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// GitHubAppConfigService owns a team's own GitHub App credentials (#478):
// authorization, encryption, secret lifecycle, and the GET /app probe.
//
// There is no ssrfGuard here, unlike the provider services. Those take a
// caller-supplied base_url, so the destination is attacker-chosen; this one
// always talks to a hardcoded github.com (there is no base_url column by
// design). GitHub Enterprise Server is the future change that would introduce
// both a base_url and the guard that must come with it.
type GitHubAppConfigService struct {
	repo  repositories.GitHubAppConfigRepository
	enc   EncryptionServiceInterface
	authz AuthorizationServiceInterface
	// clients is the per-team client cache, notified when a config is deleted.
	// Credential EDITS need no notification — the cache keys on the config's
	// version, so a bumped version invalidates itself. A DELETE has no new
	// version to observe, so it must be pushed. Optional: nil simply means no
	// cache to notify (#480).
	clients GitHubAppClientResolver
	// apiBaseURL is the GitHub API root the validate probe targets. It is not
	// caller-supplied and not configurable — the field exists only so tests can
	// point the probe at an httptest fake.
	apiBaseURL string
	httpClient *http.Client
	// webhookBaseURL builds the webhook URL an admin pastes into GitHub.
	webhookBaseURL string
}

// Ensure GitHubAppConfigService implements GitHubAppConfigServiceInterface
var _ GitHubAppConfigServiceInterface = (*GitHubAppConfigService)(nil)

const (
	// githubAPIBaseURL is the fixed destination of the validate probe.
	githubAPIBaseURL = "https://api.github.com"

	// validateGitHubAppTimeout bounds a single outbound probe.
	validateGitHubAppTimeout = 30 * time.Second

	// githubAppSecretBytes is the entropy behind a generated webhook secret and
	// webhook token: 32 bytes from crypto/rand.
	githubAppSecretBytes = 32

	// githubAppJWTLifetime is GitHub's maximum accepted App-JWT lifetime.
	githubAppJWTLifetime = 10 * time.Minute

	// githubAppJWTBackdate absorbs clock skew between us and GitHub, per their docs.
	githubAppJWTBackdate = 60 * time.Second
)

// Fixed categories for ValidateGitHubAppDetails.ErrorDetails.
//
// Same discipline as the provider probes (#464): the raw upstream error is an
// oracle, so it is logged server-side and the caller gets a category that says
// what to fix without revealing what the server reached.
const (
	// #nosec G101 - fixed error-category name returned to the client, not a credential
	githubAppErrInvalidCredentials     = "invalid_credentials"
	githubAppErrAppNotFound            = "app_not_found"
	githubAppErrSlugMismatch           = "slug_mismatch"
	githubAppErrInsufficientPermission = "insufficient_permissions"
	githubAppErrConnectionFailed       = "connection_failed"
)

// NewGitHubAppConfigService creates a new GitHubAppConfigService. webhookBaseURL
// is the instance's public base URL, used only to compose the webhook URL shown
// to the admin.
func NewGitHubAppConfigService(
	repo repositories.GitHubAppConfigRepository,
	enc EncryptionServiceInterface,
	authzSvc AuthorizationServiceInterface,
	clients GitHubAppClientResolver,
	webhookBaseURL string,
) *GitHubAppConfigService {
	return &GitHubAppConfigService{
		repo:           repo,
		enc:            enc,
		authz:          authzSvc,
		clients:        clients,
		apiBaseURL:     githubAPIBaseURL,
		httpClient:     &http.Client{Timeout: validateGitHubAppTimeout},
		webhookBaseURL: strings.TrimRight(webhookBaseURL, "/"),
	}
}

// authorizeAppConfigMutation gates create/update/delete/validate/rotate. A nil
// authz is a wiring bug — fail closed rather than allow.
//
// TeamSettingsUpdate rather than TeamUpdate: a GitHub App registration is
// team-level CONFIGURATION, not team identity (name/slug/description). The two
// permissions are deliberately separate (#489) precisely so configuration and
// identity can diverge later without a breaking rename — both grant owner+admin
// today, so this is a semantic choice with no behavioural difference now.
func (s *GitHubAppConfigService) authorizeAppConfigMutation(
	ctx context.Context, userID, teamID string,
) error {
	if s.authz == nil {
		return fmt.Errorf("%w: authorization service is not configured", ErrPermissionDenied)
	}
	return s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate)
}

// generateSecret returns githubAppSecretBytes of crypto/rand, hex-encoded.
func generateSecret() (string, error) {
	buf := make([]byte, githubAppSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// generateWebhookToken returns a routing token that is unpadded and URL-safe BY
// CONSTRUCTION (base64.RawURLEncoding), so the webhook route has nothing to
// percent-decode — the #251/#257 rule, applied at the point the value is minted
// rather than left for every reader to remember.
func generateWebhookToken() (string, error) {
	buf := make([]byte, githubAppSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate webhook token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// parsePrivateKey accepts a raw PEM or a base64-encoded PEM, mirroring what
// config.GetGitHubAppConfig accepted so operators moving off config.yaml do not
// have to re-encode their key (#483 deletes that copy). An unparseable key is a
// field-level validation error, never a 500 — it is user input.
func parsePrivateKey(key string) (*rsa.PrivateKey, error) {
	keyBytes := []byte(key)
	if decoded, err := base64.StdEncoding.DecodeString(key); err == nil {
		keyBytes = decoded
	}

	parsed, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
	if err != nil {
		return nil, apierrors.NewValidationError(
			"The private key could not be parsed",
			[]apierrors.ValidationError{{
				Field:   "private_key",
				Message: "must be an RSA private key in PEM format (raw or base64-encoded)",
			}},
		)
	}
	return parsed, nil
}

func (s *GitHubAppConfigService) encrypt(plaintext string) (string, error) {
	return s.enc.Encrypt(plaintext)
}

func (s *GitHubAppConfigService) decrypt(ciphertext string) (string, error) {
	return s.enc.Decrypt(ciphertext)
}

// requiredFieldError is the shared shape for "this field may not be blank". It
// covers the plain identity fields as well as the secrets: for all of them an
// explicit empty value is a client bug, not an intent to clear.
func requiredFieldError(field string) error {
	return apierrors.NewValidationError(
		fmt.Sprintf("%s cannot be empty", field),
		[]apierrors.ValidationError{{
			Field:   field,
			Message: "omit the field to keep the stored value; an empty value is not allowed",
		}},
	)
}

// CreateAppConfig registers a team's GitHub App. The webhook secret and routing
// token are generated here, never accepted from the caller, and the secret is
// disclosed exactly once in the returned payload.
func (s *GitHubAppConfigService) CreateAppConfig(
	ctx context.Context, teamID, userID string, req models.CreateGitHubAppConfigRequest,
) (*models.GitHubAppConfigCreated, error) {
	if authzErr := s.authorizeAppConfigMutation(ctx, userID, teamID); authzErr != nil {
		return nil, authzErr
	}

	// Parse before storing: a key that cannot sign a JWT is useless, and finding
	// out at install time is a far worse experience than a 400 here.
	if _, err := parsePrivateKey(req.PrivateKey); err != nil {
		return nil, err
	}
	if req.ClientSecret == "" {
		return nil, requiredFieldError("client_secret")
	}

	webhookSecret, err := generateSecret()
	if err != nil {
		return nil, err
	}

	config := &models.GitHubAppConfig{
		TeamID:   teamID,
		UserID:   &userID,
		AppID:    req.AppID,
		AppSlug:  req.AppSlug,
		ClientID: req.ClientID,
	}
	if err := s.encryptSecretsInto(config, req.PrivateKey, req.ClientSecret, webhookSecret); err != nil {
		return nil, err
	}

	if err := s.createWithTokenRetry(ctx, config); err != nil {
		return nil, err
	}

	return &models.GitHubAppConfigCreated{
		GitHubAppConfigResponse: s.toResponse(config),
		WebhookSecret:           webhookSecret,
	}, nil
}

// encryptSecretsInto encrypts all three secrets onto the config. Plaintext lives
// only in the caller's local scope and is never logged or returned (the one
// deliberate webhook-secret disclosure aside).
func (s *GitHubAppConfigService) encryptSecretsInto(
	config *models.GitHubAppConfig, privateKey, clientSecret, webhookSecret string,
) error {
	encryptedKey, err := s.encrypt(privateKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt private key: %w", err)
	}
	encryptedClientSecret, err := s.encrypt(clientSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt client secret: %w", err)
	}
	encryptedWebhookSecret, err := s.encrypt(webhookSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt webhook secret: %w", err)
	}

	config.PrivateKeyEncrypted = encryptedKey
	config.ClientSecretEncrypted = encryptedClientSecret
	config.WebhookSecretEncrypted = encryptedWebhookSecret
	return nil
}

// createWithTokenRetry mints a webhook token and inserts, retrying once on a
// token collision. A collision is astronomically unlikely with 32 bytes of
// crypto/rand; the retry exists so that if it ever happens it is a transparent
// non-event rather than a spurious 500.
func (s *GitHubAppConfigService) createWithTokenRetry(
	ctx context.Context, config *models.GitHubAppConfig,
) error {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := generateWebhookToken()
		if err != nil {
			return err
		}
		config.WebhookToken = token

		err = s.repo.Create(ctx, config)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, repositories.ErrGitHubAppAlreadyRegistered):
			return ErrGitHubAppAlreadyRegistered
		case errors.Is(err, repositories.ErrGitHubAppConfigTeamTaken):
			return ErrGitHubAppConfigExists
		case errors.Is(err, repositories.ErrGitHubAppWebhookTokenTaken) && attempt == 0:
			slog.Warn("GitHub App webhook token collision; re-minting", "team_id", config.TeamID)
			continue
		default:
			return fmt.Errorf("failed to create GitHub App config: %w", err)
		}
	}
	return fmt.Errorf("failed to create GitHub App config: webhook token collision persisted")
}

// GetAppConfig returns the team's App config. Reads are not gated here: the
// route-level team-membership middleware is the boundary, matching the provider
// services. Secrets never leave this method — the response carries has_* masks.
func (s *GitHubAppConfigService) GetAppConfig(
	ctx context.Context, teamID string,
) (*models.GitHubAppConfigResponse, error) {
	config, err := s.repo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrGitHubAppConfigNotFound) {
			return nil, ErrGitHubAppNotConfigured
		}
		return nil, fmt.Errorf("failed to get GitHub App config: %w", err)
	}

	response := s.toResponse(config)
	return &response, nil
}

// UpdateAppConfig applies a partial edit under the repository's optimistic lock.
func (s *GitHubAppConfigService) UpdateAppConfig(
	ctx context.Context, teamID, userID string, req models.UpdateGitHubAppConfigRequest,
) (*models.GitHubAppConfigResponse, error) {
	if authzErr := s.authorizeAppConfigMutation(ctx, userID, teamID); authzErr != nil {
		return nil, authzErr
	}

	config, err := s.repo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrGitHubAppConfigNotFound) {
			return nil, ErrGitHubAppNotConfigured
		}
		return nil, fmt.Errorf("failed to get GitHub App config: %w", err)
	}

	if err := s.applyUpdate(req, config); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, config); err != nil {
		switch {
		case errors.Is(err, repositories.ErrGitHubAppAlreadyRegistered):
			return nil, ErrGitHubAppAlreadyRegistered
		case errors.Is(err, repositories.ErrGitHubAppConfigVersionConflict):
			return nil, ErrGitHubAppConfigConflict
		default:
			return nil, fmt.Errorf("failed to update GitHub App config: %w", err)
		}
	}

	response := s.toResponse(config)
	return &response, nil
}

// applyUpdate folds the request onto the stored config. Every field follows the
// same three-case rule: absent keeps the stored value, non-empty replaces it,
// and an explicit empty string is a validation error rather than a silent clear.
func (s *GitHubAppConfigService) applyUpdate(
	req models.UpdateGitHubAppConfigRequest, config *models.GitHubAppConfig,
) error {
	if err := applyRequiredStringUpdate(req.AppID, "app_id", &config.AppID); err != nil {
		return err
	}
	if err := applyRequiredStringUpdate(req.AppSlug, "app_slug", &config.AppSlug); err != nil {
		return err
	}
	if err := applyRequiredStringUpdate(req.ClientID, "client_id", &config.ClientID); err != nil {
		return err
	}

	if req.PrivateKey != nil {
		if *req.PrivateKey == "" {
			return requiredFieldError("private_key")
		}
		if _, err := parsePrivateKey(*req.PrivateKey); err != nil {
			return err
		}
		encrypted, err := s.encrypt(*req.PrivateKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt private key: %w", err)
		}
		config.PrivateKeyEncrypted = encrypted
	}

	if req.ClientSecret != nil {
		if *req.ClientSecret == "" {
			return requiredFieldError("client_secret")
		}
		encrypted, err := s.encrypt(*req.ClientSecret)
		if err != nil {
			return fmt.Errorf("failed to encrypt client secret: %w", err)
		}
		config.ClientSecretEncrypted = encrypted
	}

	return nil
}

// applyRequiredStringUpdate is the non-secret half of the same three-case rule.
func applyRequiredStringUpdate(incoming *string, field string, target *string) error {
	if incoming == nil {
		return nil
	}
	if *incoming == "" {
		return requiredFieldError(field)
	}
	*target = *incoming
	return nil
}

// DeleteAppConfig removes the team's App config. The FK from
// github_installations cascades, so this also disconnects every installation
// made through the App.
func (s *GitHubAppConfigService) DeleteAppConfig(ctx context.Context, teamID, userID string) error {
	if authzErr := s.authorizeAppConfigMutation(ctx, userID, teamID); authzErr != nil {
		return authzErr
	}

	config, err := s.repo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrGitHubAppConfigNotFound) {
			return ErrGitHubAppNotConfigured
		}
		return fmt.Errorf("failed to get GitHub App config: %w", err)
	}

	if err := s.repo.Delete(ctx, teamID, config.ID); err != nil {
		if errors.Is(err, repositories.ErrGitHubAppConfigNotFound) {
			return ErrGitHubAppNotConfigured
		}
		return fmt.Errorf("failed to delete GitHub App config: %w", err)
	}

	// Drop the cached client for an App that no longer exists. A resolve would
	// fail on the missing row anyway, so this is about not holding credentials
	// in memory after they were deleted.
	if s.clients != nil {
		s.clients.Evict(config.ID)
	}

	return nil
}

// RotateWebhookToken re-mints the routing token. The webhook URL changes, so the
// admin must update it on GitHub; the new URL is in the returned config.
func (s *GitHubAppConfigService) RotateWebhookToken(
	ctx context.Context, teamID, userID string,
) (*models.GitHubAppConfigResponse, error) {
	if authzErr := s.authorizeAppConfigMutation(ctx, userID, teamID); authzErr != nil {
		return nil, authzErr
	}

	config, err := s.loadForRotation(ctx, teamID)
	if err != nil {
		return nil, err
	}

	token, err := generateWebhookToken()
	if err != nil {
		return nil, err
	}
	config.WebhookToken = token

	if err := s.repo.Update(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to rotate webhook token: %w", err)
	}

	response := s.toResponse(config)
	return &response, nil
}

// RotateWebhookSecret regenerates the webhook secret and discloses it once. This
// is the recovery path for a lost secret — it is never readable after creation,
// so rotation is the only way back.
func (s *GitHubAppConfigService) RotateWebhookSecret(
	ctx context.Context, teamID, userID string,
) (*models.GitHubAppConfigCreated, error) {
	if authzErr := s.authorizeAppConfigMutation(ctx, userID, teamID); authzErr != nil {
		return nil, authzErr
	}

	config, err := s.loadForRotation(ctx, teamID)
	if err != nil {
		return nil, err
	}

	secret, err := generateSecret()
	if err != nil {
		return nil, err
	}
	encrypted, err := s.encrypt(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt webhook secret: %w", err)
	}
	config.WebhookSecretEncrypted = encrypted

	if err := s.repo.Update(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to rotate webhook secret: %w", err)
	}

	return &models.GitHubAppConfigCreated{
		GitHubAppConfigResponse: s.toResponse(config),
		WebhookSecret:           secret,
	}, nil
}

func (s *GitHubAppConfigService) loadForRotation(
	ctx context.Context, teamID string,
) (*models.GitHubAppConfig, error) {
	config, err := s.repo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrGitHubAppConfigNotFound) {
			return nil, ErrGitHubAppNotConfigured
		}
		return nil, fmt.Errorf("failed to get GitHub App config: %w", err)
	}
	return config, nil
}

// toResponse projects a config into its API view: secrets replaced by has_*
// masks, plus the webhook URL the admin pastes into GitHub.
func (s *GitHubAppConfigService) toResponse(config *models.GitHubAppConfig) models.GitHubAppConfigResponse {
	return models.GitHubAppConfigResponse{
		GitHubAppConfig:  *config,
		HasPrivateKey:    config.PrivateKeyEncrypted != "",
		HasClientSecret:  config.ClientSecretEncrypted != "",
		HasWebhookSecret: config.WebhookSecretEncrypted != "",
		WebhookURL:       s.webhookURL(config.WebhookToken),
	}
}

func (s *GitHubAppConfigService) webhookURL(token string) string {
	if s.webhookBaseURL == "" || token == "" {
		return ""
	}
	return fmt.Sprintf("%s/api/v1/webhooks/github/%s", s.webhookBaseURL, token)
}

// ValidateAppConfig proves the stored credentials actually work, by minting an
// App JWT from the private key and calling GET /app.
//
// It answers three questions a team would otherwise only get answered at install
// time: does the key belong to this app_id, is the stored slug the real one (a
// typo there produces a broken install URL), and does the App hold the
// permissions an import needs.
//
// Gated as a mutation: it makes the server perform an authenticated outbound
// call on the team's behalf, the same reasoning as ValidateModelProvider.
func (s *GitHubAppConfigService) ValidateAppConfig(
	ctx context.Context, teamID, userID string,
) (*models.ValidateGitHubAppResponse, error) {
	if authzErr := s.authorizeAppConfigMutation(ctx, userID, teamID); authzErr != nil {
		return nil, authzErr
	}

	config, err := s.repo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrGitHubAppConfigNotFound) {
			return nil, ErrGitHubAppNotConfigured
		}
		return nil, fmt.Errorf("failed to get GitHub App config: %w", err)
	}

	privateKeyPEM, err := s.decrypt(config.PrivateKeyEncrypted)
	if err != nil {
		logGitHubAppValidationFailure(teamID, "decrypt private key", err)
		return failedValidation(
			"The stored private key could not be read", githubAppErrInvalidCredentials, 0,
		), nil
	}

	token, err := s.mintAppJWT(config.AppID, privateKeyPEM)
	if err != nil {
		logGitHubAppValidationFailure(teamID, "mint app JWT", err)
		return failedValidation(
			"The stored private key is not a usable RSA key", githubAppErrInvalidCredentials, 0,
		), nil
	}

	return s.probeApp(ctx, teamID, config.AppSlug, token)
}

// mintAppJWT signs the short-lived RS256 JWT GitHub accepts as App
// authentication. IssuedAt is backdated to absorb clock skew, per GitHub's docs.
func (s *GitHubAppConfigService) mintAppJWT(appID, privateKeyPEM string) (string, error) {
	key, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	if _, err := strconv.ParseInt(appID, 10, 64); err != nil {
		return "", fmt.Errorf("invalid GitHub App ID: %w", err)
	}

	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-githubAppJWTBackdate)),
		ExpiresAt: jwt.NewNumericDate(now.Add(githubAppJWTLifetime)),
		Issuer:    appID,
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
}

// githubAppPayload is the subset of GET /app this probe reads.
type githubAppPayload struct {
	Slug        string            `json:"slug"`
	Permissions map[string]string `json:"permissions"`
}

// githubAppRequiredPermissions are what a blueprint import needs at minimum.
var githubAppRequiredPermissions = []string{"contents", "metadata"}

func (s *GitHubAppConfigService) probeApp(
	ctx context.Context, teamID, expectedSlug, token string,
) (*models.ValidateGitHubAppResponse, error) {
	started := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiBaseURL+"/app", nil)
	if err != nil {
		logGitHubAppValidationFailure(teamID, "build request", err)
		return failedValidation("Could not reach GitHub", githubAppErrConnectionFailed, 0), nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		logGitHubAppValidationFailure(teamID, "probe GET /app", err)
		return failedValidation("Could not reach GitHub", githubAppErrConnectionFailed, 0), nil
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Warn("Failed to close GitHub probe response body", "error", closeErr)
		}
	}()

	elapsed := int(time.Since(started).Milliseconds())

	if categorised := categoriseProbeStatus(resp.StatusCode, elapsed); categorised != nil {
		logGitHubAppValidationFailure(teamID, "probe GET /app",
			fmt.Errorf("unexpected status %d", resp.StatusCode))
		return categorised, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		logGitHubAppValidationFailure(teamID, "read probe body", err)
		return failedValidation("Could not read GitHub's response", githubAppErrConnectionFailed, elapsed), nil
	}

	var payload githubAppPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		logGitHubAppValidationFailure(teamID, "decode probe body", err)
		return failedValidation("Could not read GitHub's response", githubAppErrConnectionFailed, elapsed), nil
	}

	return buildValidationOutcome(payload, expectedSlug, elapsed, resp.StatusCode), nil
}

// categoriseProbeStatus maps a non-200 status onto a fixed category, or returns
// nil when the status is 200 and the body should be read.
func categoriseProbeStatus(status, elapsed int) *models.ValidateGitHubAppResponse {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return withStatus(failedValidation(
			"GitHub rejected these credentials", githubAppErrInvalidCredentials, elapsed), status)
	case http.StatusNotFound:
		return withStatus(failedValidation(
			"GitHub does not recognise this App", githubAppErrAppNotFound, elapsed), status)
	default:
		return withStatus(failedValidation(
			"GitHub returned an unexpected response", githubAppErrConnectionFailed, elapsed), status)
	}
}

// buildValidationOutcome turns a successful GET /app into a verdict: the slug
// must match, and the required permissions must be present.
func buildValidationOutcome(
	payload githubAppPayload, expectedSlug string, elapsed, status int,
) *models.ValidateGitHubAppResponse {
	if !strings.EqualFold(payload.Slug, expectedSlug) {
		// Caught here rather than at install time, where the only symptom would
		// be a 404 on a URL built from the wrong slug.
		out := withStatus(failedValidation(
			"The App slug does not match the one GitHub reports",
			githubAppErrSlugMismatch, elapsed), status)
		out.AppSlug = payload.Slug
		out.Permissions = payload.Permissions
		return out
	}

	if missing := missingPermissions(payload.Permissions); len(missing) > 0 {
		out := withStatus(failedValidation(
			fmt.Sprintf("The App is missing required permissions: %s", strings.Join(missing, ", ")),
			githubAppErrInsufficientPermission, elapsed), status)
		out.AppSlug = payload.Slug
		out.Permissions = payload.Permissions
		return out
	}

	return &models.ValidateGitHubAppResponse{
		IsValid:     true,
		Message:     "GitHub App configuration is valid",
		AppSlug:     payload.Slug,
		Permissions: payload.Permissions,
		Details: models.ValidateGitHubAppDetails{
			ResponseTime: elapsed,
			StatusCode:   status,
		},
	}
}

// missingPermissions reports which required permissions the App lacks. The
// message names them, which is safe: they are our own constants, not upstream text.
func missingPermissions(granted map[string]string) []string {
	var missing []string
	for _, required := range githubAppRequiredPermissions {
		if granted[required] == "" {
			missing = append(missing, required)
		}
	}
	return missing
}

func failedValidation(message, category string, elapsed int) *models.ValidateGitHubAppResponse {
	return &models.ValidateGitHubAppResponse{
		IsValid: false,
		Message: message,
		Details: models.ValidateGitHubAppDetails{
			ResponseTime: elapsed,
			ErrorDetails: category,
		},
	}
}

func withStatus(
	resp *models.ValidateGitHubAppResponse, status int,
) *models.ValidateGitHubAppResponse {
	resp.Details.StatusCode = status
	return resp
}

// logGitHubAppValidationFailure records the real failure server-side, where the
// detail is useful, instead of returning it to the caller.
func logGitHubAppValidationFailure(teamID, stage string, err error) {
	slog.Warn("GitHub App validation probe failed",
		"team_id", teamID,
		"stage", stage,
		"error", err,
	)
}

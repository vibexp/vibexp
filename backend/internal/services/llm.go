package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// completionTimeout bounds one outbound completion. It is an order of magnitude
// longer than validateModelProviderTimeout because generation is the slow part —
// a reachability probe that takes 30s is broken, a 500-token answer that does is
// ordinary. Retries and backoff are deliberately out of scope (#1069), so this is
// the single ceiling on a completion.
const completionTimeout = 120 * time.Second

// contextLengthMarkers are the phrases OpenAI-compatible servers use when a
// prompt exceeds the model's context window. They arrive as a plain 400, which is
// indistinguishable from any other bad request without reading the body — so this
// is the one classification that has to look at what the provider said.
var contextLengthMarkers = []string{
	"context_length_exceeded",
	"maximum context length",
	"context window",
	"too many tokens",
	"reduce the length",
	"reduce your prompt",
}

// LLMService is the ONE seam every AI feature resolves a completion through.
// It owns provider resolution, key decryption and error classification so no
// consumer reimplements them; a consumer declares the narrow interface it needs
// and calls Complete.
//
// It deliberately holds no cache: a provider carries a decrypted credential and a
// live HTTP client, and a credential edit must take effect on the next call.
type LLMService struct {
	repo repositories.ModelProviderRepository
	enc  EncryptionServiceInterface
	// guard bounds every outbound request made with a caller-supplied base_url (#464).
	guard  *ssrfGuard
	logger *slog.Logger
}

// NewLLMService builds the completion service. A nil logger falls back to the
// default one, because classification logs the provider's body and losing that is
// losing the only record of why a completion failed.
func NewLLMService(
	repo repositories.ModelProviderRepository,
	enc EncryptionServiceInterface,
	cfg *config.Config,
	logger *slog.Logger,
) *LLMService {
	if logger == nil {
		logger = slog.Default()
	}
	return &LLMService{
		repo:   repo,
		enc:    enc,
		guard:  ssrfGuardForConfig(cfg),
		logger: logger,
	}
}

// Resolve builds the completion provider for teamID. A non-nil providerID names a
// specific row; nil selects the team's default.
//
// There is NO silent fallback on ANY error — a repository failure, a key that will
// not decrypt and an unsupported provider type all return an error, the same rule
// the email sender resolver enforces (#499). Falling back to "some other provider"
// would spend a team's credits on a model they did not choose.
//
// The returned provider's OWN errors are UNCLASSIFIED and carry up to 512 bytes of
// the provider's response body. Do not surface them to a caller: route completions
// through Complete, which is what maps them onto the six sentinels and keeps the
// body server-side (#464). Resolve exists for callers that need the provider's
// identity — its model or its type — not as a second completion path.
func (s *LLMService) Resolve(
	ctx context.Context, teamID string, providerID *string,
) (ModelProvider, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("LLMService is nil")
	}
	if strings.TrimSpace(teamID) == "" {
		return nil, fmt.Errorf("%w: team id is required", ErrNoModelProvider)
	}

	row, err := s.loadProvider(ctx, teamID, providerID)
	if err != nil {
		return nil, err
	}

	apiKey, err := s.decryptAPIKey(row)
	if err != nil {
		return nil, err
	}

	provider, err := NewModelProvider(row, apiKey, row.Model, completionTimeout, s.guard)
	if err != nil {
		return nil, fmt.Errorf("failed to build the model provider: %w", err)
	}
	return provider, nil
}

// loadProvider fetches the named row, or the team's default when no id is given.
// Both "not found" cases collapse onto ErrNoModelProvider; every other repository
// failure stays a plain error, so a caller can tell "nothing configured" from
// "the database is unavailable".
func (s *LLMService) loadProvider(
	ctx context.Context, teamID string, providerID *string,
) (*models.ModelProvider, error) {
	if providerID != nil && strings.TrimSpace(*providerID) != "" {
		id := strings.TrimSpace(*providerID)
		row, err := s.repo.GetByID(ctx, teamID, id)
		if err != nil {
			if errors.Is(err, repositories.ErrModelProviderNotFound) {
				return nil, fmt.Errorf("%w: provider %q is not configured for this team", ErrNoModelProvider, id)
			}
			return nil, fmt.Errorf("failed to load the model provider: %w", err)
		}
		return row, nil
	}

	row, err := s.repo.GetDefault(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrDefaultModelProviderNotFound) {
			return nil, fmt.Errorf("%w: the team has no default model provider", ErrNoModelProvider)
		}
		return nil, fmt.Errorf("failed to load the default model provider: %w", err)
	}
	return row, nil
}

// decryptAPIKey unseals the stored credential through the shared, fail-closed
// EncryptionService — never an inline AES implementation (#294). A row with no
// stored key is legitimate (a local Ollama needs none); a row WITH one and no
// encryption service is a hard failure rather than an unauthenticated call.
func (s *LLMService) decryptAPIKey(row *models.ModelProvider) (string, error) {
	if row.APIKeyEncrypted == nil || *row.APIKeyEncrypted == "" {
		return "", nil
	}
	if s.enc == nil {
		return "", fmt.Errorf("%w: cannot decrypt the model provider API key", ErrEncryptionUnavailable)
	}
	apiKey, err := s.enc.Decrypt(*row.APIKeyEncrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt the model provider API key: %w", err)
	}
	return apiKey, nil
}

// Complete resolves the team's provider and runs one non-streaming completion.
//
// Every provider fault surfaces as one of the six sentinels in errors.go and
// carries no part of the provider's response body — that is logged here instead,
// capped by the provider's io.LimitReader, because a body echoed back to a caller
// is a success/failure oracle for the operator's internal network (#464).
//
// A cancelled context is the one failure returned unchanged: the caller abandoned
// the request, so dressing it as a provider fault would misattribute it.
func (s *LLMService) Complete(
	ctx context.Context, teamID string, providerID *string, req models.CompletionRequest,
) (*models.CompletionResponse, error) {
	// Checked before resolution so a caller's own bug costs neither a database read
	// nor an AES decrypt, and is not reported as a provider fault. The provider
	// keeps its own guard as defence in depth.
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("completion request requires at least one message")
	}

	provider, err := s.Resolve(ctx, teamID, providerID)
	if err != nil {
		return nil, err
	}

	resp, err := provider.Complete(ctx, req)
	if err != nil {
		return nil, s.classifyCompletionError(ctx, teamID, err)
	}
	return resp, nil
}

// classifyCompletionError maps a provider failure onto exactly one sentinel and
// logs what really happened server-side.
func (s *LLMService) classifyCompletionError(ctx context.Context, teamID string, err error) error {
	var httpErr *completionHTTPError
	if errors.As(err, &httpErr) {
		sentinel := sentinelForCompletionStatus(httpErr.StatusCode, httpErr.Body)
		s.logger.WarnContext(ctx, "Model provider refused a completion",
			slog.String("team_id", teamID),
			slog.Int("status_code", httpErr.StatusCode),
			slog.String("provider_response", httpErr.Body),
			slog.String("classified_as", sentinel.Error()),
		)
		return fmt.Errorf("%w: provider returned status %d", sentinel, httpErr.StatusCode)
	}

	if errors.Is(err, context.Canceled) {
		return err
	}

	sentinel := ErrProviderUnreachable
	switch {
	case errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err):
		sentinel = ErrCompletionTimeout
	case errors.Is(err, errUnusableCompletionResponse):
		// The provider answered; the answer is unusable. Reporting that as
		// unreachability would send an operator to the network when the real fault is
		// usually a base_url missing its /v1 suffix.
		sentinel = ErrModelRejected
	}
	// Neutral wording on purpose: this branch is reached both when nothing answered
	// and when a provider answered unusably, and the real error names the host and
	// the dial outcome, which is exactly the detail that must stay server-side.
	s.logger.WarnContext(ctx, "Completion request failed",
		slog.String("team_id", teamID),
		slog.String("error", err.Error()),
		slog.String("classified_as", sentinel.Error()),
	)
	return sentinel
}

// sentinelForCompletionStatus classifies an HTTP status, consulting the body only
// for the one case the status cannot express.
func sentinelForCompletionStatus(status int, body string) error {
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return ErrProviderUnauthorized
	case status == http.StatusRequestTimeout, status == http.StatusGatewayTimeout:
		return ErrCompletionTimeout
	case status == http.StatusRequestEntityTooLarge:
		return ErrContextTooLarge
	case status == http.StatusBadRequest && mentionsContextLength(body):
		return ErrContextTooLarge
	case status >= http.StatusBadRequest && status < http.StatusInternalServerError:
		// Includes 429: rate limiting is a refusal this service does not retry
		// (out of scope), and ErrModelRejected is the honest "it said no".
		return ErrModelRejected
	default:
		return ErrProviderUnreachable
	}
}

// mentionsContextLength reports whether the provider blamed the context window.
func mentionsContextLength(body string) bool {
	lowered := strings.ToLower(body)
	for _, marker := range contextLengthMarkers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

// isTimeoutError reports whether err is a transport timeout. http.Client's own
// Timeout surfaces as a *url.Error whose Timeout() is true, not as
// context.DeadlineExceeded, so both checks are needed.
func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

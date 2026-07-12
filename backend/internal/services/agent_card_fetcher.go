package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
)

const (
	// MaxResponseSize limits agent card response size to 1MB to prevent DoS attacks
	MaxResponseSize = 1024 * 1024 // 1MB
	// RequestTimeout defines the maximum time to wait for agent card responses
	RequestTimeout = 30 * time.Second
	// wellKnownAgentCardPath is the A2A-mandated location of the agent card.
	wellKnownAgentCardPath = "/.well-known/agent-card.json"
)

// Sentinel errors surfaced by the card HTTP transport / parser so FetchAgentCard
// can translate them into stable, user-facing messages via errors.Is.
var (
	errResponseTooLarge = errors.New("agent card response exceeds maximum allowed size")
	errInvalidCardJSON  = errors.New("agent card response is not valid JSON")
)

// AgentCardFetcherInterface defines methods for fetching agent cards
type AgentCardFetcherInterface interface {
	// FetchAgentCard discovers the agent card at cardURL. authHeaders, when
	// non-empty, are attached to the discovery request so cards that sit behind
	// header authentication can be fetched; pass nil for a public card. Derive
	// authHeaders from the stored agent via AgentAuthenticator.AuthHeaders.
	FetchAgentCard(ctx context.Context, cardURL string, authHeaders map[string]string) (*models.AgentCard, error)
}

// cardParser is the a2a-go card parser. It delegates to the SDK's typed
// unmarshal but returns a sentinel error on malformed JSON so callers can map
// it to a stable user-facing message.
var cardParser agentcard.Parser = func(body []byte) (*a2a.AgentCard, error) {
	var card a2a.AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return nil, errInvalidCardJSON
	}
	return &card, nil
}

// limitedResponseTransport caps the size of the response body read from the
// wrapped transport, defeating memory-exhaustion via an oversized agent card.
type limitedResponseTransport struct {
	base  http.RoundTripper
	limit int64
}

func (t *limitedResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &limitedReadCloser{
		reader: io.LimitReader(resp.Body, t.limit+1),
		closer: resp.Body,
		limit:  t.limit,
	}
	return resp, nil
}

// limitedReadCloser returns errResponseTooLarge once more than limit bytes have
// been read, so an unbounded body never fills memory.
type limitedReadCloser struct {
	reader io.Reader
	closer io.Closer
	limit  int64
	read   int64
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	n, err := l.reader.Read(p)
	l.read += int64(n)
	if l.read > l.limit {
		return n, errResponseTooLarge
	}
	return n, err
}

func (l *limitedReadCloser) Close() error { return l.closer.Close() }

// newAgentCardHTTPClient builds the HTTP client used to fetch agent cards. The
// transport uses an SSRF-safe dialer (from guard) that rejects connections to
// reserved IP ranges at connect time, defeating DNS rebinding on top of URL host
// validation, and caps the response body at MaxResponseSize.
func newAgentCardHTTPClient(guard *ssrfGuard) *http.Client {
	transport := guard.newSSRFSafeTransport(&http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ForceAttemptHTTP2:     true,
	})
	return &http.Client{
		Timeout:   RequestTimeout,
		Transport: &limitedResponseTransport{base: transport, limit: MaxResponseSize},
	}
}

// AgentCardFetcher discovers agent cards through the official a2a-go SDK's
// agentcard.Resolver, layering VibeXP's SSRF protections, response-size cap, and
// user-facing required-field validation on top.
type AgentCardFetcher struct {
	resolver   *agentcard.Resolver
	httpClient *http.Client
	guard      *ssrfGuard
}

// NewAgentCardFetcher creates a new AgentCardFetcher whose SSRF policy is derived
// from cfg: loopback/private destinations are permitted only in local development
// (see ssrfGuardForConfig), so a local checkout can preview a localhost A2A agent
// card while every real deployment stays fail-closed. A nil cfg yields the strict
// production policy.
func NewAgentCardFetcher(cfg *config.Config) *AgentCardFetcher {
	return newAgentCardFetcher(ssrfGuardForConfig(cfg))
}

// newAgentCardFetcher wires a fetcher around the supplied SSRF guard. Tests use
// this with a private-allowing guard to target loopback httptest servers.
func newAgentCardFetcher(guard *ssrfGuard) *AgentCardFetcher {
	client := newAgentCardHTTPClient(guard)
	return &AgentCardFetcher{
		resolver:   &agentcard.Resolver{Client: client, CardParser: cardParser},
		httpClient: client,
		guard:      guard,
	}
}

// Close gracefully shuts down the HTTP client connections
// This method should be called when the service is shutting down
func (f *AgentCardFetcher) Close() {
	if lt, ok := f.httpClient.Transport.(*limitedResponseTransport); ok {
		if transport, ok := lt.base.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
}

// validateAgentCardURL validates the agent card URL format, scheme, path, and that
// the host does not resolve to a reserved/internal IP range (SSRF protection).
func (f *AgentCardFetcher) validateAgentCardURL(ctx context.Context, cardURL string) error {
	parsedURL, err := url.Parse(cardURL)
	if err != nil {
		return fmt.Errorf("invalid agent card URL format: not a valid URL")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: agent card URL must use HTTP or HTTPS protocol")
	}

	if parsedURL.Path != wellKnownAgentCardPath {
		return fmt.Errorf("invalid URL path: agent card must be served from " +
			"'/.well-known/agent-card.json' path according to A2A specification")
	}

	if err := f.guard.validateOutboundHost(ctx, cardURL); err != nil {
		return fmt.Errorf("agent card URL is not allowed: %w", err)
	}

	return nil
}

// handleHTTPError converts HTTP status codes to user-friendly error messages
func handleHTTPError(statusCode int) error {
	switch statusCode {
	case http.StatusNotFound:
		return fmt.Errorf("agent card not found: received 404 Not Found")
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized: agent card requires authentication")
	case http.StatusForbidden:
		return fmt.Errorf("access forbidden: server refused the request")
	case http.StatusInternalServerError:
		return fmt.Errorf("server error: remote server is experiencing issues")
	case http.StatusBadGateway:
		return fmt.Errorf("bad gateway: proxy or gateway error")
	case http.StatusServiceUnavailable:
		return fmt.Errorf("service unavailable: remote service is temporarily unavailable")
	case http.StatusGatewayTimeout:
		return fmt.Errorf("gateway timeout: remote server response timeout")
	default:
		return fmt.Errorf("HTTP error: received status %d, expected 200 OK", statusCode)
	}
}

// handleRequestError converts network errors to user-friendly error messages
func handleRequestError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return fmt.Errorf("request timeout: agent card URL took too long to respond")
		}
		if urlErr.Temporary() { //nolint:staticcheck // Temporary() still meaningful for net timeouts
			return fmt.Errorf("temporary network error: unable to connect to agent card URL")
		}
	}
	return fmt.Errorf("network error: unable to fetch agent card")
}

// translateResolveError maps errors returned by agentcard.Resolver.Resolve into
// the stable, user-facing messages VibeXP surfaces in the card preview.
func translateResolveError(err error) error {
	if errors.Is(err, errResponseTooLarge) {
		return fmt.Errorf("agent card response too large: maximum allowed size is %d bytes", MaxResponseSize)
	}
	if errors.Is(err, errInvalidCardJSON) {
		return fmt.Errorf("invalid JSON format: unable to parse agent card response")
	}
	var statusErr *agentcard.ErrStatusNotOK
	if errors.As(err, &statusErr) {
		return handleHTTPError(statusErr.StatusCode)
	}
	return handleRequestError(err)
}

// FetchAgentCard discovers an agent card via the a2a-go resolver. The stored
// cardURL is the full well-known URL; the resolver appends the well-known path
// to a base URL, so we validate then hand it the origin. authHeaders, when
// non-empty, are attached to the discovery request so cards behind header auth
// can be fetched; the credential values are never logged.
func (f *AgentCardFetcher) FetchAgentCard(
	ctx context.Context, cardURL string, authHeaders map[string]string,
) (*models.AgentCard, error) {
	// Validate URL (scheme, path, and SSRF-safe host)
	if err := f.validateAgentCardURL(ctx, cardURL); err != nil {
		return nil, err
	}

	slog.With(
		"service", "agent-card-fetcher",
		"url", cardURL,
		"authenticated", len(authHeaders) > 0,
	).Info("Fetching agent card")

	parsedURL, err := url.Parse(cardURL)
	if err != nil {
		return nil, fmt.Errorf("invalid agent card URL format: not a valid URL")
	}
	baseURL := parsedURL.Scheme + "://" + parsedURL.Host

	opts := []agentcard.ResolveOption{
		agentcard.WithRequestHeader("Accept", "application/json"),
		agentcard.WithRequestHeader("User-Agent", "VibExp-Agent-Discovery/1.0"),
	}
	for name, value := range authHeaders {
		opts = append(opts, agentcard.WithRequestHeader(name, value))
	}

	card, err := f.resolver.Resolve(ctx, baseURL, opts...)
	if err != nil {
		return nil, translateResolveError(err)
	}

	if err := validateAgentCard(card); err != nil {
		return nil, fmt.Errorf("invalid agent card format: %v", err)
	}

	slog.With(
		"service", "agent-card-fetcher",
		"url", cardURL,
		"name", card.Name,
		"version", card.Version,
	).Info("Successfully fetched agent card")

	return card, nil
}

// validateAgentCardStringField reports a user-facing error for an empty required field.
func validateAgentCardStringField(fieldValue, fieldName string) error {
	if fieldValue == "" {
		return fmt.Errorf("the '%s' field is required in the agent card but was not found or is empty", fieldName)
	}
	return nil
}

// validateAgentCardRequiredFields enforces the required fields of an A2A v1.0
// agent card that VibeXP surfaces to users.
func validateAgentCardRequiredFields(card *a2a.AgentCard) error {
	stringFields := map[string]string{
		"name":        card.Name,
		"description": card.Description,
		"version":     card.Version,
	}

	for fieldName, fieldValue := range stringFields {
		if err := validateAgentCardStringField(fieldValue, fieldName); err != nil {
			return err
		}
	}

	if len(card.SupportedInterfaces) == 0 {
		return fmt.Errorf(
			"the 'supportedInterfaces' field is required in the agent card but was not found or is empty",
		)
	}
	for i, iface := range card.SupportedInterfaces {
		if iface == nil || iface.URL == "" {
			return fmt.Errorf(
				"supportedInterfaces #%d: the 'url' field is required but was not found or is empty", i+1,
			)
		}
	}
	if card.DefaultInputModes == nil {
		return fmt.Errorf("the 'defaultInputModes' field is required in the agent card but was not found")
	}
	if card.DefaultOutputModes == nil {
		return fmt.Errorf("the 'defaultOutputModes' field is required in the agent card but was not found")
	}
	if card.Skills == nil {
		return fmt.Errorf("the 'skills' field is required in the agent card but was not found")
	}

	return nil
}

// validateAgentCardSkill enforces the required per-skill fields.
func validateAgentCardSkill(i int, skill a2a.AgentSkill) error {
	if skill.ID == "" {
		return fmt.Errorf("skill #%d: the 'id' field is required but was not found or is empty", i+1)
	}
	if skill.Name == "" {
		return fmt.Errorf(
			"skill #%d ('%s'): the 'name' field is required but was not found or is empty",
			i+1, skill.ID,
		)
	}
	if skill.Description == "" {
		return fmt.Errorf(
			"skill #%d ('%s'): the 'description' field is required but was not found or is empty",
			i+1, skill.ID,
		)
	}
	if skill.Tags == nil {
		return fmt.Errorf("skill #%d ('%s'): the 'tags' field is required but was not found", i+1, skill.ID)
	}
	return nil
}

// validateAgentCard performs validation on the fetched agent card based on the A2A specification
func validateAgentCard(card *a2a.AgentCard) error {
	if err := validateAgentCardRequiredFields(card); err != nil {
		return err
	}

	for i, skill := range card.Skills {
		if err := validateAgentCardSkill(i, skill); err != nil {
			return err
		}
	}

	return nil
}

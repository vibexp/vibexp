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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/idp/github"
	"github.com/vibexp/vibexp/internal/auth/idp/google"
	"github.com/vibexp/vibexp/internal/auth/idp/oidc"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Defaults of the identity provider resolver's timing knobs.
const (
	// defaultIDPDiscoveryTimeout bounds the OIDC discovery (and the GitHub
	// credential check) of one provider, so a slow or unreachable issuer fails
	// instead of stalling sign-in.
	defaultIDPDiscoveryTimeout = 10 * time.Second
	// defaultIDPRetryBackoff is how long a provider that failed to build stays
	// excluded before the next resolve retries it, even when the auth settings
	// version is unchanged.
	defaultIDPRetryBackoff = 60 * time.Second
)

// defaultGitHubExchangeURL is GitHub's OAuth code-exchange endpoint, which
// TestProvider probes to check a GitHub provider's credentials.
const defaultGitHubExchangeURL = "https://github.com/login/oauth/access_token"

// ErrIdentityProviderCredentialsInvalid is returned by TestProvider when the
// identity provider rejected the client credentials.
var ErrIdentityProviderCredentialsInvalid = errors.New("identity provider rejected the client credentials")

// ProviderHealth is the last build outcome of one enabled identity provider,
// as seen by this replica. Health is held in memory and not persisted.
type ProviderHealth struct {
	// Healthy is true when the provider built and is offered for sign-in.
	Healthy bool
	// LastError is the build error of an unhealthy provider; empty when healthy.
	LastError string
	// CheckedAt is when the provider was last built (or tried).
	CheckedAt time.Time
}

// IdentityProviderResolver resolves the sign-in identity providers from the
// instance_auth_providers table at runtime (#1234), replacing the boot-time,
// config-built registry. Adding, editing or disabling a provider takes effect on
// the next sign-in, with no restart.
//
// Resolves are cached by the shared auth settings version: every call reads the
// version (one primary-key lookup) and reuses the cached snapshot while it is
// unchanged. A version change rebuilds the snapshot once (concurrent callers
// share one rebuild), and a rebuild reuses every provider whose row did not
// change, so an allowlist write never triggers OIDC rediscovery.
type IdentityProviderResolver interface {
	// Snapshot returns the enabled, successfully built providers keyed by slug,
	// in sort order. A provider that failed to build is left out and reported by
	// Health. The error is non-nil only when the providers could not be read.
	Snapshot(ctx context.Context) (*idp.Registry, error)
	// Provider returns the enabled provider with slug, and whether it exists.
	Provider(ctx context.Context, slug string) (idp.IdentityProvider, bool, error)
	// Health reports, keyed by slug, the last build outcome of every enabled
	// provider as of the last resolve on this replica.
	Health() map[string]ProviderHealth
	// TestProvider checks candidate without persisting anything and without
	// touching the cache: OIDC discovery for oidc and google, a credential probe
	// of GitHub's token endpoint for github. plaintextSecret is the secret to
	// test; nil means the candidate's stored ClientSecretEncrypted. Discovery
	// never sends the secret, and the GitHub probe goes only to GitHub, so a
	// stored secret is never disclosed to a candidate-chosen endpoint.
	TestProvider(ctx context.Context, candidate models.InstanceAuthProvider, plaintextSecret *string) error
}

// IdentityProviderResolverDeps are the collaborators of the resolver.
type IdentityProviderResolverDeps struct {
	Providers repositories.InstanceAuthProviderRepository
	Versions  repositories.InstanceAuthSettingsVersionRepository
	Enc       EncryptionServiceInterface
	// CallbackURL is the derived redirect URI every provider is built with
	// (config.Config.AuthCallbackURL).
	CallbackURL string
	Logger      *slog.Logger
}

// idpSnapshot is one resolved set of providers and the version it was built at.
type idpSnapshot struct {
	version  int64
	registry *idp.Registry
	// retryAt is the earliest time an unhealthy provider is due for a retry;
	// zero when every provider built.
	retryAt time.Time
}

// builtIDP memoizes one row's build, keyed by row id, so an unchanged row is
// never rebuilt.
type builtIDP struct {
	slug      string
	updatedAt time.Time
	entry     idp.Entry
	err       error
	checkedAt time.Time
}

// identityProviderResolver implements IdentityProviderResolver.
type identityProviderResolver struct {
	providers   repositories.InstanceAuthProviderRepository
	versions    repositories.InstanceAuthSettingsVersionRepository
	enc         EncryptionServiceInterface
	callbackURL string
	logger      *slog.Logger

	// Injectable for tests.
	discoveryTimeout  time.Duration
	retryBackoff      time.Duration
	now               func() time.Time
	googleIssuerURL   string
	githubExchangeURL string
	httpClient        *http.Client

	snap  atomic.Pointer[idpSnapshot]
	group singleflight.Group
	// mu guards memo. Rebuilds are serialized by group, but Health reads memo
	// concurrently.
	mu   sync.Mutex
	memo map[string]builtIDP
}

var _ IdentityProviderResolver = (*identityProviderResolver)(nil)

// WarnIgnoredLegacyRedirectURIs logs one WARN for each provider enabled in the
// legacy config.yaml (auth.providers / auth.provider) whose redirect_uri
// differs from the derived callbackURL. Providers not enabled there, and a
// redirect_uri still at the built-in default (config.docker.yaml bakes it for
// every provider), are skipped, so the WARN never fires for a value nobody set.
// The
// resolver builds every provider with the derived URL (#1234), so such a
// redirect_uri no longer has any effect, and an IdP console registered with it
// rejects sign-in until the derived URL is registered there instead.
func WarnIgnoredLegacyRedirectURIs(auth config.AuthConfig, callbackURL string, logger *slog.Logger) {
	redirectURIs := map[string]string{
		string(idp.ProviderGoogle): auth.LegacyGoogle.RedirectURI,
		string(idp.ProviderGitHub): auth.LegacyGitHub.RedirectURI,
		string(idp.ProviderOIDC):   auth.LegacyOIDC.RedirectURI,
	}
	for _, name := range auth.LegacyEnabledProviderNames() {
		configured := strings.TrimSpace(redirectURIs[name])
		if configured == "" || configured == callbackURL || config.IsDefaultAuthRedirectURI(configured) {
			// Unset, already right, or the built-in default nobody chose.
			continue
		}
		logger.With(
			"provider", name,
			"configured_redirect_uri", configured,
			"callback_url", callbackURL,
		).Warn("auth." + name + ".redirect_uri is ignored: every sign-in provider now redirects to the " +
			"derived callback URL; register callback_url in the identity provider's console")
	}
}

// NewIdentityProviderResolver creates the runtime identity provider resolver.
func NewIdentityProviderResolver(deps IdentityProviderResolverDeps) IdentityProviderResolver {
	return newIdentityProviderResolver(deps)
}

func newIdentityProviderResolver(deps IdentityProviderResolverDeps) *identityProviderResolver {
	return &identityProviderResolver{
		providers:         deps.Providers,
		versions:          deps.Versions,
		enc:               deps.Enc,
		callbackURL:       deps.CallbackURL,
		logger:            deps.Logger,
		discoveryTimeout:  defaultIDPDiscoveryTimeout,
		retryBackoff:      defaultIDPRetryBackoff,
		now:               time.Now,
		githubExchangeURL: defaultGitHubExchangeURL,
		httpClient:        &http.Client{},
		memo:              map[string]builtIDP{},
	}
}

// Snapshot implements IdentityProviderResolver.
func (r *identityProviderResolver) Snapshot(ctx context.Context) (*idp.Registry, error) {
	version, err := r.versions.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("read auth settings version: %w", err)
	}
	if s := r.snap.Load(); r.fresh(s, version) {
		return s.registry, nil
	}
	// One rebuild at a time, shared by every caller that arrives meanwhile. It
	// runs detached from the caller's cancellation, so one aborted sign-in
	// cannot fail the rebuild for everyone waiting on it; each discovery is
	// bounded by discoveryTimeout instead. A caller whose own request ends
	// stops waiting, while the rebuild carries on for the others.
	ch := r.group.DoChan("rebuild", func() (any, error) {
		if s := r.snap.Load(); r.fresh(s, version) || (s != nil && s.version > version) {
			// Built meanwhile, possibly at a newer version than this caller read.
			return s, nil
		}
		return r.rebuild(context.WithoutCancel(ctx), version)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.(*idpSnapshot).registry, nil
	}
}

// fresh reports whether s can be served for version: built at that version,
// with no unhealthy provider due for a retry.
func (r *identityProviderResolver) fresh(s *idpSnapshot, version int64) bool {
	if s == nil || s.version != version {
		return false
	}
	return s.retryAt.IsZero() || r.now().Before(s.retryAt)
}

// Provider implements IdentityProviderResolver.
func (r *identityProviderResolver) Provider(ctx context.Context, slug string) (idp.IdentityProvider, bool, error) {
	reg, err := r.Snapshot(ctx)
	if err != nil {
		return nil, false, err
	}
	p, ok := reg.Get(idp.ProviderName(slug))
	return p, ok, nil
}

// Health implements IdentityProviderResolver.
func (r *identityProviderResolver) Health() map[string]ProviderHealth {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]ProviderHealth, len(r.memo))
	for _, b := range r.memo {
		h := ProviderHealth{Healthy: b.err == nil, CheckedAt: b.checkedAt}
		if b.err != nil {
			h.LastError = b.err.Error()
		}
		out[b.slug] = h
	}
	return out
}

// rebuild reads the enabled providers and builds the snapshot for version,
// reusing every memoized build whose row is unchanged (and, for a failed build,
// whose retry backoff has not elapsed).
func (r *identityProviderResolver) rebuild(ctx context.Context, version int64) (*idpSnapshot, error) {
	rows, err := r.providers.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list auth providers: %w", err)
	}

	r.mu.Lock()
	previous := r.memo
	r.mu.Unlock()

	next := make(map[string]builtIDP, len(rows))
	entries := make([]idp.Entry, 0, len(rows))
	var retryAt time.Time
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		b := r.buildOrReuse(ctx, row, previous)
		next[row.ID] = b
		if b.err != nil {
			if due := b.checkedAt.Add(r.retryBackoff); retryAt.IsZero() || due.Before(retryAt) {
				retryAt = due
			}
			continue
		}
		entries = append(entries, b.entry)
	}

	s := &idpSnapshot{version: version, registry: idp.NewRegistryFromEntries(entries...), retryAt: retryAt}
	r.mu.Lock()
	r.memo = next
	r.mu.Unlock()
	r.snap.Store(s)
	return s, nil
}

// buildOrReuse returns the memoized build of row when it is still valid, and
// builds the row otherwise, logging a failure once per transition.
func (r *identityProviderResolver) buildOrReuse(
	ctx context.Context, row *models.InstanceAuthProvider, previous map[string]builtIDP,
) builtIDP {
	prev, had := previous[row.ID]
	sameRow := had && prev.updatedAt.Equal(row.UpdatedAt) && prev.slug == row.Slug
	if sameRow && (prev.err == nil || r.now().Before(prev.checkedAt.Add(r.retryBackoff))) {
		return prev
	}

	entry, err := r.buildRow(ctx, row, nil)
	r.logBuild(row, err, sameRow && prev.err != nil)
	return builtIDP{slug: row.Slug, updatedAt: row.UpdatedAt, entry: entry, err: err, checkedAt: r.now()}
}

// logBuild logs a build outcome once per transition: a failure is not logged
// again while the same row keeps failing (wasFailing), so a broken issuer
// never produces a WARN per sign-in.
func (r *identityProviderResolver) logBuild(row *models.InstanceAuthProvider, err error, wasFailing bool) {
	log := r.logger.With("provider", row.Slug, "type", string(row.Type))
	switch {
	case err != nil && !wasFailing:
		log.With("error", err).Warn("Identity provider failed to build; it is excluded from sign-in")
	case err != nil:
		// Still failing: already reported.
	case wasFailing:
		log.Info("Identity provider recovered")
	default:
		log.Info("Identity provider enabled")
	}
}

// buildRow builds the provider for row. plaintextSecret overrides the stored
// secret when non-nil. Discovery is bounded by discoveryTimeout.
func (r *identityProviderResolver) buildRow(
	ctx context.Context, row *models.InstanceAuthProvider, plaintextSecret *string,
) (idp.Entry, error) {
	secret, err := r.secretFor(row, plaintextSecret)
	if err != nil {
		return idp.Entry{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.discoveryTimeout)
	defer cancel()

	name := idp.ProviderName(row.Slug)
	var provider idp.IdentityProvider
	switch row.Type {
	case models.InstanceAuthProviderGoogle:
		provider, err = google.New(ctx, google.Config{
			Name: name, ClientID: row.ClientID, ClientSecret: secret,
			RedirectURL: r.callbackURL, IssuerURL: r.googleIssuerURL,
		})
	case models.InstanceAuthProviderGitHub:
		provider, err = github.New(github.Config{
			Name: name, ClientID: row.ClientID, ClientSecret: secret, RedirectURL: r.callbackURL,
		})
	case models.InstanceAuthProviderOIDC:
		issuer := ""
		if row.IssuerURL != nil {
			issuer = *row.IssuerURL
		}
		provider, err = oidc.New(ctx, oidc.Config{
			Name: name, IssuerURL: issuer, ClientID: row.ClientID, ClientSecret: secret,
			RedirectURL: r.callbackURL,
		})
	default:
		return idp.Entry{}, fmt.Errorf("unknown identity provider type %q", row.Type)
	}
	if err != nil {
		return idp.Entry{}, err
	}
	return idp.Entry{Provider: provider, Type: idp.ProviderName(row.Type), DisplayName: row.DisplayName}, nil
}

// secretFor returns plaintextSecret when set, and the decrypted stored secret
// otherwise.
func (r *identityProviderResolver) secretFor(
	row *models.InstanceAuthProvider, plaintextSecret *string,
) (string, error) {
	if plaintextSecret != nil {
		return *plaintextSecret, nil
	}
	if !row.HasClientSecret() {
		return "", errors.New("no client secret is stored")
	}
	if r.enc == nil {
		return "", errors.New("security.encryption_key is not configured, so the stored client secret cannot be read")
	}
	secret, err := r.enc.Decrypt(*row.ClientSecretEncrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt client secret: %w", err)
	}
	return secret, nil
}

// TestProvider implements IdentityProviderResolver.
func (r *identityProviderResolver) TestProvider(
	ctx context.Context, candidate models.InstanceAuthProvider, plaintextSecret *string,
) error {
	if candidate.Slug == "" {
		// The builders need a name; the slug is irrelevant to the check itself.
		candidate.Slug = string(candidate.Type)
	}
	if _, err := r.buildRow(ctx, &candidate, plaintextSecret); err != nil {
		return err
	}
	if candidate.Type != models.InstanceAuthProviderGitHub {
		return nil
	}
	secret, err := r.secretFor(&candidate, plaintextSecret)
	if err != nil {
		return err
	}
	return r.checkGitHubCredentials(ctx, candidate.ClientID, secret)
}

// checkGitHubCredentials probes GitHub's token endpoint with the credentials and
// a code that cannot be valid. GitHub answers bad_verification_code when it
// accepted the client credentials and incorrect_client_credentials when it did
// not.
func (r *identityProviderResolver) checkGitHubCredentials(ctx context.Context, clientID, secret string) error {
	ctx, cancel := context.WithTimeout(ctx, r.discoveryTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", secret)
	form.Set("code", "vibexp-credential-check")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.githubExchangeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("github: build credential check: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github: credential check: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			r.logger.With("error", cerr).Debug("Failed to close the GitHub credential check body")
		}
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("github: read credential check: %w", err)
	}
	var out struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("github: credential check returned HTTP %d with an unreadable body", resp.StatusCode)
	}
	switch out.Error {
	case "bad_verification_code":
		return nil
	case "incorrect_client_credentials":
		return ErrIdentityProviderCredentialsInvalid
	default:
		return fmt.Errorf("github: unexpected credential check response (HTTP %d, error %q)", resp.StatusCode, out.Error)
	}
}

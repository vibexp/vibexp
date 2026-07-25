package services

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/external/implementations"
	"github.com/vibexp/vibexp/internal/repositories"
)

// GitHubAppClientResolver selects a GitHub App client by team at call time
// (#480, epic #476).
//
// It replaces a process-wide singleton built once at wire time from instance
// config. Every method on the underlying client closes over one App's
// credentials — JWT minting, installation transports, the OAuth user-code
// exchange — so with per-team Apps the credentials have to be chosen per call.
type GitHubAppClientResolver interface {
	// ResolveForTeam builds (or returns a cached) client bound to the team's own
	// App credentials. Returns ErrGitHubAppNotConfigured when the team has no
	// App row, which callers surface as 409 so the UI can say "configure your
	// GitHub App first" rather than showing a generic failure.
	ResolveForTeam(ctx context.Context, teamID string) (*ResolvedGitHubApp, error)
	// Evict drops any cached client for an App config. A credential EDIT
	// invalidates itself through the version key, so this is for delete and
	// rotation, where the config id stops being valid entirely.
	Evict(appConfigID string)
}

// ResolvedGitHubApp is a team's client plus the identity of the App it came
// from.
//
// The App config id is part of the result rather than something callers look up
// separately because #477 made github_installations.app_config_id NOT NULL: an
// installation records which App produced it, so whoever resolves a client to
// perform an install must persist that same id. Returning them together makes
// it impossible to store an installation against the wrong App.
type ResolvedGitHubApp struct {
	Client      external.GitHubAppClient
	AppConfigID string
}

// githubAppClientCacheSize bounds the client cache. One client per team is
// small, but the map is unbounded in team count and would otherwise be a slow
// leak on a large instance. Least-recently-used entries are dropped past this.
const githubAppClientCacheSize = 256

// cachedGitHubAppClient is one cache entry. version is the App config's
// optimistic-lock counter at build time: a credential edit bumps it, so a stale
// entry is detected on the next resolve with no explicit invalidation to forget.
type cachedGitHubAppClient struct {
	appConfigID string
	version     int64
	client      external.GitHubAppClient
}

// githubAppClientResolver is the default GitHubAppClientResolver.
type githubAppClientResolver struct {
	repo   repositories.GitHubAppConfigRepository
	enc    EncryptionServiceInterface
	logger *slog.Logger

	// build constructs a client from resolved credentials. It is a field so
	// tests can observe construction (and count it, to prove caching) without
	// reaching GitHub.
	build func(cfg *config.GitHubAppConfig, logger *slog.Logger) external.GitHubAppClient

	mu    sync.Mutex
	cache map[string]*list.Element // appConfigID -> element holding *cachedGitHubAppClient
	lru   *list.List               // front = most recently used
}

// NewGitHubAppClientResolver creates the resolver.
func NewGitHubAppClientResolver(
	repo repositories.GitHubAppConfigRepository,
	enc EncryptionServiceInterface,
	logger *slog.Logger,
) GitHubAppClientResolver {
	return &githubAppClientResolver{
		repo:   repo,
		enc:    enc,
		logger: logger,
		build:  implementations.NewGitHubAppClient,
		cache:  make(map[string]*list.Element),
		lru:    list.New(),
	}
}

// ResolveForTeam returns the team's client, building it if necessary.
func (r *githubAppClientResolver) ResolveForTeam(
	ctx context.Context, teamID string,
) (*ResolvedGitHubApp, error) {
	stored, err := r.repo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrGitHubAppConfigNotFound) {
			return nil, ErrGitHubAppNotConfigured
		}
		return nil, fmt.Errorf("failed to load GitHub App config: %w", err)
	}

	if client, ok := r.cached(stored.ID, stored.Version); ok {
		return &ResolvedGitHubApp{Client: client, AppConfigID: stored.ID}, nil
	}

	appCfg, err := r.decryptConfig(stored.ID, stored.AppID, stored.ClientID,
		stored.PrivateKeyEncrypted, stored.ClientSecretEncrypted, stored.WebhookSecretEncrypted)
	if err != nil {
		return nil, err
	}

	client := r.build(appCfg, r.logger)
	r.store(stored.ID, stored.Version, client)

	return &ResolvedGitHubApp{Client: client, AppConfigID: stored.ID}, nil
}

// decryptConfig turns stored ciphertext into the credential struct the client
// needs. The plaintext lives only in this call's scope and in the returned
// struct; it is never logged — an error here names the App config id, never the
// value it failed on.
func (r *githubAppClientResolver) decryptConfig(
	appConfigID, appID, clientID, encPrivateKey, encClientSecret, encWebhookSecret string,
) (*config.GitHubAppConfig, error) {
	privateKeyPEM, err := r.enc.Decrypt(encPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt private key for App config %s: %w", appConfigID, err)
	}
	clientSecret, err := r.enc.Decrypt(encClientSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt client secret for App config %s: %w", appConfigID, err)
	}
	webhookSecret, err := r.enc.Decrypt(encWebhookSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt webhook secret for App config %s: %w", appConfigID, err)
	}

	privateKey, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("stored private key for App config %s is unusable: %w", appConfigID, err)
	}

	return &config.GitHubAppConfig{
		AppID:         appID,
		PrivateKey:    privateKey,
		PrivateKeyPEM: []byte(privateKeyPEM),
		WebhookSecret: webhookSecret,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
	}, nil
}

// cached returns the client for an App config when one is cached AT THE GIVEN
// VERSION. A version mismatch means the credentials were edited, so the entry is
// dropped rather than served — that is what makes a credential edit take effect
// with no explicit invalidation at the write site.
func (r *githubAppClientResolver) cached(appConfigID string, version int64) (external.GitHubAppClient, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	element, ok := r.cache[appConfigID]
	if !ok {
		return nil, false
	}

	entry, _ := element.Value.(*cachedGitHubAppClient)
	if entry.version != version {
		r.removeElement(element)
		return nil, false
	}

	r.lru.MoveToFront(element)
	return entry.client, true
}

func (r *githubAppClientResolver) store(appConfigID string, version int64, client external.GitHubAppClient) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.cache[appConfigID]; ok {
		// Two resolves racing across a credential edit can finish out of order:
		// the one that read the OLD version may store last. Keeping the newer
		// entry means that race costs nothing, rather than a wasted rebuild on
		// the next call.
		if entry, _ := existing.Value.(*cachedGitHubAppClient); entry.version > version {
			return
		}
		r.removeElement(existing)
	}

	element := r.lru.PushFront(&cachedGitHubAppClient{
		appConfigID: appConfigID,
		version:     version,
		client:      client,
	})
	r.cache[appConfigID] = element

	for r.lru.Len() > githubAppClientCacheSize {
		if oldest := r.lru.Back(); oldest != nil {
			r.removeElement(oldest)
		}
	}
}

// Evict drops a cached client outright.
func (r *githubAppClientResolver) Evict(appConfigID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if element, ok := r.cache[appConfigID]; ok {
		r.removeElement(element)
	}
}

// removeElement drops an entry from both the map and the LRU list. Callers must
// hold r.mu.
func (r *githubAppClientResolver) removeElement(element *list.Element) {
	entry, _ := element.Value.(*cachedGitHubAppClient)
	delete(r.cache, entry.appConfigID)
	r.lru.Remove(element)
}

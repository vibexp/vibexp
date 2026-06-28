package oauthserver

import (
	"context"
	"sync"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// In-memory repository fakes used to drive the Authorization Server end-to-end in
// tests without a database. They implement the repositories.OAuth* interfaces.

type memClientRepo struct {
	mu      sync.Mutex
	clients map[string]*models.OAuthClient
}

func newMemClientRepo() *memClientRepo {
	return &memClientRepo{clients: map[string]*models.OAuthClient{}}
}

func (r *memClientRepo) Create(_ context.Context, c *models.OAuthClient) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *c
	r.clients[c.ID] = &cp
	return nil
}

func (r *memClientRepo) GetByID(_ context.Context, id string) (*models.OAuthClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.clients[id]
	if !ok {
		return nil, repositories.ErrOAuthClientNotFound
	}
	cp := *c
	return &cp, nil
}

type memRequestRepo struct {
	mu   sync.Mutex
	rows map[string]*models.OAuthRequest
}

func newMemRequestRepo() *memRequestRepo {
	return &memRequestRepo{rows: map[string]*models.OAuthRequest{}}
}

func (r *memRequestRepo) Create(_ context.Context, req *models.OAuthRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *req
	r.rows[req.Signature] = &cp
	return nil
}

func (r *memRequestRepo) Get(_ context.Context, sig string) (*models.OAuthRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[sig]
	if !ok {
		return nil, repositories.ErrOAuthRequestNotFound
	}
	cp := *row
	return &cp, nil
}

func (r *memRequestRepo) Delete(_ context.Context, sig string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, sig)
	return nil
}

func (r *memRequestRepo) Deactivate(_ context.Context, sig string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if row, ok := r.rows[sig]; ok {
		row.Active = false
	}
	return nil
}

func (r *memRequestRepo) DeactivateByRequestID(_ context.Context, requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.RequestID == requestID {
			row.Active = false
		}
	}
	return nil
}

func (r *memRequestRepo) DeleteByRequestID(_ context.Context, requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for sig, row := range r.rows {
		if row.RequestID == requestID {
			delete(r.rows, sig)
		}
	}
	return nil
}

type memSigningKeyRepo struct {
	mu   sync.Mutex
	keys []*models.OAuthSigningKey
}

func newMemSigningKeyRepo() *memSigningKeyRepo {
	return &memSigningKeyRepo{}
}

func (r *memSigningKeyRepo) Create(_ context.Context, k *models.OAuthSigningKey) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *k
	r.keys = append(r.keys, &cp)
	return nil
}

func (r *memSigningKeyRepo) GetActive(_ context.Context) (*models.OAuthSigningKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, k := range r.keys {
		if k.Active {
			cp := *k
			return &cp, nil
		}
	}
	return nil, repositories.ErrOAuthSigningKeyNotFound
}

func (r *memSigningKeyRepo) ListAll(_ context.Context) ([]*models.OAuthSigningKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*models.OAuthSigningKey, 0, len(r.keys))
	for _, k := range r.keys {
		cp := *k
		out = append(out, &cp)
	}
	return out, nil
}

func (r *memSigningKeyRepo) Activate(_ context.Context, kid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	found := false
	for _, k := range r.keys {
		switch {
		case k.KID == kid:
			k.Active = true
			found = true
		case k.Active:
			k.Active = false
		}
	}
	if !found {
		return repositories.ErrOAuthSigningKeyNotFound
	}
	return nil
}

type memLoginSessionRepo struct {
	mu       sync.Mutex
	sessions map[string]*models.OAuthLoginSession
}

func newMemLoginSessionRepo() *memLoginSessionRepo {
	return &memLoginSessionRepo{sessions: map[string]*models.OAuthLoginSession{}}
}

func (r *memLoginSessionRepo) Create(_ context.Context, s *models.OAuthLoginSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *s
	r.sessions[s.ID] = &cp
	return nil
}

func (r *memLoginSessionRepo) Get(_ context.Context, id string) (*models.OAuthLoginSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil, repositories.ErrOAuthLoginSessionNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *memLoginSessionRepo) AttachUser(_ context.Context, id, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return repositories.ErrOAuthLoginSessionNotFound
	}
	s.UserID = &userID
	return nil
}

func (r *memLoginSessionRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	return nil
}

func (r *memLoginSessionRepo) DeleteExpired(_ context.Context) (int64, error) {
	return 0, nil
}

// fakeProvider is a stub idp.IdentityProvider whose callback URL echoes a fixed
// authorization code, and whose ExchangeCode yields fixed claims.
type fakeProvider struct {
	name   idp.ProviderName
	claims *idp.Claims
}

func (p *fakeProvider) Name() idp.ProviderName { return p.name }

func (p *fakeProvider) AuthorizeURL(state, redirectURI, _ string) string {
	return redirectURI + "?code=upstream-code&state=" + state
}

func (p *fakeProvider) ExchangeCode(
	_ context.Context, _, _ string,
) (*idp.Tokens, *idp.Claims, error) {
	return &idp.Tokens{AccessToken: "upstream-at"}, p.claims, nil
}

func (p *fakeProvider) Refresh(_ context.Context, _ string) (*idp.Tokens, error) {
	return &idp.Tokens{}, nil
}

// fakeProvisioner returns a fixed user, standing in for AuthService provisioning.
type fakeProvisioner struct {
	user *models.User
}

func (p *fakeProvisioner) ProvisionFromClaims(
	_ context.Context, _ string, _ *idp.Claims,
) (*models.User, error) {
	return p.user, nil
}

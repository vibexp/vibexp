package oauthserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/ory/fosite"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Store adapts VibeXP's repositories to the fosite storage interfaces it needs:
// fosite.ClientManager, oauth2.CoreStorage, oauth2.TokenRevocationStorage, and
// pkce.PKCERequestStorage. All fosite<->persistence marshalling lives here so the
// repository layer stays free of fosite types.
type Store struct {
	clients repositories.OAuthClientRepository
	codes   repositories.OAuthRequestRepository
	access  repositories.OAuthRequestRepository
	refresh repositories.OAuthRequestRepository
	pkce    repositories.OAuthRequestRepository
}

// NewStore builds a fosite storage adapter from the OAuth repositories.
func NewStore(
	clients repositories.OAuthClientRepository,
	codes, access, refresh, pkce repositories.OAuthRequestRepository,
) *Store {
	return &Store{clients: clients, codes: codes, access: access, refresh: refresh, pkce: pkce}
}

// --- fosite.ClientManager ---

// GetClient returns the registered client or fosite.ErrNotFound.
func (s *Store) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	c, err := s.clients.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repositories.ErrOAuthClientNotFound) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}
	return &fosite.DefaultClient{
		ID:            c.ID,
		Secret:        c.SecretHash,
		RedirectURIs:  c.RedirectURIs,
		GrantTypes:    c.GrantTypes,
		ResponseTypes: c.ResponseTypes,
		Scopes:        c.Scopes,
		Audience:      c.Audience,
		Public:        c.Public,
	}, nil
}

// ClientAssertionJWTValid is part of the JWT client-authentication replay guard.
// VibeXP only registers public PKCE clients (no private_key_jwt), so there is
// nothing to track and every assertion id is considered unused.
func (s *Store) ClientAssertionJWTValid(_ context.Context, _ string) error { return nil }

// SetClientAssertionJWT is a no-op for the same reason as ClientAssertionJWTValid.
func (s *Store) SetClientAssertionJWT(_ context.Context, _ string, _ time.Time) error { return nil }

// --- oauth2.AuthorizeCodeStorage ---

func (s *Store) CreateAuthorizeCodeSession(ctx context.Context, code string, req fosite.Requester) error {
	row, err := toRow(code, "", req)
	if err != nil {
		return err
	}
	return s.codes.Create(ctx, row)
}

func (s *Store) GetAuthorizeCodeSession(
	ctx context.Context, code string, session fosite.Session,
) (fosite.Requester, error) {
	row, err := s.codes.Get(ctx, code)
	if err != nil {
		return nil, mapGetErr(err)
	}
	requester, err := s.toRequester(ctx, row, session)
	if err != nil {
		return nil, err
	}
	if !row.Active {
		// Must return the requester alongside ErrInvalidatedAuthorizeCode.
		return requester, fosite.ErrInvalidatedAuthorizeCode
	}
	return requester, nil
}

func (s *Store) InvalidateAuthorizeCodeSession(ctx context.Context, code string) error {
	return s.codes.Deactivate(ctx, code)
}

// --- oauth2.AccessTokenStorage ---

func (s *Store) CreateAccessTokenSession(ctx context.Context, sig string, req fosite.Requester) error {
	row, err := toRow(sig, "", req)
	if err != nil {
		return err
	}
	return s.access.Create(ctx, row)
}

func (s *Store) GetAccessTokenSession(
	ctx context.Context, sig string, session fosite.Session,
) (fosite.Requester, error) {
	row, err := s.access.Get(ctx, sig)
	if err != nil {
		return nil, mapGetErr(err)
	}
	if !row.Active {
		return nil, fosite.ErrNotFound
	}
	return s.toRequester(ctx, row, session)
}

func (s *Store) DeleteAccessTokenSession(ctx context.Context, sig string) error {
	return s.access.Delete(ctx, sig)
}

// --- oauth2.RefreshTokenStorage ---

func (s *Store) CreateRefreshTokenSession(
	ctx context.Context, sig, accessSig string, req fosite.Requester,
) error {
	row, err := toRow(sig, accessSig, req)
	if err != nil {
		return err
	}
	return s.refresh.Create(ctx, row)
}

func (s *Store) GetRefreshTokenSession(
	ctx context.Context, sig string, session fosite.Session,
) (fosite.Requester, error) {
	row, err := s.refresh.Get(ctx, sig)
	if err != nil {
		return nil, mapGetErr(err)
	}
	requester, err := s.toRequester(ctx, row, session)
	if err != nil {
		return nil, err
	}
	if !row.Active {
		// Signals refresh-token reuse; the handler then revokes the family.
		return requester, fosite.ErrInactiveToken
	}
	return requester, nil
}

func (s *Store) DeleteRefreshTokenSession(ctx context.Context, sig string) error {
	return s.refresh.Delete(ctx, sig)
}

// RotateRefreshToken deactivates the rotated family's prior refresh token and
// removes its access token, mirroring fosite's reference semantics. The handler
// creates the replacement tokens (same request id) immediately afterwards.
func (s *Store) RotateRefreshToken(ctx context.Context, requestID, _ string) error {
	if err := s.RevokeRefreshToken(ctx, requestID); err != nil {
		return err
	}
	return s.RevokeAccessToken(ctx, requestID)
}

// --- oauth2.TokenRevocationStorage ---

func (s *Store) RevokeRefreshToken(ctx context.Context, requestID string) error {
	return s.refresh.DeactivateByRequestID(ctx, requestID)
}

// RevokeRefreshTokenMaybeGracePeriod revokes without a grace period (we do not
// implement graceful rotation).
func (s *Store) RevokeRefreshTokenMaybeGracePeriod(ctx context.Context, requestID, _ string) error {
	return s.RevokeRefreshToken(ctx, requestID)
}

func (s *Store) RevokeAccessToken(ctx context.Context, requestID string) error {
	return s.access.DeleteByRequestID(ctx, requestID)
}

// --- pkce.PKCERequestStorage ---

func (s *Store) CreatePKCERequestSession(ctx context.Context, sig string, req fosite.Requester) error {
	row, err := toRow(sig, "", req)
	if err != nil {
		return err
	}
	return s.pkce.Create(ctx, row)
}

func (s *Store) GetPKCERequestSession(
	ctx context.Context, sig string, session fosite.Session,
) (fosite.Requester, error) {
	row, err := s.pkce.Get(ctx, sig)
	if err != nil {
		return nil, mapGetErr(err)
	}
	return s.toRequester(ctx, row, session)
}

func (s *Store) DeletePKCERequestSession(ctx context.Context, sig string) error {
	return s.pkce.Delete(ctx, sig)
}

// --- helpers ---

// mapGetErr converts a repository not-found into fosite.ErrNotFound.
func mapGetErr(err error) error {
	if errors.Is(err, repositories.ErrOAuthRequestNotFound) {
		return fosite.ErrNotFound
	}
	return err
}

// toRow serializes a fosite requester into the persistence-neutral model. Rows
// are always created active; invalidation/rotation flips Active later.
func toRow(signature, accessSignature string, req fosite.Requester) (*models.OAuthRequest, error) {
	sessionData, err := json.Marshal(req.GetSession())
	if err != nil {
		return nil, fmt.Errorf("oauthserver: marshal session: %w", err)
	}
	formData, err := json.Marshal(req.GetRequestForm())
	if err != nil {
		return nil, fmt.Errorf("oauthserver: marshal form: %w", err)
	}
	return &models.OAuthRequest{
		Signature:         signature,
		AccessSignature:   accessSignature,
		RequestID:         req.GetID(),
		ClientID:          req.GetClient().GetID(),
		Subject:           req.GetSession().GetSubject(),
		RequestedScope:    req.GetRequestedScopes(),
		GrantedScope:      req.GetGrantedScopes(),
		RequestedAudience: req.GetRequestedAudience(),
		GrantedAudience:   req.GetGrantedAudience(),
		RequestedAt:       req.GetRequestedAt(),
		FormData:          formData,
		SessionData:       sessionData,
		Active:            true,
	}, nil
}

// toRequester rehydrates a fosite requester from a stored row, unmarshalling the
// session into the caller-provided session value and re-resolving the client.
func (s *Store) toRequester(
	ctx context.Context, row *models.OAuthRequest, session fosite.Session,
) (fosite.Requester, error) {
	client, err := s.GetClient(ctx, row.ClientID)
	if err != nil {
		return nil, err
	}
	if len(row.SessionData) > 0 {
		if uerr := json.Unmarshal(row.SessionData, session); uerr != nil {
			return nil, uerr
		}
	}
	var form url.Values
	if len(row.FormData) > 0 {
		if uerr := json.Unmarshal(row.FormData, &form); uerr != nil {
			return nil, uerr
		}
	}
	return &fosite.Request{
		ID:                row.RequestID,
		RequestedAt:       row.RequestedAt,
		Client:            client,
		RequestedScope:    fosite.Arguments(row.RequestedScope),
		GrantedScope:      fosite.Arguments(row.GrantedScope),
		RequestedAudience: fosite.Arguments(row.RequestedAudience),
		GrantedAudience:   fosite.Arguments(row.GrantedAudience),
		Form:              form,
		Session:           session,
	}, nil
}

package oauthserver

import (
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/token/jwt"
)

// newEmptySession returns an empty JWT session for fosite to hydrate from storage
// (used at the token endpoint).
func newEmptySession() *oauth2.JWTSession {
	return &oauth2.JWTSession{
		JWTClaims: &jwt.JWTClaims{},
		JWTHeader: &jwt.Headers{},
		ExpiresAt: map[fosite.TokenType]time.Time{},
	}
}

// newIssuingSession builds the session for a freshly-authorized user. The access
// token's `sub` comes from JWTClaims.Subject; its `aud` and `scope` are taken by
// fosite from the request's granted audience/scopes, and `iss` from the
// configured AccessTokenIssuer.
func newIssuingSession(userID string) *oauth2.JWTSession {
	return &oauth2.JWTSession{
		JWTClaims: &jwt.JWTClaims{Subject: userID},
		JWTHeader: &jwt.Headers{},
		Subject:   userID,
		ExpiresAt: map[fosite.TokenType]time.Time{},
	}
}

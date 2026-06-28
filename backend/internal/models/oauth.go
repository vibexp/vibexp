package models

import "time"

// OAuthClient is a persisted OAuth 2.1 client (dynamically registered via RFC 7591).
// Public PKCE clients have a nil SecretHash. The fields map directly onto the
// columns of the oauth_clients table and onto fosite's client model.
type OAuthClient struct {
	ID                      string
	SecretHash              []byte
	RedirectURIs            []string
	GrantTypes              []string
	ResponseTypes           []string
	Scopes                  []string
	Audience                []string
	Public                  bool
	TokenEndpointAuthMethod string
	ClientName              string
	CreatedAt               time.Time
}

// OAuthRequest is the persistence-neutral representation of a fosite request
// (authorization code, access token, refresh token, or PKCE session). The
// oauthserver package marshals fosite's requester/session into SessionData and
// FormData and rehydrates them on read; the repository layer stays free of any
// fosite types. Active is false once a code is invalidated or a refresh token is
// rotated, which drives reuse detection.
type OAuthRequest struct {
	Signature         string
	AccessSignature   string // refresh tokens only; links to the paired access token
	RequestID         string
	ClientID          string
	Subject           string
	RequestedScope    []string
	GrantedScope      []string
	RequestedAudience []string
	GrantedAudience   []string
	RequestedAt       time.Time
	FormData          []byte
	SessionData       []byte
	Active            bool
}

// OAuthSigningKey is a DB-backed JWT signing key. The active key signs new access
// tokens; inactive keys are retained so already-issued tokens still validate
// against the JWKS until they expire. PrivateKeyEncrypted holds the PEM-encoded
// RSA private key sealed with the app encryption key (AES-256-GCM). PublicJWK is
// the public JSON Web Key (JSON) published at the JWKS endpoint.
type OAuthSigningKey struct {
	KID                 string
	Algorithm           string
	PrivateKeyEncrypted []byte
	PublicJWK           []byte
	Active              bool
	CreatedAt           time.Time
	RotatedAt           *time.Time
}

// OAuthLoginSession is the short-lived stash for the federated login leg: it
// holds the original /authorize query while the user authenticates against the
// upstream IdP, plus the resolved user id once the IdP callback completes, until
// consent is granted and the authorization code is issued.
type OAuthLoginSession struct {
	ID             string
	AuthorizeQuery string
	Provider       string
	IDPState       string
	UserID         *string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

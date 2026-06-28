-- Embedded OAuth 2.1 Authorization Server (issue #31).
--
-- VibeXP mints its own MCP-audience-bound JWT access tokens via ory/fosite,
-- federating the login leg to the #29 identity-provider registry. These tables
-- back fosite's storage interfaces (clients, authorization codes, access &
-- refresh tokens, PKCE request sessions), the DB-backed rotating signing keys
-- served at the JWKS endpoint, and the short-lived login-flow stash used while
-- the user authenticates against an upstream IdP.
--
-- All token/code tables share the same shape: a `signature` primary key (the
-- fosite token signature), the serialized request (`session_data`, `form_data`)
-- plus denormalized columns fosite needs to rehydrate a requester, and an
-- `active` flag used for authorization-code invalidation and refresh-token
-- reuse detection.

-- Dynamically-registered OAuth clients (RFC 7591). Public PKCE clients have a
-- NULL secret_hash; confidential clients (none today) would store a bcrypt hash.
CREATE TABLE public.oauth_clients (
    id                         varchar(255) PRIMARY KEY,
    secret_hash                bytea,
    redirect_uris              text[] NOT NULL DEFAULT '{}',
    grant_types                text[] NOT NULL DEFAULT '{}',
    response_types             text[] NOT NULL DEFAULT '{}',
    scopes                     text[] NOT NULL DEFAULT '{}',
    audience                   text[] NOT NULL DEFAULT '{}',
    public                     boolean NOT NULL DEFAULT true,
    token_endpoint_auth_method varchar(64) NOT NULL DEFAULT 'none',
    client_name                varchar(255) NOT NULL DEFAULT '',
    created_at                 timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Authorization codes issued by /authorize and redeemed at /token.
CREATE TABLE public.oauth_authorization_codes (
    signature          varchar(512) PRIMARY KEY,
    request_id         varchar(255) NOT NULL,
    client_id          varchar(255) NOT NULL,
    subject            varchar(255) NOT NULL DEFAULT '',
    requested_scope    text[] NOT NULL DEFAULT '{}',
    granted_scope      text[] NOT NULL DEFAULT '{}',
    requested_audience text[] NOT NULL DEFAULT '{}',
    granted_audience   text[] NOT NULL DEFAULT '{}',
    requested_at       timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    form_data          jsonb NOT NULL DEFAULT '{}'::jsonb,
    session_data       jsonb NOT NULL DEFAULT '{}'::jsonb,
    active             boolean NOT NULL DEFAULT true,
    created_at         timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_oauth_authorization_codes_request_id ON public.oauth_authorization_codes (request_id);

-- Access-token sessions. Access tokens are self-contained JWTs; rows exist only
-- so they can be revoked (RFC 7009) and introspected.
CREATE TABLE public.oauth_access_tokens (
    signature          varchar(512) PRIMARY KEY,
    request_id         varchar(255) NOT NULL,
    client_id          varchar(255) NOT NULL,
    subject            varchar(255) NOT NULL DEFAULT '',
    requested_scope    text[] NOT NULL DEFAULT '{}',
    granted_scope      text[] NOT NULL DEFAULT '{}',
    requested_audience text[] NOT NULL DEFAULT '{}',
    granted_audience   text[] NOT NULL DEFAULT '{}',
    requested_at       timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    form_data          jsonb NOT NULL DEFAULT '{}'::jsonb,
    session_data       jsonb NOT NULL DEFAULT '{}'::jsonb,
    active             boolean NOT NULL DEFAULT true,
    created_at         timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_oauth_access_tokens_request_id ON public.oauth_access_tokens (request_id);

-- Refresh-token sessions. `active=false` marks a rotated token; replaying it
-- triggers reuse detection, which revokes the whole family by request_id.
CREATE TABLE public.oauth_refresh_tokens (
    signature          varchar(512) PRIMARY KEY,
    access_signature   varchar(512) NOT NULL DEFAULT '',
    request_id         varchar(255) NOT NULL,
    client_id          varchar(255) NOT NULL,
    subject            varchar(255) NOT NULL DEFAULT '',
    requested_scope    text[] NOT NULL DEFAULT '{}',
    granted_scope      text[] NOT NULL DEFAULT '{}',
    requested_audience text[] NOT NULL DEFAULT '{}',
    granted_audience   text[] NOT NULL DEFAULT '{}',
    requested_at       timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    form_data          jsonb NOT NULL DEFAULT '{}'::jsonb,
    session_data       jsonb NOT NULL DEFAULT '{}'::jsonb,
    active             boolean NOT NULL DEFAULT true,
    created_at         timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_oauth_refresh_tokens_request_id ON public.oauth_refresh_tokens (request_id);

-- PKCE request sessions, keyed by the authorization-code signature.
CREATE TABLE public.oauth_pkce_sessions (
    signature          varchar(512) PRIMARY KEY,
    request_id         varchar(255) NOT NULL,
    client_id          varchar(255) NOT NULL,
    subject            varchar(255) NOT NULL DEFAULT '',
    requested_scope    text[] NOT NULL DEFAULT '{}',
    granted_scope      text[] NOT NULL DEFAULT '{}',
    requested_audience text[] NOT NULL DEFAULT '{}',
    granted_audience   text[] NOT NULL DEFAULT '{}',
    requested_at       timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    form_data          jsonb NOT NULL DEFAULT '{}'::jsonb,
    session_data       jsonb NOT NULL DEFAULT '{}'::jsonb,
    active             boolean NOT NULL DEFAULT true,
    created_at         timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- DB-backed signing keys. The active key signs new access tokens; inactive keys
-- are retained so previously-issued tokens still validate against the JWKS until
-- they expire. The RSA private key is encrypted at rest with the app encryption
-- key (AES-256-GCM) before storage.
CREATE TABLE public.oauth_signing_keys (
    kid                  varchar(64) PRIMARY KEY,
    algorithm            varchar(16) NOT NULL DEFAULT 'RS256',
    private_key_encrypted bytea NOT NULL,
    public_jwk           jsonb NOT NULL,
    active               boolean NOT NULL DEFAULT false,
    created_at           timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    rotated_at           timestamp with time zone
);
-- At most one active signing key at a time.
CREATE UNIQUE INDEX idx_oauth_signing_keys_active ON public.oauth_signing_keys (active) WHERE active;

-- Short-lived stash for the federated login leg: holds the original /authorize
-- query while the user authenticates upstream, and the resolved user id once the
-- IdP callback completes, until consent is granted and the code is issued.
CREATE TABLE public.oauth_login_sessions (
    id              varchar(255) PRIMARY KEY,
    authorize_query text NOT NULL,
    provider        varchar(50) NOT NULL,
    idp_state       varchar(255) NOT NULL,
    user_id         uuid REFERENCES public.users (id) ON DELETE CASCADE,
    created_at      timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at      timestamp with time zone NOT NULL
);
CREATE INDEX idx_oauth_login_sessions_expires_at ON public.oauth_login_sessions (expires_at);

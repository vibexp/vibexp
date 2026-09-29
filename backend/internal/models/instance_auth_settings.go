package models

import "time"

// InstanceAuthProviderType is the kind of a DB-managed sign-in identity
// provider (#1231, epic #1230). The set mirrors the CHECK on
// instance_auth_providers.type; change both together.
type InstanceAuthProviderType string

const (
	// InstanceAuthProviderGoogle is Google sign-in. At most one is stored.
	InstanceAuthProviderGoogle InstanceAuthProviderType = "google"
	// InstanceAuthProviderGitHub is GitHub sign-in. At most one is stored.
	InstanceAuthProviderGitHub InstanceAuthProviderType = "github"
	// InstanceAuthProviderOIDC is a generic OpenID Connect provider, discovered
	// from its issuer URL. Any number may be stored.
	InstanceAuthProviderOIDC InstanceAuthProviderType = "oidc"
)

// Valid reports whether t is one of the known provider types.
func (t InstanceAuthProviderType) Valid() bool {
	switch t {
	case InstanceAuthProviderGoogle, InstanceAuthProviderGitHub, InstanceAuthProviderOIDC:
		return true
	}
	return false
}

// InstanceAuthProvider is one DB-managed sign-in identity provider: the
// database-stored counterpart of config.yaml's auth provider entries.
//
// Slug and Type are immutable once created. ClientSecretEncrypted holds
// ciphertext produced by the encryption service; the repository stores whatever
// it is handed and never encrypts or decrypts. nil means no secret is stored.
// It is tagged json:"-" so the ciphertext can never be marshaled into a response
// or an audit snapshot.
type InstanceAuthProvider struct {
	ID                    string                   `json:"id" db:"id"`
	Type                  InstanceAuthProviderType `json:"type" db:"type"`
	Slug                  string                   `json:"slug" db:"slug"`
	DisplayName           string                   `json:"display_name" db:"display_name"`
	Enabled               bool                     `json:"enabled" db:"enabled"`
	SortOrder             int                      `json:"sort_order" db:"sort_order"`
	ClientID              string                   `json:"client_id" db:"client_id"`
	ClientSecretEncrypted *string                  `json:"-" db:"client_secret_encrypted"`
	// IssuerURL is set on exactly the OIDC providers (a database CHECK).
	IssuerURL *string   `json:"issuer_url,omitempty" db:"issuer_url"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	// UpdatedBy is the user who last saved the provider; nil for the boot-time
	// config.yaml import, or once that user has been deleted.
	UpdatedBy *string `json:"updated_by,omitempty" db:"updated_by"`
}

// HasClientSecret reports whether a client secret is stored, without
// disclosing it.
func (p *InstanceAuthProvider) HasClientSecret() bool {
	return p.ClientSecretEncrypted != nil && *p.ClientSecretEncrypted != ""
}

// InstanceAuthAllowlist is the instance's sign-in access allowlist: a user may
// sign in when their email's domain is in Domains or the address is in Emails.
// The table is a singleton, and no row stored means open access.
//
// Both lists are stored normalized (see services.ValidateInstanceAuthAllowlist).
type InstanceAuthAllowlist struct {
	Domains   []string  `json:"domains" db:"domains"`
	Emails    []string  `json:"emails" db:"emails"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	// UpdatedBy is nil for the boot-time import, or once that user is deleted.
	UpdatedBy *string `json:"updated_by,omitempty" db:"updated_by"`
	// Version is the row's own compare-and-set counter. Every write to the
	// allowlist also bumps the shared auth settings version.
	Version int64 `json:"version" db:"version"`
}

// InstanceAdminGrant is one instance admin granted in the database, in addition
// to those named in config.yaml.
type InstanceAdminGrant struct {
	UserID string `json:"user_id" db:"user_id"`
	// GrantedBy is nil for a grant with no actor, or once that user is deleted.
	GrantedBy *string   `json:"granted_by,omitempty" db:"granted_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

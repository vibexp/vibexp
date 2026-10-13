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
// The table is a singleton. Access is OPEN — every user may sign in — both when
// no row is stored and when a stored row has both lists empty (IsOpenAccess),
// the same rule config.yaml's access allowlist has always had. An empty row
// never means "nobody may sign in".
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

// IsOpenAccess reports whether the allowlist admits every user: both lists
// empty. A nil allowlist (none stored) is open access too.
func (a *InstanceAuthAllowlist) IsOpenAccess() bool {
	return a == nil || (len(a.Domains) == 0 && len(a.Emails) == 0)
}

// Registration modes of the instance authentication policy. The values are
// stored in instance_auth_policy.registration_mode (its CHECK lists them) and
// published in the admin API; change all three together.
const (
	// RegistrationModeOpen lets anyone the access allowlist admits get an
	// account by signing in through a provider. It is the behaviour the
	// instance had before the policy existed, and the default.
	RegistrationModeOpen = "open"
	// RegistrationModeInviteOnly admits only people who were invited to a team
	// or created by an administrator, whatever sign-in method they use.
	RegistrationModeInviteOnly = "invite_only"
)

// ValidRegistrationMode reports whether mode is a registration mode.
func ValidRegistrationMode(mode string) bool {
	return mode == RegistrationModeOpen || mode == RegistrationModeInviteOnly
}

// InstanceAuthPolicy is the instance authentication policy (#1429): whether
// people may sign in with an email and password, and who may get an account.
// The table is a singleton. No row stored means DefaultInstanceAuthPolicy.
//
// It holds no credential, so its audit snapshot is the row itself.
type InstanceAuthPolicy struct {
	// PasswordLoginEnabled is whether email and password sign-in is offered.
	PasswordLoginEnabled bool `json:"password_login_enabled" db:"password_login_enabled"`
	// RegistrationMode is one of the RegistrationMode* values.
	RegistrationMode string    `json:"registration_mode" db:"registration_mode"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
	// UpdatedBy is nil once that user is deleted.
	UpdatedBy *string `json:"updated_by,omitempty" db:"updated_by"`
	// Version is the row's own compare-and-set counter; 0 when the policy is
	// the default and no row is stored. Every write to the policy also bumps
	// the shared auth settings version.
	Version int64 `json:"version" db:"version"`
}

// DefaultInstanceAuthPolicy is the policy of an instance that has stored none:
// password login off and open registration, which is how every instance behaved
// before the policy existed.
func DefaultInstanceAuthPolicy() InstanceAuthPolicy {
	return InstanceAuthPolicy{RegistrationMode: RegistrationModeOpen}
}

// IsStored reports whether the policy was read from a stored row, as opposed to
// being the default of an instance that has saved none.
func (p InstanceAuthPolicy) IsStored() bool {
	return p.Version > 0
}

// InviteOnly reports whether only invited or admin-created people may get an
// account.
func (p InstanceAuthPolicy) InviteOnly() bool {
	return p.RegistrationMode == RegistrationModeInviteOnly
}

// InstanceAdminGrant is one instance admin granted in the database, in addition
// to those named in config.yaml.
type InstanceAdminGrant struct {
	UserID string `json:"user_id" db:"user_id"`
	// GrantedBy is nil for a grant with no actor, or once that user is deleted.
	GrantedBy *string   `json:"granted_by,omitempty" db:"granted_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

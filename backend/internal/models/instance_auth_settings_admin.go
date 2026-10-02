package models

import "time"

// Views and requests of the instance authentication settings admin API
// (#1238, epic #1230). None of them can carry a client secret out: the views
// expose only HasClientSecret, and the requests carry plaintext inward only.

// Health states of a sign-in identity provider, as one replica sees it.
const (
	// InstanceAuthProviderHealthy is an enabled provider that built and is
	// offered for sign-in.
	InstanceAuthProviderHealthy = "healthy"
	// InstanceAuthProviderUnhealthy is an enabled provider that failed to build
	// and is excluded from sign-in.
	InstanceAuthProviderUnhealthy = "unhealthy"
	// InstanceAuthProviderDisabled is a provider that is not enabled, so it is
	// never built.
	InstanceAuthProviderDisabled = "disabled"
	// InstanceAuthProviderHealthUnknown is an enabled provider this replica
	// could not resolve for the request.
	InstanceAuthProviderHealthUnknown = "unknown"
)

// InstanceAuthProviderHealth is a provider's health on the replica that served
// the request. It is held in memory and never persisted.
type InstanceAuthProviderHealth struct {
	Status string
	// LastError is set only for InstanceAuthProviderUnhealthy.
	LastError *string
	// CheckedAt is nil for a disabled or unknown provider.
	CheckedAt *time.Time
}

// InstanceAuthProviderView is one stored provider as the admin API shows it.
type InstanceAuthProviderView struct {
	Provider InstanceAuthProvider
	Health   InstanceAuthProviderHealth
	// RedirectURI is the derived callback URL every provider is built with.
	RedirectURI string
}

// InstanceAuthProviderList is every stored provider with the shared auth
// settings version they were read at.
type InstanceAuthProviderList struct {
	Providers []InstanceAuthProviderView
	Version   int64
}

// InstanceAuthProviderSaved is a provider as written, with the shared auth
// settings version after the write.
type InstanceAuthProviderSaved struct {
	Provider InstanceAuthProviderView
	Version  int64
}

// CreateInstanceAuthProviderRequest is a new provider. ClientSecret is
// plaintext and is encrypted before it is stored.
type CreateInstanceAuthProviderRequest struct {
	Type         InstanceAuthProviderType
	Slug         string
	DisplayName  string
	Enabled      bool
	SortOrder    int
	ClientID     string
	ClientSecret string
	IssuerURL    *string
	// ExpectedVersion, when set, must equal the shared auth settings version.
	ExpectedVersion *int64
}

// UpdateInstanceAuthProviderRequest replaces a provider's editable fields; the
// slug and the type are immutable. A nil ClientSecret keeps the stored one.
type UpdateInstanceAuthProviderRequest struct {
	DisplayName     string
	Enabled         bool
	SortOrder       int
	ClientID        string
	ClientSecret    *string
	IssuerURL       *string
	ExpectedVersion int64
	Lockout         InstanceAuthLockoutContext
}

// DeleteInstanceAuthProviderRequest removes a provider.
type DeleteInstanceAuthProviderRequest struct {
	// ExpectedVersion, when set, must equal the shared auth settings version.
	ExpectedVersion *int64
	Lockout         InstanceAuthLockoutContext
}

// InstanceAuthLockoutContext is what the lockout guard needs to know about the
// caller of a provider change.
type InstanceAuthLockoutContext struct {
	// SessionProvider is the slug of the provider the caller's session was
	// issued by; empty for an API key, a setup session, dev login and sessions
	// that predate multi-provider sign-in, which have no provider of their own
	// to lose.
	SessionProvider string
	// Confirmed is the caller's confirm_lockout_risk: it applies a change the
	// guard would otherwise refuse.
	Confirmed bool
}

// TestInstanceAuthProviderRequest names what to test: a stored provider (ID
// alone), an unsaved candidate (no ID), or a stored provider with the set
// fields replaced (ID plus fields).
type TestInstanceAuthProviderRequest struct {
	ID           *string
	Type         *InstanceAuthProviderType
	ClientID     *string
	ClientSecret *string
	IssuerURL    *string
}

// InstanceAuthProviderTestResult is the outcome of a provider test. A failed
// test is a result, not an error.
type InstanceAuthProviderTestResult struct {
	Valid bool
	// Message says why the test failed; empty when it passed.
	Message string
}

// InstanceAdminView is one DB-granted instance admin with the user it names.
type InstanceAdminView struct {
	UserID    string
	Email     string
	Name      string
	GrantedBy *string
	GrantedAt time.Time
}

// InstanceAdminList is the instance's admins: the root admins' emails from
// config (read-only) and the DB grants, oldest first.
type InstanceAdminList struct {
	RootAdmins []string
	Admins     []InstanceAdminView
}

// InstanceAdminGrantTarget names the existing user to grant, by exactly one of
// UserID or Email.
type InstanceAdminGrantTarget struct {
	UserID string
	Email  string
}

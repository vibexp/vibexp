package models

import "time"

// GitHubAppConfig is a team's own GitHub App registration (#477, epic #476).
// It replaces the instance-wide App that used to live in config.yaml, so each
// team can use its own GitHub org, permission scope and rate-limit budget.
//
// The three *Encrypted fields hold ciphertext produced by the encryption
// service; the repository stores whatever it is handed and never encrypts. They
// are tagged json:"-" so a config can never be marshaled into a response with
// its secrets attached -- GitHubAppConfigResponse exposes has_* booleans
// instead.
type GitHubAppConfig struct {
	ID     string `json:"id" db:"id"`
	TeamID string `json:"team_id" db:"team_id"`
	// UserID records who registered the App. Informational only: the team FK is
	// the tenancy boundary, and no read is ever scoped by this column.
	UserID *string `json:"user_id,omitempty" db:"user_id"`
	// AppID is GitHub's numeric App id, carried as a string because it is only
	// ever echoed back or signed into a JWT, never used arithmetically.
	AppID string `json:"app_id" db:"app_id"`
	// AppSlug builds the https://github.com/apps/<slug>/installations/new URL.
	AppSlug string `json:"app_slug" db:"app_slug"`
	// ClientID is not a secret -- GitHub shows it on the App settings page and
	// the UI echoes it so an operator can confirm which App is wired up.
	ClientID               string `json:"client_id" db:"client_id"`
	PrivateKeyEncrypted    string `json:"-" db:"private_key_encrypted"`
	ClientSecretEncrypted  string `json:"-" db:"client_secret_encrypted"`
	WebhookSecretEncrypted string `json:"-" db:"webhook_secret_encrypted"`
	// WebhookToken is the opaque routing token embedded in this App's webhook
	// URL. The public webhook route has no team context, so this token is what
	// resolves an inbound delivery to a team. It is not a shared secret (the
	// webhook secret authenticates the payload), but it is not published
	// either, so it stays out of JSON here and is surfaced only as part of the
	// full webhook URL on the response type.
	WebhookToken string    `json:"-" db:"webhook_token"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
	Version      int64     `json:"version" db:"version"`
}

// CreateGitHubAppConfigRequest is the payload for registering a team's App.
//
// There is deliberately no webhook_secret field: the service generates it from
// crypto/rand (#478). That is one fewer value for an admin to invent and one
// fewer weak-secret failure mode. It is disclosed exactly once, in
// GitHubAppConfigCreated, so it can be pasted into the App's settings.
type CreateGitHubAppConfigRequest struct {
	AppID    string `json:"app_id" validate:"required,min=1,max=50"`
	AppSlug  string `json:"app_slug" validate:"required,min=1,max=255"`
	ClientID string `json:"client_id" validate:"required,min=1,max=255"`
	// #nosec G117 - Request struct field for private key input, not a hardcoded secret
	PrivateKey string `json:"private_key" validate:"required"`
	// #nosec G117 - Request struct field for client secret input, not a hardcoded secret
	ClientSecret string `json:"client_secret" validate:"required"`
}

// UpdateGitHubAppConfigRequest is the payload for editing a team's App. Every
// field is a pointer so an omitted secret means "keep the stored one" rather
// than "clear it" -- the UI never receives the current secrets, so it cannot
// resend them. An explicitly EMPTY secret is a validation error, not a clear:
// a GitHub App without a private key or client secret is not a meaningful
// state, so the request is far more likely a client bug than an intent.
//
// webhook_secret is absent for the same reason it is absent from create -- it
// is server-generated, and replaced through RotateWebhookSecret.
type UpdateGitHubAppConfigRequest struct {
	AppID    *string `json:"app_id,omitempty" validate:"omitempty,min=1,max=50"`
	AppSlug  *string `json:"app_slug,omitempty" validate:"omitempty,min=1,max=255"`
	ClientID *string `json:"client_id,omitempty" validate:"omitempty,min=1,max=255"`
	// #nosec G117 - Request struct field for private key input, not a hardcoded secret
	PrivateKey *string `json:"private_key,omitempty"`
	// #nosec G117 - Request struct field for client secret input, not a hardcoded secret
	ClientSecret *string `json:"client_secret,omitempty"`
}

// GitHubAppConfigResponse is the API view of a team's App: the entity minus its
// secrets, plus a boolean per secret so the UI can show "configured" without
// ever receiving the value, plus the webhook URL to paste into the App's
// settings. WebhookURL is composed by the service from the instance base URL
// and the config's webhook token -- the model carries no base URL of its own.
type GitHubAppConfigResponse struct {
	GitHubAppConfig
	HasPrivateKey    bool   `json:"has_private_key"`
	HasClientSecret  bool   `json:"has_client_secret"`
	HasWebhookSecret bool   `json:"has_webhook_secret"`
	WebhookURL       string `json:"webhook_url"`
}

// GitHubAppConfigCreated is the one and only response that carries a plaintext
// webhook secret. Create and RotateWebhookSecret return it; every subsequent
// read returns GitHubAppConfigResponse, which exposes only has_webhook_secret.
// The admin must paste this value into the App's settings on GitHub, and
// recovering from a lost one means rotating rather than reading it back.
type GitHubAppConfigCreated struct {
	GitHubAppConfigResponse
	// #nosec G117 - Response field carrying a generated secret, not a hardcoded one
	WebhookSecret string `json:"webhook_secret"`
}

// ValidateGitHubAppResponse reports the outcome of the GET /app probe.
//
// A failed probe is reported in the body with IsValid=false, not as an error --
// a wrong key is a user-correctable condition, not a server fault. ErrorDetails
// is always one of the fixed categories so the response cannot become an oracle
// for what the server reached; the real upstream error is logged server-side.
type ValidateGitHubAppResponse struct {
	IsValid bool   `json:"is_valid"`
	Message string `json:"message"`
	// AppSlug is the slug GitHub reports for the authenticated App, echoed so a
	// mismatch with the stored value is visible rather than silently producing a
	// broken install URL later.
	AppSlug string `json:"app_slug,omitempty"`
	// Permissions are the App's granted permissions, so the UI can warn about a
	// missing contents/metadata read before the team hits it during an import.
	Permissions map[string]string        `json:"permissions,omitempty"`
	Details     ValidateGitHubAppDetails `json:"details,omitempty"`
}

// ValidateGitHubAppDetails carries the fixed-category diagnostics for a probe.
type ValidateGitHubAppDetails struct {
	ResponseTime int    `json:"response_time_ms,omitempty"`
	StatusCode   int    `json:"status_code,omitempty"`
	ErrorDetails string `json:"error_details,omitempty"`
}

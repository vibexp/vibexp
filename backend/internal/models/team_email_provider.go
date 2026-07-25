package models

import (
	"encoding/json"
	"time"
)

// TeamEmailProvider is a team's own outbound email provider (#501, epic #499).
//
// Mail configuration is instance-wide today, so every team sends through the
// operator's provider and from the operator's address. A row here overrides that
// for one team; the ABSENCE of a row is what selects the instance fallback, which
// is why there is no Enabled field. At most one row exists per team, enforced by
// unique_team_email_provider rather than by application code.
//
// SecretEncrypted holds ciphertext produced by the encryption service. The
// repository stores whatever it is handed and never encrypts or decrypts. It is
// tagged json:"-" so a provider can never be marshaled into a response with the
// credential attached -- the API view exposes a has_secret boolean instead.
type TeamEmailProvider struct {
	ID     string `json:"id" db:"id"`
	TeamID string `json:"team_id" db:"team_id"`
	// UserID records who last configured the provider. Informational only: the
	// team FK is the tenancy boundary, and no read is ever scoped by this column.
	UserID *string `json:"user_id,omitempty" db:"user_id"`
	// ProviderType is one of smtp|mailgun|postmark|sendgrid, matching the values
	// implementations.NewEmailProvider accepts.
	ProviderType string `json:"provider_type" db:"provider_type"`
	// Settings holds the non-secret per-type fields (SMTP host/port/username,
	// Mailgun domain/base_url, Postmark message_stream). Raw JSON rather than a
	// map so the stored shape round-trips untouched through this layer.
	Settings json.RawMessage `json:"settings" db:"settings"`
	// SecretEncrypted is the provider's single credential as base64
	// AES-256-GCM ciphertext. Non-pointer because the column is NOT NULL: a row
	// always represents a fully configured provider.
	SecretEncrypted string `json:"-" db:"secret_encrypted"`
	// FromAddress is the envelope sender the team's mail is sent as.
	FromAddress string  `json:"from_address" db:"from_address"`
	FromName    *string `json:"from_name,omitempty" db:"from_name"`
	ReplyTo     *string `json:"reply_to,omitempty" db:"reply_to"`
	// LastSuccessAt, LastError and LastErrorAt are delivery health, written by
	// RecordSendResult. The CURRENT state is derived by comparing the two
	// timestamps (later wins) rather than stored, so the last failure stays
	// readable for diagnosis after the provider recovers.
	LastSuccessAt *time.Time `json:"last_success_at,omitempty" db:"last_success_at"`
	LastError     *string    `json:"last_error,omitempty" db:"last_error"`
	LastErrorAt   *time.Time `json:"last_error_at,omitempty" db:"last_error_at"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
	Version       int64      `json:"version" db:"version"`
}

// SMTPProviderSettings are the non-secret SMTP fields. The password is the
// provider's secret and is carried separately.
type SMTPProviderSettings struct {
	Host     string `json:"host" validate:"required"`
	Port     string `json:"port" validate:"required"`
	Username string `json:"username,omitempty"`
}

// MailgunProviderSettings are the non-secret Mailgun fields. The sending key is
// the provider's secret. BaseURL is optional and selects a non-US region.
type MailgunProviderSettings struct {
	Domain  string `json:"domain" validate:"required"`
	BaseURL string `json:"base_url,omitempty"`
}

// PostmarkProviderSettings are the non-secret Postmark fields. The server token
// is the provider's secret; MessageStream is optional and defaults to
// Postmark's "outbound" stream.
type PostmarkProviderSettings struct {
	MessageStream string `json:"message_stream,omitempty"`
}

// TeamEmailProviderSettings is the discriminated union of per-type non-secret
// settings. Exactly the block matching ProviderType may be present; the others
// must be absent, so a body that names one type but configures another is a
// validation error rather than a silently ignored field.
//
// There is deliberately no SendGrid block: SendGrid's only configuration is its
// API key, which is the secret.
type TeamEmailProviderSettings struct {
	SMTP     *SMTPProviderSettings     `json:"smtp,omitempty"`
	Mailgun  *MailgunProviderSettings  `json:"mailgun,omitempty"`
	Postmark *PostmarkProviderSettings `json:"postmark,omitempty"`
}

// UpsertTeamEmailProviderRequest is the payload for configuring a team's email
// provider. The endpoint is an upsert, so one request type covers create and
// update.
type UpsertTeamEmailProviderRequest struct {
	ProviderType string                    `json:"provider_type" validate:"required"`
	Settings     TeamEmailProviderSettings `json:"settings"`
	// Secret is the provider's single credential. A nil Secret on a provider
	// that already exists means "keep the stored one" — the UI never receives
	// the current secret, so it cannot resend it. An empty string is rejected
	// rather than treated as "clear": a provider with no credential could not
	// send, and silently disabling a team's mail is worse than a 400.
	// #nosec G117 - Request struct field for the provider secret, not a hardcoded secret
	Secret      *string `json:"secret,omitempty"`
	FromAddress string  `json:"from_address" validate:"required,email"`
	FromName    *string `json:"from_name,omitempty"`
	ReplyTo     *string `json:"reply_to,omitempty" validate:"omitempty,email"`
}

// TestTeamEmailProviderRequest is the payload for a test send. It carries a full
// configuration rather than a reference to the stored row so an admin can verify
// credentials BEFORE saving them.
//
// It deliberately carries no recipient: the test message always goes to the
// acting user's own account email, so the endpoint cannot be used as an open
// relay by an admin of any team.
type TestTeamEmailProviderRequest struct {
	UpsertTeamEmailProviderRequest
}

// TeamEmailProviderTestResult is the outcome of a test send. A delivery failure
// is a result, not an error: the caller asked "does this configuration work?"
// and "no, because X" is a successful answer.
type TeamEmailProviderTestResult struct {
	Success bool `json:"success"`
	// Recipient echoes where the test message was sent, so the UI can tell the
	// admin which mailbox to check.
	Recipient string `json:"recipient"`
	Message   string `json:"message"`
}

// TeamEmailProviderResponse is the API view of a team's provider: the entity
// minus its secret, plus a boolean so the UI can show "configured" without ever
// receiving the value, plus derived delivery health.
type TeamEmailProviderResponse struct {
	TeamEmailProvider
	HasSecret bool `json:"has_secret"`
	IsHealthy bool `json:"is_healthy"`
}

// NewTeamEmailProviderResponse builds the API view of a provider.
func NewTeamEmailProviderResponse(provider *TeamEmailProvider) *TeamEmailProviderResponse {
	return &TeamEmailProviderResponse{
		TeamEmailProvider: *provider,
		HasSecret:         provider.SecretEncrypted != "",
		IsHealthy:         provider.IsHealthy(),
	}
}

// IsHealthy reports whether the last observed send succeeded.
//
// Health is derived from the two timestamps instead of a stored status column:
// a provider that has recovered still carries its last error for diagnosis, so
// the presence of LastError alone does not mean "currently broken". A provider
// that has never sent anything is treated as healthy — there is no evidence
// against it yet.
func (p *TeamEmailProvider) IsHealthy() bool {
	if p.LastErrorAt == nil {
		return true
	}
	if p.LastSuccessAt == nil {
		return false
	}
	return p.LastSuccessAt.After(*p.LastErrorAt)
}

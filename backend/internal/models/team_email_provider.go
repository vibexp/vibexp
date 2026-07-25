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

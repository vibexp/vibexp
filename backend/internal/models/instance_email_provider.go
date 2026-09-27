package models

import (
	"encoding/json"
	"time"
)

// InstanceEmailProvider is the instance's own outbound email provider (#1186,
// epic #1185): the database-stored counterpart of config.yaml's `email:`
// section, editable by an instance admin without file access or a restart.
//
// The table is a singleton (primary key `id boolean CHECK (id)`), so there is no
// ID field: the row is never addressed, only present or absent.
//
// SecretEncrypted holds ciphertext produced by the encryption service; the
// repository stores whatever it is handed and never encrypts or decrypts. It is
// a pointer because, unlike TeamEmailProvider, the instance may legitimately
// send without a credential (an unauthenticated SMTP relay): nil means "no
// credential". It is tagged json:"-" so the ciphertext can never be marshaled
// into a response.
type InstanceEmailProvider struct {
	// ProviderType is one of smtp|mailgun|postmark|sendgrid, matching the
	// values implementations.NewEmailProvider accepts.
	ProviderType string `json:"provider_type" db:"provider_type"`
	// Settings holds the non-secret per-type fields, kept as raw JSON so the
	// stored shape round-trips untouched through this layer.
	Settings        json.RawMessage `json:"settings" db:"settings"`
	SecretEncrypted *string         `json:"-" db:"secret_encrypted"`
	FromAddress     string          `json:"from_address" db:"from_address"`
	FromName        *string         `json:"from_name,omitempty" db:"from_name"`
	ReplyTo         *string         `json:"reply_to,omitempty" db:"reply_to"`
	// ContactRecipientAddress and PrivacyPolicyURL are instance-only: where the
	// contact form delivers and the privacy policy linked from outbound mail.
	// Nil means "fall back".
	ContactRecipientAddress *string `json:"contact_recipient_address,omitempty" db:"contact_recipient_address"`
	PrivacyPolicyURL        *string `json:"privacy_policy_url,omitempty" db:"privacy_policy_url"`
	// LastSuccessAt, LastError and LastErrorAt are delivery health, written by
	// RecordSuccess / RecordError. See IsHealthy.
	LastSuccessAt *time.Time `json:"last_success_at,omitempty" db:"last_success_at"`
	LastError     *string    `json:"last_error,omitempty" db:"last_error"`
	LastErrorAt   *time.Time `json:"last_error_at,omitempty" db:"last_error_at"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
	// UpdatedBy is the user who last saved the configuration; nil for the
	// boot-time config.yaml import, or once that user has been deleted.
	UpdatedBy *string `json:"updated_by,omitempty" db:"updated_by"`
	Version   int64   `json:"version" db:"version"`
}

// HasCredential reports whether a credential is stored, without disclosing it.
func (p *InstanceEmailProvider) HasCredential() bool {
	return p.SecretEncrypted != nil && *p.SecretEncrypted != ""
}

// IsHealthy reports whether the last observed send succeeded, by the same rule
// as TeamEmailProvider.IsHealthy: a recovered provider keeps its last error for
// diagnosis, and a provider that has never sent is healthy.
func (p *InstanceEmailProvider) IsHealthy() bool {
	return emailDeliveryHealthy(p.LastSuccessAt, p.LastErrorAt)
}

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

// UpsertInstanceEmailProviderRequest is the payload for configuring the
// instance's email provider (#1188). It is the team request plus the two
// instance-only fields, so both paths share one validation of the provider
// settings, the sender identity and the secret.
type UpsertInstanceEmailProviderRequest struct {
	UpsertTeamEmailProviderRequest
	// ContactRecipientAddress is where the contact form delivers; empty falls
	// back to the from-address.
	ContactRecipientAddress *string `json:"contact_recipient_address,omitempty" validate:"omitempty,email"`
	// PrivacyPolicyURL is the absolute http(s) URL linked from outbound mail.
	PrivacyPolicyURL *string `json:"privacy_policy_url,omitempty"`
}

// TestInstanceEmailProviderRequest is the payload for an instance test send.
//
// A nil Config tests the STORED configuration. A Config that omits its secret
// reuses the stored secret, but only when the stored provider type matches, so
// a credential is never sent to a provider it was not issued for.
//
// Unlike the team test, the recipient may be overridden: the caller is the
// operator, not a tenant, so the relay concern that pins a team test to the
// acting user's own mailbox does not apply. It defaults to the acting admin.
type TestInstanceEmailProviderRequest struct {
	Config    *UpsertInstanceEmailProviderRequest `json:"config,omitempty"`
	Recipient *string                             `json:"recipient,omitempty"`
}

// InstanceEmailProviderEffective is the read view of the instance's email
// configuration. It has no secret field at all, so a response built from it
// cannot leak one: only HasCredential says whether a credential is stored.
type InstanceEmailProviderEffective struct {
	// Configured is false when no instance row exists; every other field is
	// then zero, and instance mail is discarded by the stub provider.
	Configured   bool    `json:"configured"`
	ProviderType *string `json:"provider_type"`
	// Settings is the per-type union, shaped exactly like the request body.
	Settings                *TeamEmailProviderSettings `json:"settings,omitempty"`
	HasCredential           bool                       `json:"has_credential"`
	FromAddress             string                     `json:"from_address"`
	FromName                *string                    `json:"from_name,omitempty"`
	ReplyTo                 *string                    `json:"reply_to,omitempty"`
	ContactRecipientAddress *string                    `json:"contact_recipient_address,omitempty"`
	PrivacyPolicyURL        *string                    `json:"privacy_policy_url,omitempty"`
	IsHealthy               *bool                      `json:"is_healthy,omitempty"`
	LastSuccessAt           *time.Time                 `json:"last_success_at,omitempty"`
	LastError               *string                    `json:"last_error,omitempty"`
	LastErrorAt             *time.Time                 `json:"last_error_at,omitempty"`
	UpdatedAt               *time.Time                 `json:"updated_at,omitempty"`
	UpdatedBy               *string                    `json:"updated_by,omitempty"`
}

// NewInstanceEmailProviderEffective builds the read view of row. A nil row is
// the unconfigured instance. settings is the row's stored block lifted into the
// per-type union; pass nil when it could not be decoded, so the rest of the
// configuration stays readable by the admin who has to repair it.
func NewInstanceEmailProviderEffective(
	row *InstanceEmailProvider, settings *TeamEmailProviderSettings,
) *InstanceEmailProviderEffective {
	if row == nil {
		return &InstanceEmailProviderEffective{Configured: false}
	}

	providerType := row.ProviderType
	healthy := row.IsHealthy()
	updatedAt := row.UpdatedAt

	return &InstanceEmailProviderEffective{
		Configured:              true,
		ProviderType:            &providerType,
		Settings:                settings,
		HasCredential:           row.HasCredential(),
		FromAddress:             row.FromAddress,
		FromName:                row.FromName,
		ReplyTo:                 row.ReplyTo,
		ContactRecipientAddress: row.ContactRecipientAddress,
		PrivacyPolicyURL:        row.PrivacyPolicyURL,
		IsHealthy:               &healthy,
		LastSuccessAt:           row.LastSuccessAt,
		LastError:               row.LastError,
		LastErrorAt:             row.LastErrorAt,
		UpdatedAt:               &updatedAt,
		UpdatedBy:               row.UpdatedBy,
	}
}

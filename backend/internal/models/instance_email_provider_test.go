package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceEmailProvider_HasCredential(t *testing.T) {
	empty := ""
	secret := "ciphertext"

	assert.False(t, (&InstanceEmailProvider{}).HasCredential(), "nil secret")
	assert.False(t, (&InstanceEmailProvider{SecretEncrypted: &empty}).HasCredential(), "empty secret")
	assert.True(t, (&InstanceEmailProvider{SecretEncrypted: &secret}).HasCredential())
}

func TestInstanceEmailProvider_IsHealthy(t *testing.T) {
	earlier := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Hour)

	tests := []struct {
		name    string
		success *time.Time
		failure *time.Time
		want    bool
	}{
		{"never sent", nil, nil, true},
		{"only successes", &later, nil, true},
		{"only failures", nil, &later, false},
		{"recovered after a failure", &later, &earlier, true},
		{"failed after a success", &earlier, &later, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &InstanceEmailProvider{LastSuccessAt: tt.success, LastErrorAt: tt.failure}
			assert.Equal(t, tt.want, p.IsHealthy())
		})
	}
}

func TestInstanceEmailProvider_JSONOmitsSecret(t *testing.T) {
	secret := "base64-ciphertext-must-not-leak"
	p := &InstanceEmailProvider{
		ProviderType:    "smtp",
		Settings:        json.RawMessage(`{"host":"smtp.example.com"}`),
		SecretEncrypted: &secret,
		FromAddress:     "noreply@example.com",
	}

	out, err := json.Marshal(p)
	require.NoError(t, err)

	assert.NotContains(t, string(out), secret)
	assert.NotContains(t, string(out), "secret")
	assert.Contains(t, string(out), `"from_address":"noreply@example.com"`)
}

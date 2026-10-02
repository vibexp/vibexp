package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceAuthSetup_HasLiveToken(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	future, past := now.Add(time.Minute), now.Add(-time.Minute)
	hash := make([]byte, 32)

	tests := []struct {
		name string
		row  InstanceAuthSetup
		want bool
	}{
		{"stored, unconsumed, unexpired", InstanceAuthSetup{TokenHash: hash, ExpiresAt: &future}, true},
		{"no token", InstanceAuthSetup{ExpiresAt: &future}, false},
		{"no expiry", InstanceAuthSetup{TokenHash: hash}, false},
		{"expired", InstanceAuthSetup{TokenHash: hash, ExpiresAt: &past}, false},
		{"expires exactly now", InstanceAuthSetup{TokenHash: hash, ExpiresAt: &now}, false},
		{"consumed", InstanceAuthSetup{TokenHash: hash, ExpiresAt: &future, ConsumedAt: &past}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.row.HasLiveToken(now))
		})
	}
}

func TestInstanceAuthSetup_JSONNeverCarriesTheTokenHash(t *testing.T) {
	expires := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	encoded, err := json.Marshal(InstanceAuthSetup{TokenHash: []byte("0123456789abcdef0123456789abcdef"), ExpiresAt: &expires})
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(encoded, &doc))
	assert.NotContains(t, doc, "token_hash")
	assert.NotContains(t, doc, "TokenHash")
	assert.NotContains(t, string(encoded), "0123456789abcdef")
	assert.Contains(t, doc, "generation")
}

package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAccountTokenLifetime(t *testing.T) {
	for _, tc := range []struct {
		purpose, delivery string
		want              time.Duration
	}{
		{AccountTokenPurposeSetup, AccountTokenDeliveryEmail, 7 * 24 * time.Hour},
		{AccountTokenPurposeSetup, AccountTokenDeliveryLink, 7 * 24 * time.Hour},
		{AccountTokenPurposeReset, AccountTokenDeliveryEmail, time.Hour},
		{AccountTokenPurposeReset, AccountTokenDeliveryLink, 24 * time.Hour},
	} {
		assert.Equal(t, tc.want, AccountTokenLifetime(tc.purpose, tc.delivery), "%s by %s", tc.purpose, tc.delivery)
	}
}

func TestAccountTokenValidValues(t *testing.T) {
	assert.True(t, ValidAccountTokenPurpose(AccountTokenPurposeSetup))
	assert.True(t, ValidAccountTokenPurpose(AccountTokenPurposeReset))
	assert.False(t, ValidAccountTokenPurpose(""))
	assert.False(t, ValidAccountTokenPurpose("Setup"))
	assert.True(t, ValidAccountTokenDelivery(AccountTokenDeliveryEmail))
	assert.True(t, ValidAccountTokenDelivery(AccountTokenDeliveryLink))
	assert.False(t, ValidAccountTokenDelivery("sms"))
}

func TestAccountToken_IsLiveAt(t *testing.T) {
	expires := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	token := &AccountToken{ExpiresAt: expires}

	assert.True(t, token.IsLiveAt(expires.Add(-time.Nanosecond)))
	assert.False(t, token.IsLiveAt(expires), "dead at the instant it expires")
	assert.False(t, token.IsLiveAt(expires.Add(time.Second)))

	consumedAt := expires.Add(-time.Hour)
	token.ConsumedAt = &consumedAt
	assert.False(t, token.IsLiveAt(expires.Add(-time.Minute)), "a consumed link is dead before it expires")
}

func TestAccountToken_ProvesMailbox(t *testing.T) {
	assert.True(t, (&AccountToken{Delivery: AccountTokenDeliveryEmail}).ProvesMailbox())
	assert.False(t, (&AccountToken{Delivery: AccountTokenDeliveryLink}).ProvesMailbox())
}

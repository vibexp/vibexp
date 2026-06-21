package server

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"

	"github.com/vibexp/vibexp/internal/config"
)

// TestVerifyServiceAccount locks in the hardened Pub/Sub OIDC check: when a
// PUBSUB_PUSH_SERVICE_ACCOUNT_SUFFIX is configured, only that IAM
// service-account domain is accepted. The previously-accepted broad
// "@accounts.google.com" suffix must be rejected.
func TestVerifyServiceAccount(t *testing.T) {
	srv := testServerWithConfig(&config.Config{
		PubSubPushServiceAccountSuffix: "@example-project.iam.gserviceaccount.com",
	})
	r := httptest.NewRequest("POST", "/api/v1/events/pubsub", nil)

	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{
			name:    "project service account accepted",
			email:   "pubsub@example-project.iam.gserviceaccount.com",
			wantErr: false,
		},
		{
			name:    "broad accounts.google.com rejected",
			email:   "attacker@accounts.google.com",
			wantErr: true,
		},
		{
			name:    "unrelated google service account rejected",
			email:   "someone@other-project.iam.gserviceaccount.com",
			wantErr: true,
		},
		{
			name:    "arbitrary email rejected",
			email:   "attacker@example.com",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := &idtoken.Payload{
				Claims: map[string]interface{}{"email": tc.email},
			}

			email, err := srv.verifyServiceAccount(r, payload)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Empty(t, email)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.email, email)
		})
	}
}

// TestVerifyServiceAccount_EmptySuffixSkipsDomainCheck documents that when no
// PUBSUB_PUSH_SERVICE_ACCOUNT_SUFFIX is configured, the service-account-domain
// allowlist is skipped (a present email claim is accepted). Issuer, signature
// and audience are still enforced upstream in validateOIDCToken.
func TestVerifyServiceAccount_EmptySuffixSkipsDomainCheck(t *testing.T) {
	srv := testServer() // zero-value config: empty suffix
	r := httptest.NewRequest("POST", "/api/v1/events/pubsub", nil)

	payload := &idtoken.Payload{
		Claims: map[string]interface{}{"email": "anything@example.com"},
	}

	email, err := srv.verifyServiceAccount(r, payload)

	require.NoError(t, err)
	assert.Equal(t, "anything@example.com", email)
}

// TestVerifyServiceAccount_MissingEmailClaim rejects a token with no email claim.
func TestVerifyServiceAccount_MissingEmailClaim(t *testing.T) {
	srv := testServer()
	r := httptest.NewRequest("POST", "/api/v1/events/pubsub", nil)

	payload := &idtoken.Payload{Claims: map[string]interface{}{}}

	email, err := srv.verifyServiceAccount(r, payload)

	assert.Error(t, err)
	assert.Empty(t, email)
}

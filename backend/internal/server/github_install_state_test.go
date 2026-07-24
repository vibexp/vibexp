package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

const githubStateTestSecret = "webhook-secret-for-state-tests"

// newStateTestServer builds a Server carrying a non-empty GitHub webhook secret,
// which is what the install-state MAC key is derived from.
func newStateTestServer() *Server {
	return &Server{
		logger: slog.New(slog.DiscardHandler),
		config: &config.Config{
			GitHub: config.GitHubConfig{WebhookSecret: githubStateTestSecret},
		},
	}
}

// TestGitHubState_RoundTrip covers the two shapes the flow mints: unbound
// (install-url time, installation id unknown) and bound to an installation.
func TestGitHubState_RoundTrip(t *testing.T) {
	srv := newStateTestServer()

	for _, installationID := range []int64{0, 4242} {
		state := srv.signGitHubState(githubTestTeamID, installationID)

		gotTeamID, gotInstallationID, ok := srv.verifyGitHubState(state)

		assert.True(t, ok, "freshly signed state should verify")
		assert.Equal(t, githubTestTeamID, gotTeamID)
		assert.Equal(t, installationID, gotInstallationID)
	}
}

// TestGitHubState_Tampered verifies that every field is covered by the signature.
func TestGitHubState_Tampered(t *testing.T) {
	srv := newStateTestServer()
	state := srv.signGitHubState(githubTestTeamID, 4242)
	parts := strings.Split(state, ":")
	require.Len(t, parts, 4, "state layout is teamID:installationID:timestamp:signature")

	tests := []struct {
		name  string
		state string
	}{
		{"swapped team", fmt.Sprintf("other-team:%s:%s:%s", parts[1], parts[2], parts[3])},
		{"swapped installation", fmt.Sprintf("%s:9999:%s:%s", parts[0], parts[2], parts[3])},
		{"moved timestamp", fmt.Sprintf("%s:%s:%d:%s", parts[0], parts[1], time.Now().Unix()+30, parts[3])},
		{"forged signature", fmt.Sprintf("%s:%s:%s:not-a-signature", parts[0], parts[1], parts[2])},
		{"legacy three-part state", fmt.Sprintf("%s:%s:%s", parts[0], parts[2], parts[3])},
		{"non-numeric installation", fmt.Sprintf("%s:abc:%s:%s", parts[0], parts[2], parts[3])},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ok := srv.verifyGitHubState(tt.state)
			assert.False(t, ok, "tampered state must not verify")
		})
	}
}

// TestGitHubState_Expired verifies the TTL is enforced.
func TestGitHubState_Expired(t *testing.T) {
	srv := newStateTestServer()

	stale := time.Now().Add(-githubStateTTL - time.Minute).Unix()
	message := fmt.Sprintf("%s:%d:%d", githubTestTeamID, int64(0), stale)
	mac := hmac.New(sha256.New, srv.githubStateMACKey())
	mac.Write([]byte(message))
	state := fmt.Sprintf("%s:%d:%d:%s",
		githubTestTeamID, 0, stale, base64.URLEncoding.EncodeToString(mac.Sum(nil)))

	_, _, ok := srv.verifyGitHubState(state)

	assert.False(t, ok, "a state older than the TTL must not verify")
}

// TestGitHubStateMACKey_NotTheWebhookSecret pins the #463 requirement that the
// state signer no longer uses the webhook secret as its HMAC key. The key is
// derived from it with a domain separator instead, so compromising one signing
// purpose does not hand over the other.
func TestGitHubStateMACKey_NotTheWebhookSecret(t *testing.T) {
	srv := newStateTestServer()

	key := srv.githubStateMACKey()

	assert.NotEqual(t, []byte(githubStateTestSecret), key,
		"install-state MAC key must not be the raw webhook secret")

	// A state signed with the raw webhook secret (the pre-#463 scheme) must be
	// rejected, so an attacker holding only that secret cannot mint one.
	message := fmt.Sprintf("%s:%d:%d", githubTestTeamID, int64(0), time.Now().Unix())
	legacy := hmac.New(sha256.New, []byte(githubStateTestSecret))
	legacy.Write([]byte(message))
	legacyState := fmt.Sprintf("%s:%d:%d:%s",
		githubTestTeamID, 0, time.Now().Unix(), base64.URLEncoding.EncodeToString(legacy.Sum(nil)))

	_, _, ok := srv.verifyGitHubState(legacyState)
	assert.False(t, ok, "a state signed with the raw webhook secret must not verify")
}

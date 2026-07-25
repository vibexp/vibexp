package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// githubStateTestKey builds a 32-byte instance encryption key from a readable
// seed — the length config validation demands of a real one.
//
// It is derived rather than written as a literal because a 32-character
// high-entropy constant in a test file is indistinguishable from a real leaked
// key to gitleaks, and silencing the scanner for a fixture blunts it for
// everyone.
func githubStateTestKey(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:16])
}

// githubStateTestEncryptionKey is the instance encryption key the state MAC is
// derived from (#482).
var githubStateTestEncryptionKey = githubStateTestKey("vibexp-install-state-tests")

// githubStateTestLegacySecret is the webhook secret the pre-#482 scheme derived
// the state MAC key from. Kept only to prove states signed that way no longer
// verify.
const githubStateTestLegacySecret = "webhook-secret-for-state-tests"

// githubStateTestAppConfigID is the App config a test state is bound to.
const githubStateTestAppConfigID = "11111111-1111-4111-8111-111111111111"

// newStateTestServer builds a Server carrying a non-empty instance encryption
// key, which is what the install-state MAC key is derived from.
func newStateTestServer() *Server {
	return &Server{
		logger: slog.New(slog.DiscardHandler),
		config: &config.Config{
			Security: config.SecurityConfig{EncryptionKey: githubStateTestEncryptionKey},
		},
	}
}

// TestGitHubState_RoundTrip covers the two shapes the flow mints: unbound
// (install-url time, installation id unknown) and bound to an installation.
func TestGitHubState_RoundTrip(t *testing.T) {
	srv := newStateTestServer()

	for _, installationID := range []int64{0, 4242} {
		state := srv.signGitHubState(githubTestTeamID, githubStateTestAppConfigID, installationID)

		got, ok := srv.verifyGitHubState(state)

		assert.True(t, ok, "freshly signed state should verify")
		assert.Equal(t, githubTestTeamID, got.teamID)
		assert.Equal(t, githubStateTestAppConfigID, got.appConfigID)
		assert.Equal(t, installationID, got.installationID)
	}
}

// TestGitHubState_Tampered verifies that every field is covered by the
// signature, and that malformed shapes are rejected rather than panicking on a
// short index.
func TestGitHubState_Tampered(t *testing.T) {
	srv := newStateTestServer()
	state := srv.signGitHubState(githubTestTeamID, githubStateTestAppConfigID, 4242)
	parts := strings.Split(state, ":")
	require.Len(t, parts, githubStateParts,
		"state layout is teamID:appConfigID:installationID:timestamp:signature")

	tests := []struct {
		name  string
		state string
	}{
		{"swapped team", fmt.Sprintf("other-team:%s:%s:%s:%s", parts[1], parts[2], parts[3], parts[4])},
		{"swapped app config", fmt.Sprintf("%s:other-app-config:%s:%s:%s",
			parts[0], parts[2], parts[3], parts[4])},
		{"swapped installation", fmt.Sprintf("%s:%s:9999:%s:%s", parts[0], parts[1], parts[3], parts[4])},
		{"moved timestamp", fmt.Sprintf("%s:%s:%s:%d:%s",
			parts[0], parts[1], parts[2], time.Now().Unix()+30, parts[4])},
		{"forged signature", fmt.Sprintf("%s:%s:%s:%s:not-a-signature",
			parts[0], parts[1], parts[2], parts[3])},
		{"legacy four-part state", fmt.Sprintf("%s:%s:%s:%s", parts[0], parts[2], parts[3], parts[4])},
		{"legacy three-part state", fmt.Sprintf("%s:%s:%s", parts[0], parts[3], parts[4])},
		{"extra part", state + ":extra"},
		{"non-numeric installation", fmt.Sprintf("%s:%s:abc:%s:%s", parts[0], parts[1], parts[3], parts[4])},
		{"non-numeric timestamp", fmt.Sprintf("%s:%s:%s:abc:%s", parts[0], parts[1], parts[2], parts[4])},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := srv.verifyGitHubState(tt.state)
			assert.False(t, ok, "tampered state must not verify")
		})
	}
}

// TestGitHubState_Expired verifies the TTL is enforced.
func TestGitHubState_Expired(t *testing.T) {
	srv := newStateTestServer()

	stale := time.Now().Add(-githubStateTTL - time.Minute).Unix()
	message := githubStateMessage(githubTestTeamID, githubStateTestAppConfigID, 0, stale)
	mac := hmac.New(sha256.New, srv.githubStateMACKey())
	mac.Write([]byte(message))
	state := message + ":" + base64.URLEncoding.EncodeToString(mac.Sum(nil))

	_, ok := srv.verifyGitHubState(state)

	assert.False(t, ok, "a state older than the TTL must not verify")
}

// TestGitHubStateMACKey_DerivedFromEncryptionKey pins the #482 re-key: the state
// MAC key comes from security.encryption_key, and is derived from it rather
// than being it, so compromising one signing purpose does not hand over the
// other. The #463 property that the raw key is never the MAC key survives the
// change of key material.
func TestGitHubStateMACKey_DerivedFromEncryptionKey(t *testing.T) {
	srv := newStateTestServer()

	key := srv.githubStateMACKey()

	assert.NotEqual(t, []byte(githubStateTestEncryptionKey), key,
		"install-state MAC key must not be the raw encryption key")

	// Deriving the same way must reproduce it — this is what makes the key a
	// function of the encryption key rather than of anything left over.
	expected := hmac.New(sha256.New, []byte(githubStateTestEncryptionKey))
	expected.Write([]byte(githubStateMACDomain))
	assert.Equal(t, expected.Sum(nil), key)

	// Changing the encryption key must change the MAC key, or the "re-key" is
	// not actually keyed on it.
	other := newStateTestServer()
	other.config.Security.EncryptionKey = githubStateTestKey("a different instance")
	assert.NotEqual(t, key, other.githubStateMACKey())
}

// TestGitHubState_OldWebhookSecretKeyRejected is the migration guarantee: a
// state signed under the pre-#482 scheme — key derived from the instance
// webhook secret, four-part layout — must not verify. Both the key material and
// the domain separator changed, so this holds twice over.
func TestGitHubState_OldWebhookSecretKeyRejected(t *testing.T) {
	srv := newStateTestServer()

	now := time.Now().Unix()

	legacyKey := hmac.New(sha256.New, []byte(githubStateTestLegacySecret))
	legacyKey.Write([]byte("vx-github-install-state-mac-v1"))

	// The old four-part message, signed with the old key.
	oldMessage := fmt.Sprintf("%s:%d:%d", githubTestTeamID, 0, now)
	oldMAC := hmac.New(sha256.New, legacyKey.Sum(nil))
	oldMAC.Write([]byte(oldMessage))
	oldState := oldMessage + ":" + base64.URLEncoding.EncodeToString(oldMAC.Sum(nil))

	_, ok := srv.verifyGitHubState(oldState)
	assert.False(t, ok, "a pre-#482 four-part state must not verify")

	// The new five-part message, still signed with the old key: rejected on the
	// signature alone, proving the key material really moved.
	newMessage := githubStateMessage(githubTestTeamID, githubStateTestAppConfigID, 0, now)
	forged := hmac.New(sha256.New, legacyKey.Sum(nil))
	forged.Write([]byte(newMessage))
	forgedState := newMessage + ":" + base64.URLEncoding.EncodeToString(forged.Sum(nil))

	_, ok = srv.verifyGitHubState(forgedState)
	assert.False(t, ok, "a state signed with the old webhook-secret-derived key must not verify")
}

// TestGitHubState_AppConfigBinding verifies a verified state reports the App
// config it was minted for rather than whatever the team holds now — that is
// what the callback's equality check compares against.
func TestGitHubState_AppConfigBinding(t *testing.T) {
	srv := newStateTestServer()

	const replacementAppConfigID = "22222222-2222-4222-8222-222222222222"

	state := srv.signGitHubState(githubTestTeamID, githubStateTestAppConfigID, 0)

	got, ok := srv.verifyGitHubState(state)
	require.True(t, ok)
	assert.Equal(t, githubStateTestAppConfigID, got.appConfigID)
	assert.NotEqual(t, replacementAppConfigID, got.appConfigID)
}

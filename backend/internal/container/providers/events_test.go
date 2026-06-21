package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/config"
)

// TestShouldInitializePubSubForwarder verifies the Pub/Sub forwarder is gated on
// the effective event backend (EVENT_BACKEND / legacy PUBSUB_FORWARDING_ENABLED)
// AND a configured GCP project.
func TestShouldInitializePubSubForwarder(t *testing.T) {
	tests := []struct {
		name             string
		eventBackend     string
		pubSubForwarding bool
		gcpProjectID     string
		want             bool
	}{
		{"sync backend skips forwarder", config.EventBackendSync, false, "my-project", false},
		{"pubsub backend without project skips", config.EventBackendPubSub, false, "", false},
		{"pubsub backend with project initializes", config.EventBackendPubSub, false, "my-project", true},
		{"legacy flag with project initializes", config.EventBackendSync, true, "my-project", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				EventBackend:            tc.eventBackend,
				PubSubForwardingEnabled: tc.pubSubForwarding,
				GCPProjectID:            tc.gcpProjectID,
			}

			assert.Equal(t, tc.want, shouldInitializePubSubForwarder(cfg, testLogger()))
		})
	}
}

// TestShouldInitializeHTTPSyncListener verifies the broker-free sync listener is
// the default path: selected for the sync backend whenever AI_SERVICE_URL is set,
// and skipped when the pubsub backend is active.
func TestShouldInitializeHTTPSyncListener(t *testing.T) {
	tests := []struct {
		name             string
		eventBackend     string
		pubSubForwarding bool
		aiServiceURL     string
		want             bool
	}{
		{"sync backend with ai service initializes", config.EventBackendSync, false, "http://localhost:8000", true},
		{"default (empty) backend initializes", "", false, "http://localhost:8000", true},
		{"sync backend without ai service skips", config.EventBackendSync, false, "", false},
		{"pubsub backend skips sync listener", config.EventBackendPubSub, false, "http://localhost:8000", false},
		{"legacy flag skips sync listener", config.EventBackendSync, true, "http://localhost:8000", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				EventBackend:            tc.eventBackend,
				PubSubForwardingEnabled: tc.pubSubForwarding,
				AIServiceURL:            tc.aiServiceURL,
			}

			assert.Equal(t, tc.want, shouldInitializeHTTPSyncListener(cfg, testLogger()))
		})
	}
}

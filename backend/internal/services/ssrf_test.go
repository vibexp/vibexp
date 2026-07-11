package services

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestAgentCardFetcher builds a fetcher that permits private/loopback hosts so
// tests can target httptest servers (which bind to 127.0.0.1). Production code uses
// NewAgentCardFetcher with the strict default guard.
func newTestAgentCardFetcher() *AgentCardFetcher {
	return newAgentCardFetcher(&ssrfGuard{allowPrivate: true})
}

func TestSSRFGuard_IsBlockedIP(t *testing.T) {
	guard := &ssrfGuard{}

	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"loopback range", "127.0.0.53", true},
		{"private 10", "10.1.2.3", true},
		{"private 172.16", "172.16.0.1", true},
		{"private 192.168", "192.168.1.1", true},
		{"link-local / metadata", "169.254.169.254", true},
		{"unspecified v4", "0.0.0.0", true},
		{"multicast v4", "224.0.0.1", true},
		{"loopback v6", "::1", true},
		{"unique-local v6", "fc00::1", true},
		{"link-local v6", "fe80::1", true},
		{"public v4", "8.8.8.8", false},
		{"public v4 cloudflare", "1.1.1.1", false},
		{"public v6", "2606:4700:4700::1111", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			require.NotNil(t, ip, "test IP must parse")
			assert.Equal(t, tc.blocked, guard.isBlockedIP(ip))
		})
	}

	t.Run("nil ip is blocked", func(t *testing.T) {
		assert.True(t, guard.isBlockedIP(nil))
	})
}

func TestSSRFGuard_AllowPrivate(t *testing.T) {
	guard := &ssrfGuard{allowPrivate: true}
	// When allowPrivate is set (tests only), loopback/private are permitted.
	assert.False(t, guard.isBlockedIP(net.ParseIP("127.0.0.1")))
	assert.False(t, guard.isBlockedIP(net.ParseIP("10.0.0.1")))
	// nil is still blocked.
	assert.True(t, guard.isBlockedIP(nil))
}

func TestSSRFGuard_ValidateOutboundHost(t *testing.T) {
	guard := &ssrfGuard{}

	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{"loopback literal", "http://127.0.0.1/.well-known/agent-card.json", true},
		{"private literal", "https://10.0.0.5/path", true},
		{"metadata IP literal", "http://169.254.169.254/computeMetadata/v1/", true},
		{"metadata hostname", "http://metadata.google.internal/", true},
		{"metadata hostname uppercase", "http://Metadata.Google.Internal/", true},
		{"metadata hostname trailing dot", "http://metadata.google.internal./", true},
		{"ipv6 loopback literal", "http://[::1]/x", true},
		{"public literal", "https://8.8.8.8/", false},
		{"empty host", "http:///path", true},
		{"garbage url", "://not a url", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := guard.validateOutboundHost(context.Background(), tc.rawURL)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSSRFGuard_Control(t *testing.T) {
	guard := &ssrfGuard{}

	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{"loopback", "127.0.0.1:443", true},
		{"private", "10.0.0.1:80", true},
		{"metadata", "169.254.169.254:80", true},
		{"public", "8.8.8.8:443", false},
		{"missing port", "8.8.8.8", true},
		{"non-ip host", "example.com:443", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := guard.control("tcp", tc.address, nil)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFetchAgentCard_RejectsInternalHost(t *testing.T) {
	fetcher := NewAgentCardFetcher()
	defer fetcher.Close()

	_, err := fetcher.FetchAgentCard(
		context.Background(),
		"http://169.254.169.254/.well-known/agent-card.json",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

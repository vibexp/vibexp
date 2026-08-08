package services

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
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

// mustCIDRs parses CIDR literals for guard fixtures. Using net.ParseCIDR here
// (rather than the config accessor) lets these tests inject prefixes the config
// validator would refuse to load, which is how the "even then, still blocked"
// cases below are proved.
func mustCIDRs(t *testing.T, entries ...string) []*net.IPNet {
	t.Helper()
	nets := make([]*net.IPNet, 0, len(entries))
	for _, entry := range entries {
		_, network, err := net.ParseCIDR(entry)
		require.NoError(t, err, "fixture CIDR %q must parse", entry)
		nets = append(nets, network)
	}
	return nets
}

// TestSSRFGuard_AllowedCIDRs pins the #745 policy: an operator-declared prefix
// unblocks that range and nothing else.
func TestSSRFGuard_AllowedCIDRs(t *testing.T) {
	t.Run("declared private range is reachable", func(t *testing.T) {
		guard := &ssrfGuard{allowedCIDRs: mustCIDRs(t, "10.42.0.0/24")}
		assert.False(t, guard.isBlockedIP(net.ParseIP("10.42.0.2")),
			"the embedding sidecar's address must be reachable")
		assert.True(t, guard.isBlockedIP(net.ParseIP("10.99.0.1")),
			"an undeclared private address stays blocked")
		assert.True(t, guard.isBlockedIP(net.ParseIP("127.0.0.1")),
			"declaring a private range must not also unblock loopback")
	})

	t.Run("declared loopback is reachable", func(t *testing.T) {
		guard := &ssrfGuard{allowedCIDRs: mustCIDRs(t, "127.0.0.0/8")}
		assert.False(t, guard.isBlockedIP(net.ParseIP("127.0.0.1")),
			"same-host Ollama/TEI is the most common self-hosted shape")
		assert.True(t, guard.isBlockedIP(net.ParseIP("10.0.0.1")))
	})

	t.Run("ipv6 unique-local and loopback", func(t *testing.T) {
		guard := &ssrfGuard{allowedCIDRs: mustCIDRs(t, "fc00::/7", "::1/128")}
		assert.False(t, guard.isBlockedIP(net.ParseIP("fc00::1")))
		assert.False(t, guard.isBlockedIP(net.ParseIP("::1")))
	})

	t.Run("public addresses are unaffected", func(t *testing.T) {
		guard := &ssrfGuard{allowedCIDRs: mustCIDRs(t, "10.42.0.0/24")}
		assert.False(t, guard.isBlockedIP(net.ParseIP("8.8.8.8")))
		assert.True(t, guard.isBlockedIP(nil))
	})

	// The load-bearing half. The config validator refuses these prefixes at
	// startup, so the guard should never see them — but the block must not
	// DEPEND on that, or a future config path could reopen #464.
	t.Run("link-local and multicast stay blocked even if declared", func(t *testing.T) {
		guard := &ssrfGuard{allowedCIDRs: mustCIDRs(t, "0.0.0.0/0", "::/0")}
		for _, ip := range []string{
			"169.254.169.254", // GCP/AWS metadata
			"169.254.0.1",
			"224.0.0.1",
			"0.0.0.0",
			"fe80::1",
			"ff02::1",
		} {
			assert.True(t, guard.isBlockedIP(net.ParseIP(ip)),
				"%s must never be reachable via the allowlist", ip)
		}
	})

	t.Run("metadata hostnames stay blocked even if declared", func(t *testing.T) {
		guard := &ssrfGuard{allowedCIDRs: mustCIDRs(t, "0.0.0.0/0")}
		err := guard.validateOutboundHost(context.Background(), "http://metadata.google.internal/")
		assert.Error(t, err, "the name blocklist is independent of the IP policy")
	})

	// DNS-rebinding defence: the dial-time hook shares isBlockedIP, so the
	// allowlist has to hold at connect time too — that is where the #745 failure
	// actually surfaced.
	t.Run("control hook honours the allowlist", func(t *testing.T) {
		guard := &ssrfGuard{allowedCIDRs: mustCIDRs(t, "10.42.0.0/24")}
		assert.NoError(t, guard.control("tcp", "10.42.0.2:80", nil))
		assert.Error(t, guard.control("tcp", "10.99.0.1:80", nil))
		assert.Error(t, guard.control("tcp", "169.254.169.254:80", nil))
	})

	t.Run("range rejections name the remediation knob", func(t *testing.T) {
		guard := &ssrfGuard{}
		err := guard.control("tcp", "10.42.0.2:80", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "security.outbound_allowed_cidrs",
			"a dead-end error is what made #745 undiagnosable")

		err = guard.validateOutboundHost(context.Background(), "http://10.42.0.2/v1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "security.outbound_allowed_cidrs")
	})
}

func TestSSRFGuardForConfig(t *testing.T) {
	loopback := net.ParseIP("127.0.0.1")

	t.Run("nil config uses strict production policy", func(t *testing.T) {
		guard := ssrfGuardForConfig(nil)
		assert.Same(t, defaultSSRFGuard, guard)
		assert.True(t, guard.isBlockedIP(loopback))
	})

	t.Run("production config blocks loopback", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Frontend.BaseURL = "https://app.example.com"
		guard := ssrfGuardForConfig(cfg)
		assert.True(t, guard.isBlockedIP(loopback))
	})

	t.Run("empty base url is treated as production (fail-closed)", func(t *testing.T) {
		cfg := &config.Config{}
		guard := ssrfGuardForConfig(cfg)
		assert.True(t, guard.isBlockedIP(loopback))
	})

	t.Run("local development permits loopback for local A2A agents", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Frontend.BaseURL = "http://localhost:5173"
		guard := ssrfGuardForConfig(cfg)
		assert.False(t, guard.isBlockedIP(loopback))
		// Cloud-metadata hostnames stay blocked regardless of allowPrivate.
		err := guard.validateOutboundHost(context.Background(), "http://metadata.google.internal/")
		assert.Error(t, err)
	})

	// #745: the operator's declared ranges must reach the guard every consumer
	// gets — embedding/model providers, A2A, and team email all build theirs
	// here, so wiring it in one place is what keeps the policy single.
	t.Run("production config honours declared outbound CIDRs", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Frontend.BaseURL = "https://app.example.com"
		cfg.Security.OutboundAllowedCIDRs = config.EnvStringSlice{"10.42.0.0/24"}

		guard := ssrfGuardForConfig(cfg)
		assert.False(t, guard.isBlockedIP(net.ParseIP("10.42.0.2")))
		assert.True(t, guard.isBlockedIP(loopback), "only the declared range opens")
		assert.True(t, guard.isBlockedIP(net.ParseIP("169.254.169.254")))
	})

	t.Run("production config with no declared CIDRs keeps the strict singleton", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Frontend.BaseURL = "https://app.example.com"
		assert.Same(t, defaultSSRFGuard, ssrfGuardForConfig(cfg),
			"the default posture must be byte-identical to pre-#745")
	})
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
	fetcher := NewAgentCardFetcher(nil)
	defer fetcher.Close()

	_, err := fetcher.FetchAgentCard(
		context.Background(),
		"http://169.254.169.254/.well-known/agent-card.json",
		nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

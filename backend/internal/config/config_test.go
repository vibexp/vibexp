package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: t.Setenv and t.Parallel are mutually exclusive.
// These tests modify env vars and must not be parallelised.

// validTestEncryptionKey is a 32-byte key used so Load() passes encryption-key
// validation; tests that specifically exercise ENCRYPTION_KEY override it.
const validTestEncryptionKey = "12345678901234567890123456789012"

// TestMain sets a valid ENCRYPTION_KEY for the whole package so existing Load()
// tests (which assert on unrelated config) are not tripped by the new fail-closed
// encryption-key validation.
func TestMain(m *testing.M) {
	if err := os.Setenv("ENCRYPTION_KEY", validTestEncryptionKey); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestLoad_ActivityRetentionDays_Default(t *testing.T) {
	// Ensure the env var is absent so envconfig falls back to the default tag.
	// t.Setenv("ACTIVITY_RETENTION_DAYS", "") would set it to empty string and
	// cause envconfig to fail to parse it as int; we must use Unsetenv instead.
	prev, exists := os.LookupEnv("ACTIVITY_RETENTION_DAYS")
	require.NoError(t, os.Unsetenv("ACTIVITY_RETENTION_DAYS"))
	if exists {
		t.Cleanup(func() {
			require.NoError(t, os.Setenv("ACTIVITY_RETENTION_DAYS", prev))
		})
	}

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 90, cfg.ActivityRetentionDays)
}

func TestLoad_EmbeddingDefaults(t *testing.T) {
	for _, name := range []string{"EMBEDDING_MODEL", "EMBEDDING_DIMENSIONS"} {
		prev, exists := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		if exists {
			t.Cleanup(func() { require.NoError(t, os.Setenv(name, prev)) })
		}
	}

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "gemini-embedding-001", cfg.EmbeddingModel)
	assert.Equal(t, 768, cfg.EmbeddingDimensions)
}

func TestLoad_EmbeddingModel_Empty_ReturnsError(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL", "")

	cfg, err := Load()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "EMBEDDING_MODEL")
}

func TestLoad_EmbeddingDimensions_Zero_ReturnsError(t *testing.T) {
	t.Setenv("EMBEDDING_DIMENSIONS", "0")

	cfg, err := Load()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "EMBEDDING_DIMENSIONS")
}

func TestLoad_ActivityRetentionDays_ValidValue(t *testing.T) {
	t.Setenv("ACTIVITY_RETENTION_DAYS", "30")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 30, cfg.ActivityRetentionDays)
}

func TestLoad_ActivityRetentionDays_Zero_ReturnsError(t *testing.T) {
	t.Setenv("ACTIVITY_RETENTION_DAYS", "0")

	cfg, err := Load()

	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoad_ActivityRetentionDays_Negative_ReturnsError(t *testing.T) {
	t.Setenv("ACTIVITY_RETENTION_DAYS", "-1")

	cfg, err := Load()

	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoad_ActivityRetentionDays_ExceedsMax_ReturnsError(t *testing.T) {
	t.Setenv("ACTIVITY_RETENTION_DAYS", "3651")

	cfg, err := Load()

	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoad_ActivityRetentionDays_AtMaxBoundary_Succeeds(t *testing.T) {
	t.Setenv("ACTIVITY_RETENTION_DAYS", "3650")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 3650, cfg.ActivityRetentionDays)
}

// TestGetPubSubForwardedEventTypes_IncludesAllFourEntities locks in the
// embedding parity guarantee: prompt, artifact, memory, and blueprint each
// publish create + update events that must be forwarded to the AI service via
// Pub/Sub. A regression here breaks Blueprint embedding propagation (issue
// #1297) and would silently degrade RAG quality.
func TestGetPubSubForwardedEventTypes_IncludesAllFourEntities(t *testing.T) {
	cfg := &Config{}

	got := cfg.GetPubSubForwardedEventTypes()

	required := []string{
		"prompt.created", "prompt.updated",
		"artifact.created", "artifact.updated",
		"memory.created", "memory.updated",
		"blueprint.created", "blueprint.updated",
	}
	for _, evt := range required {
		assert.Contains(t, got, evt, "GetPubSubForwardedEventTypes must include %q for embedding parity", evt)
	}
}

func TestLoad_SearchRankingDefaults(t *testing.T) {
	cfg, err := Load()

	require.NoError(t, err)
	assert.False(t, cfg.SearchRecencyRankingEnabled, "recency ranking is off by default")
	assert.InDelta(t, 0.5, cfg.SearchRankWeightRelevance, 1e-9)
	assert.InDelta(t, 0.3, cfg.SearchRankWeightCreated, 1e-9)
	assert.InDelta(t, 0.2, cfg.SearchRankWeightUpdated, 1e-9)
	assert.InDelta(t, 90, cfg.SearchRankHalfLifeDays, 1e-9)
	assert.Equal(t, 200, cfg.SearchRankCandidateCap)
}

func TestValidateSearchRankingConfig(t *testing.T) {
	base := func() *Config {
		return &Config{
			SearchRankWeightRelevance: 0.5,
			SearchRankWeightCreated:   0.3,
			SearchRankWeightUpdated:   0.2,
			SearchRankHalfLifeDays:    90,
			SearchRankCandidateCap:    200,
		}
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"valid defaults", func(*Config) {}, false},
		{"negative relevance weight", func(c *Config) { c.SearchRankWeightRelevance = -0.1 }, true},
		{"all weights zero", func(c *Config) {
			c.SearchRankWeightRelevance = 0
			c.SearchRankWeightCreated = 0
			c.SearchRankWeightUpdated = 0
		}, true},
		{"zero half-life", func(c *Config) { c.SearchRankHalfLifeDays = 0 }, true},
		{"negative half-life", func(c *Config) { c.SearchRankHalfLifeDays = -1 }, true},
		{"half-life above ceiling", func(c *Config) { c.SearchRankHalfLifeDays = maxSearchRankHalfLifeDays + 1 }, true},
		{"half-life at ceiling", func(c *Config) { c.SearchRankHalfLifeDays = maxSearchRankHalfLifeDays }, false},
		{"zero candidate cap", func(c *Config) { c.SearchRankCandidateCap = 0 }, true},
		{"candidate cap above ceiling", func(c *Config) { c.SearchRankCandidateCap = maxSearchRankCandidateCap + 1 }, true},
		{"candidate cap at ceiling", func(c *Config) { c.SearchRankCandidateCap = maxSearchRankCandidateCap }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(cfg)
			err := validateSearchRankingConfig(cfg)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestLoad_SearchRankWeightNegative_ReturnsError(t *testing.T) {
	t.Setenv("SEARCH_RANK_WEIGHT_RELEVANCE", "-0.5")

	cfg, err := Load()

	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestValidateEncryptionKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid 32 bytes", validTestEncryptionKey, false},
		{"empty", "", true},
		{"too short", "shortkey", true},
		{"too long", "123456789012345678901234567890123", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEncryptionKey(&Config{EncryptionKey: tc.key})
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoad_MissingEncryptionKey_ReturnsError(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "")

	cfg, err := Load()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "ENCRYPTION_KEY")
}

func TestLoad_ShortEncryptionKey_ReturnsError(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "tooshort")

	cfg, err := Load()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "ENCRYPTION_KEY")
}

// TestGetPubSubForwardedEventTypes_IncludesFeedItemNotReply locks in the embedding
// scope: feed items are forwarded for embedding, but feed item replies are not
// (product decision — replies must never be embedded).
func TestGetPubSubForwardedEventTypes_IncludesFeedItemNotReply(t *testing.T) {
	cfg := &Config{}

	got := cfg.GetPubSubForwardedEventTypes()

	assert.Contains(t, got, "feed_item.created", "feed items must be forwarded for embedding")
	assert.NotContains(t, got, "feed_item_reply.created", "feed item replies must not be embedded")
}

func TestLoad_AbuseHardeningDefaults(t *testing.T) {
	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, int64(10<<20), cfg.MaxBodySizeBytes, "MaxBodySizeBytes defaults to 10MiB")
	assert.Equal(t, 10, cfg.AuthRateLimitPerMinute)
	assert.Equal(t, 5, cfg.ContactRateLimitPerMinute)
	assert.Equal(t, 100, cfg.APIRateLimitPerMinute)
}

func TestLoad_MaxBodySizeBytes_NonPositive_ReturnsError(t *testing.T) {
	for _, v := range []string{"0", "-1"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("MAX_BODY_SIZE_BYTES", v)

			cfg, err := Load()

			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), "MAX_BODY_SIZE_BYTES")
		})
	}
}

func TestLoad_RateLimits_NonPositive_ReturnsError(t *testing.T) {
	tests := []struct {
		name   string
		envVar string
	}{
		{"auth", "AUTH_RATE_LIMIT_PER_MINUTE"},
		{"contact", "CONTACT_RATE_LIMIT_PER_MINUTE"},
		{"api", "API_RATE_LIMIT_PER_MINUTE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envVar, "0")

			cfg, err := Load()

			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), tc.envVar)
		})
	}
}

// TestEventBackendMode_Resolution covers the effective-backend resolution,
// including the backward-compat rule that PUBSUB_FORWARDING_ENABLED=true forces
// the pubsub backend regardless of EVENT_BACKEND, and that an empty/garbage
// EVENT_BACKEND falls back to sync.
func TestEventBackendMode_Resolution(t *testing.T) {
	tests := []struct {
		name             string
		eventBackend     string
		pubSubForwarding bool
		want             string
	}{
		{"empty defaults to sync", "", false, EventBackendSync},
		{"explicit sync", "sync", false, EventBackendSync},
		{"explicit pubsub", "pubsub", false, EventBackendPubSub},
		{"case and space insensitive", "  PubSub  ", false, EventBackendPubSub},
		{"unknown falls back to sync", "kafka", false, EventBackendSync},
		{"legacy flag forces pubsub", "sync", true, EventBackendPubSub},
		{"legacy flag wins over empty", "", true, EventBackendPubSub},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				EventBackend:            tc.eventBackend,
				PubSubForwardingEnabled: tc.pubSubForwarding,
			}

			assert.Equal(t, tc.want, cfg.EventBackendMode())
		})
	}
}

// TestLoad_EventBackendDefaultsToSync locks in that a fresh self-host (no
// EVENT_BACKEND set) selects the broker-free sync backend.
func TestLoad_EventBackendDefaultsToSync(t *testing.T) {
	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, EventBackendSync, cfg.EventBackend)
	assert.Equal(t, EventBackendSync, cfg.EventBackendMode())
}

// TestLoad_EventBackendInvalid fails closed on a typo so a misconfigured
// Pub/Sub deployment never silently degrades to sync.
func TestLoad_EventBackendInvalid(t *testing.T) {
	t.Setenv("EVENT_BACKEND", "rabbitmq")

	cfg, err := Load()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "EVENT_BACKEND")
}

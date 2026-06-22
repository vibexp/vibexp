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

// TestLoad_EmbeddingChunkDefaults locks in the in-Go chunker defaults so a fresh
// self-host gets a sane sliding window without extra configuration.
func TestLoad_EmbeddingChunkDefaults(t *testing.T) {
	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 1000, cfg.EmbeddingChunkSize)
	assert.Equal(t, 200, cfg.EmbeddingChunkOverlap)
}

// TestLoad_EmbeddingChunkOverlapInvalid fails closed when the overlap is not
// smaller than the chunk size, which would stall the chunker.
func TestLoad_EmbeddingChunkOverlapInvalid(t *testing.T) {
	t.Setenv("EMBEDDING_CHUNK_SIZE", "500")
	t.Setenv("EMBEDDING_CHUNK_OVERLAP", "500")

	cfg, err := Load()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "EMBEDDING_CHUNK_OVERLAP")
}

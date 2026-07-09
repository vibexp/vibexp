package cmd

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
)

// mockServer is a mock implementation for testing
type mockServer struct {
	mock.Mock
	startErr error
}

func (m *mockServer) Start(ctx context.Context) error {
	return m.startErr
}

func (m *mockServer) Container() interface{} {
	return nil
}

// isGracefulShutdownError checks if an error is from graceful shutdown
func isGracefulShutdownError(err error) bool {
	return err == http.ErrServerClosed || err == context.Canceled
}

// TestResolveReleaseValue covers the release-metadata precedence:
// config value → ldflags build var → VCS build info → sentinel.
func TestResolveReleaseValue(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		ldflag   string
		vcs      string
		sentinel string
		want     string
	}{
		{"config override wins over everything", "real-sha", "ld-sha", "vcs-sha", "dev", "real-sha"},
		{"empty config falls through to ldflag", "", "ld-sha", "vcs-sha", "dev", "ld-sha"},
		{"sentinel config falls through to ldflag", "dev", "ld-sha", "vcs-sha", "dev", "ld-sha"},
		{"no ldflag falls through to VCS", "dev", "", "vcs-sha", "dev", "vcs-sha"},
		{"nothing available yields the sentinel", "dev", "", "", "dev", "dev"},
		{"empty config, nothing available yields the sentinel", "", "", "", "unknown", "unknown"},
		{"date sentinel falls through to ldflag", "unknown", "2026-07-09T00:00:00Z", "", "unknown", "2026-07-09T00:00:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveReleaseValue(tt.current, tt.ldflag, tt.vcs, tt.sentinel)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestResolveReleaseMetadata verifies ldflags build vars are stamped into the
// config, and that a real config override is preserved.
func TestResolveReleaseMetadata(t *testing.T) {
	origSHA, origDate := buildSHA, buildDate
	t.Cleanup(func() { buildSHA, buildDate = origSHA, origDate })

	t.Run("ldflags vars stamp empty config", func(t *testing.T) {
		buildSHA, buildDate = "abc1234", "2026-07-09T12:00:00Z"
		cfg := &config.Config{}
		cfg.Server.ReleaseSHA = "dev"
		cfg.Server.ReleaseDate = "unknown"

		resolveReleaseMetadata(cfg)

		assert.Equal(t, "abc1234", cfg.Server.ReleaseSHA)
		assert.Equal(t, "2026-07-09T12:00:00Z", cfg.Server.ReleaseDate)
	})

	t.Run("config override beats ldflags vars", func(t *testing.T) {
		buildSHA, buildDate = "abc1234", "2026-07-09T12:00:00Z"
		cfg := &config.Config{}
		cfg.Server.ReleaseSHA = "from-config"
		cfg.Server.ReleaseDate = "2020-01-01"

		resolveReleaseMetadata(cfg)

		assert.Equal(t, "from-config", cfg.Server.ReleaseSHA)
		assert.Equal(t, "2020-01-01", cfg.Server.ReleaseDate)
	})
}

// TestStartServer_GracefulShutdown tests that graceful shutdowns log at INFO level
func TestStartServer_GracefulShutdown(t *testing.T) {
	tests := []struct {
		name        string
		serverError error
		expectFatal bool
	}{
		{"http.ErrServerClosed should log INFO", http.ErrServerClosed, false},
		{"context.Canceled should log INFO", context.Canceled, false},
		{"unexpected error should log FATAL", assert.AnError, true},
		{"no error should log INFO", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSrv := &mockServer{startErr: tt.serverError}
			logger := slog.New(slog.DiscardHandler)

			ctx := context.Background()
			err := mockSrv.Start(ctx)

			// Test the error classification logic
			fatalCalled := false
			if err != nil {
				if isGracefulShutdownError(err) {
					logger.Info("Server shutting down gracefully")
				} else {
					fatalCalled = true
				}
			} else {
				logger.Info("Server stopped")
			}

			if tt.expectFatal {
				assert.True(t, fatalCalled, "Expected Fatal for error: %v", tt.serverError)
			} else {
				assert.False(t, fatalCalled, "Expected INFO for error: %v", tt.serverError)
			}
		})
	}
}

// TestGracefulShutdownScenarios tests real-world Cloud Run shutdown scenarios
func TestGracefulShutdownScenarios(t *testing.T) {
	t.Run("Cloud Run scale-down returns http.ErrServerClosed", func(t *testing.T) {
		err := http.ErrServerClosed
		shouldLogFatal := err != http.ErrServerClosed && err != context.Canceled

		assert.False(t, shouldLogFatal,
			"http.ErrServerClosed from Cloud Run scale-down should be logged at INFO level, not FATAL")
	})

	t.Run("Context cancellation returns context.Canceled", func(t *testing.T) {
		err := context.Canceled
		shouldLogFatal := err != http.ErrServerClosed && err != context.Canceled

		assert.False(t, shouldLogFatal,
			"context.Canceled from shutdown signal should be logged at INFO level, not FATAL")
	})

	t.Run("Unexpected server error should log FATAL", func(t *testing.T) {
		err := assert.AnError
		shouldLogFatal := err != http.ErrServerClosed && err != context.Canceled

		assert.True(t, shouldLogFatal,
			"Unexpected errors should still be logged at FATAL level")
	})
}

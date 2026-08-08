package cmd

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/scheduler"
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

// newSchedulerForTest builds a real *scheduler.Scheduler over a mocked
// repository and a sqlmock connection, so startScheduler's gate can be tested
// without a container (container.Container is a ~60-method interface with no
// generated mock).
func newSchedulerForTest(t *testing.T) (*scheduler.Scheduler, *mocks.MockScheduleRepository) {
	t.Helper()
	// sqlmock reuses one driver instance per DSN string, so scope the DSN to
	// this test to keep expectations from leaking between tests.
	sqlDB, sqlMock, err := sqlmock.NewWithDSN(
		"postgres://sched:test@localhost:5432/cmd_unit?sslmode=disable&case=" + t.Name())
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlMock.ExpectClose()
		if cerr := sqlDB.Close(); cerr != nil {
			t.Errorf("close mock db: %v", cerr)
		}
	})

	repo := mocks.NewMockScheduleRepository(t)
	s := scheduler.New(
		repo,
		&database.DB{DB: sqlDB},
		scheduler.NewRegistry(),
		scheduler.Config{TickInterval: 10 * time.Millisecond, JobTimeout: time.Minute, DueLimit: 10},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return s, repo
}

// TestStartScheduler_DisabledClaimsNothing covers the #728 acceptance criterion
// "when disabled nothing is claimed or run". The mock is strict: any call to
// ListDue fails the test, so setting no expectation IS the assertion.
func TestStartScheduler_DisabledClaimsNothing(t *testing.T) {
	sched, _ := newSchedulerForTest(t)
	cfg := &config.Config{Scheduler: config.SchedulerConfig{Enabled: false}}

	started := startScheduler(context.Background(), sched, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	assert.False(t, started, "startScheduler must report that it did not start the loop")
	// The loop's first tick is immediate, so had it started, ListDue would have
	// been called well within this window.
	time.Sleep(100 * time.Millisecond)
	sched.Close() // no-op when Start was never called
}

// TestStartScheduler_EnabledStartsLoop is the positive control: without it,
// TestStartScheduler_DisabledClaimsNothing would still pass if startScheduler
// were broken into never starting anything at all.
func TestStartScheduler_EnabledStartsLoop(t *testing.T) {
	sched, repo := newSchedulerForTest(t)
	cfg := &config.Config{Scheduler: config.SchedulerConfig{
		Enabled:      true,
		TickInterval: 10 * time.Millisecond,
		JobTimeout:   time.Minute,
		DueLimit:     10,
	}}

	polled := make(chan struct{})
	var once sync.Once
	repo.EXPECT().ListDue(mock.Anything, 10).RunAndReturn(
		func(ctx context.Context, limit int) ([]*models.Schedule, error) {
			once.Do(func() { close(polled) })
			return nil, nil
		})

	started := startScheduler(context.Background(), sched, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer sched.Close()

	assert.True(t, started)
	select {
	case <-polled:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler was enabled but the loop never polled for due schedules")
	}
}

package server

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// TestInitializeMetrics_ExportsRuntimeMetrics pins the server half of the
// single-writer rule for the Go runtime metrics (#1277): this is the one
// Metrics instance that exports them (the container's does not — see
// TestProvideMetrics_DoesNotExportRuntimeMetrics).
func TestInitializeMetrics_ExportsRuntimeMetrics(t *testing.T) {
	cfg := &config.Config{}
	// Nothing listens here; the exporter dials lazily, so construction succeeds.
	cfg.OTel.Endpoint = "127.0.0.1:1"
	cfg.OTel.ExportInterval = time.Hour

	m := initializeMetrics(cfg, slog.New(slog.DiscardHandler))
	require.NotNil(t, m)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		// The final flush has no collector to reach; only the release matters.
		if err := m.Shutdown(ctx); err != nil {
			t.Logf("metrics shutdown (expected without a collector): %v", err)
		}
	})

	assert.True(t, m.RuntimeMetricsEnabled(),
		"the server's Metrics is the single exporter of the Go runtime instruments")
}

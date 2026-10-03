package providers

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// TestProvideMetrics_DoesNotExportRuntimeMetrics pins the container half of the
// single-writer rule for the Go runtime metrics (#1277): the server's Metrics
// exports them, so the container's must not, or every go.* series would be
// exported twice under the same resource.
func TestProvideMetrics_DoesNotExportRuntimeMetrics(t *testing.T) {
	cfg := &config.Config{}
	// Nothing listens here; the exporter dials lazily, so construction succeeds.
	cfg.OTel.Endpoint = "127.0.0.1:1"
	cfg.OTel.ExportInterval = time.Hour

	m := ProvideMetrics(cfg, slog.New(slog.DiscardHandler))
	require.NotNil(t, m)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		// The final flush has no collector to reach; only the release matters.
		if err := m.Shutdown(ctx); err != nil {
			t.Logf("metrics shutdown (expected without a collector): %v", err)
		}
	})

	assert.False(t, m.RuntimeMetricsEnabled(),
		"the container's Metrics must not start the Go runtime instruments")
}

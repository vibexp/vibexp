package providers

import (
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/observability/metrics"
)

// ProvideMetrics creates and initializes the application metrics
func ProvideMetrics(cfg *config.Config, logger *slog.Logger) *metrics.Metrics {
	serviceVersion := "dev"
	if v := cfg.ServiceVersion; v != "" {
		serviceVersion = v
	}

	appMetrics, err := metrics.New(
		serviceVersion,
		metrics.WithConfig(cfg),
		metrics.WithOTelEndpoint(cfg.OTel.Endpoint),
		metrics.WithExportInterval(cfg.OTel.ExportInterval),
		metrics.WithLogger(logger),
	)
	if err != nil {
		logger.Warn(
			"Failed to initialize metrics, continuing without metrics",
			"service", "vibexp-api",
			"error", fmt.Sprintf("%+v", err),
		)
		return nil
	}

	logger.Info(
		"Metrics initialized successfully",
		"service", "vibexp-api",
		"component", "metrics",
		"otlp_endpoint", cfg.OTel.Endpoint,
		"export_interval", cfg.OTel.ExportInterval,
	)

	return appMetrics
}

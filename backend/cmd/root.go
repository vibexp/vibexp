package cmd

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/logging"
	"github.com/vibexp/vibexp/internal/server"
)

// buildSHA is set at build time via ldflags:
// go build -ldflags "-X github.com/vibexp/vibexp/cmd.buildSHA=abc1234"
var buildSHA = ""

// configPath is the --config flag value: the path to the config.yaml to load.
// Empty falls back to VIBEXP_CONFIG_FILE, then ./config.yaml (handled by config.Load).
var configPath string

var rootCmd = &cobra.Command{
	Use:   "vibexp",
	Short: "Vibexp.io - Web application with server endpoints",
	Long:  `Vibexp.io application that provides web server functionality with various endpoints.`,
	Run:   runServer,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "",
		"path to the config.yaml file (default: $VIBEXP_CONFIG_FILE or ./config.yaml)")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("Failed to execute command", "error", err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	cfg := loadConfiguration()
	logger := configureLogger(cfg)
	db := initializeDatabase(cfg, logger)
	defer closeDatabase(db, logger)

	runMigrations(db, logger)
	srv := server.New(cfg.Server.Port, db, cfg.Security.APIKeyCommon, cfg, logger)

	ctx, cancel := setupShutdownContext(logger)
	defer cancel()
	defer closeContainer(srv.Container(), logger)

	startServer(ctx, srv, logger)
}

func loadConfiguration() *config.Config {
	cfg, err := config.Load(configPath)
	if err != nil {
		// Use the default slog logger for bootstrap errors (config not yet available)
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Precedence: config server.release_sha → ldflags buildSHA → runtime/debug VCS → "dev"
	if cfg.Server.ReleaseSHA == "" || cfg.Server.ReleaseSHA == "dev" {
		if buildSHA != "" {
			cfg.Server.ReleaseSHA = buildSHA
		} else if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && s.Value != "" {
					cfg.Server.ReleaseSHA = s.Value
					break
				}
			}
		}
	}

	return cfg
}

func configureLogger(cfg *config.Config) *slog.Logger {
	logger := logging.New(logging.Config{
		Format: cfg.Server.LogFormat,
		Level:  cfg.Server.LogLevel,
	})
	// Make this the process-wide default so code logging via slog's package-level
	// functions (and the context-logger fallback) shares the same configuration.
	slog.SetDefault(logger)

	logger.Info("Starting server",
		"port", cfg.Server.Port,
		"log_level", cfg.Server.LogLevel,
		"log_format", cfg.Server.LogFormat,
		"release_sha", cfg.Server.ReleaseSHA,
		"release_date", cfg.Server.ReleaseDate,
	)

	return logger
}

func initializeDatabase(cfg *config.Config, logger *slog.Logger) *database.DB {
	db, err := database.NewConnection(cfg)
	if err != nil {
		logger.Error("Failed to connect to database - database connection is required", "error", err)
		os.Exit(1)
	}
	logger.Info("Database connection established")
	return db
}

func closeDatabase(db *database.DB, logger *slog.Logger) {
	if err := db.Close(); err != nil {
		logger.Error("Failed to close database connection", "error", err)
	}
}

func runMigrations(db *database.DB, logger *slog.Logger) {
	if err := db.RunMigrations(); err != nil {
		logger.Error("Failed to run database migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("Database migrations completed successfully")
}

func setupShutdownContext(logger *slog.Logger) (context.Context, context.CancelFunc) {
	// #nosec G118 -- cancel is returned to caller who owns its lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Received shutdown signal, starting graceful shutdown")
		cancel()
	}()

	return ctx, cancel
}

func closeContainer(c container.Container, logger *slog.Logger) {
	logger.Info("Closing container resources (event manager, etc.)")
	if err := c.Close(); err != nil {
		logger.Error("Failed to close container", "error", err)
	}
}

func startServer(ctx context.Context, srv *server.Server, logger *slog.Logger) {
	if err := srv.Start(ctx); err != nil {
		// http.ErrServerClosed is returned during graceful shutdown.
		// context.Canceled is returned when a shutdown signal is received.
		// Both are expected during normal operation, not actual errors.
		if err == http.ErrServerClosed || err == context.Canceled {
			logger.Info("Server shutting down gracefully")
		} else {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}
	logger.Info("Server stopped")
}

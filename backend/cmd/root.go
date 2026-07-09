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

// buildSHA and buildDate are set at build time via ldflags, e.g.:
//
//	go build -ldflags "-X github.com/vibexp/vibexp/cmd.buildSHA=abc1234 \
//	                    -X github.com/vibexp/vibexp/cmd.buildDate=2026-07-09T12:00:00Z"
//
// The release image (backend/Dockerfile + .github/workflows/release.yml) injects
// them so production builds are self-identifying; the default local build leaves
// them empty and falls back to the VCS build info, then the "dev"/"unknown"
// sentinels.
var (
	buildSHA  = ""
	buildDate = ""
)

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

	resolveReleaseMetadata(cfg)

	return cfg
}

// resolveReleaseMetadata fills in server.release_sha / server.release_date from
// the strongest available source. Each field follows the same precedence:
//
//	config value → ldflags build var → runtime/debug VCS info → sentinel
//
// The sentinels ("dev" for the SHA, "unknown" for the date) match the config
// defaults, so a config value only wins when it is a real override.
func resolveReleaseMetadata(cfg *config.Config) {
	vcsRevision, vcsTime := vcsBuildInfo()
	cfg.Server.ReleaseSHA = resolveReleaseValue(cfg.Server.ReleaseSHA, buildSHA, vcsRevision, "dev")
	cfg.Server.ReleaseDate = resolveReleaseValue(cfg.Server.ReleaseDate, buildDate, vcsTime, "unknown")
}

// resolveReleaseValue applies the release-metadata precedence for one field.
// The config value wins unless it is empty or still the sentinel default; then
// the ldflags value, then the VCS build-info value, then the sentinel.
func resolveReleaseValue(current, ldflag, vcs, sentinel string) string {
	if current != "" && current != sentinel {
		return current
	}
	if ldflag != "" {
		return ldflag
	}
	if vcs != "" {
		return vcs
	}
	return sentinel
}

// vcsBuildInfo returns the VCS revision and commit time recorded by the Go
// toolchain (via -buildvcs). Both are empty when the build carries no VCS info,
// e.g. the container build stage, which has no .git.
func vcsBuildInfo() (revision, buildTime string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", ""
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			buildTime = s.Value
		}
	}
	return revision, buildTime
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

package cmd

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/sirupsen/logrus"
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

var rootCmd = &cobra.Command{
	Use:   "vibexp",
	Short: "Vibexp.io - Web application with server endpoints",
	Long:  `Vibexp.io application that provides web server functionality with various endpoints.`,
	Run:   runServer,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logrus.WithError(err).Fatal("Failed to execute command")
	}
}

func runServer(cmd *cobra.Command, args []string) {
	cfg := loadConfiguration()
	logger := configureLogger(cfg)
	db := initializeDatabase(cfg, logger)
	defer closeDatabase(db, logger)

	runMigrations(db, logger)
	srv := server.New(cfg.Port, db, cfg.APIKeyCommon, cfg, logger)

	ctx, cancel := setupShutdownContext(logger)
	defer cancel()
	defer closeContainer(srv.Container(), logger)

	startServer(ctx, srv, logger)
}

func loadConfiguration() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		// Use standard logrus for bootstrap errors (config not yet available)
		logrus.WithError(err).Fatal("Failed to load configuration")
	}

	// Precedence: env var RELEASE_SHA → ldflags buildSHA → runtime/debug VCS → "unknown"
	if cfg.ReleaseSHA == "" || cfg.ReleaseSHA == "dev" {
		if buildSHA != "" {
			cfg.ReleaseSHA = buildSHA
		} else if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && s.Value != "" {
					cfg.ReleaseSHA = s.Value
					break
				}
			}
		}
	}

	return cfg
}

func configureLogger(cfg *config.Config) *logrus.Logger {
	logger := logging.NewCloudLogger(logging.CloudLoggerConfig{
		ServiceName:    cfg.KService,
		ServiceVersion: cfg.KRevision,
		LogLevel:       cfg.LogLevel,
	})
	// Also configure the global logrus logger for any code that uses it directly
	logrus.SetFormatter(logger.Formatter)
	logrus.SetOutput(logger.Out)
	logrus.SetLevel(logger.Level)

	logger.WithFields(logrus.Fields{
		"port":         cfg.Port,
		"log_level":    cfg.LogLevel,
		"release_sha":  cfg.ReleaseSHA,
		"release_date": cfg.ReleaseDate,
	}).Info("Starting server")

	return logger
}

func initializeDatabase(cfg *config.Config, logger *logrus.Logger) *database.DB {
	db, err := database.NewConnection(cfg)
	if err != nil {
		logger.WithError(err).Fatal("Failed to connect to database - database connection is required")
	}
	logger.Info("Database connection established")
	return db
}

func closeDatabase(db *database.DB, logger *logrus.Logger) {
	if err := db.Close(); err != nil {
		logger.WithError(err).Error("Failed to close database connection")
	}
}

func runMigrations(db *database.DB, logger *logrus.Logger) {
	if err := db.RunMigrations(); err != nil {
		logger.WithError(err).Fatal("Failed to run database migrations")
	}
	logger.Info("Database migrations completed successfully")
}

func setupShutdownContext(logger *logrus.Logger) (context.Context, context.CancelFunc) {
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

func closeContainer(c container.Container, logger *logrus.Logger) {
	logger.Info("Closing container resources (event manager, etc.)")
	if err := c.Close(); err != nil {
		logger.WithError(err).Error("Failed to close container")
	}
}

func startServer(ctx context.Context, srv *server.Server, logger *logrus.Logger) {
	if err := srv.Start(ctx); err != nil {
		// http.ErrServerClosed is returned during graceful shutdown (Cloud Run scale-down/rotation)
		// context.Canceled is returned when shutdown signal is received
		// Both are expected during normal Cloud Run operations, not actual errors
		if err == http.ErrServerClosed || err == context.Canceled {
			logger.Info("Server shutting down gracefully")
		} else {
			logger.WithError(err).Fatal("Server failed")
		}
	}
	logger.Info("Server stopped")
}

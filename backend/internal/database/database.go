package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file" // registers the file:// migration source driver
	_ "github.com/lib/pq"                                // registers the "postgres" database/sql driver
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/vibexp/vibexp/internal/config"
)

type DB struct {
	*sql.DB
}

// buildDSN assembles the lib/pq connection string from the database config. A
// Unix-socket host (Cloud SQL, host starts with '/') omits the port; both shapes
// carry the configured sslmode. Kept as a pure function so the DSN — especially
// the sslmode substitution — is unit-testable without opening a connection.
func buildDSN(cfg config.DatabaseConfig) string {
	if cfg.Host[0] == '/' {
		// Unix socket connection (Cloud SQL)
		return fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=%s",
			cfg.Host, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode)
	}
	// TCP connection (local development)
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode)
}

func NewConnection(cfg *config.Config) (*DB, error) {
	dsn := buildDSN(cfg.Database)

	// Use otelsql to wrap the postgres driver so that every SQL query produces
	// a child span under the current context. This makes DB latency visible in
	// Cloud Trace without any changes to the query call sites.
	db, err := otelsql.Open("postgres", dsn,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Register DB stats metrics (connection pool utilisation) with the global meter.
	// The error here is non-fatal: metrics registration failure does not affect
	// query tracing, so we log and continue rather than aborting startup.
	// The returned Registration is intentionally discarded here; Cloud Run
	// processes exit cleanly on shutdown so explicit Unregister is not needed.
	if _, err := otelsql.RegisterDBStatsMetrics(db, otelsql.WithAttributes(semconv.DBSystemPostgreSQL)); err != nil {
		slog.Warn("Failed to register DB stats metrics with OTel, continuing without DB pool metrics", "error", err)
	}

	// Configure connection pool settings for Cloud SQL
	db.SetMaxOpenConns(25)                   // Maximum number of open connections to the database
	db.SetMaxIdleConns(25)                   // Maximum number of connections in the idle connection pool
	db.SetConnMaxLifetime(300 * time.Second) // Maximum amount of time a connection may be reused (5 minutes)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	slog.Info("Database connection established successfully")
	return &DB{db}, nil
}

func (db *DB) RunMigrations() error {
	// Create migrations directory path
	migrationsPath := "file://migrations"

	// Check if migrations directory exists
	if _, err := filepath.Glob("migrations/*.sql"); err != nil {
		slog.Info("No migrations directory found, skipping migrations")
		return nil
	}

	driver, err := postgres.WithInstance(db.DB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create postgres driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(migrationsPath, "postgres", driver)
	if err != nil {
		// Log the actual error message for debugging duplicate migrations and other issues
		slog.Error("Failed to create migrate instance", "error", err.Error())
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		slog.Error("Failed to apply migrations", "error", err.Error())
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	slog.Info("Database migrations completed successfully")
	return nil
}

func (db *DB) TestConnection() error {
	if err := db.Ping(); err != nil {
		return fmt.Errorf("database connection test failed: %w", err)
	}

	var result int
	if err := db.QueryRow("SELECT 1").Scan(&result); err != nil {
		return fmt.Errorf("database query test failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("database query returned unexpected result: %d", result)
	}

	return nil
}

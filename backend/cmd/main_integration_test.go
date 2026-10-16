//go:build integration

package cmd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
)

// defaultIntegrationDSN targets a test database OWNED BY THIS PACKAGE, never
// shared with another package's suite (`go test -tags=integration ./...` runs
// packages in parallel, and two harnesses truncating one database race, #390)
// and never the development database. It is created on first run if missing.
// Overriding POSTGRES_TEST_DSN reintroduces sharing: only do that when running
// this package alone.
const defaultIntegrationDSN = "postgres://vibexp_app:local_password@localhost:5432/vibexp_io_test_cmd?sslmode=disable"

// integrationDB is shared by every integration test in this package. The
// migrations run once per test binary; each test resets the tables it uses.
var integrationDB *database.DB

func TestMain(m *testing.M) {
	dsn, ok := os.LookupEnv("POSTGRES_TEST_DSN")
	if !ok || dsn == "" {
		dsn = defaultIntegrationDSN
	}
	db, err := openIntegrationDatabase(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration harness: %v\n", err)
		os.Exit(1)
	}
	integrationDB = &database.DB{DB: db}
	if err := runIntegrationMigrations(db); err != nil {
		fmt.Fprintf(os.Stderr, "integration harness: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	closeIntegrationDB(db)
	os.Exit(code)
}

func openIntegrationDatabase(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open test database: %w", err)
	}
	pingErr := db.Ping()
	if pingErr == nil {
		return db, nil
	}
	closeIntegrationDB(db)

	// 3D000 = invalid_catalog_name: the server is up but the test database
	// does not exist yet.
	var pqErr *pq.Error
	if !errors.As(pingErr, &pqErr) || pqErr.Code != "3D000" {
		return nil, fmt.Errorf("ping test database (is the docker-compose postgres running?): %w", pingErr)
	}
	if createErr := createIntegrationDatabase(dsn); createErr != nil {
		return nil, createErr
	}
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open test database after creating it: %w", err)
	}
	if err := db.Ping(); err != nil {
		closeIntegrationDB(db)
		return nil, fmt.Errorf("ping test database after creating it: %w", err)
	}
	return db, nil
}

func closeIntegrationDB(db *sql.DB) {
	if err := db.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "integration harness: close database: %v\n", err)
	}
}

func createIntegrationDatabase(dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("parse test DSN: %w", err)
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	u.Path = "/postgres"
	admin, err := sql.Open("postgres", u.String())
	if err != nil {
		return fmt.Errorf("open admin connection: %w", err)
	}
	defer closeIntegrationDB(admin)
	if _, err := admin.Exec("CREATE DATABASE " + pq.QuoteIdentifier(dbName)); err != nil {
		return fmt.Errorf("create test database %s: %w", dbName, err)
	}
	return nil
}

// runIntegrationMigrations applies the migrations the server runs at startup,
// resolved from this package's directory.
func runIntegrationMigrations(db *sql.DB) error {
	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		return fmt.Errorf("create migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../migrations", "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

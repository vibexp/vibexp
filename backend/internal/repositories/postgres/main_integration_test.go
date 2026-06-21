//go:build integration

package postgres

import (
	"context"
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
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
)

// defaultTestDSN targets a dedicated test database on the docker-compose
// Postgres server (backend-api/docker-compose.yml). It deliberately does NOT
// point at the vibexp_io development database: the harness truncates tables
// between tests (cascading through users), which would destroy local
// development data. The database is created on first run if missing.
const defaultTestDSN = "postgres://vibexp_app:local_password@localhost:5432/vibexp_io_test?sslmode=disable"

// integrationDB is shared by every integration test in this package.
// Migrations run once per test binary (TestMain); tests isolate themselves
// via resetIntegrationTables.
var integrationDB *database.DB

func TestMain(m *testing.M) {
	dsn, ok := os.LookupEnv("POSTGRES_TEST_DSN")
	if !ok || dsn == "" {
		dsn = defaultTestDSN
	}

	db, err := openTestDatabase(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration harness: %v\n", err)
		os.Exit(1)
	}
	integrationDB = &database.DB{DB: db}

	if err := runTestMigrations(db); err != nil {
		fmt.Fprintf(os.Stderr, "integration harness: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	closeQuietly(db)
	os.Exit(code)
}

func openTestDatabase(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open test database: %w", err)
	}

	pingErr := db.Ping()
	if pingErr == nil {
		return db, nil
	}
	closeQuietly(db)

	// 3D000 = invalid_catalog_name: the server is up but the test database
	// doesn't exist yet — create it and reconnect.
	var pqErr *pq.Error
	if !errors.As(pingErr, &pqErr) || pqErr.Code != "3D000" {
		return nil, fmt.Errorf("ping test database (is the docker-compose postgres running?): %w", pingErr)
	}
	if createErr := createTestDatabase(dsn); createErr != nil {
		return nil, createErr
	}

	db, err = sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open test database after creating it: %w", err)
	}
	if err := db.Ping(); err != nil {
		closeQuietly(db)
		return nil, fmt.Errorf("ping test database after creating it: %w", err)
	}
	return db, nil
}

func closeQuietly(db *sql.DB) {
	if err := db.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "integration harness: close database: %v\n", err)
	}
}

func createTestDatabase(dsn string) error {
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
	defer closeQuietly(admin)

	if _, err := admin.Exec("CREATE DATABASE " + pq.QuoteIdentifier(dbName)); err != nil {
		return fmt.Errorf("create test database %s: %w", dbName, err)
	}
	return nil
}

// runTestMigrations applies the same migrations production runs at startup
// (database.DB.RunMigrations resolves "file://migrations" against the process
// working directory, so it can't be reused from a test binary running in this
// package directory).
func runTestMigrations(db *sql.DB) error {
	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		return fmt.Errorf("create migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../../migrations", "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// resetIntegrationTables clears the tables this package's suites write to.
// CASCADE follows every foreign key hanging off users (api_keys,
// user_preferences, the permissions junction, …); the migration-seeded
// api_key_integrations_catalog has no such FK and stays intact.
func resetIntegrationTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, api_keys, user_preferences CASCADE")
	require.NoError(t, err)
}

// insertTestUser seeds a minimal users row (the FK target for api_keys and
// user_preferences) and returns its ID.
func insertTestUser(t *testing.T) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
		id, id+"@integration.test", "Integration Test User")
	require.NoError(t, err)
	return id
}

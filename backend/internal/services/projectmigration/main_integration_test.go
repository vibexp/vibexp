//go:build integration

package projectmigration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// defaultTestDSN targets a dedicated test database OWNED BY THIS PACKAGE.
// It must NOT share vibexp_io_test with the internal/repositories/postgres
// suite: `go test -tags=integration ./...` runs packages in parallel, and two
// TestMains truncating tables in the same database race each other (#390).
// It also deliberately does NOT point at the vibexp_io development database:
// the harness truncates tables between tests, which would destroy local
// development data. The database is created on first run if missing (via the
// /postgres maintenance DB). Overriding POSTGRES_TEST_DSN reintroduces
// sharing — only do that when running a single package.
const defaultTestDSN = "postgres://vibexp_app:local_password@localhost:5432/vibexp_io_test_projectmigration?sslmode=disable"

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

// newIntegrationService wires the real service exactly the way the DI
// container does (providers.ProvideProjectMigrationService): the shared
// integration database plus the real postgres ProjectRepository.
func newIntegrationService() *Service {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(integrationDB, postgres.NewProjectRepository(integrationDB), logger)
}

// resetIntegrationTables clears every table this package's tests write to.
// CASCADE follows the foreign keys hanging off users and teams (projects,
// prompts, artifacts, blueprints, feeds, feed_items, team_members, …), so
// each test starts from a clean slate on the shared test database.
func resetIntegrationTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, projects, prompts, artifacts, blueprints, feeds, feed_items CASCADE")
	require.NoError(t, err)
}

// insertTestUser seeds a minimal users row and returns its ID.
func insertTestUser(t *testing.T) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
		id, id+"@integration.test", "Integration Test User")
	require.NoError(t, err)
	return id
}

// insertTestTeam seeds a team owned by ownerID and returns its ID. Team
// ownership is what grants ownerID access to the team's projects in
// ProjectRepository.GetByID, so no team_members row is needed.
func insertTestTeam(t *testing.T, ownerID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		id, ownerID, "Migration Team", "mig-team-"+id[:8])
	require.NoError(t, err)
	return id
}

// insertTestProject seeds a project in the given team and returns its ID.
func insertTestProject(t *testing.T, userID, teamID, name string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO projects (id, user_id, team_id, name, slug) VALUES ($1, $2, $3, $4, $5)",
		id, userID, teamID, name, "proj-"+id[:8])
	require.NoError(t, err)
	return id
}

// insertTestPrompt seeds a prompt in the given project and returns its ID.
func insertTestPrompt(t *testing.T, userID, teamID, projectID, slug string) string {
	t.Helper()
	return insertTestPromptWithVersion(t, userID, teamID, projectID, slug, 1)
}

// insertTestPromptWithVersion seeds a prompt with an explicit version and
// returns its ID. The rollback test passes math.MaxInt64 so the service's
// in-transaction "version = version + 1" overflows bigint.
func insertTestPromptWithVersion(t *testing.T, userID, teamID, projectID, slug string, version int64) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO prompts (id, name, slug, body, user_id, project_id, team_id, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, slug, slug, "prompt body", userID, projectID, teamID, version)
	require.NoError(t, err)
	return id
}

// insertTestArtifact seeds an artifact in the given project and returns its ID.
func insertTestArtifact(t *testing.T, userID, teamID, projectID, slug string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO artifacts (id, slug, user_id, content, title, team_id, project_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, slug, userID, "artifact content", "Artifact "+slug, teamID, projectID)
	require.NoError(t, err)
	return id
}

// insertTestBlueprint seeds a blueprint in the given project with an explicit
// version and returns its ID. The rollback test passes math.MaxInt64 so the
// service's in-transaction "version = version + 1" overflows bigint.
func insertTestBlueprint(t *testing.T, userID, teamID, projectID, slug string, version int64) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO blueprints (id, slug, user_id, content, title, team_id, project_id, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, slug, userID, "blueprint content", "Blueprint "+slug, teamID, projectID, version)
	require.NoError(t, err)
	return id
}

// insertTestFeed seeds a feed for the team and returns its ID.
func insertTestFeed(t *testing.T, teamID, userID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO feeds (id, team_id, name, created_by_user_id) VALUES ($1, $2, $3, $4)",
		id, teamID, "Migration Feed", userID)
	require.NoError(t, err)
	return id
}

// insertTestFeedItem seeds a feed item explicitly assigned to projectID and
// returns its ID.
func insertTestFeedItem(t *testing.T, teamID, feedID, projectID, userID, title string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO feed_items (id, team_id, feed_id, project_id, title, content, excerpt, ai_assistant_name, posted_by_user_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, teamID, feedID, projectID, title, "feed item content", "excerpt", "claude", userID)
	require.NoError(t, err)
	return id
}

// projectIDQueries maps a resource table to its project_id lookup, keeping
// the SQL static (no string interpolation) for the placement assertions.
var projectIDQueries = map[string]string{
	"prompts":    "SELECT project_id FROM prompts WHERE id = $1",
	"artifacts":  "SELECT project_id FROM artifacts WHERE id = $1",
	"blueprints": "SELECT project_id FROM blueprints WHERE id = $1",
	"feed_items": "SELECT project_id FROM feed_items WHERE id = $1",
}

// projectIDOf returns the current project_id of a resource row.
func projectIDOf(t *testing.T, table, id string) string {
	t.Helper()
	query, ok := projectIDQueries[table]
	require.True(t, ok, "unknown table %q", table)
	var projectID string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), query, id).Scan(&projectID))
	return projectID
}

// countByProjectQueries maps a resource table to its per-project count query.
var countByProjectQueries = map[string]string{
	"prompts":    "SELECT COUNT(*) FROM prompts WHERE project_id = $1",
	"artifacts":  "SELECT COUNT(*) FROM artifacts WHERE project_id = $1",
	"blueprints": "SELECT COUNT(*) FROM blueprints WHERE project_id = $1",
	"feed_items": "SELECT COUNT(*) FROM feed_items WHERE project_id = $1",
}

// countInProject returns how many rows of table live in projectID.
func countInProject(t *testing.T, table, projectID string) int {
	t.Helper()
	query, ok := countByProjectQueries[table]
	require.True(t, ok, "unknown table %q", table)
	var n int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), query, projectID).Scan(&n))
	return n
}

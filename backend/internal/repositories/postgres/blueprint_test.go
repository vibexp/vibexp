package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *BlueprintRepository) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: sqlDB}
	repo := &BlueprintRepository{db: db}
	return sqlDB, mock, repo
}

// TestGetByID_NilMetadata tests handling of NULL metadata in database
func TestGetByID_NilMetadata(t *testing.T) {
	db, mock, repo := setupMockDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	}()

	userID := "user-123"
	blueprintID := "spec-lib-456"

	rows := sqlmock.NewRows([]string{
		"id", "project_id", "slug", "user_id", "team_id", "title", "description",
		"content", "status", "type", "subtype", "metadata", "created_at", "updated_at", "version",
	}).AddRow(
		blueprintID, "550e8400-e29b-41d4-a716-446655440000", "test-slug",
		userID, "team-123", "Test Title", "Test Description",
		"Test Content", "active", "general", nil, nil, // NULL subtype and metadata
		time.Now(), time.Now(), 1,
	)

	mock.ExpectQuery("SELECT (.+) FROM blueprints s.*EXISTS.*teams.*").
		WithArgs(blueprintID, "team-123", userID).
		WillReturnRows(rows)

	result, err := repo.GetByID(context.Background(), userID, "team-123", blueprintID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Metadata)
	assert.Empty(t, result.Metadata)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestGetByProjectIDAndSlug_NilMetadata tests handling of NULL metadata
func TestGetByProjectIDAndSlug_NilMetadata(t *testing.T) {
	db, mock, repo := setupMockDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	}()

	userID := "user-123"
	projectID := "550e8400-e29b-41d4-a716-446655440000"
	slug := "test-slug"

	rows := sqlmock.NewRows([]string{
		"id", "project_id", "slug", "user_id", "team_id", "title", "description",
		"content", "status", "type", "subtype", "metadata", "created_at", "updated_at", "version",
	}).AddRow(
		"spec-lib-456", projectID, slug, userID, "team-123", "Test Title", "Test Description",
		"Test Content", "active", "general", nil, nil, // NULL subtype and metadata
		time.Now(), time.Now(), 1,
	)

	query := "SELECT (.+) FROM blueprints s.*EXISTS.*teams.*"
	mock.ExpectQuery(query).
		WithArgs(projectID, slug, "team-123", userID).
		WillReturnRows(rows)

	result, err := repo.GetByProjectIDAndSlug(context.Background(), userID, "team-123", projectID, slug)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Metadata)
	assert.Empty(t, result.Metadata)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestList_InvalidMetadataKey tests SQL injection protection
//
//nolint:funlen // Test function with comprehensive test cases
func TestList_InvalidMetadataKey(t *testing.T) {
	db, _, repo := setupMockDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	}()

	testCases := []struct {
		name        string
		metadataKey string
		shouldError bool
	}{
		{
			name:        "SQL injection attempt with semicolon",
			metadataKey: "key'; DROP TABLE blueprints; --",
			shouldError: true,
		},
		{
			name:        "SQL injection with quotes",
			metadataKey: "key' OR '1'='1",
			shouldError: true,
		},
		{
			name:        "Invalid special characters",
			metadataKey: "key@#$%",
			shouldError: true,
		},
		{
			name:        "Empty key",
			metadataKey: "",
			shouldError: true,
		},
		{
			name:        "Key too long",
			metadataKey: string(make([]byte, 256)),
			shouldError: true,
		},
		{
			name:        "Valid alphanumeric key",
			metadataKey: "valid_key-123", // gitleaks:allow
			shouldError: false,
		},
		{
			name:        "Valid underscore key",
			metadataKey: "valid_key",
			shouldError: false,
		},
		{
			name:        "Valid hyphen key",
			metadataKey: "valid-key",
			shouldError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filters := repositories.BlueprintFilters{
				TeamID: "team-123", // Required TeamID
				Metadata: map[string]string{
					tc.metadataKey: "value",
				},
			}

			_, _, err := repo.List(context.Background(), "user-123", filters)

			if tc.shouldError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid metadata key")
			} else if err != nil {
				// For valid keys, we expect a different error (no mock setup)
				// but NOT the metadata key validation error
				assert.NotContains(t, err.Error(), "invalid metadata key")
			}
		})
	}
}

// Context cancellation is handled by Go's database/sql package and doesn't need explicit testing
// The repository correctly propagates context to database operations

// TestGetStats_NoData tests empty database scenario
func TestGetStats_NoData(t *testing.T) {
	db, mock, repo := setupMockDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	}()

	userID := "user-123"

	// Mock the optimized query to return no rows
	mock.ExpectQuery("WITH base_stats AS").
		WithArgs(userID).
		WillReturnError(sql.ErrNoRows)

	result, err := repo.GetStats(context.Background(), userID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.TotalProjects)
	assert.Equal(t, 0, result.TotalBlueprints)
	assert.Equal(t, 0, result.AddedThisWeek)
	assert.NotNil(t, result.TotalByType)
	assert.Empty(t, result.TotalByType)
	assert.NotNil(t, result.TotalByStatus)
	assert.Empty(t, result.TotalByStatus)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestList_EmptyMetadataFilter tests list with no metadata filter
func TestList_EmptyMetadataFilter(t *testing.T) {
	db, mock, repo := setupMockDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	}()

	userID := "user-123"
	teamID := "team-123"
	filters := repositories.BlueprintFilters{
		TeamID:   teamID,
		Metadata: nil, // No metadata filter
	}

	countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	// squirrel binds team/user once per EXISTS branch: (team, team, user, team, user).
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM blueprints s.*"+
		"EXISTS.*teams.*").
		WithArgs(teamID, teamID, userID, teamID, userID).
		WillReturnRows(countRows)

	dataRows := sqlmock.NewRows([]string{
		"id", "project_id", "slug", "user_id", "team_id", "title", "description",
		"status", "type", "subtype", "metadata", "created_at", "updated_at",
	})
	mock.ExpectQuery("SELECT (.+) FROM blueprints s.*EXISTS.*teams.*").
		WillReturnRows(dataRows)

	result, total, err := repo.List(context.Background(), userID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestList_MetadataKeyBoundAsParameter verifies the metadata key is passed as a
// bound query parameter (->>$N = $M) rather than interpolated into the SQL string,
// so a malicious-looking but allowlist-passing key can never break out of the value slot.
func TestList_MetadataKeyBoundAsParameter(t *testing.T) {
	db, mock, repo := setupMockDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	}()

	userID := "user-123"
	teamID := "team-123"
	metadataKey := "environment"
	metadataValue := "production"

	filters := repositories.BlueprintFilters{
		TeamID:   teamID,
		Metadata: map[string]string{metadataKey: metadataValue},
	}

	// The generated SQL must use the parameterized form metadata->>$N, never an
	// interpolated key. Assert the regex on the parameterized shape and that the
	// key + value are supplied as bound args.
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	// squirrel binds the team-access pair once per EXISTS branch, then the
	// metadata key/value: (team, team, user, team, user, key, value).
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blueprints s.*metadata->>\$\d+ = \$\d+`).
		WithArgs(teamID, teamID, userID, teamID, userID, metadataKey, metadataValue).
		WillReturnRows(countRows)

	dataRows := sqlmock.NewRows([]string{
		"id", "project_id", "slug", "user_id", "team_id", "title", "description",
		"status", "type", "subtype", "metadata", "created_at", "updated_at",
	})
	mock.ExpectQuery(`SELECT (.+) FROM blueprints s.*metadata->>\$\d+ = \$\d+`).
		WillReturnRows(dataRows)

	_, _, err := repo.List(context.Background(), userID, filters)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestIsValidMetadataKey tests the validation function directly
func TestIsValidMetadataKey(t *testing.T) {
	testCases := []struct {
		name     string
		key      string
		expected bool
	}{
		{"Valid alphanumeric", "key123", true},
		{"Valid with underscore", "valid_key", true},
		{"Valid with hyphen", "valid-key", true},
		{"Valid mixed", "Valid_Key-123", true},
		{"Empty string", "", false},
		{"Too long", string(make([]byte, 256)), false},
		{"SQL injection", "key'; DROP TABLE", false},
		{"Special chars", "key@#$", false},
		{"Spaces", "key with spaces", false},
		{"Quotes", "key'value", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidMetadataKey(tc.key)
			assert.Equal(t, tc.expected, result)
		})
	}
}

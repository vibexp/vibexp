package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupResourceLastAccessedTest(t *testing.T) (*ResourceLastAccessedRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewResourceLastAccessedRepository(&database.DB{DB: mockDB})
	return repo.(*ResourceLastAccessedRepository), mock
}

// Each resource type must reach its own table, and each source its own column.
// A wrong pairing here would write a real timestamp to the wrong place, which
// no behavioural test downstream would notice.
func TestResourceLastAccessedRepository_UpdateLastAccessed_DispatchMatrix(t *testing.T) {
	at := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		resourceType string
		source       string
		wantTable    string
		wantColumn   string
	}{
		{resourceType: "prompt", source: "web", wantTable: "prompts", wantColumn: "last_accessed_web_at"},
		{resourceType: "artifact", source: "cli", wantTable: "artifacts", wantColumn: "last_accessed_cli_at"},
		{resourceType: "blueprint", source: "mcp", wantTable: "blueprints", wantColumn: "last_accessed_mcp_at"},
		{resourceType: "memory", source: "api", wantTable: "memories", wantColumn: "last_accessed_api_at"},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType+"/"+tt.source, func(t *testing.T) {
			repo, mock := setupResourceLastAccessedTest(t)

			// GREATEST is what makes the write monotonic; updated_at must be
			// absent from the SET list entirely.
			mock.ExpectExec(
				`UPDATE `+tt.wantTable+` SET `+tt.wantColumn+
					` = GREATEST\(`+tt.wantColumn+`, \$1\) WHERE id = \$2`,
			).WithArgs(at, "res-1").WillReturnResult(sqlmock.NewResult(0, 1))

			err := repo.UpdateLastAccessed(context.Background(), tt.resourceType, "res-1", tt.source, at)

			require.NoError(t, err)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// An unsupported type or source must not reach the database at all — sqlmock
// fails the test if any statement is issued, since none is expected.
func TestResourceLastAccessedRepository_UpdateLastAccessed_UnsupportedIsSentinel(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		source       string
		wantIn       string
	}{
		{name: "project has no columns", resourceType: "project", source: "web", wantIn: "project"},
		{name: "agent has no columns", resourceType: "agent", source: "web", wantIn: "agent"},
		{name: "unknown type", resourceType: "wat", source: "web", wantIn: "wat"},
		{name: "unknown source", resourceType: "prompt", source: "carrier-pigeon", wantIn: "carrier-pigeon"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupResourceLastAccessedTest(t)

			err := repo.UpdateLastAccessed(context.Background(), tt.resourceType, "res-1", tt.source, time.Now())

			require.Error(t, err)
			assert.ErrorIs(t, err, repositories.ErrUnsupportedLastAccessedResource)
			assert.Contains(t, err.Error(), tt.wantIn, "the error should name what was unsupported")
			assert.NoError(t, mock.ExpectationsWereMet(), "no statement may be issued")
		})
	}
}

func TestResourceLastAccessedRepository_UpdateLastAccessed_Error(t *testing.T) {
	repo, mock := setupResourceLastAccessedTest(t)

	mock.ExpectExec(`UPDATE prompts`).WillReturnError(errors.New("boom"))

	err := repo.UpdateLastAccessed(context.Background(), "prompt", "res-1", "web", time.Now())

	require.Error(t, err)
	assert.NotErrorIs(t, err, repositories.ErrUnsupportedLastAccessedResource)
	assert.Contains(t, err.Error(), "failed to update last accessed for prompt")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A resource id that matches no row is not an error: the resource may have been
// deleted between the read and this asynchronous write.
func TestResourceLastAccessedRepository_UpdateLastAccessed_MissingRowIsNotAnError(t *testing.T) {
	repo, mock := setupResourceLastAccessedTest(t)

	mock.ExpectExec(`UPDATE prompts`).WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.UpdateLastAccessed(context.Background(), "prompt", "gone", "web", time.Now())

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The dispatch allowlists must cover exactly the resource types that gained
// columns in migration 013 and exactly the sources DeriveSource can return.
// This is the guard against a medium being added to the access path and
// silently never denormalizing.
func TestLastAccessedTargets_CoverEveryTypeAndMedium(t *testing.T) {
	tables, columns := LastAccessedTargets()

	assert.Equal(t, map[string]string{
		"prompt":    "prompts",
		"artifact":  "artifacts",
		"blueprint": "blueprints",
		"memory":    "memories",
	}, tables, "exactly the four tables migration 013 added columns to")

	assert.Equal(t, map[string]string{
		"web": "last_accessed_web_at",
		"cli": "last_accessed_cli_at",
		"mcp": "last_accessed_mcp_at",
		"api": "last_accessed_api_at",
	}, columns, "exactly the four sources resourceaccess.DeriveSource can return")

	// Returned copies must not alias the package maps, or a caller could
	// corrupt the dispatch for the whole process.
	tables["prompt"] = "tampered"
	freshTables, _ := LastAccessedTargets()
	assert.Equal(t, "prompts", freshTables["prompt"])
}

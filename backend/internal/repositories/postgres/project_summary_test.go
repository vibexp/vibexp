package postgres

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// The four resource DETAIL reads resolve their owning project in the same query
// (#929), replacing a client-side scan of the project list that returned nothing
// once a team had more than the list's 100-project cap. Two branches matter per
// resource and neither is visible in the response shape: the join HITS (a
// summary is reported) and the join MISSES (NULL columns must yield a nil
// summary, not an error and not a zero-valued object) — the latter is the whole
// reason the join is LEFT rather than INNER.

const (
	projSumProjectID = "550e8400-e29b-41d4-a716-446655440090"
	projSumTeamID    = "team-929"
	projSumUserID    = "user-929"
)

// projectSummaryColumns are the three columns projectSummaryProjection adds, in
// order, to every detail read's SELECT list.
func projectSummaryColumns() []string {
	return []string{"proj_id", "proj_name", "proj_slug"}
}

// projectSummaryHit / projectSummaryMiss are the two row tails the LEFT JOIN can
// produce.
func projectSummaryHit() []driverValue {
	return []driverValue{projSumProjectID, "Platform", "platform"}
}

func projectSummaryMiss() []driverValue {
	return []driverValue{nil, nil, nil}
}

// driverValue is sqlmock's AddRow element type.
type driverValue = driver.Value

func projectSummaryMockDB(t *testing.T) (*database.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("failed to close db: %v", err)
		}
	})
	return &database.DB{DB: sqlDB}, mock
}

func TestPromptGetBySlug_ProjectSummary(t *testing.T) {
	now := time.Now()
	base := []driverValue{
		"prompt-1", "Test Prompt", "test-prompt", "desc", "body", projSumUserID, projSumTeamID,
		projSumProjectID, "published", true, pq.StringArray{}, now, now, int64(1), false,
	}

	for _, tc := range []struct {
		name string
		tail []driverValue
		want *models.ProjectSummary
	}{
		{"join hits", projectSummaryHit(),
			&models.ProjectSummary{ID: projSumProjectID, Name: "Platform", Slug: "platform"}},
		{"join misses", projectSummaryMiss(), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := projectSummaryMockDB(t)
			repo := NewPromptRepository(db)

			cols := append([]string{
				"id", "name", "slug", "description", "body", "user_id", "team_id", "project_id",
				"status", "mcp_expose", "labels", "created_at", "updated_at", "version", "is_shared",
			}, projectSummaryColumns()...)
			mock.ExpectQuery("SELECT (.+) FROM prompts p.*LEFT JOIN projects proj").
				WithArgs("test-prompt", projSumTeamID, projSumUserID).
				WillReturnRows(sqlmock.NewRows(cols).AddRow(append(append([]driverValue{}, base...), tc.tail...)...))

			got, err := repo.GetBySlug(context.Background(), projSumUserID, projSumTeamID, "test-prompt")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Project)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestArtifactGetByProjectIDAndSlug_ProjectSummary(t *testing.T) {
	now := time.Now()
	base := []driverValue{
		"art-1", projSumProjectID, "test-slug", projSumUserID, projSumTeamID, "Title", "Description",
		"content", "active", "general", []byte(`{}`), now, now, int64(1), pq.StringArray{},
	}

	for _, tc := range []struct {
		name string
		tail []driverValue
		want *models.ProjectSummary
	}{
		{"join hits", projectSummaryHit(),
			&models.ProjectSummary{ID: projSumProjectID, Name: "Platform", Slug: "platform"}},
		{"join misses", projectSummaryMiss(), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := projectSummaryMockDB(t)
			repo := NewArtifactRepository(db)

			cols := append([]string{
				"id", "project_id", "slug", "user_id", "team_id", "title", "description", "content",
				"status", "type", "metadata", "created_at", "updated_at", "version", "labels",
			}, projectSummaryColumns()...)
			mock.ExpectQuery("SELECT (.+) FROM artifacts a.*LEFT JOIN projects proj").
				WithArgs(projSumProjectID, "test-slug", projSumTeamID, projSumUserID).
				WillReturnRows(sqlmock.NewRows(cols).AddRow(append(append([]driverValue{}, base...), tc.tail...)...))

			got, err := repo.GetByProjectIDAndSlug(
				context.Background(), projSumUserID, projSumTeamID, projSumProjectID, "test-slug")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Project)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestBlueprintGetByProjectIDAndSlug_ProjectSummary(t *testing.T) {
	now := time.Now()
	base := []driverValue{
		"bp-1", projSumProjectID, "test-slug", projSumUserID, projSumTeamID, "Title", "Description",
		"content", "active", "claude-code", nil, []byte(`{}`), now, now, int64(1),
		"CLAUDE.md", true, nil, nil, nil, nil, nil, nil, nil, pq.StringArray{},
	}

	for _, tc := range []struct {
		name string
		tail []driverValue
		want *models.ProjectSummary
	}{
		{"join hits", projectSummaryHit(),
			&models.ProjectSummary{ID: projSumProjectID, Name: "Platform", Slug: "platform"}},
		{"join misses", projectSummaryMiss(), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := projectSummaryMockDB(t)
			repo := NewBlueprintRepository(db)

			cols := append([]string{
				"id", "project_id", "slug", "user_id", "team_id", "title", "description", "content",
				"status", "type", "subtype", "metadata", "created_at", "updated_at", "version",
				"path", "path_derived", "raw_content", "content_sha",
				"source_repo", "source_commit_sha", "source_blob_sha", "source_content_sha", "imported_at",
				"labels",
			}, projectSummaryColumns()...)
			mock.ExpectQuery("SELECT (.+) FROM blueprints s.*LEFT JOIN projects proj").
				WithArgs(projSumProjectID, "test-slug", projSumTeamID, projSumUserID).
				WillReturnRows(sqlmock.NewRows(cols).AddRow(append(append([]driverValue{}, base...), tc.tail...)...))

			got, err := repo.GetByProjectIDAndSlug(
				context.Background(), projSumUserID, projSumTeamID, projSumProjectID, "test-slug")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Project)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMemoryGetByID_ProjectSummary(t *testing.T) {
	now := time.Now()
	base := []driverValue{
		"mem-1", projSumUserID, projSumTeamID, projSumProjectID, nil, "text", "active",
		[]byte(`{}`), now, now, int64(1), pq.StringArray{},
	}

	for _, tc := range []struct {
		name string
		tail []driverValue
		want *models.ProjectSummary
	}{
		{"join hits", projectSummaryHit(),
			&models.ProjectSummary{ID: projSumProjectID, Name: "Platform", Slug: "platform"}},
		{"join misses", projectSummaryMiss(), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := projectSummaryMockDB(t)
			repo := NewMemoryRepository(db)

			cols := append([]string{
				"id", "user_id", "team_id", "project_id", "title", "text", "status", "metadata",
				"created_at", "updated_at", "version", "labels",
			}, projectSummaryColumns()...)
			mock.ExpectQuery("SELECT (.+) FROM memories m.*LEFT JOIN projects proj").
				WithArgs("mem-1", projSumTeamID, projSumUserID).
				WillReturnRows(sqlmock.NewRows(cols).AddRow(append(append([]driverValue{}, base...), tc.tail...)...))

			got, err := repo.GetByID(context.Background(), projSumUserID, projSumTeamID, "mem-1")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Project)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

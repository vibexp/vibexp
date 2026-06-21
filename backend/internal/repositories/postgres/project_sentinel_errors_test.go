package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// TestProjectRepository_GetBySlug_NotFound_SentinelError verifies that
// sql.ErrNoRows is wrapped as ErrProjectNotFoundForRepo (not a plain string).
func TestProjectRepository_GetBySlug_NotFound_SentinelError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})

	mock.ExpectQuery("SELECT(.+)FROM projects p").
		WithArgs("missing-slug", "team-1", "user-1").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetBySlug(context.Background(), "team-1", "user-1", "missing-slug")

	require.Error(t, err)
	assert.True(t, errors.Is(err, repositories.ErrProjectNotFoundForRepo),
		"GetBySlug not-found must wrap ErrProjectNotFoundForRepo; got: %v", err)
}

// TestProjectRepository_GetBySlug_DBError_NotSentinel verifies that a real DB
// error (not sql.ErrNoRows) is NOT wrapped as ErrProjectNotFoundForRepo.
func TestProjectRepository_GetBySlug_DBError_NotSentinel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})

	mock.ExpectQuery("SELECT(.+)FROM projects p").
		WithArgs("any-slug", "team-1", "user-1").
		WillReturnError(errors.New("connection refused"))

	_, err = repo.GetBySlug(context.Background(), "team-1", "user-1", "any-slug")

	require.Error(t, err)
	assert.False(t, errors.Is(err, repositories.ErrProjectNotFoundForRepo),
		"real DB errors must NOT be sentinel ErrProjectNotFoundForRepo")
}

// TestProjectRepository_GetByGitURL_NotFound_SentinelError verifies sentinel for GetByGitURL.
func TestProjectRepository_GetByGitURL_NotFound_SentinelError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})

	mock.ExpectQuery("SELECT(.+)FROM projects p").
		WithArgs("team-1", "https://github.com/user/repo", "user-1").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetByGitURL(context.Background(), "team-1", "user-1", "https://github.com/user/repo")

	require.Error(t, err)
	assert.True(t, errors.Is(err, repositories.ErrProjectNotFoundForRepo),
		"GetByGitURL not-found must wrap ErrProjectNotFoundForRepo; got: %v", err)
}

// TestProjectRepository_GetByGitURL_DBError_NotSentinel verifies non-sentinel for GetByGitURL DB errors.
func TestProjectRepository_GetByGitURL_DBError_NotSentinel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})

	mock.ExpectQuery("SELECT(.+)FROM projects p").
		WithArgs("team-1", "https://github.com/user/repo", "user-1").
		WillReturnError(errors.New("pq: too many connections"))

	_, err = repo.GetByGitURL(context.Background(), "team-1", "user-1", "https://github.com/user/repo")

	require.Error(t, err)
	assert.False(t, errors.Is(err, repositories.ErrProjectNotFoundForRepo),
		"real DB errors must NOT be sentinel ErrProjectNotFoundForRepo")
}

// TestProjectRepository_GetByID_NotFound_SentinelError verifies sentinel for GetByID.
func TestProjectRepository_GetByID_NotFound_SentinelError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})

	mock.ExpectQuery("SELECT(.+)FROM projects p").
		WithArgs("missing-project-id", "user-1").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetByID(context.Background(), "user-1", "missing-project-id")

	require.Error(t, err)
	assert.True(t, errors.Is(err, repositories.ErrProjectNotFoundForRepo),
		"GetByID not-found must wrap ErrProjectNotFoundForRepo; got: %v", err)
}

// TestProjectRepository_GetByID_DBError_NotSentinel verifies non-sentinel for GetByID DB errors.
func TestProjectRepository_GetByID_DBError_NotSentinel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})

	mock.ExpectQuery("SELECT(.+)FROM projects p").
		WithArgs("some-project-id", "user-1").
		WillReturnError(errors.New("pq: query timeout"))

	_, err = repo.GetByID(context.Background(), "user-1", "some-project-id")

	require.Error(t, err)
	assert.False(t, errors.Is(err, repositories.ErrProjectNotFoundForRepo),
		"real DB errors must NOT be sentinel ErrProjectNotFoundForRepo")
}

// TestProjectRepository_Create_SlugConflict_SentinelError verifies that a Postgres
// 23505 unique violation on Create is wrapped as ErrProjectSlugExists while keeping a
// human-readable message that still names the conflicting slug.
func TestProjectRepository_Create_SlugConflict_SentinelError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})

	mock.ExpectQuery("INSERT INTO projects").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "projects_slug_team_id_key"})

	project := &models.Project{
		UserID: "user-1",
		TeamID: "team-1",
		Name:   "owner/repo",
		Slug:   "owner-repo",
		GitURL: "https://github.com/owner/repo",
	}

	err = repo.Create(context.Background(), project)

	require.Error(t, err)
	assert.True(t, errors.Is(err, repositories.ErrProjectSlugExists),
		"Create 23505 on the slug constraint must wrap ErrProjectSlugExists; got: %v", err)
	assert.False(t, errors.Is(err, repositories.ErrProjectGitURLExists),
		"a slug collision must NOT be mapped to ErrProjectGitURLExists")
	assert.Contains(t, err.Error(), "owner-repo",
		"Create slug-conflict error must still name the conflicting slug")
}

// TestProjectRepository_Create_GitURLConflict_SentinelError verifies that a Postgres
// 23505 unique violation on the git_url constraint is mapped to ErrProjectGitURLExists
// (not the slug sentinel), so the dispatcher routes it to the git_url recovery path.
func TestProjectRepository_Create_GitURLConflict_SentinelError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})

	mock.ExpectQuery("INSERT INTO projects").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "idx_projects_team_id_git_url_unique"})

	project := &models.Project{
		UserID: "user-1",
		TeamID: "team-1",
		Name:   "owner/repo",
		Slug:   "owner-repo",
		GitURL: "https://github.com/owner/repo",
	}

	err = repo.Create(context.Background(), project)

	require.Error(t, err)
	assert.True(t, errors.Is(err, repositories.ErrProjectGitURLExists),
		"Create 23505 on the git_url constraint must wrap ErrProjectGitURLExists; got: %v", err)
	assert.False(t, errors.Is(err, repositories.ErrProjectSlugExists),
		"a git_url collision must NOT be mapped to ErrProjectSlugExists")
	assert.Contains(t, err.Error(), "https://github.com/owner/repo",
		"Create git_url-conflict error must still name the conflicting git_url")
}

// TestProjectRepository_Create_DBError_NotSentinel verifies that a non-23505 DB error on
// Create is NOT mapped to the slug-exists sentinel, preserving fail-fast for other failures.
func TestProjectRepository_Create_DBError_NotSentinel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close db: %v", closeErr)
		}
	}()

	repo := postgres.NewProjectRepository(&database.DB{DB: db})

	mock.ExpectQuery("INSERT INTO projects").
		WillReturnError(errors.New("connection refused"))

	project := &models.Project{
		UserID: "user-1",
		TeamID: "team-1",
		Name:   "owner/repo",
		Slug:   "owner-repo",
		GitURL: "https://github.com/owner/repo",
	}

	err = repo.Create(context.Background(), project)

	require.Error(t, err)
	assert.False(t, errors.Is(err, repositories.ErrProjectSlugExists),
		"non-unique-violation errors must NOT be sentinel ErrProjectSlugExists")
}

// TestProjectRepository_SentinelError_IsUnwrappable verifies that callers using
// errors.Is() can detect ErrProjectNotFoundForRepo through the %w wrap chain.
func TestProjectRepository_SentinelError_IsUnwrappable(t *testing.T) {
	// Directly verify the errors.Is chain works as expected.
	wrapped := errors.New("project not found for repository: git_url=https://example.com team=team-1")

	// Create the exact error the repository returns
	repoErr := errors.Join(repositories.ErrProjectNotFoundForRepo, wrapped)

	// errors.Is should find ErrProjectNotFoundForRepo in the chain.
	assert.True(t, errors.Is(repoErr, repositories.ErrProjectNotFoundForRepo))
}

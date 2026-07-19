package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// TestBlueprintRepository_TeamMember_CanListOtherMembersSpecs verifies that team members can list
// specs created by other team members
func TestBlueprintRepository_TeamMember_CanListOtherMembersSpecs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	// Scenario:
	// - Alice creates team "Engineering"
	// - Alice creates spec "api-spec"
	// - Bob joins team as member
	// - Bob lists specs
	// - Expected: Bob sees Alice's "api-spec" in the list

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	aliceSpecID := "spec-alice"

	filters := repositories.BlueprintFilters{
		TeamID: teamID,
		Page:   1,
		Limit:  20,
	}

	// Mock count query - no JOINs needed with EXISTS subqueries.
	// squirrel binds team/user once per EXISTS branch, so the base args are
	// (team, team, user, team, user); LIMIT/OFFSET are literals, not args.
	countQuery := `SELECT COUNT\(\*\) FROM blueprints s WHERE`
	mock.ExpectQuery(countQuery).
		WithArgs(teamID, teamID, bobUserID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Mock list query - no DISTINCT or JOINs needed with EXISTS subqueries
	listQuery := `SELECT s\.id, s\.project_id, s\.slug, s\.user_id, s\.team_id, s\.title, ` +
		`s\.description, s\.status, s\.type, s\.subtype, s\.metadata, s\.created_at, s\.updated_at, ` +
		`s\.path, s\.path_derived, s\.content_sha, s\.source_repo, s\.source_commit_sha, ` +
		`s\.source_blob_sha, s\.imported_at ` +
		`FROM blueprints s WHERE`
	mock.ExpectQuery(listQuery).
		WithArgs(teamID, teamID, bobUserID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "slug", "user_id", "team_id", "title", "description",
			"status", "type", "subtype", "metadata", "created_at", "updated_at",
			"path", "path_derived", "content_sha",
			"source_repo", "source_commit_sha", "source_blob_sha", "imported_at",
		}).AddRow(
			aliceSpecID, "project-123", "api-spec", aliceUserID, teamID,
			"API Specification", "REST API spec",
			"active", "api", "openapi", []byte("{}"), now, now,
			"api-spec.md", true, nil, nil, nil, nil, nil,
		))

	// Bob lists specs
	specs, total, err := repo.List(ctx, bobUserID, filters)

	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, specs, 1)
	assert.Equal(t, aliceSpecID, specs[0].ID)
	assert.Equal(t, aliceUserID, specs[0].UserID) // Created by Alice
	assert.Equal(t, "API Specification", specs[0].Title)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_TeamMember_CanGetOtherMembersSpecs verifies that team members can get
// specs created by other team members
func TestBlueprintRepository_TeamMember_CanGetOtherMembersSpecs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	// Scenario:
	// - Alice creates spec in team
	// - Bob (team member) gets spec by ID
	// - Expected: Bob successfully retrieves Alice's spec

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceUserID := "user-alice"
	aliceSpecID := "spec-alice"

	// Mock get query with EXISTS subqueries (no JOINs) - note alias is 's'
	getQuery := `SELECT s\.id, s\.project_id, s\.slug, s\.user_id, s\.team_id, s\.title, s\.description, ` +
		`s\.content, s\.status, s\.type, s\.subtype, s\.metadata, s\.created_at, s\.updated_at, s\.version,\s+` +
		`s\.path, s\.path_derived, s\.raw_content, s\.content_sha, s\.source_repo, s\.source_commit_sha, ` +
		`s\.source_blob_sha, s\.source_content_sha, s\.imported_at\s+` +
		`FROM blueprints s\s+WHERE.*`
	mock.ExpectQuery(getQuery).
		WithArgs(aliceSpecID, teamID, bobUserID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "slug", "user_id", "team_id", "title", "description",
			"content", "status", "type", "subtype", "metadata", "created_at", "updated_at", "version",
			"path", "path_derived", "raw_content", "content_sha",
			"source_repo", "source_commit_sha", "source_blob_sha", "source_content_sha", "imported_at",
		}).AddRow(
			aliceSpecID, "project-123", "api-spec", aliceUserID, teamID,
			"API Specification", "REST API spec", "OpenAPI content",
			"active", "api", "openapi", []byte("{}"), now, now, 1,
			"api-spec.md", true, nil, nil, nil, nil, nil, nil, nil,
		))

	// Bob gets Alice's spec
	spec, err := repo.GetByID(ctx, bobUserID, teamID, aliceSpecID)

	assert.NoError(t, err)
	require.NotNil(t, spec)
	assert.Equal(t, aliceSpecID, spec.ID)
	assert.Equal(t, aliceUserID, spec.UserID)
	assert.Equal(t, "API Specification", spec.Title)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_TeamMember_CanUpdateOtherMembersSpecs verifies that team members can update
// specs created by other team members
func TestBlueprintRepository_TeamMember_CanUpdateOtherMembersSpecs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()
	now := time.Now()

	// Scenario:
	// - Alice creates spec in team
	// - Bob (team member) updates the spec
	// - Expected: Update succeeds

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceSpecID := "spec-alice"

	subtype := "openapi"
	spec := &models.Blueprint{
		ID:          aliceSpecID,
		ProjectID:   "project-123",
		Slug:        "api-spec",
		UserID:      bobUserID, // Bob is updating
		TeamID:      teamID,
		Title:       "API Specification - Updated by Bob",
		Description: "Updated description",
		Content:     "Updated OpenAPI content",
		Status:      "active",
		Type:        "api",
		Subtype:     &subtype,
		Metadata:    map[string]interface{}{},
		UpdatedAt:   now,
		Version:     1,
	}

	// Mock ownership validation
	ownershipQuery := `SELECT EXISTS\(`
	mock.ExpectQuery(ownershipQuery).
		WithArgs(aliceSpecID, teamID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Mock update query - note table name is blueprints (singular)
	updateQuery := `UPDATE blueprints`
	mock.ExpectQuery(updateQuery).
		WithArgs(
			aliceSpecID, "project-123", "api-spec",
			"API Specification - Updated by Bob", "Updated description", "Updated OpenAPI content",
			"active", "api", "openapi", sqlmock.AnyArg(), teamID, now,
			spec.Path, spec.PathDerived, sqlmock.AnyArg(), sqlmock.AnyArg(),
			teamID, 1,
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at", "version"}).AddRow(now, 2))

	// Bob updates Alice's spec
	err = repo.Update(ctx, spec)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), spec.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_RegularMember_CannotDeleteOtherMembersSpecs verifies that regular members
// cannot delete specs created by other team members
func TestBlueprintRepository_RegularMember_CannotDeleteOtherMembersSpecs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates spec in team
	// - Bob (regular member) tries to delete
	// - Expected: Delete fails

	teamID := "team-123"
	bobUserID := "user-bob"
	aliceSpecID := "spec-alice"

	deleteQuery := `DELETE FROM blueprints`
	mock.ExpectExec(deleteQuery).
		WithArgs(aliceSpecID, teamID, bobUserID).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	// Bob tries to delete Alice's spec
	err = repo.Delete(ctx, bobUserID, teamID, aliceSpecID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_TeamAdmin_CanDeleteOtherMembersSpecs verifies that team admins can delete
// specs created by other team members
func TestBlueprintRepository_TeamAdmin_CanDeleteOtherMembersSpecs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Alice creates spec in team
	// - Carol (team admin) deletes the spec
	// - Expected: Delete succeeds

	teamID := "team-123"
	carolUserID := "user-carol" // Carol is admin
	aliceSpecID := "spec-alice"

	deleteQuery := `DELETE FROM blueprints`
	mock.ExpectExec(deleteQuery).
		WithArgs(aliceSpecID, teamID, carolUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Carol (admin) deletes Alice's spec
	err = repo.Delete(ctx, carolUserID, teamID, aliceSpecID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestBlueprintRepository_TeamOwner_CanDeleteAnyTeamSpec verifies that team owners can delete any spec in the team
func TestBlueprintRepository_TeamOwner_CanDeleteAnyTeamSpec(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewBlueprintRepository(&database.DB{DB: db})
	ctx := context.Background()

	// Scenario:
	// - Bob creates spec in team
	// - Alice (team owner) deletes Bob's spec
	// - Expected: Delete succeeds

	teamID := "team-123"
	aliceUserID := "user-alice" // Alice is team owner
	bobSpecID := "spec-bob"

	deleteQuery := `DELETE FROM blueprints`
	mock.ExpectExec(deleteQuery).
		WithArgs(bobSpecID, teamID, aliceUserID).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

	// Alice (owner) deletes Bob's spec
	err = repo.Delete(ctx, aliceUserID, teamID, bobSpecID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

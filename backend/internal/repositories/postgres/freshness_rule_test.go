package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// freshnessRuleCols mirrors the freshnessRuleColumns projection order.
func freshnessRuleCols() []string {
	return []string{
		"id", "team_id", "project_id", "resource_types", "mediums",
		"threshold_days", "enabled", "created_at", "updated_at",
	}
}

func setupFreshnessRuleTest(t *testing.T) (*FreshnessRuleRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewFreshnessRuleRepository(&database.DB{DB: mockDB})
	return repo.(*FreshnessRuleRepository), mock
}

func freshnessRuleRow(projectID interface{}, types, mediums string) *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows(freshnessRuleCols()).AddRow(
		"rule-1", "team-1", projectID, []byte(types), []byte(mediums),
		90, true, now, now,
	)
}

func TestFreshnessRuleRepository_Create_PopulatesModel(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)
	projectID := "proj-1"

	mock.ExpectQuery(`INSERT INTO freshness_rules`).
		WithArgs(
			"team-1", &projectID,
			pq.Array([]string{"artifact", "prompt"}), pq.Array([]string{"web"}),
			90, true,
		).
		WillReturnRows(freshnessRuleRow("proj-1", "{artifact,prompt}", "{web}"))

	rule := &models.FreshnessRule{
		TeamID: "team-1", ProjectID: &projectID,
		ResourceTypes: []string{"artifact", "prompt"}, Mediums: []string{"web"},
		ThresholdDays: 90, Enabled: true,
	}
	require.NoError(t, repo.Create(context.Background(), rule))

	assert.Equal(t, "rule-1", rule.ID)
	assert.Equal(t, []string{"artifact", "prompt"}, rule.ResourceTypes)
	assert.Equal(t, []string{"web"}, rule.Mediums)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A nil Mediums must persist as `{}` ("any medium") and never as NULL: the
// column is NOT NULL, and the empty array is the value that carries meaning.
func TestFreshnessRuleRepository_Create_NilMediumsBecomesEmptyArray(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`INSERT INTO freshness_rules`).
		WithArgs(
			"team-1", (*string)(nil),
			pq.Array([]string{"memory"}), pq.Array([]string{}),
			30, false,
		).
		WillReturnRows(freshnessRuleRow(nil, "{memory}", "{}"))

	rule := &models.FreshnessRule{
		TeamID: "team-1", ResourceTypes: []string{"memory"},
		Mediums: nil, ThresholdDays: 30,
	}
	require.NoError(t, repo.Create(context.Background(), rule))

	assert.Nil(t, rule.ProjectID, "a nil project_id means the rule spans every project")
	assert.Equal(t, []string{}, rule.Mediums)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessRuleRepository_Create_Error(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`INSERT INTO freshness_rules`).WillReturnError(errors.New("boom"))

	err := repo.Create(context.Background(), &models.FreshnessRule{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create freshness rule")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The lookup is scoped by team_id as well as id: another team's rule id must
// be indistinguishable from a non-existent one.
func TestFreshnessRuleRepository_GetByID_ScopedToTeam(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`FROM freshness_rules WHERE team_id = \$1 AND id = \$2`).
		WithArgs("team-1", "rule-1").
		WillReturnRows(freshnessRuleRow("proj-1", "{blueprint}", "{web,mcp}"))

	got, err := repo.GetByID(context.Background(), "team-1", "rule-1")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "rule-1", got.ID)
	assert.Equal(t, []string{"web", "mcp"}, got.Mediums)
	assert.Equal(t, 90, got.ThresholdDays)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessRuleRepository_GetByID_MissingReturnsNilNil(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`FROM freshness_rules`).WillReturnError(sql.ErrNoRows)

	got, err := repo.GetByID(context.Background(), "team-1", "rule-1")

	require.NoError(t, err)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessRuleRepository_GetByID_Error(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`FROM freshness_rules`).WillReturnError(errors.New("boom"))

	_, err := repo.GetByID(context.Background(), "team-1", "rule-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get freshness rule")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// enabledOnly is what rule evaluation loads; without the predicate a disabled
// rule would still mark resources stale.
func TestFreshnessRuleRepository_ListByTeam_EnabledOnlyNarrowsQuery(t *testing.T) {
	tests := []struct {
		name        string
		enabledOnly bool
		wantQuery   string
	}{
		{
			name:        "all rules",
			enabledOnly: false,
			wantQuery:   `FROM freshness_rules WHERE team_id = \$1 ORDER BY created_at ASC, id ASC`,
		},
		{
			name:        "enabled only",
			enabledOnly: true,
			wantQuery:   `FROM freshness_rules WHERE team_id = \$1 AND enabled = true ORDER BY`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupFreshnessRuleTest(t)

			mock.ExpectQuery(tt.wantQuery).WithArgs("team-1").
				WillReturnRows(freshnessRuleRow("proj-1", "{artifact}", "{}"))

			rules, err := repo.ListByTeam(context.Background(), "team-1", tt.enabledOnly)

			require.NoError(t, err)
			require.Len(t, rules, 1)
			assert.Equal(t, "rule-1", rules[0].ID)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestFreshnessRuleRepository_ListByTeam_EmptyIsNeverNil(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`FROM freshness_rules`).WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows(freshnessRuleCols()))

	rules, err := repo.ListByTeam(context.Background(), "team-1", false)

	require.NoError(t, err)
	assert.Equal(t, []*models.FreshnessRule{}, rules)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessRuleRepository_ListByTeam_QueryError(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`FROM freshness_rules`).WillReturnError(errors.New("boom"))

	_, err := repo.ListByTeam(context.Background(), "team-1", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list freshness rules")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessRuleRepository_ListByTeam_ScanError(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`FROM freshness_rules`).WillReturnRows(
		sqlmock.NewRows([]string{"id"}).AddRow("only-one-column"),
	)

	_, err := repo.ListByTeam(context.Background(), "team-1", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan freshness rule")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessRuleRepository_Update_RefreshesModel(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`UPDATE freshness_rules SET .* WHERE team_id = \$1 AND id = \$2 RETURNING`).
		WithArgs(
			"team-1", "rule-1", (*string)(nil),
			pq.Array([]string{"artifact"}), pq.Array([]string{}), 45, false,
		).
		WillReturnRows(freshnessRuleRow(nil, "{artifact}", "{}"))

	rule := &models.FreshnessRule{
		TeamID: "team-1", ID: "rule-1",
		ResourceTypes: []string{"artifact"}, ThresholdDays: 45, Enabled: false,
	}
	require.NoError(t, repo.Update(context.Background(), rule))

	assert.Equal(t, 90, rule.ThresholdDays, "the model is refreshed from the persisted row")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// No matching row must surface as the sentinel, not as a generic error, so the
// service layer can turn it into a 404.
func TestFreshnessRuleRepository_Update_NoRowReturnsSentinel(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`UPDATE freshness_rules`).WillReturnError(sql.ErrNoRows)

	err := repo.Update(context.Background(), &models.FreshnessRule{TeamID: "team-1", ID: "rule-1"})
	require.Error(t, err)
	assert.ErrorIs(t, err, repositories.ErrFreshnessRuleNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessRuleRepository_Update_Error(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectQuery(`UPDATE freshness_rules`).WillReturnError(errors.New("boom"))

	err := repo.Update(context.Background(), &models.FreshnessRule{})
	require.Error(t, err)
	assert.NotErrorIs(t, err, repositories.ErrFreshnessRuleNotFound)
	assert.Contains(t, err.Error(), "failed to update freshness rule")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessRuleRepository_Delete(t *testing.T) {
	tests := []struct {
		name        string
		affected    int64
		wantDeleted bool
	}{
		{name: "rule removed", affected: 1, wantDeleted: true},
		{name: "no such rule in team", affected: 0, wantDeleted: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupFreshnessRuleTest(t)

			mock.ExpectExec(`DELETE FROM freshness_rules WHERE team_id = \$1 AND id = \$2`).
				WithArgs("team-1", "rule-1").
				WillReturnResult(sqlmock.NewResult(0, tt.affected))

			deleted, err := repo.Delete(context.Background(), "team-1", "rule-1")

			require.NoError(t, err)
			assert.Equal(t, tt.wantDeleted, deleted)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestFreshnessRuleRepository_Delete_Error(t *testing.T) {
	repo, mock := setupFreshnessRuleTest(t)

	mock.ExpectExec(`DELETE FROM freshness_rules`).WillReturnError(errors.New("boom"))

	_, err := repo.Delete(context.Background(), "team-1", "rule-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete freshness rule")
	assert.NoError(t, mock.ExpectationsWereMet())
}

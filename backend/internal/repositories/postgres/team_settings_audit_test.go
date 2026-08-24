package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// teamSettingsAuditCols mirrors the teamSettingsAuditColumns projection order.
// Unlike the freshness audit there is only ONE list: Append's RETURNING and
// ListByTeam's SELECT read the same columns, because nothing on this entry is
// resolved by joining a live row.
func teamSettingsAuditCols() []string {
	return []string{
		"id", "team_id", "actor_user_id", "surface", "source_team_id",
		"source_resource_id", "created_resource_id", "detail", "created_at",
	}
}

func setupTeamSettingsAuditTest(t *testing.T) (*TeamSettingsAuditRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewTeamSettingsAuditRepository(&database.DB{DB: mockDB})
	return repo.(*TeamSettingsAuditRepository), mock
}

// teamSettingsAuditRow builds one row in the canonical column order. Any of the
// optional references may be nil, which is how a custom-types copy and a
// deleted actor are modelled.
func teamSettingsAuditRow(
	id string, actorUserID, sourceTeamID, sourceResourceID, createdResourceID interface{},
	surface string, detail string, createdAt time.Time,
) *sqlmock.Rows {
	return sqlmock.NewRows(teamSettingsAuditCols()).AddRow(
		id, "team-1", actorUserID, surface, sourceTeamID,
		sourceResourceID, createdResourceID, []byte(detail), createdAt,
	)
}

func TestTeamSettingsAuditRepository_Append_Provider(t *testing.T) {
	repo, mock := setupTeamSettingsAuditTest(t)

	detail := json.RawMessage(`{"name":"OpenAI (copy)"}`)
	mock.ExpectQuery(`INSERT INTO team_settings_audit`).
		WithArgs("team-1", strPtr("user-1"), models.SettingsAuditSurfaceModelProvider,
			strPtr("team-2"), strPtr("src-1"), strPtr("new-1"), detail).
		WillReturnRows(teamSettingsAuditRow(
			"audit-1", "user-1", "team-2", "src-1", "new-1",
			models.SettingsAuditSurfaceModelProvider, `{"name":"OpenAI (copy)"}`,
			time.Now().UTC(),
		))

	entry := &models.TeamSettingsAudit{
		TeamID:            "team-1",
		ActorUserID:       strPtr("user-1"),
		Surface:           models.SettingsAuditSurfaceModelProvider,
		SourceTeamID:      strPtr("team-2"),
		SourceResourceID:  strPtr("src-1"),
		CreatedResourceID: strPtr("new-1"),
		Detail:            detail,
	}

	require.NoError(t, repo.Append(context.Background(), entry))
	assert.Equal(t, "audit-1", entry.ID)
	assert.Equal(t, "team-1", entry.TeamID)
	assert.Equal(t, models.SettingsAuditSurfaceModelProvider, entry.Surface)
	assert.Equal(t, strPtr("team-2"), entry.SourceTeamID)
	assert.False(t, entry.CreatedAt.IsZero())
	assert.NoError(t, mock.ExpectationsWereMet())
}

// An absent Detail must be sent as an empty JSON OBJECT, not as NULL: the
// column is NOT NULL, and a nil json.RawMessage binds as NULL rather than
// falling back to the column default, so without the substitution every entry
// written with no detail would be rejected.
func TestTeamSettingsAuditRepository_Append_NilDetailBecomesEmptyObject(t *testing.T) {
	repo, mock := setupTeamSettingsAuditTest(t)

	mock.ExpectQuery(`INSERT INTO team_settings_audit`).
		WithArgs("team-1", strPtr("user-1"), models.SettingsAuditSurfaceCustomTypes,
			strPtr("team-2"), nil, nil, json.RawMessage(`{}`)).
		WillReturnRows(teamSettingsAuditRow(
			"audit-2", "user-1", "team-2", nil, nil,
			models.SettingsAuditSurfaceCustomTypes, `{}`, time.Now().UTC(),
		))

	entry := &models.TeamSettingsAudit{
		TeamID:       "team-1",
		ActorUserID:  strPtr("user-1"),
		Surface:      models.SettingsAuditSurfaceCustomTypes,
		SourceTeamID: strPtr("team-2"),
	}

	require.NoError(t, repo.Append(context.Background(), entry))
	assert.Equal(t, "audit-2", entry.ID)
	assert.Nil(t, entry.SourceResourceID)
	assert.Nil(t, entry.CreatedResourceID)
	assert.JSONEq(t, `{}`, string(entry.Detail))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A deleted actor leaves actor_user_id NULL (ON DELETE SET NULL), so the scan
// must tolerate it — the entry outliving its actor is the whole reason the
// column is not a cascade.
func TestTeamSettingsAuditRepository_Append_NullActorScans(t *testing.T) {
	repo, mock := setupTeamSettingsAuditTest(t)

	mock.ExpectQuery(`INSERT INTO team_settings_audit`).
		WithArgs("team-1", nil, models.SettingsAuditSurfaceEmbeddingProvider,
			strPtr("team-2"), nil, nil, json.RawMessage(`{}`)).
		WillReturnRows(teamSettingsAuditRow(
			"audit-3", nil, "team-2", nil, nil,
			models.SettingsAuditSurfaceEmbeddingProvider, `{}`, time.Now().UTC(),
		))

	entry := &models.TeamSettingsAudit{
		TeamID:       "team-1",
		Surface:      models.SettingsAuditSurfaceEmbeddingProvider,
		SourceTeamID: strPtr("team-2"),
	}

	require.NoError(t, repo.Append(context.Background(), entry))
	assert.Nil(t, entry.ActorUserID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSettingsAuditRepository_Append_Error(t *testing.T) {
	repo, mock := setupTeamSettingsAuditTest(t)

	mock.ExpectQuery(`INSERT INTO team_settings_audit`).
		WillReturnError(errors.New("insert boom"))

	err := repo.Append(context.Background(), &models.TeamSettingsAudit{
		TeamID:       "team-1",
		Surface:      models.SettingsAuditSurfaceModelProvider,
		SourceTeamID: strPtr("team-2"),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to append team settings audit entry")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The page is ordered `created_at DESC, id DESC`, and the id tiebreaker is
// load-bearing: `now()` is transaction-start time, so a copy action writing
// several entries stamps them all identically. This pins both the ordering in
// the SQL and that the rows come back in the order the database returned them.
func TestTeamSettingsAuditRepository_ListByTeam_PagedOrdering(t *testing.T) {
	repo, mock := setupTeamSettingsAuditTest(t)

	stamp := time.Now().UTC()
	rows := sqlmock.NewRows(teamSettingsAuditCols()).
		AddRow("audit-9", "team-1", "user-1", models.SettingsAuditSurfaceModelProvider,
			"team-2", "src-2", "new-2", []byte(`{}`), stamp).
		AddRow("audit-8", "team-1", "user-1", models.SettingsAuditSurfaceCustomTypes,
			"team-2", nil, nil, []byte(`{"copied":3}`), stamp)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_settings_audit WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))
	mock.ExpectQuery(`FROM team_settings_audit\s+WHERE team_id = \$1\s+ORDER BY created_at DESC, id DESC\s+LIMIT \$2 OFFSET \$3`).
		WithArgs("team-1", 2, 4).
		WillReturnRows(rows)

	entries, total, err := repo.ListByTeam(context.Background(), "team-1", 2, 4)

	require.NoError(t, err)
	assert.Equal(t, 7, total)
	require.Len(t, entries, 2)
	assert.Equal(t, "audit-9", entries[0].ID)
	assert.Equal(t, "audit-8", entries[1].ID)
	assert.Equal(t, stamp, entries[1].CreatedAt, "identical stamps are what the id tiebreaker exists for")
	assert.JSONEq(t, `{"copied":3}`, string(entries[1].Detail))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A zero limit means "no limit" and binds NULL, so the query shape stays static
// instead of needing a second statement without a LIMIT clause. A negative
// offset is clamped to 0, which Postgres would otherwise reject outright.
func TestTeamSettingsAuditRepository_ListByTeam_ClampsPaging(t *testing.T) {
	repo, mock := setupTeamSettingsAuditTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_settings_audit`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`FROM team_settings_audit`).
		WithArgs("team-1", nil, 0).
		WillReturnRows(sqlmock.NewRows(teamSettingsAuditCols()))

	entries, total, err := repo.ListByTeam(context.Background(), "team-1", 0, -5)

	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Equal(t, []*models.TeamSettingsAudit{}, entries, "an empty page is [] and never nil")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSettingsAuditRepository_ListByTeam_CountError(t *testing.T) {
	repo, mock := setupTeamSettingsAuditTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_settings_audit`).
		WithArgs("team-1").
		WillReturnError(errors.New("count boom"))

	entries, total, err := repo.ListByTeam(context.Background(), "team-1", 10, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count team settings audit entries")
	assert.Nil(t, entries)
	assert.Zero(t, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSettingsAuditRepository_ListByTeam_QueryError(t *testing.T) {
	repo, mock := setupTeamSettingsAuditTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_settings_audit`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM team_settings_audit`).
		WillReturnError(errors.New("select boom"))

	_, _, err := repo.ListByTeam(context.Background(), "team-1", 10, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list team settings audit entries")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSettingsAuditRepository_ListByTeam_ScanError(t *testing.T) {
	repo, mock := setupTeamSettingsAuditTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_settings_audit`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM team_settings_audit`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("audit-1"))

	_, _, err := repo.ListByTeam(context.Background(), "team-1", 10, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan team settings audit entry")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTeamSettingsAuditRepository_ListByTeam_RowsError(t *testing.T) {
	repo, mock := setupTeamSettingsAuditTest(t)

	rows := sqlmock.NewRows(teamSettingsAuditCols()).
		AddRow("audit-1", "team-1", nil, models.SettingsAuditSurfaceCustomTypes,
			"team-2", nil, nil, []byte(`{}`), time.Now().UTC()).
		RowError(0, errors.New("iterate boom"))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM team_settings_audit`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM team_settings_audit`).
		WillReturnRows(rows)

	_, _, err := repo.ListByTeam(context.Background(), "team-1", 10, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to iterate team settings audit entries")
	assert.NoError(t, mock.ExpectationsWereMet())
}

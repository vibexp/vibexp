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
	"github.com/vibexp/vibexp/internal/repositories"
)

// instanceSettingsAuditLeakSentinel stands in for a real credential. Fixtures
// are built from Go maps through json.Marshal rather than as JSON literals, so
// gitleaks does not read `"secret":"<value>"` in this file as a leaked key.
const instanceSettingsAuditLeakSentinel = "plaintext-sentinel-value"

// instanceSettingsAuditCols mirrors the instanceSettingsAuditColumns order.
func instanceSettingsAuditCols() []string {
	return []string{"id", "setting", "action", "actor_user_id", "before", "after", "created_at"}
}

func setupInstanceSettingsAuditTest(t *testing.T) (*InstanceSettingsAuditRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewInstanceSettingsAuditRepository(&database.DB{DB: mockDB})
	return repo.(*InstanceSettingsAuditRepository), mock
}

func mustInstanceSettingsAuditJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	doc, err := json.Marshal(v)
	require.NoError(t, err)
	return doc
}

func TestInstanceSettingsAuditRepository_Append(t *testing.T) {
	repo, mock := setupInstanceSettingsAuditTest(t)

	before := mustInstanceSettingsAuditJSON(t, map[string]interface{}{
		"from_address": "old@example.com", "secret": models.InstanceSettingsAuditSecretUnchanged,
	})
	after := mustInstanceSettingsAuditJSON(t, map[string]interface{}{
		"from_address": "new@example.com", "secret": models.InstanceSettingsAuditSecretChanged,
	})
	stamp := time.Now().UTC()

	mock.ExpectQuery(`INSERT INTO instance_settings_audit`).
		WithArgs(models.InstanceSettingEmailProvider, models.InstanceSettingsAuditActionUpsert,
			strPtr("user-1"), []byte(before), []byte(after)).
		WillReturnRows(sqlmock.NewRows(instanceSettingsAuditCols()).AddRow(
			"audit-1", models.InstanceSettingEmailProvider, models.InstanceSettingsAuditActionUpsert,
			"user-1", []byte(before), []byte(after), stamp))

	entry := &models.InstanceSettingsAuditEntry{
		Setting:     models.InstanceSettingEmailProvider,
		Action:      models.InstanceSettingsAuditActionUpsert,
		ActorUserID: strPtr("user-1"),
		Before:      before,
		After:       after,
	}

	require.NoError(t, repo.Append(context.Background(), entry))
	assert.Equal(t, "audit-1", entry.ID)
	assert.Equal(t, stamp, entry.CreatedAt)
	assert.JSONEq(t, string(after), string(entry.After))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The boot-time import has no actor and no prior state: both bind as SQL NULL.
// An empty-but-non-nil snapshot must bind as NULL too, since jsonb rejects an empty string.
func TestInstanceSettingsAuditRepository_Append_ImportWithoutActorOrBefore(t *testing.T) {
	repo, mock := setupInstanceSettingsAuditTest(t)

	after := mustInstanceSettingsAuditJSON(t, map[string]interface{}{"provider_type": "smtp"})
	mock.ExpectQuery(`INSERT INTO instance_settings_audit`).
		WithArgs(models.InstanceSettingEmailProvider, models.InstanceSettingsAuditActionImport,
			nil, nil, []byte(after)).
		WillReturnRows(sqlmock.NewRows(instanceSettingsAuditCols()).AddRow(
			"audit-2", models.InstanceSettingEmailProvider, models.InstanceSettingsAuditActionImport,
			nil, nil, []byte(after), time.Now().UTC()))

	entry := &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingEmailProvider,
		Action:  models.InstanceSettingsAuditActionImport,
		Before:  json.RawMessage{},
		After:   after,
	}

	require.NoError(t, repo.Append(context.Background(), entry))
	assert.Nil(t, entry.ActorUserID)
	assert.Nil(t, entry.Before)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSettingsAuditRepository_Append_Error(t *testing.T) {
	repo, mock := setupInstanceSettingsAuditTest(t)

	mock.ExpectQuery(`INSERT INTO instance_settings_audit`).
		WillReturnError(errors.New("insert boom"))

	err := repo.Append(context.Background(), &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingEmailProvider,
		Action:  models.InstanceSettingsAuditActionDelete,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to append instance settings audit entry")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A snapshot carrying a credential is rejected BEFORE any query: the mock has
// no expectation, so a query reaching it would fail ExpectationsWereMet.
func TestInstanceSettingsAuditRepository_Append_RejectsUnredactedSecret(t *testing.T) {
	cases := map[string]interface{}{
		"plaintext secret":           map[string]interface{}{"secret": instanceSettingsAuditLeakSentinel},
		"plaintext secret_encrypted": map[string]interface{}{"secret_encrypted": instanceSettingsAuditLeakSentinel},
		"differently cased key":      map[string]interface{}{"Secret": instanceSettingsAuditLeakSentinel},
		"plaintext client_secret":    map[string]interface{}{"client_secret": instanceSettingsAuditLeakSentinel},
		"plaintext client_secret_encrypted": map[string]interface{}{
			"client_secret_encrypted": instanceSettingsAuditLeakSentinel,
		},
		"setup token_hash": map[string]interface{}{"token_hash": instanceSettingsAuditLeakSentinel},
		"nested under settings": map[string]interface{}{
			"settings": map[string]interface{}{"secret": instanceSettingsAuditLeakSentinel},
		},
		"inside an array": map[string]interface{}{
			"providers": []interface{}{map[string]interface{}{"secret_encrypted": instanceSettingsAuditLeakSentinel}},
		},
		"non-string value": map[string]interface{}{"secret": map[string]interface{}{"value": "x"}},
		"null value":       map[string]interface{}{"secret": nil},
		"marker-like text": map[string]interface{}{"secret": "Changed"},
	}

	for name, snapshot := range cases {
		for _, side := range []string{"before", "after"} {
			t.Run(name+"/"+side, func(t *testing.T) {
				repo, mock := setupInstanceSettingsAuditTest(t)
				doc := mustInstanceSettingsAuditJSON(t, snapshot)

				entry := &models.InstanceSettingsAuditEntry{
					Setting: models.InstanceSettingEmailProvider,
					Action:  models.InstanceSettingsAuditActionUpsert,
				}
				if side == "before" {
					entry.Before = doc
				} else {
					entry.After = doc
				}

				err := repo.Append(context.Background(), entry)

				require.ErrorIs(t, err, repositories.ErrInstanceSettingsAuditUnredacted)
				assert.NotContains(t, err.Error(), instanceSettingsAuditLeakSentinel,
					"the error must name the key, never echo the secret")
				assert.Empty(t, entry.ID, "nothing was written")
				assert.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestInstanceSettingsAuditRepository_Append_RejectsInvalidSnapshot(t *testing.T) {
	repo, mock := setupInstanceSettingsAuditTest(t)

	err := repo.Append(context.Background(), &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingEmailProvider,
		Action:  models.InstanceSettingsAuditActionUpsert,
		After:   json.RawMessage(`{"secret":`),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid instance settings audit snapshot")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// First page: no keyset predicate, and limit+1 rows are requested so the
// repository can tell whether a next page exists.
func TestInstanceSettingsAuditRepository_List_FirstPageWithNextCursor(t *testing.T) {
	repo, mock := setupInstanceSettingsAuditTest(t)

	stamp := time.Now().UTC()
	rows := sqlmock.NewRows(instanceSettingsAuditCols()).
		AddRow("audit-3", models.InstanceSettingEmailProvider, "upsert", "user-1", nil, []byte(`{}`), stamp).
		AddRow("audit-2", models.InstanceSettingEmailProvider, "upsert", "user-1", []byte(`{}`), []byte(`{}`), stamp).
		AddRow("audit-1", models.InstanceSettingEmailProvider, "import", nil, nil, []byte(`{}`), stamp)

	mock.ExpectQuery(`FROM instance_settings_audit\s+WHERE setting = \$1\s+ORDER BY created_at DESC, id DESC\s+LIMIT \$2`).
		WithArgs(models.InstanceSettingEmailProvider, 3).
		WillReturnRows(rows)

	entries, next, err := repo.List(context.Background(), models.InstanceSettingEmailProvider, 2, nil)

	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "audit-3", entries[0].ID)
	assert.Equal(t, "audit-2", entries[1].ID)
	require.NotNil(t, next)
	assert.Equal(t, models.InstanceSettingsAuditCursor{CreatedAt: stamp, ID: "audit-2"}, *next,
		"the cursor points at the last entry RETURNED, not the probe row")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSettingsAuditRepository_List_WithCursorLastPage(t *testing.T) {
	repo, mock := setupInstanceSettingsAuditTest(t)

	stamp := time.Now().UTC()
	cursor := &models.InstanceSettingsAuditCursor{CreatedAt: stamp, ID: "audit-2"}
	mock.ExpectQuery(`WHERE setting = \$1\s+AND \(created_at, id\) < \(\$3::timestamptz, \$4::uuid\)\s+ORDER BY created_at DESC, id DESC\s+LIMIT \$2`).
		WithArgs(models.InstanceSettingEmailProvider, 3, stamp, "audit-2").
		WillReturnRows(sqlmock.NewRows(instanceSettingsAuditCols()).
			AddRow("audit-1", models.InstanceSettingEmailProvider, "import", nil, nil, []byte(`{}`), stamp))

	entries, next, err := repo.List(context.Background(), models.InstanceSettingEmailProvider, 2, cursor)

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Nil(t, next, "no cursor once the log is exhausted")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSettingsAuditRepository_List_ClampsLimit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested int
		fetched   int
	}{
		{"zero means default", 0, instanceSettingsAuditDefaultLimit + 1},
		{"negative means default", -7, instanceSettingsAuditDefaultLimit + 1},
		{"above max is clamped", 10_000, instanceSettingsAuditMaxLimit + 1},
		{"in range is kept", 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := setupInstanceSettingsAuditTest(t)
			mock.ExpectQuery(`FROM instance_settings_audit`).
				WithArgs(models.InstanceSettingEmailProvider, tc.fetched).
				WillReturnRows(sqlmock.NewRows(instanceSettingsAuditCols()))

			entries, next, err := repo.List(context.Background(), models.InstanceSettingEmailProvider, tc.requested, nil)

			require.NoError(t, err)
			assert.Equal(t, []*models.InstanceSettingsAuditEntry{}, entries, "an empty page is [] and never nil")
			assert.Nil(t, next)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestInstanceSettingsAuditRepository_List_QueryError(t *testing.T) {
	repo, mock := setupInstanceSettingsAuditTest(t)

	mock.ExpectQuery(`FROM instance_settings_audit`).WillReturnError(errors.New("select boom"))

	_, _, err := repo.List(context.Background(), models.InstanceSettingEmailProvider, 10, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list instance settings audit entries")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSettingsAuditRepository_List_ScanError(t *testing.T) {
	repo, mock := setupInstanceSettingsAuditTest(t)

	mock.ExpectQuery(`FROM instance_settings_audit`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("audit-1"))

	_, _, err := repo.List(context.Background(), models.InstanceSettingEmailProvider, 10, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan instance settings audit entry")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInstanceSettingsAuditRepository_List_RowsError(t *testing.T) {
	repo, mock := setupInstanceSettingsAuditTest(t)

	rows := sqlmock.NewRows(instanceSettingsAuditCols()).
		AddRow("audit-1", models.InstanceSettingEmailProvider, "import", nil, nil, []byte(`{}`), time.Now().UTC()).
		RowError(0, errors.New("iterate boom"))
	mock.ExpectQuery(`FROM instance_settings_audit`).WillReturnRows(rows)

	_, _, err := repo.List(context.Background(), models.InstanceSettingEmailProvider, 10, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to iterate instance settings audit entries")
	assert.NoError(t, mock.ExpectationsWereMet())
}

package postgres

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// newGitHubInstallationMockRepo wires the repository under test to a sqlmock
// connection. GitHubInstallationRepository holds a raw *sql.DB (not the
// database.DB wrapper), so the mock connection goes straight into the
// constructor.
func newGitHubInstallationMockRepo(t *testing.T) (repositories.GitHubInstallationRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	return NewGitHubInstallationRepository(mockDB, slog.New(slog.DiscardHandler)), mock, mockDB
}

// githubInstallationColumns mirrors the 13-column projection scanned by
// GetByTeamID and GetByInstallationID.
var githubInstallationColumns = []string{
	"id", "team_id", "installation_id", "account_login", "account_type", "target_type",
	"encrypted_access_token", "token_expires_at", "permissions", "events", "suspended_at",
	"created_at", "updated_at",
}

// sampleGitHubInstallation is the fixture shared by the sqlmock tests; the
// permissions map round-trips as JSON and the events slice through pq.Array.
func sampleGitHubInstallation() *models.GitHubInstallation {
	return &models.GitHubInstallation{
		ID:                   "8c6f2c1e-0b0a-4a1e-9f7d-1c2d3e4f5a6b",
		TeamID:               "team-1",
		InstallationID:       4242,
		AccountLogin:         "octo-org",
		AccountType:          "Organization",
		TargetType:           "organization",
		EncryptedAccessToken: "enc-token",
		TokenExpiresAt:       time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
		Permissions:          map[string]interface{}{"contents": "read", "metadata": "read"},
		Events:               []string{"push", "pull_request"},
	}
}

// unmarshalablePermissions cannot be serialized by encoding/json, forcing the
// marshal-error path before any SQL is issued.
func unmarshalablePermissions() map[string]interface{} {
	return map[string]interface{}{"bad": make(chan int)}
}

func TestGitHubInstallationRepository_Create(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 30, 0, 0, time.UTC)

	t.Run("success writes DB-assigned timestamps back onto the struct", func(t *testing.T) {
		repo, mock, mockDB := newGitHubInstallationMockRepo(t)
		defer closeMockDB(t, mockDB)

		inst := sampleGitHubInstallation()
		mock.ExpectQuery(`INSERT INTO github_installations`).
			WithArgs(
				inst.ID, inst.TeamID, inst.InstallationID, inst.AccountLogin, inst.AccountType,
				inst.TargetType, inst.EncryptedAccessToken, inst.TokenExpiresAt,
				[]byte(`{"contents":"read","metadata":"read"}`), pq.Array(inst.Events), nil,
			).
			WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).
				AddRow(now, now.Add(time.Minute)))

		require.NoError(t, repo.Create(context.Background(), inst))
		assert.Equal(t, now, inst.CreatedAt)
		assert.Equal(t, now.Add(time.Minute), inst.UpdatedAt)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("driver error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := newGitHubInstallationMockRepo(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`INSERT INTO github_installations`).WillReturnError(sql.ErrConnDone)

		err := repo.Create(context.Background(), sampleGitHubInstallation())
		require.Error(t, err)
		assert.ErrorIs(t, err, sql.ErrConnDone)
		assert.Contains(t, err.Error(), "failed to create GitHub installation")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unmarshalable permissions fail before hitting the DB", func(t *testing.T) {
		repo, mock, mockDB := newGitHubInstallationMockRepo(t)
		defer closeMockDB(t, mockDB)

		inst := sampleGitHubInstallation()
		inst.Permissions = unmarshalablePermissions()

		err := repo.Create(context.Background(), inst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to marshal permissions")
		assert.NoError(t, mock.ExpectationsWereMet(), "no query must reach the DB")
	})
}

func TestGitHubInstallationRepository_Get(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 30, 0, 0, time.UTC)
	expires := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	happyRows := func() *sqlmock.Rows {
		return sqlmock.NewRows(githubInstallationColumns).AddRow(
			"inst-1", "team-1", int64(4242), "octo-org", "Organization", "organization",
			"enc-token", expires, []byte(`{"contents":"read"}`), []byte(`{push,pull_request}`),
			nil, now, now,
		)
	}
	badPermissionsRows := func() *sqlmock.Rows {
		return sqlmock.NewRows(githubInstallationColumns).AddRow(
			"inst-1", "team-1", int64(4242), "octo-org", "Organization", "organization",
			"enc-token", expires, []byte(`{not json`), []byte(`{push}`),
			nil, now, now,
		)
	}

	methods := []struct {
		name    string
		pattern string
		arg     interface{}
		call    func(repo repositories.GitHubInstallationRepository) (*models.GitHubInstallation, error)
	}{
		{
			name:    "GetByTeamID",
			pattern: `FROM github_installations\s+WHERE team_id = \$1`,
			arg:     "team-1",
			call: func(repo repositories.GitHubInstallationRepository) (*models.GitHubInstallation, error) {
				return repo.GetByTeamID(context.Background(), "team-1")
			},
		},
		{
			name:    "GetByInstallationID",
			pattern: `FROM github_installations\s+WHERE installation_id = \$1`,
			arg:     int64(4242),
			call: func(repo repositories.GitHubInstallationRepository) (*models.GitHubInstallation, error) {
				return repo.GetByInstallationID(context.Background(), 4242)
			},
		},
	}

	scenarios := []struct {
		name     string
		rows     func() *sqlmock.Rows
		queryErr error
		wantIs   error
		wantSub  string
		wantOK   bool
	}{
		{name: "happy path round-trips permissions JSON and events array", rows: happyRows, wantOK: true},
		{name: "no rows maps to the not-found sentinel", queryErr: sql.ErrNoRows, wantIs: repositories.ErrGitHubInstallationNotFound},
		{name: "driver error is wrapped", queryErr: sql.ErrConnDone, wantIs: sql.ErrConnDone, wantSub: "failed to get GitHub installation"},
		{name: "invalid permissions JSON fails unmarshal", rows: badPermissionsRows, wantSub: "failed to unmarshal permissions"},
	}

	for _, m := range methods {
		for _, sc := range scenarios {
			t.Run(m.name+"/"+sc.name, func(t *testing.T) {
				repo, mock, mockDB := newGitHubInstallationMockRepo(t)
				defer closeMockDB(t, mockDB)

				exp := mock.ExpectQuery(m.pattern).WithArgs(m.arg)
				if sc.queryErr != nil {
					exp.WillReturnError(sc.queryErr)
				} else {
					exp.WillReturnRows(sc.rows())
				}

				got, err := m.call(repo)
				if sc.wantOK {
					require.NoError(t, err)
					assert.Equal(t, "inst-1", got.ID)
					assert.Equal(t, "team-1", got.TeamID)
					assert.Equal(t, int64(4242), got.InstallationID)
					assert.Equal(t, "octo-org", got.AccountLogin)
					assert.Equal(t, "Organization", got.AccountType)
					assert.Equal(t, "organization", got.TargetType)
					assert.Equal(t, "enc-token", got.EncryptedAccessToken)
					assert.Equal(t, expires, got.TokenExpiresAt)
					assert.Equal(t, map[string]interface{}{"contents": "read"}, got.Permissions)
					assert.Equal(t, []string{"push", "pull_request"}, got.Events)
					assert.Nil(t, got.SuspendedAt)
				} else {
					require.Error(t, err)
					assert.Nil(t, got)
					if sc.wantIs != nil {
						assert.ErrorIs(t, err, sc.wantIs)
					}
					if sc.wantSub != "" {
						assert.Contains(t, err.Error(), sc.wantSub)
					}
				}
				assert.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestGitHubInstallationRepository_Update(t *testing.T) {
	cases := []struct {
		name        string
		permissions map[string]interface{}
		setupMock   func(mock sqlmock.Sqlmock, inst *models.GitHubInstallation)
		wantIs      error
		wantSub     string
	}{
		{
			name: "success with matching args",
			setupMock: func(mock sqlmock.Sqlmock, inst *models.GitHubInstallation) {
				mock.ExpectExec(`UPDATE github_installations`).
					WithArgs(
						inst.InstallationID, inst.AccountLogin, inst.AccountType, inst.TargetType,
						inst.EncryptedAccessToken, inst.TokenExpiresAt,
						[]byte(`{"contents":"read","metadata":"read"}`), pq.Array(inst.Events),
						nil, inst.ID,
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name: "zero rows affected maps to the not-found sentinel",
			setupMock: func(mock sqlmock.Sqlmock, _ *models.GitHubInstallation) {
				mock.ExpectExec(`UPDATE github_installations`).WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantIs: repositories.ErrGitHubInstallationNotFound,
		},
		{
			name: "driver error is wrapped",
			setupMock: func(mock sqlmock.Sqlmock, _ *models.GitHubInstallation) {
				mock.ExpectExec(`UPDATE github_installations`).WillReturnError(sql.ErrConnDone)
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to update GitHub installation",
		},
		{
			name: "rows-affected error is wrapped",
			setupMock: func(mock sqlmock.Sqlmock, _ *models.GitHubInstallation) {
				mock.ExpectExec(`UPDATE github_installations`).
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to get rows affected",
		},
		{
			name:        "unmarshalable permissions fail before hitting the DB",
			permissions: unmarshalablePermissions(),
			setupMock:   func(_ sqlmock.Sqlmock, _ *models.GitHubInstallation) {},
			wantSub:     "failed to marshal permissions",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, mockDB := newGitHubInstallationMockRepo(t)
			defer closeMockDB(t, mockDB)

			inst := sampleGitHubInstallation()
			if tc.permissions != nil {
				inst.Permissions = tc.permissions
			}
			tc.setupMock(mock, inst)

			err := repo.Update(context.Background(), inst)
			if tc.wantIs == nil && tc.wantSub == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				if tc.wantIs != nil {
					assert.ErrorIs(t, err, tc.wantIs)
				}
				if tc.wantSub != "" {
					assert.Contains(t, err.Error(), tc.wantSub)
				}
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGitHubInstallationRepository_Delete(t *testing.T) {
	deletePattern := `DELETE FROM github_installations WHERE team_id = \$1`

	cases := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantIs    error
		wantSub   string
	}{
		{
			name: "success",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deletePattern).WithArgs("team-1").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name: "zero rows affected maps to the not-found sentinel",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deletePattern).WithArgs("team-1").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantIs: repositories.ErrGitHubInstallationNotFound,
		},
		{
			name: "driver error is wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deletePattern).WithArgs("team-1").WillReturnError(sql.ErrConnDone)
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to delete GitHub installation",
		},
		{
			name: "rows-affected error is wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deletePattern).WithArgs("team-1").
					WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to get rows affected",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, mockDB := newGitHubInstallationMockRepo(t)
			defer closeMockDB(t, mockDB)

			tc.setupMock(mock)

			err := repo.Delete(context.Background(), "team-1")
			if tc.wantIs == nil && tc.wantSub == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				if tc.wantIs != nil {
					assert.ErrorIs(t, err, tc.wantIs)
				}
				if tc.wantSub != "" {
					assert.Contains(t, err.Error(), tc.wantSub)
				}
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

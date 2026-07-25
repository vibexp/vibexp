package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupMetadataCatalogTest(t *testing.T) (*MetadataCatalogRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewMetadataCatalogRepository(&database.DB{DB: mockDB}).(*MetadataCatalogRepository)
	return repo, mock, mockDB
}

// metadataCatalogTenancyArgs is the argument prefix every catalog query binds:
// team_id, then the team/user pair repeated per EXISTS clause. It is the same
// predicate the corresponding List query uses — a catalog that carried a weaker
// one would leak key names across teams (the #517 bug class).
func metadataCatalogTenancyArgs() []driver.Value {
	return []driver.Value{"team-123", "team-123", "user-123", "team-123", "user-123"}
}

func metadataCatalogQuery() repositories.MetadataCatalogQuery {
	return repositories.MetadataCatalogQuery{
		UserID:       "user-123",
		TeamID:       "team-123",
		ResourceType: repositories.MetadataResourceBlueprints,
		Limit:        10,
	}
}

func TestMetadataCatalogRepository_Keys_CarriesTheListTenancyPredicate(t *testing.T) {
	repo, mock, mockDB := setupMetadataCatalogTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(
		`SELECT DISTINCT k AS entry FROM blueprints t ` +
			`CROSS JOIN LATERAL jsonb_object_keys\(t\.metadata\) AS k ` +
			`WHERE \(t\.team_id = \$1 ` +
			`AND \(EXISTS \(SELECT 1 FROM teams WHERE id = \$2 AND owner_id = \$3\) ` +
			`OR EXISTS \(SELECT 1 FROM team_members WHERE team_id = \$4 AND user_id = \$5\)\) ` +
			`AND jsonb_typeof\(t\.metadata\) = 'object'\) ORDER BY entry LIMIT 11`,
	).
		WithArgs(metadataCatalogTenancyArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"entry"}).AddRow("env").AddRow("owner"))

	result, err := repo.Keys(context.Background(), metadataCatalogQuery())

	require.NoError(t, err)
	assert.Equal(t, []string{"env", "owner"}, result.Entries)
	assert.False(t, result.Truncated)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataCatalogRepository_Values_CarriesTheListTenancyPredicate(t *testing.T) {
	repo, mock, mockDB := setupMetadataCatalogTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	query := metadataCatalogQuery()
	query.Key = "env"

	// squirrel binds in clause order — JOIN before WHERE — so the key's three
	// binds in the LATERAL coercion come first, then the tenancy args, then the
	// key's two WHERE guards.
	args := append([]driver.Value{"env", "env", "env"}, metadataCatalogTenancyArgs()...)
	args = append(args, "env", "env")

	mock.ExpectQuery(
		`SELECT DISTINCT v AS entry FROM blueprints t ` +
			`CROSS JOIN LATERAL jsonb_array_elements_text\(` +
			`CASE WHEN jsonb_typeof\(t\.metadata -> \$1\) = 'array' THEN t\.metadata -> \$2 ` +
			`ELSE jsonb_build_array\(t\.metadata -> \$3\) END\) AS v ` +
			`WHERE \(t\.team_id = \$4 ` +
			`AND \(EXISTS \(SELECT 1 FROM teams WHERE id = \$5 AND owner_id = \$6\) ` +
			`OR EXISTS \(SELECT 1 FROM team_members WHERE team_id = \$7 AND user_id = \$8\)\) ` +
			`AND jsonb_typeof\(t\.metadata\) = 'object' ` +
			`AND jsonb_exists\(t\.metadata, \$9\) ` +
			`AND jsonb_typeof\(t\.metadata -> \$10\) <> 'object' ` +
			`AND v IS NOT NULL\) ORDER BY entry LIMIT 11`,
	).
		WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"entry"}).AddRow("prod"))

	result, err := repo.Values(context.Background(), query)

	require.NoError(t, err)
	assert.Equal(t, []string{"prod"}, result.Entries)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataCatalogRepository_ProjectNarrowing(t *testing.T) {
	repo, mock, mockDB := setupMetadataCatalogTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	projectID := "project-9"
	query := metadataCatalogQuery()
	query.ProjectID = &projectID

	mock.ExpectQuery(`FROM blueprints t .*AND t\.project_id = \$6`).
		WithArgs(append(metadataCatalogTenancyArgs(), "project-9")...).
		WillReturnRows(sqlmock.NewRows([]string{"entry"}))

	_, err := repo.Keys(context.Background(), query)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataCatalogRepository_ValuesTypeaheadBindsILIKE(t *testing.T) {
	repo, mock, mockDB := setupMetadataCatalogTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	search := "pro"
	query := metadataCatalogQuery()
	query.Key = "env"
	query.Search = &search

	args := append([]driver.Value{"env", "env", "env"}, metadataCatalogTenancyArgs()...)
	args = append(args, "env", "env", "%pro%")

	mock.ExpectQuery(`FROM blueprints t .*AND v ILIKE \$11`).
		WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"entry"}).AddRow("prod"))

	result, err := repo.Values(context.Background(), query)

	require.NoError(t, err)
	assert.Equal(t, []string{"prod"}, result.Entries)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMetadataCatalogRepository_TruncationDropsTheProbeRow verifies the
// limit+1 trick: the extra row sets Truncated without a second count query and
// must not appear in the page.
func TestMetadataCatalogRepository_TruncationDropsTheProbeRow(t *testing.T) {
	repo, mock, mockDB := setupMetadataCatalogTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	query := metadataCatalogQuery()
	query.Limit = 2

	mock.ExpectQuery(`FROM blueprints t .*LIMIT 3`).
		WithArgs(metadataCatalogTenancyArgs()...).
		WillReturnRows(sqlmock.NewRows([]string{"entry"}).AddRow("a").AddRow("b").AddRow("c"))

	result, err := repo.Keys(context.Background(), query)

	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, result.Entries)
	assert.True(t, result.Truncated)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMetadataCatalogRepository_ResolvesEachResourceTypeToItsTable(t *testing.T) {
	tests := []struct {
		resourceType repositories.MetadataResourceType
		wantTable    string
	}{
		{repositories.MetadataResourceArtifacts, "artifacts"},
		{repositories.MetadataResourceBlueprints, "blueprints"},
		{repositories.MetadataResourceMemories, "memories"},
	}

	for _, tt := range tests {
		t.Run(string(tt.resourceType), func(t *testing.T) {
			repo, mock, mockDB := setupMetadataCatalogTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			query := metadataCatalogQuery()
			query.ResourceType = tt.resourceType

			mock.ExpectQuery(`FROM ` + tt.wantTable + ` t`).
				WithArgs(metadataCatalogTenancyArgs()...).
				WillReturnRows(sqlmock.NewRows([]string{"entry"}))

			_, err := repo.Keys(context.Background(), query)

			require.NoError(t, err)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestMetadataCatalogRepository_UnknownResourceTypeBuildsNoSQL is the guard
// that makes interpolating the table name safe: a value outside the closed map
// must be refused before any query is constructed.
func TestMetadataCatalogRepository_UnknownResourceTypeBuildsNoSQL(t *testing.T) {
	for _, name := range []string{"keys", "values"} {
		t.Run(name, func(t *testing.T) {
			repo, mock, mockDB := setupMetadataCatalogTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			query := metadataCatalogQuery()
			query.ResourceType = "artifacts; DROP TABLE artifacts"
			query.Key = "env"

			var err error
			if name == "keys" {
				_, err = repo.Keys(context.Background(), query)
			} else {
				_, err = repo.Values(context.Background(), query)
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown metadata resource type")
			// No query must have been issued.
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMetadataCatalogRepository_ErrorPaths(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "query failure",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM blueprints t`).WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to query metadata keys",
		},
		{
			name: "scan failure",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM blueprints t`).
					WillReturnRows(sqlmock.NewRows([]string{"entry"}).AddRow(nil))
			},
			wantErr: "failed to scan metadata keys",
		},
		{
			name: "iteration failure",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM blueprints t`).
					WillReturnRows(sqlmock.NewRows([]string{"entry"}).AddRow("env").RowError(0, sql.ErrConnDone))
			},
			wantErr: "failed to iterate metadata keys",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupMetadataCatalogTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()
			tt.setupMock(mock)

			_, err := repo.Keys(context.Background(), metadataCatalogQuery())

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestClampMetadataCatalogLimit(t *testing.T) {
	assert.Equal(t, maxMetadataCatalogLimit, clampMetadataCatalogLimit(0))
	assert.Equal(t, maxMetadataCatalogLimit, clampMetadataCatalogLimit(-1))
	assert.Equal(t, maxMetadataCatalogLimit, clampMetadataCatalogLimit(maxMetadataCatalogLimit+1))
	assert.Equal(t, 25, clampMetadataCatalogLimit(25))
}

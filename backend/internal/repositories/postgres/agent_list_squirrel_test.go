package postgres

import (
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories"
)

// agentListColumns mirrors the 14 columns scanned by the List page query.
var agentListColumns = []string{
	"id", "user_id", "team_id", "name", "description", "status",
	"card_url", "agent_card", "last_run", "last_synced_at",
	"total_runs", "success_rate", "created_at", "updated_at",
}

func setupAgentListTest(t *testing.T) (repositories.AgentRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewAgentRepository(&database.DB{DB: mockDB})
	return repo, mock, mockDB
}

// agentOneRow builds a single fully-populated result row for the list projection.
func agentOneRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(agentListColumns).AddRow(
		"agent-1", "user-123", "team-123", "Code Reviewer", "Reviews code",
		"active", nil, nil, nil, nil,
		0, 0.0, now, now,
	)
}

// TestAgentRepository_List_SquirrelMigration exercises the WHERE-clause binding,
// optional filters, ORDER BY allowlist, and the defaulting pagination contract.
// The team-access predicate binds (team_id, team, user, team, user) because
// squirrel emits one positional placeholder per argument; LIMIT/OFFSET are
// literals, never bound args.
//
//nolint:funlen // table-driven test exercising every filter, sort, and pagination shape
func TestAgentRepository_List_SquirrelMigration(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	teamID := "team-123"
	userID := "user-123"
	base := []driver.Value{teamID, teamID, userID, teamID, userID}

	tests := []struct {
		name        string
		filters     repositories.AgentFilters
		countArgs   []driver.Value
		listMatcher string
		listArgs    []driver.Value
	}{
		{
			name:      "baseline team-access predicate, default order and pagination",
			filters:   repositories.AgentFilters{TeamID: teamID, Page: 1, Limit: 10},
			countArgs: base,
			listMatcher: `FROM agents a WHERE \(a\.team_id = \$1 AND ` +
				`\(EXISTS .*teams.*OR EXISTS .*team_members.*\)\) ` +
				`ORDER BY a\.updated_at DESC LIMIT 10 OFFSET 0`,
			listArgs: base,
		},
		{
			name:        "Status filter binds status equality",
			filters:     repositories.AgentFilters{TeamID: teamID, Status: "active", Page: 1, Limit: 10},
			countArgs:   append(append([]driver.Value{}, base...), "active"),
			listMatcher: `FROM agents a WHERE \(.*AND a\.status = \$6\)`,
			listArgs:    append(append([]driver.Value{}, base...), "active"),
		},
		{
			name:        "Search filter binds the pattern twice for name and description",
			filters:     repositories.AgentFilters{TeamID: teamID, Search: "rev", Page: 1, Limit: 10},
			countArgs:   append(append([]driver.Value{}, base...), "%rev%", "%rev%"),
			listMatcher: `FROM agents a WHERE \(.*AND \(a\.name ILIKE \$6 OR a\.description ILIKE \$7\)\)`,
			listArgs:    append(append([]driver.Value{}, base...), "%rev%", "%rev%"),
		},
		{
			name:      "Status and Search combine in order base, status, search, search",
			filters:   repositories.AgentFilters{TeamID: teamID, Status: "active", Search: "rev", Page: 1, Limit: 10},
			countArgs: append(append([]driver.Value{}, base...), "active", "%rev%", "%rev%"),
			listMatcher: `FROM agents a WHERE \(.*AND a\.status = \$6 ` +
				`AND \(a\.name ILIKE \$7 OR a\.description ILIKE \$8\)\)`,
			listArgs: append(append([]driver.Value{}, base...), "active", "%rev%", "%rev%"),
		},
		{
			name:        "invalid SortBy is rejected and falls back to default order",
			filters:     repositories.AgentFilters{TeamID: teamID, SortBy: "name; DROP TABLE agents", Page: 1, Limit: 10},
			countArgs:   base,
			listMatcher: `ORDER BY a\.updated_at DESC`,
			listArgs:    base,
		},
		{
			name: "valid SortBy success_rate desc maps to allowlisted column",
			filters: repositories.AgentFilters{
				TeamID: teamID, SortBy: "success_rate", SortOrder: "desc", Page: 1, Limit: 10,
			},
			countArgs:   base,
			listMatcher: `ORDER BY a\.success_rate DESC`,
			listArgs:    base,
		},
		{
			name:        "valid SortBy name asc maps to allowlisted column ascending",
			filters:     repositories.AgentFilters{TeamID: teamID, SortBy: "name", SortOrder: "asc", Page: 1, Limit: 10},
			countArgs:   base,
			listMatcher: `ORDER BY a\.name ASC`,
			listArgs:    base,
		},
		{
			name:        "defaulting: zero page and limit yield LIMIT 10 OFFSET 0",
			filters:     repositories.AgentFilters{TeamID: teamID, Page: 0, Limit: 0},
			countArgs:   base,
			listMatcher: `ORDER BY a\.updated_at DESC LIMIT 10 OFFSET 0`,
			listArgs:    base,
		},
		{
			name:        "defaulting: negative page and limit yield LIMIT 10 OFFSET 0",
			filters:     repositories.AgentFilters{TeamID: teamID, Page: -2, Limit: -5},
			countArgs:   base,
			listMatcher: `ORDER BY a\.updated_at DESC LIMIT 10 OFFSET 0`,
			listArgs:    base,
		},
		{
			name:        "pagination: page 3 limit 5 computes LIMIT 5 OFFSET 10",
			filters:     repositories.AgentFilters{TeamID: teamID, Page: 3, Limit: 5},
			countArgs:   base,
			listMatcher: `ORDER BY a\.updated_at DESC LIMIT 5 OFFSET 10`,
			listArgs:    base,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentListTest(t)
			defer closeMockDB(t, mockDB)

			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agents a`).
				WithArgs(tt.countArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery(tt.listMatcher).
				WithArgs(tt.listArgs...).
				WillReturnRows(agentOneRow(now))

			agents, total, err := repo.List(ctx, userID, tt.filters)

			assert.NoError(t, err)
			assert.NotNil(t, agents, "List must return a non-nil empty slice, never nil")
			assert.Len(t, agents, 1)
			assert.Equal(t, 1, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestAgentRepository_List_ExplicitProjection pins the full 14-column projection
// for the default path. A `.+` matcher would not catch column drift, so the
// projection is asserted verbatim.
func TestAgentRepository_List_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupAgentListTest(t)
	defer closeMockDB(t, mockDB)

	ctx := contextWithLogger()
	now := time.Now()
	teamID := "team-123"
	userID := "user-123"
	filters := repositories.AgentFilters{TeamID: teamID, Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agents a`).
		WithArgs(teamID, teamID, userID, teamID, userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(
		`SELECT a\.id, a\.user_id, a\.team_id, a\.name, a\.description, `+
			`a\.status, a\.card_url, a\.agent_card, a\.last_run, `+
			`a\.last_synced_at, a\.total_runs, a\.success_rate, `+
			`a\.created_at, a\.updated_at FROM agents a WHERE`,
	).
		WithArgs(teamID, teamID, userID, teamID, userID).
		WillReturnRows(agentOneRow(now))

	agents, total, err := repo.List(ctx, userID, filters)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, agents, 1)
	assert.Equal(t, "agent-1", agents[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentRepository_List_TeamIDGuard verifies the required-TeamID guard fails
// before any query is issued.
func TestAgentRepository_List_TeamIDGuard(t *testing.T) {
	repo, mock, mockDB := setupAgentListTest(t)
	defer closeMockDB(t, mockDB)

	agents, total, err := repo.List(contextWithLogger(), "user-123", repositories.AgentFilters{Page: 1, Limit: 10})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "TeamID is required but was empty")
	assert.Nil(t, agents)
	assert.Zero(t, total)
	assert.NoError(t, mock.ExpectationsWereMet(), "no queries must be issued when TeamID is empty")
}

// TestAgentRepository_List_CardHandling covers the agent_card WARN-and-continue
// contract and the nullable-column mapping.
//
//nolint:funlen // multiple subtests covering each card and null-column branch
func TestAgentRepository_List_CardHandling(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()
	teamID := "team-123"
	userID := "user-123"
	filters := repositories.AgentFilters{TeamID: teamID, Page: 1, Limit: 10}
	baseArgs := []driver.Value{teamID, teamID, userID, teamID, userID}

	expectQueries := func(mock sqlmock.Sqlmock, row *sqlmock.Rows) {
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agents a`).
			WithArgs(baseArgs...).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`FROM agents a WHERE`).
			WithArgs(baseArgs...).
			WillReturnRows(row)
	}

	t.Run("oversize card JSON keeps the agent without a card", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		oversize := make([]byte, MaxAgentCardJSONSize+1)
		for i := range oversize {
			oversize[i] = 'a'
		}
		row := sqlmock.NewRows(agentListColumns).AddRow(
			"agent-1", userID, teamID, "A", "d", "active",
			nil, oversize, nil, nil, 0, 0.0, now, now,
		)
		expectQueries(mock, row)

		agents, _, err := repo.List(ctx, userID, filters)
		require.NoError(t, err, "oversize card must not surface an error")
		require.Len(t, agents, 1)
		assert.Nil(t, agents[0].AgentCard, "oversize card must be dropped")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("invalid card JSON keeps the agent without a card", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		row := sqlmock.NewRows(agentListColumns).AddRow(
			"agent-1", userID, teamID, "A", "d", "active",
			nil, []byte(`{not valid json`), nil, nil, 0, 0.0, now, now,
		)
		expectQueries(mock, row)

		agents, _, err := repo.List(ctx, userID, filters)
		require.NoError(t, err, "malformed card must not surface an error")
		require.Len(t, agents, 1)
		assert.Nil(t, agents[0].AgentCard, "malformed card must be dropped")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("valid card JSON populates AgentCard", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		row := sqlmock.NewRows(agentListColumns).AddRow(
			"agent-1", userID, teamID, "A", "d", "active",
			nil, []byte(`{"name":"my-card"}`), nil, nil, 0, 0.0, now, now,
		)
		expectQueries(mock, row)

		agents, _, err := repo.List(ctx, userID, filters)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		require.NotNil(t, agents[0].AgentCard)
		assert.Equal(t, "my-card", agents[0].AgentCard.Name)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("NULL nullable columns map to nil pointers", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		row := sqlmock.NewRows(agentListColumns).AddRow(
			"agent-1", userID, teamID, "A", "d", "active",
			nil, nil, nil, nil, 0, 0.0, now, now,
		)
		expectQueries(mock, row)

		agents, _, err := repo.List(ctx, userID, filters)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		assert.Nil(t, agents[0].CardURL)
		assert.Nil(t, agents[0].LastRun)
		assert.Nil(t, agents[0].LastSyncedAt)
		assert.Nil(t, agents[0].AgentCard)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("populated nullable columns map to set pointers", func(t *testing.T) {
		repo, mock, mockDB := setupAgentListTest(t)
		defer closeMockDB(t, mockDB)

		cardURL := "https://example.com/card"
		lastRun := now.Add(-time.Hour)
		lastSynced := now.Add(-2 * time.Hour)
		row := sqlmock.NewRows(agentListColumns).AddRow(
			"agent-1", userID, teamID, "A", "d", "active",
			cardURL, nil, lastRun, lastSynced, 0, 0.0, now, now,
		)
		expectQueries(mock, row)

		agents, _, err := repo.List(ctx, userID, filters)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		require.NotNil(t, agents[0].CardURL)
		assert.Equal(t, cardURL, *agents[0].CardURL)
		require.NotNil(t, agents[0].LastRun)
		assert.WithinDuration(t, lastRun, *agents[0].LastRun, time.Second)
		require.NotNil(t, agents[0].LastSyncedAt)
		assert.WithinDuration(t, lastSynced, *agents[0].LastSyncedAt, time.Second)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestAgentRepository_List_ErrorPaths verifies count/list/scan/iterate failures
// propagate as wrapped errors.
//
//nolint:funlen // table-driven error-path test with multiple scenarios
func TestAgentRepository_List_ErrorPaths(t *testing.T) {
	now := time.Now()
	teamID := "team-123"
	userID := "user-123"
	filters := repositories.AgentFilters{TeamID: teamID, Page: 1, Limit: 10}
	baseArgs := []driver.Value{teamID, teamID, userID, teamID, userID}

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agents a`).
					WithArgs(baseArgs...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count agents",
		},
		{
			name: "list query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agents a`).
					WithArgs(baseArgs...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM agents a WHERE`).
					WithArgs(baseArgs...).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list agents",
		},
		{
			name: "scan error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agents a`).
					WithArgs(baseArgs...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// One column instead of the fourteen the scan expects forces a scan error.
				mock.ExpectQuery(`FROM agents a WHERE`).
					WithArgs(baseArgs...).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("agent-1"))
			},
			wantErr: "failed to scan agent",
		},
		{
			name: "row iteration error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agents a`).
					WithArgs(baseArgs...).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				rows := agentOneRow(now).RowError(0, sql.ErrConnDone)
				mock.ExpectQuery(`FROM agents a WHERE`).
					WithArgs(baseArgs...).
					WillReturnRows(rows)
			},
			wantErr: "failed to iterate agents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentListTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			agents, total, err := repo.List(contextWithLogger(), userID, filters)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, agents)
			assert.Zero(t, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

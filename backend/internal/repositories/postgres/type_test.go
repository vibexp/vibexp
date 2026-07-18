package postgres

// sqlmock unit tests for TypeRepository (type.go): argument wiring, scanning of
// nullable team_id/created_by (global system defaults have NULLs), sentinel
// mapping, and the DeleteCustom transaction with its artifacts-only
// reassignment step. Create must force is_system = FALSE regardless of the
// caller's input.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Fixed timestamp shared by the type fixtures and their assertions.
var typeTestNow = time.Date(2026, 7, 4, 11, 0, 0, 0, time.UTC)

var typeTestColumns = []string{
	"id", "team_id", "resource_type", "slug", "name", "is_system", "created_by", "created_at", "updated_at",
}

func newTypeMockRepo(t *testing.T) (repositories.TypeRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, mockDB := newSquirrelMockRepo(t)
	return NewTypeRepository(db), mock, mockDB
}

// typeSystemRows is a global system default: NULL team_id and created_by.
func typeSystemRows() *sqlmock.Rows {
	return sqlmock.NewRows(typeTestColumns).AddRow(
		"type-1", nil, "artifacts", "bug-report", "Bug report", true, nil, typeTestNow, typeTestNow,
	)
}

// assertTypeError asserts the sentinel and/or message fragment a type-repo
// call must return; both zero values mean "no error expected".
func assertTypeError(t *testing.T, err error, wantIs error, wantSub string) {
	t.Helper()
	if wantIs == nil && wantSub == "" {
		require.NoError(t, err)
		return
	}
	require.Error(t, err)
	if wantIs != nil {
		assert.ErrorIs(t, err, wantIs)
	}
	if wantSub != "" {
		assert.Contains(t, err.Error(), wantSub)
	}
}

// assertSystemType pins the mapping of the system-default fixture row,
// including NULL team_id/created_by scanning to empty strings.
func assertSystemType(t *testing.T, got *models.Type) {
	t.Helper()
	require.NotNil(t, got)
	assert.Equal(t, "type-1", got.ID)
	assert.Empty(t, got.TeamID, "NULL team_id must map to an empty string")
	assert.Equal(t, "artifacts", got.ResourceType)
	assert.Equal(t, "bug-report", got.Slug)
	assert.Equal(t, "Bug report", got.Name)
	assert.True(t, got.IsSystem)
	assert.Empty(t, got.CreatedBy, "NULL created_by must map to an empty string")
	assert.Equal(t, typeTestNow, got.CreatedAt)
	assert.Equal(t, typeTestNow, got.UpdatedAt)
}

type typeCreateScenario struct {
	name      string
	createdBy string
	queryErr  error
	wantIs    error
	wantSub   string
}

func runTypeCreateScenario(t *testing.T, sc typeCreateScenario) {
	t.Helper()
	repo, mock, mockDB := newTypeMockRepo(t)
	defer closeMockDB(t, mockDB)

	// created_by is nullable: an empty creator must be stored as NULL.
	var creatorArg driver.Value
	if sc.createdBy != "" {
		creatorArg = sc.createdBy
	}
	// The pattern pins the contract that is_system is the literal FALSE — a
	// caller can never insert a system type through Create.
	exp := mock.ExpectQuery(`INSERT INTO types .+ VALUES \(\$1, \$2, \$3, \$4, FALSE, \$5\)`).
		WithArgs("team-1", "artifacts", "bug-report", "Bug report", creatorArg)
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow("type-1", typeTestNow, typeTestNow))
	}

	typ := &models.Type{
		TeamID: "team-1", ResourceType: "artifacts", Slug: "bug-report",
		Name: "Bug report", IsSystem: true, CreatedBy: sc.createdBy,
	}
	err := repo.Create(context.Background(), typ)

	assertTypeError(t, err, sc.wantIs, sc.wantSub)
	if sc.queryErr == nil {
		assert.Equal(t, "type-1", typ.ID)
		assert.Equal(t, typeTestNow, typ.CreatedAt)
		assert.False(t, typ.IsSystem, "Create must force IsSystem to false on the model too")
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTypeRepository_Create(t *testing.T) {
	fkErr := &pq.Error{Code: fkViolationCode}
	scenarios := []typeCreateScenario{
		{name: "happy path binds the creator and forces is_system false", createdBy: "user-1"},
		{name: "empty creator is stored as NULL"},
		{
			name:      "unique violation maps to the already-exists sentinel",
			createdBy: "user-1",
			queryErr:  &pq.Error{Code: uniqueViolationCode},
			wantIs:    repositories.ErrTypeAlreadyExists,
		},
		{
			name:      "FK violation maps to team-not-found",
			createdBy: "user-1",
			queryErr:  fkErr,
			wantIs:    fkErr,
			wantSub:   "team not found for type",
		},
		{
			name:      "driver error is wrapped",
			createdBy: "user-1",
			queryErr:  sql.ErrConnDone,
			wantIs:    sql.ErrConnDone,
			wantSub:   "failed to create type",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runTypeCreateScenario(t, sc) })
	}
}

// typeGetPattern pins the global-or-team WHERE clause: a slug resolves to a
// global default (team_id IS NULL) or to the team's own row.
const typeGetPattern = `FROM types WHERE resource_type = \$1 AND slug = \$2 ` +
	`AND \(team_id IS NULL OR team_id = \$3\)`

type typeGetScenario struct {
	name     string
	queryErr error
	wantIs   error
	wantSub  string
}

func runTypeGetScenario(t *testing.T, sc typeGetScenario) {
	t.Helper()
	repo, mock, mockDB := newTypeMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectQuery(typeGetPattern).WithArgs("artifacts", "bug-report", "team-1")
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(typeSystemRows())
	}

	got, err := repo.GetBySlug(context.Background(), "team-1", "artifacts", "bug-report")
	if sc.queryErr == nil {
		require.NoError(t, err)
		assertSystemType(t, got)
	} else {
		assert.Nil(t, got)
		assertTypeError(t, err, sc.wantIs, sc.wantSub)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTypeRepository_GetBySlug(t *testing.T) {
	scenarios := []typeGetScenario{
		{name: "happy path maps a global system row"},
		{name: "no rows maps to the not-found sentinel", queryErr: sql.ErrNoRows, wantIs: repositories.ErrTypeNotFound},
		{
			name:     "driver error is wrapped",
			queryErr: sql.ErrConnDone,
			wantIs:   sql.ErrConnDone,
			wantSub:  "failed to get type by slug",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runTypeGetScenario(t, sc) })
	}
}

// typeListPattern pins the union WHERE and the system-first ordering.
const typeListPattern = `FROM types WHERE resource_type = \$1 AND \(team_id IS NULL OR team_id = \$2\) ` +
	`ORDER BY is_system DESC, name ASC`

type typeListScenario struct {
	name     string
	queryErr error
	rows     func() *sqlmock.Rows
	wantSub  string
	check    func(t *testing.T, types []models.Type)
}

func runTypeListScenario(t *testing.T, sc typeListScenario) {
	t.Helper()
	repo, mock, mockDB := newTypeMockRepo(t)
	defer closeMockDB(t, mockDB)

	exp := mock.ExpectQuery(typeListPattern).WithArgs("artifacts", "team-1")
	if sc.queryErr != nil {
		exp.WillReturnError(sc.queryErr)
	} else {
		exp.WillReturnRows(sc.rows())
	}

	types, err := repo.List(context.Background(), "team-1", "artifacts")
	if sc.wantSub != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), sc.wantSub)
	} else {
		require.NoError(t, err)
		require.NotNil(t, types, "empty result must be a non-nil slice")
		sc.check(t, types)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTypeRepository_List(t *testing.T) {
	scenarios := []typeListScenario{
		{
			name: "happy path maps a system row and a team row",
			rows: func() *sqlmock.Rows {
				return typeSystemRows().AddRow(
					"type-2", "team-1", "artifacts", "meeting-notes", "Meeting notes",
					false, "user-1", typeTestNow, typeTestNow,
				)
			},
			check: func(t *testing.T, types []models.Type) {
				t.Helper()
				require.Len(t, types, 2)
				assertSystemType(t, &types[0])
				assert.Equal(t, "type-2", types[1].ID)
				assert.Equal(t, "team-1", types[1].TeamID)
				assert.Equal(t, "meeting-notes", types[1].Slug)
				assert.False(t, types[1].IsSystem)
				assert.Equal(t, "user-1", types[1].CreatedBy)
			},
		},
		{
			name: "no types returns a non-nil empty slice",
			rows: func() *sqlmock.Rows { return sqlmock.NewRows(typeTestColumns) },
			check: func(t *testing.T, types []models.Type) {
				t.Helper()
				assert.Empty(t, types)
			},
		},
		{name: "query error is wrapped", queryErr: sql.ErrConnDone, wantSub: "failed to list types"},
		{
			name: "scan error is wrapped",
			rows: func() *sqlmock.Rows {
				return sqlmock.NewRows(typeTestColumns).AddRow(
					"type-1", nil, "artifacts", "bug-report", "Bug report",
					"not-a-bool", nil, typeTestNow, typeTestNow,
				)
			},
			wantSub: "failed to scan type",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runTypeListScenario(t, sc) })
	}
}

const (
	typeDeletePattern = `DELETE FROM types WHERE id = \$1 AND team_id = \$2 AND is_system = FALSE ` +
		`RETURNING slug, resource_type`
	typeReassignPattern = `UPDATE artifacts SET type = \$1, updated_at = now\(\) WHERE team_id = \$2 AND type = \$3`
)

// expectTypeDeleteRow expects the transactional DELETE ... RETURNING to hit
// and hand back the deleted row's slug plus the given resource_type.
func expectTypeDeleteRow(mock sqlmock.Sqlmock, resourceType string) {
	mock.ExpectQuery(typeDeletePattern).
		WithArgs("type-1", "team-1").
		WillReturnRows(sqlmock.NewRows([]string{"slug", "resource_type"}).AddRow("old-slug", resourceType))
}

type typeDeleteScenario struct {
	name    string
	setup   func(mock sqlmock.Sqlmock)
	wantIs  error
	wantSub string
}

func runTypeDeleteScenario(t *testing.T, sc typeDeleteScenario) {
	t.Helper()
	repo, mock, mockDB := newTypeMockRepo(t)
	defer closeMockDB(t, mockDB)
	sc.setup(mock)

	err := repo.DeleteCustom(context.Background(), "team-1", "type-1", "general")
	assertTypeError(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTypeRepository_DeleteCustom(t *testing.T) {
	scenarios := []typeDeleteScenario{
		{
			name: "artifacts type reassigns orphaned artifacts to the fallback slug",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				expectTypeDeleteRow(mock, "artifacts")
				mock.ExpectExec(typeReassignPattern).
					WithArgs("general", "team-1", "old-slug").
					WillReturnResult(sqlmock.NewResult(0, 4))
				mock.ExpectCommit()
			},
		},
		{
			name: "non-artifacts type skips the reassignment step",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				expectTypeDeleteRow(mock, "memories")
				mock.ExpectCommit()
			},
		},
		{
			name: "delete matching nothing maps to not-found and rolls back",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(typeDeletePattern).
					WithArgs("type-1", "team-1").
					WillReturnError(sql.ErrNoRows)
				mock.ExpectRollback()
			},
			wantIs: repositories.ErrTypeNotFound,
		},
		{
			name: "reassignment error rolls back",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				expectTypeDeleteRow(mock, "artifacts")
				mock.ExpectExec(typeReassignPattern).
					WithArgs("general", "team-1", "old-slug").
					WillReturnError(sql.ErrConnDone)
				mock.ExpectRollback()
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to reassign artifacts after type delete",
		},
		{
			name: "begin error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(sql.ErrConnDone)
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to begin transaction",
		},
		{
			name: "commit error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				expectTypeDeleteRow(mock, "memories")
				mock.ExpectCommit().WillReturnError(sql.ErrConnDone)
			},
			wantIs:  sql.ErrConnDone,
			wantSub: "failed to commit transaction",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runTypeDeleteScenario(t, sc) })
	}
}

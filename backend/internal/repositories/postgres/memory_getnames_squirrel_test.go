package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryRepository_GetNamesByIDsCrossTeam_Empty(t *testing.T) {
	repo, _, mockDB := setupMemoryListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	result, err := repo.GetNamesByIDsCrossTeam(context.Background(), "user-1", []string{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestMemoryRepository_GetNamesByIDsCrossTeam_Success(t *testing.T) {
	repo, mock, mockDB := setupMemoryListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow("mem-1", "first memory text").
		AddRow("mem-2", "second memory text")

	// IDs bind first ($1, $2), then user-1 binds twice for the two EXISTS clauses.
	mock.ExpectQuery(`SELECT mem\.id, LEFT\(mem\.text, 60\) FROM memories mem WHERE mem\.id IN \(\$1,\$2\)`).
		WithArgs("mem-1", "mem-2", "user-1", "user-1").
		WillReturnRows(rows)

	result, err := repo.GetNamesByIDsCrossTeam(ctx, "user-1", []string{"mem-1", "mem-2"})
	require.NoError(t, err)
	assert.Equal(t, "first memory text", result["mem-1"])
	assert.Equal(t, "second memory text", result["mem-2"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMemoryRepository_GetNamesByIDsCrossTeam_DBError(t *testing.T) {
	repo, mock, mockDB := setupMemoryListTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`SELECT mem\.id, LEFT\(mem\.text, 60\) FROM memories mem`).
		WithArgs("mem-1", "user-1", "user-1").
		WillReturnError(errors.New("db error"))

	_, err := repo.GetNamesByIDsCrossTeam(context.Background(), "user-1", []string{"mem-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get memory names by ids")
	assert.NoError(t, mock.ExpectationsWereMet())
}

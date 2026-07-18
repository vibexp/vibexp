package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
)

func TestAdminRepository_GetInstanceCounts(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewAdminRepository(&database.DB{DB: mockDB})

	rows := sqlmock.NewRows([]string{"users", "teams", "prompts", "artifacts", "memories"}).
		AddRow(42, 12, 340, 128, 512)
	mock.ExpectQuery(`SELECT`).WillReturnRows(rows)

	counts, err := repo.GetInstanceCounts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(42), counts.Users)
	assert.Equal(t, int64(12), counts.Teams)
	assert.Equal(t, int64(340), counts.Prompts)
	assert.Equal(t, int64(128), counts.Artifacts)
	assert.Equal(t, int64(512), counts.Memories)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetInstanceCounts_QueryError(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	repo := NewAdminRepository(&database.DB{DB: mockDB})
	mock.ExpectQuery(`SELECT`).WillReturnError(errors.New("boom"))

	_, err = repo.GetInstanceCounts(context.Background())
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

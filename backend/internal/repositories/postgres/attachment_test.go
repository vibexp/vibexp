package postgres

import (
	"context"
	"database/sql"
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

func setupAttachmentTest(t *testing.T) (*AttachmentRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db := &database.DB{DB: mockDB}
	return NewAttachmentRepository(db).(*AttachmentRepository), mock, mockDB
}

func attachmentRows(now time.Time, userID interface{}) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "team_id", "user_id", "owner_type", "owner_id",
		"file_name", "content_type", "size_bytes", "gcs_object_key", "created_at", "relative_path",
	}).AddRow("att-1", "team-1", userID, "artifact", "owner-1",
		"notes.txt", "text/plain", int64(12), "team-1/artifact/owner-1/uuid-notes.txt", now, nil)
}

func TestAttachmentRepository_Create(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	now := time.Now()
	att := &models.Attachment{
		TeamID:       "team-1",
		UserID:       "user-1",
		OwnerType:    "artifact",
		OwnerID:      "owner-1",
		FileName:     "notes.txt",
		ContentType:  "text/plain",
		SizeBytes:    12,
		GCSObjectKey: "team-1/artifact/owner-1/uuid-notes.txt",
	}

	mock.ExpectQuery("INSERT INTO attachments").
		WithArgs("team-1", "user-1", "artifact", "owner-1", "notes.txt", "text/plain", int64(12),
			"team-1/artifact/owner-1/uuid-notes.txt", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("att-1", now))

	err := repo.Create(context.Background(), att)
	require.NoError(t, err)
	assert.Equal(t, "att-1", att.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAttachmentRepository_Create_WithRelativePath verifies a non-empty
// relative_path is inserted verbatim (#338).
func TestAttachmentRepository_Create_WithRelativePath(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery("INSERT INTO attachments").
		WithArgs("team-1", "user-1", "artifact", "owner-1", "helper.py", "text/x-python", int64(5),
			"k", "scripts/helper.py").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("att-1", time.Now()))

	err := repo.Create(context.Background(), &models.Attachment{
		TeamID: "team-1", UserID: "user-1", OwnerType: "artifact", OwnerID: "owner-1",
		FileName: "helper.py", RelativePath: "scripts/helper.py",
		ContentType: "text/x-python", SizeBytes: 5, GCSObjectKey: "k",
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAttachmentRepository_Create_RelativePathConflict maps the partial-unique
// index violation to ErrAttachmentRelativePathConflict (#338).
func TestAttachmentRepository_Create_RelativePathConflict(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery("INSERT INTO attachments").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "attachments_owner_relative_path_unique"})

	err := repo.Create(context.Background(), &models.Attachment{
		TeamID: "team-1", OwnerType: "artifact", OwnerID: "owner-1",
		FileName: "helper.py", RelativePath: "scripts/helper.py", GCSObjectKey: "k",
	})
	require.ErrorIs(t, err, repositories.ErrAttachmentRelativePathConflict)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAttachmentRepository_Create_NullUser(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	att := &models.Attachment{TeamID: "team-1", OwnerType: "artifact", OwnerID: "owner-1",
		FileName: "n.txt", ContentType: "text/plain", SizeBytes: 1, GCSObjectKey: "k"}

	// Empty UserID must bind SQL NULL, not "".
	mock.ExpectQuery("INSERT INTO attachments").
		WithArgs("team-1", nil, "artifact", "owner-1", "n.txt", "text/plain", int64(1), "k", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("att-2", time.Now()))

	require.NoError(t, repo.Create(context.Background(), att))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAttachmentRepository_GetByID(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	now := time.Now()
	mock.ExpectQuery("SELECT (.+) FROM attachments").
		WithArgs("att-1", "artifact", "owner-1").
		WillReturnRows(attachmentRows(now, "user-1"))

	att, err := repo.GetByID(context.Background(), "artifact", "owner-1", "att-1")
	require.NoError(t, err)
	assert.Equal(t, "att-1", att.ID)
	assert.Equal(t, "user-1", att.UserID)
	assert.Equal(t, int64(12), att.SizeBytes)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAttachmentRepository_GetByIDInTeam(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	now := time.Now()
	// Scoped to (id, team_id) only — owner is read from the row.
	mock.ExpectQuery("SELECT (.+) FROM attachments").
		WithArgs("att-1", "team-1").
		WillReturnRows(attachmentRows(now, "user-1"))

	att, err := repo.GetByIDInTeam(context.Background(), "team-1", "att-1")
	require.NoError(t, err)
	assert.Equal(t, "att-1", att.ID)
	assert.Equal(t, "artifact", att.OwnerType)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAttachmentRepository_GetByIDInTeam_NotFound(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery("SELECT (.+) FROM attachments").
		WithArgs("missing", "team-1").
		WillReturnError(sql.ErrNoRows)

	_, err := repo.GetByIDInTeam(context.Background(), "team-1", "missing")
	assert.ErrorIs(t, err, repositories.ErrAttachmentNotFound)
}

func TestAttachmentRepository_GetByID_NullUser(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery("SELECT (.+) FROM attachments").
		WithArgs("att-1", "artifact", "owner-1").
		WillReturnRows(attachmentRows(time.Now(), nil))

	att, err := repo.GetByID(context.Background(), "artifact", "owner-1", "att-1")
	require.NoError(t, err)
	assert.Equal(t, "", att.UserID)
}

func TestAttachmentRepository_GetByID_NotFound(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery("SELECT (.+) FROM attachments").
		WithArgs("missing", "artifact", "owner-1").
		WillReturnError(sql.ErrNoRows)

	_, err := repo.GetByID(context.Background(), "artifact", "owner-1", "missing")
	assert.ErrorIs(t, err, repositories.ErrAttachmentNotFound)
}

func TestAttachmentRepository_ListByOwner(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "team_id", "user_id", "owner_type", "owner_id",
		"file_name", "content_type", "size_bytes", "gcs_object_key", "created_at", "relative_path",
	}).
		AddRow("a1", "team-1", "user-1", "artifact", "owner-1", "a.txt", "text/plain", int64(10), "k1", now, "scripts/a.txt").
		AddRow("a2", "team-1", nil, "artifact", "owner-1", "b.txt", "text/plain", int64(20), "k2", now, nil)

	mock.ExpectQuery("SELECT (.+) FROM attachments").
		WithArgs("artifact", "owner-1").WillReturnRows(rows)

	list, err := repo.ListByOwner(context.Background(), "artifact", "owner-1")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	assert.Equal(t, "a1", list[0].ID)
	assert.Equal(t, "", list[1].UserID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAttachmentRepository_SumSizeByOwner(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery("SELECT COALESCE\\(SUM").
		WithArgs("artifact", "owner-1").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(int64(1234)))

	total, err := repo.SumSizeByOwner(context.Background(), "artifact", "owner-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1234), total)
}

func TestAttachmentRepository_Delete(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectExec("DELETE FROM attachments").
		WithArgs("att-1", "artifact", "owner-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.Delete(context.Background(), "artifact", "owner-1", "att-1"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAttachmentRepository_Delete_NotFound(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectExec("DELETE FROM attachments").
		WithArgs("missing", "artifact", "owner-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Delete(context.Background(), "artifact", "owner-1", "missing")
	assert.ErrorIs(t, err, repositories.ErrAttachmentNotFound)
}

func TestAttachmentRepository_DeleteByOwner(t *testing.T) {
	repo, mock, mockDB := setupAttachmentTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "team_id", "user_id", "owner_type", "owner_id",
		"file_name", "content_type", "size_bytes", "gcs_object_key", "created_at", "relative_path",
	}).
		AddRow("a1", "team-1", "user-1", "artifact", "owner-1", "a.txt", "text/plain", int64(10), "k1", now, nil).
		AddRow("a2", "team-1", "user-1", "artifact", "owner-1", "b.txt", "text/plain", int64(20), "k2", now, nil)

	mock.ExpectQuery("DELETE FROM attachments (.+) RETURNING").
		WithArgs("artifact", "owner-1").WillReturnRows(rows)

	deleted, err := repo.DeleteByOwner(context.Background(), "artifact", "owner-1")
	require.NoError(t, err)
	assert.Len(t, deleted, 2)
	assert.Equal(t, "k1", deleted[0].GCSObjectKey)
	assert.NoError(t, mock.ExpectationsWereMet())
}

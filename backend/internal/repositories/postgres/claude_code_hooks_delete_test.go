package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
)

func setupClaudeCodeDeleteTest(t *testing.T) (*claudeCodeHooksRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := &claudeCodeHooksRepository{
		db: db,
	}

	return repo, mock, mockDB
}

//nolint:funlen // Test function with comprehensive scenarios
func TestClaudeCodeHooksRepository_DeleteSession(t *testing.T) {
	tests := []struct {
		name          string
		userID        string
		sessionID     string
		setupMock     func(sqlmock.Sqlmock)
		expectedError string
	}{
		{
			name:      "successful session deletion",
			userID:    "user-123",
			sessionID: "session-456",
			setupMock: func(mock sqlmock.Sqlmock) {
				// Mock SessionExists check
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery(
					`SELECT EXISTS\(SELECT 1 FROM claude_code_hooks_payload WHERE user_id = \$1 AND session_id = \$2\)`,
				).
					WithArgs("user-123", "session-456").
					WillReturnRows(rows)

				// Mock DELETE operation
				mock.ExpectExec(`DELETE FROM claude_code_hooks_payload WHERE user_id = \$1 AND session_id = \$2`).
					WithArgs("user-123", "session-456").
					WillReturnResult(sqlmock.NewResult(0, 5)) // 5 rows affected
			},
			expectedError: "",
		},
		{
			name:      "session not found",
			userID:    "user-123",
			sessionID: "non-existent",
			setupMock: func(mock sqlmock.Sqlmock) {
				// Mock SessionExists check - returns false
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery(
					`SELECT EXISTS\(SELECT 1 FROM claude_code_hooks_payload WHERE user_id = \$1 AND session_id = \$2\)`,
				).
					WithArgs("user-123", "non-existent").
					WillReturnRows(rows)
			},
			expectedError: "session not found or access denied",
		},
		{
			name:      "unauthorized access",
			userID:    "user-456",
			sessionID: "session-123",
			setupMock: func(mock sqlmock.Sqlmock) {
				// Mock SessionExists check - returns false (different user)
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery(
					`SELECT EXISTS\(SELECT 1 FROM claude_code_hooks_payload WHERE user_id = \$1 AND session_id = \$2\)`,
				).
					WithArgs("user-456", "session-123").
					WillReturnRows(rows)
			},
			expectedError: "session not found or access denied",
		},
		{
			name:      "database error during existence check",
			userID:    "user-123",
			sessionID: "session-456",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(
					`SELECT EXISTS\(SELECT 1 FROM claude_code_hooks_payload WHERE user_id = \$1 AND session_id = \$2\)`,
				).
					WithArgs("user-123", "session-456").
					WillReturnError(sql.ErrConnDone)
			},
			expectedError: "failed to check if session exists",
		},
		{
			name:      "database error during deletion",
			userID:    "user-123",
			sessionID: "session-456",
			setupMock: func(mock sqlmock.Sqlmock) {
				// Mock SessionExists check
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery(
					`SELECT EXISTS\(SELECT 1 FROM claude_code_hooks_payload WHERE user_id = \$1 AND session_id = \$2\)`,
				).
					WithArgs("user-123", "session-456").
					WillReturnRows(rows)

				// Mock DELETE operation error
				mock.ExpectExec(`DELETE FROM claude_code_hooks_payload WHERE user_id = \$1 AND session_id = \$2`).
					WithArgs("user-123", "session-456").
					WillReturnError(sql.ErrConnDone)
			},
			expectedError: "failed to delete session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			ctx := context.Background()
			tt.setupMock(mock)

			err := repo.DeleteSession(ctx, tt.userID, tt.sessionID)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestClaudeCodeHooksRepository_DeleteSession_NilDB(t *testing.T) {
	repo := &claudeCodeHooksRepository{
		db: nil,
	}

	ctx := context.Background()
	err := repo.DeleteSession(ctx, "user-123", "session-456")

	assert.Error(t, err)
	assert.Equal(t, fmt.Errorf("database connection is nil"), err)
}

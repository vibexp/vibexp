package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWebhookEventTest(t *testing.T) (*WebhookEventRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewWebhookEventRepository(mockDB)
	return repo, mock, mockDB
}

func TestWebhookEventRepository_IsProcessed(t *testing.T) {
	repo, mock, mockDB := setupWebhookEventTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	tests := []struct {
		name       string
		eventID    string
		setupMock  func()
		wantExists bool
	}{
		{
			name:    "event not processed",
			eventID: "evt_test_123",
			setupMock: func() {
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM webhook_events WHERE event_id = \$1\)`).
					WithArgs("evt_test_123").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			},
			wantExists: false,
		},
		{
			name:    "event already processed",
			eventID: "evt_test_456",
			setupMock: func() {
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM webhook_events WHERE event_id = \$1\)`).
					WithArgs("evt_test_456").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			},
			wantExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			isProcessed, err := repo.IsProcessed(ctx, tt.eventID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, isProcessed)

			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestWebhookEventRepository_MarkProcessed(t *testing.T) {
	repo, mock, mockDB := setupWebhookEventTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	eventID := "evt_test_789"
	eventType := "checkout.session.completed"
	teamID := stringPtr("team-123")

	mock.ExpectExec(`INSERT INTO webhook_events`).
		WithArgs(eventID, eventType, teamID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.MarkProcessed(ctx, eventID, eventType, teamID)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // Table-driven test with comprehensive test cases
func TestWebhookEventRepository_GetByEventID(t *testing.T) {
	repo, mock, mockDB := setupWebhookEventTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	eventID := "evt_test_999"
	teamID := "team-456"
	now := time.Now()

	tests := []struct {
		name      string
		eventID   string
		setupMock func()
		wantErr   bool
	}{
		{
			name:    "event found",
			eventID: eventID,
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "event_id", "event_type", "processed_at", "team_id", "created_at",
				}).AddRow(
					"uuid-123", eventID, "invoice.payment_succeeded", now, teamID, now,
				)
				query := `SELECT id, event_id, event_type, processed_at, team_id, created_at ` +
					`FROM webhook_events WHERE event_id = \$1`
				mock.ExpectQuery(query).WithArgs(eventID).WillReturnRows(rows)
			},
			wantErr: false,
		},
		{
			name:    "event not found",
			eventID: "evt_nonexistent",
			setupMock: func() {
				query := `SELECT id, event_id, event_type, processed_at, team_id, created_at ` +
					`FROM webhook_events WHERE event_id = \$1`
				mock.ExpectQuery(query).WithArgs("evt_nonexistent").WillReturnError(sql.ErrNoRows)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			event, err := repo.GetByEventID(ctx, tt.eventID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, event)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, event)
				assert.Equal(t, tt.eventID, event.EventID)
			}

			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

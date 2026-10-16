package services_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
)

// auditEventAt is a minimal event with a distinguishing id.
func auditEventAt(i int) models.AuditEvent {
	return models.AuditEvent{
		ID:        fmt.Sprintf("00000000-0000-4000-8000-%012d", i),
		CreatedAt: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Minute),
		ActorType: models.AuditActorUser, ActorEmail: fmt.Sprintf("u%d@example.com", i),
		Operation: models.AuditOpPromptCreate, Source: models.AuditSourceWeb, Outcome: models.AuditOutcomeSuccess,
	}
}

// pagedReader serves events in pages along a keyset cursor (the cursor's ID is
// the index of the next event), like the repository does.
type pagedReader struct {
	// The embedded interface supplies Insert and PurgeOlderThan, which the
	// export never calls.
	repositories.AuditEventRepository
	events []models.AuditEvent
	calls  []models.AuditEventFilter
}

func (r *pagedReader) List(_ context.Context, f models.AuditEventFilter) ([]models.AuditEvent, *models.AuditCursor, error) {
	r.calls = append(r.calls, f)
	start := 0
	if f.After != nil {
		n, err := strconv.Atoi(f.After.ID)
		if err != nil {
			return nil, nil, err
		}
		start = n
	}
	end := min(start+f.Limit, len(r.events))
	page := r.events[start:end]
	if end == len(r.events) {
		return page, nil, nil
	}
	return page, &models.AuditCursor{CreatedAt: r.events[end-1].CreatedAt, ID: fmt.Sprint(end)}, nil
}

func (r *pagedReader) Count(context.Context, models.AuditEventFilter) (int, error) {
	return len(r.events), nil
}

func readCSV(t *testing.T, raw []byte) [][]string {
	t.Helper()
	records, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	require.NoError(t, err)
	return records
}

// The export follows the cursor page by page, writes every event once, in
// order, under the documented header, and ignores the caller's Limit/After.
func TestAdminAuditService_ExportPagesThroughEveryEvent(t *testing.T) {
	events := make([]models.AuditEvent, 1203)
	for i := range events {
		events[i] = auditEventAt(i)
	}
	reader := &pagedReader{events: events}
	svc := services.NewAdminAuditService(reader)
	query := "ada"

	exp, err := svc.Export(context.Background(), models.AuditEventFilter{
		Query: &query, Limit: 7, After: &models.AuditCursor{ID: "999"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1203, exp.TotalCount)
	assert.False(t, exp.Truncated)

	var buf bytes.Buffer
	rows, err := exp.WriteCSV(context.Background(), &buf)
	require.NoError(t, err)
	assert.Equal(t, 1203, rows)

	records := readCSV(t, buf.Bytes())
	assert.Equal(t, []string{
		"created_at", "operation", "outcome", "source", "actor_type", "actor_email", "actor_name",
		"actor_user_id", "team_name", "team_id", "project_name", "project_id", "resource_type",
		"resource_name", "resource_id", "assistant_name", "api_key_name", "ip", "user_agent",
		"request_id", "metadata",
	}, records[0])
	require.Len(t, records, 1204)
	assert.Equal(t, "u0@example.com", records[1][5])
	assert.Equal(t, "u1202@example.com", records[1203][5])

	require.Len(t, reader.calls, 3, "three pages of at most 500")
	assert.Nil(t, reader.calls[0].After, "the export starts at the newest event, not the caller's cursor")
	for _, call := range reader.calls {
		require.NotNil(t, call.Query)
		assert.Equal(t, "ada", *call.Query, "every page keeps the filter")
		assert.LessOrEqual(t, call.Limit, 500)
	}
}

// Past the cap the export stops at the cap, never asking for more, and ends
// with the truncation marker.
func TestAdminAuditService_ExportCapsAndMarksTruncation(t *testing.T) {
	events := make([]models.AuditEvent, 12)
	for i := range events {
		events[i] = auditEventAt(i)
	}
	reader := &pagedReader{events: events}
	svc := services.NewAdminAuditService(reader)
	svc.SetExportCapForTest(5)

	exp, err := svc.Export(context.Background(), models.AuditEventFilter{})
	require.NoError(t, err)
	assert.True(t, exp.Truncated)

	var buf bytes.Buffer
	rows, err := exp.WriteCSV(context.Background(), &buf)
	require.NoError(t, err)
	assert.Equal(t, 5, rows)
	records := readCSV(t, buf.Bytes())
	require.Len(t, records, 7, "header, 5 rows, marker")
	assert.Equal(t, "# TRUNCATED: exported 5 of 12 rows; narrow the filters", records[6][0])
	require.Len(t, reader.calls, 1)
	assert.Equal(t, 5, reader.calls[0].Limit, "a page never reads past the cap")
}

// Every free-text cell, the metadata JSON included, is defused against formula
// injection; nil optionals are empty cells and missing metadata is {}.
func TestAdminAuditService_ExportGuardsFreeTextCells(t *testing.T) {
	str := func(s string) *string { return &s }
	hostile := models.AuditEvent{
		ID: "00000000-0000-4000-8000-000000000001", CreatedAt: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
		ActorType: models.AuditActorUser, ActorEmail: "=cmd@example.com", ActorName: "+SUM(A1)",
		ActorUserID: str("=actor"), TeamID: str("@team-id"), ProjectID: str("-project-id"),
		TeamName: str("@team"), ProjectName: str("-proj"), ResourceType: str("=type"),
		ResourceName: str("=HYPERLINK()"), ResourceID: str("+id"), AssistantName: str("\tassistant"),
		APIKeyName: str("@key"), IP: str("=1.2.3.4"), UserAgent: str("=ua"), RequestID: str("-req"),
		Operation: models.AuditOpPromptUpdate, Source: models.AuditSourceAPI, Outcome: models.AuditOutcomeFailure,
		Metadata: json.RawMessage(` [ "=1+1" ] `),
	}
	plain := auditEventAt(2)
	reader := repomocks.NewMockAuditEventRepository(t)
	reader.EXPECT().Count(mock.Anything, mock.Anything).Return(2, nil)
	reader.EXPECT().List(mock.Anything, mock.Anything).Return([]models.AuditEvent{hostile, plain}, nil, nil)

	exp, err := services.NewAdminAuditService(reader).Export(context.Background(), models.AuditEventFilter{})
	require.NoError(t, err)
	var buf bytes.Buffer
	_, err = exp.WriteCSV(context.Background(), &buf)
	require.NoError(t, err)
	records := readCSV(t, buf.Bytes())

	row := records[1]
	for _, i := range []int{5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19} {
		assert.Equal(t, "'", row[i][:1], "column %s must be defused: %q", records[0][i], row[i])
	}
	assert.Equal(t, `["=1+1"]`, row[20], "metadata is compact JSON; a leading [ needs no guard")
	assert.Equal(t, "2026-10-15T00:00:00Z", row[0])
	assert.Equal(t, "prompt.update", row[1])

	clean := records[2]
	assert.Equal(t, "", clean[7], "a nil id is an empty cell")

	uuidRow := auditEventAt(3)
	id := "4f6e2a10-0000-4000-8000-000000000000"
	uuidRow.ActorUserID, uuidRow.TeamID = &id, &id
	assert.Equal(t, id, services.ExportedAuditExportCell(uuidRow, "actor_user_id"),
		"the guard leaves a well-formed UUID untouched")
	assert.Equal(t, "{}", clean[20], "missing metadata is {}")
}

func TestAdminAuditService_Errors(t *testing.T) {
	boom := errors.New("db down")

	t.Run("count", func(t *testing.T) {
		reader := repomocks.NewMockAuditEventRepository(t)
		reader.EXPECT().Count(mock.Anything, mock.Anything).Return(0, boom)
		_, err := services.NewAdminAuditService(reader).Export(context.Background(), models.AuditEventFilter{})
		assert.ErrorIs(t, err, boom)
	})
	t.Run("list", func(t *testing.T) {
		reader := repomocks.NewMockAuditEventRepository(t)
		reader.EXPECT().List(mock.Anything, mock.Anything).Return(nil, nil, boom)
		_, _, err := services.NewAdminAuditService(reader).List(context.Background(), models.AuditEventFilter{})
		assert.ErrorIs(t, err, boom)
	})
	t.Run("a page failing mid-export", func(t *testing.T) {
		reader := repomocks.NewMockAuditEventRepository(t)
		reader.EXPECT().Count(mock.Anything, mock.Anything).Return(3, nil)
		reader.EXPECT().List(mock.Anything, mock.Anything).
			Return([]models.AuditEvent{auditEventAt(0)}, &models.AuditCursor{ID: "1"}, nil).Once()
		reader.EXPECT().List(mock.Anything, mock.Anything).Return(nil, nil, boom).Once()
		exp, err := services.NewAdminAuditService(reader).Export(context.Background(), models.AuditEventFilter{})
		require.NoError(t, err)
		rows, err := exp.WriteCSV(context.Background(), &bytes.Buffer{})
		assert.ErrorIs(t, err, boom)
		assert.Equal(t, 1, rows)
	})
}

func TestAdminAuditService_ListPassesThrough(t *testing.T) {
	reader := repomocks.NewMockAuditEventRepository(t)
	next := &models.AuditCursor{ID: "x"}
	f := models.AuditEventFilter{Limit: 10}
	reader.EXPECT().List(mock.Anything, f).Return([]models.AuditEvent{auditEventAt(1)}, next, nil)

	events, gotNext, err := services.NewAdminAuditService(reader).List(context.Background(), f)

	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Same(t, next, gotNext)
}

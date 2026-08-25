package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	teamsettingsgen "github.com/vibexp/vibexp/internal/server/gen/teamsettings"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Handler suite for the settings audit read path (#832).

const teamSettingsAuditPath = "/api/v1/" + testTeamSettingsTeamID + "/settings/audit"

// MockTeamSettingsAuditContainer overrides the audit service on the base
// container. A separate container from MockTeamSettingsContainer because this
// surface reaches for a different accessor entirely.
type MockTeamSettingsAuditContainer struct {
	BaseMockContainer
	auditService services.TeamSettingsAuditServiceInterface
}

func (c *MockTeamSettingsAuditContainer) TeamSettingsAuditService() services.TeamSettingsAuditServiceInterface {
	return c.auditService
}

func createTestTeamSettingsAuditServer(
	svc services.TeamSettingsAuditServiceInterface,
) *Server {
	r := chi.NewRouter()
	srv := &Server{
		container: &MockTeamSettingsAuditContainer{auditService: svc},
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}
	strict := teamsettingsgen.NewStrictHandlerWithOptions(
		&teamSettingsStrictServer{s: srv},
		nil,
		teamsettingsgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.teamSettingsBindErrorHandler,
			ResponseErrorHandlerFunc: srv.teamSettingsResponseErrorHandler,
		},
	)
	// Mounted with the same body middleware production uses, so a regression
	// that made it reject a GET would show up here rather than only in prod.
	r.Use(srv.requireCompleteSearchSettingsBody)
	teamsettingsgen.HandlerWithOptions(strict, teamsettingsgen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.teamSettingsBindErrorHandler,
	})
	return srv
}

// doAuditGet runs one GET through the real router and asserts the response
// conforms to the spec before anything else is examined — the shape gate every
// documented operation has to pass.
func doAuditGet(
	t *testing.T, svc services.TeamSettingsAuditServiceInterface, path string,
) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()

	srv := createTestTeamSettingsAuditServer(svc)
	req := makeTeamSettingsRequest(http.MethodGet, path, "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	var body map[string]any
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "body: %s", w.Body.String())
	}
	return w, body
}

func auditView(id, actorName, sourceTeamName string) *models.TeamSettingsAuditEntryView {
	actorID := "770e8400-e29b-41d4-a716-446655440002"
	sourceTeamID := "880e8400-e29b-41d4-a716-446655440003"
	sourceResourceID := "990e8400-e29b-41d4-a716-446655440004"
	createdResourceID := "aa0e8400-e29b-41d4-a716-446655440005"

	view := &models.TeamSettingsAuditEntryView{
		Entry: &models.TeamSettingsAudit{
			ID:                id,
			TeamID:            testTeamSettingsTeamID,
			ActorUserID:       &actorID,
			Surface:           models.SettingsAuditSurfaceModelProvider,
			SourceTeamID:      &sourceTeamID,
			SourceResourceID:  &sourceResourceID,
			CreatedResourceID: &createdResourceID,
			Detail:            json.RawMessage(`{"source_name":"OpenAI","has_api_key":true}`),
			CreatedAt:         time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
		},
	}
	if actorName != "" {
		view.ActorName = &actorName
	}
	if sourceTeamName != "" {
		view.SourceTeamName = &sourceTeamName
	}
	return view
}

func TestListTeamSettingsAudit_ReturnsAPage(t *testing.T) {
	svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)
	svc.EXPECT().
		ListAudit(mock.Anything, testTeamSettingsUserID, testTeamSettingsTeamID, 1, 20).
		Return(&models.TeamSettingsAuditPage{
			Entries: []*models.TeamSettingsAuditEntryView{
				auditView("bb0e8400-e29b-41d4-a716-446655440006", "Ada Lovelace", "Platform"),
			},
			TotalCount: 42,
			Page:       1,
			PerPage:    20,
		}, nil)

	w, body := doAuditGet(t, svc, teamSettingsAuditPath)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.EqualValues(t, 42, body["total_count"])
	assert.EqualValues(t, 1, body["page"])
	assert.EqualValues(t, 20, body["per_page"])
	// 42 entries at 20 per page is three pages — the ceiling, not the floor.
	assert.EqualValues(t, 3, body["total_pages"])

	entries, ok := body["entries"].([]any)
	require.True(t, ok)
	require.Len(t, entries, 1)
	row, ok := entries[0].(map[string]any)
	require.True(t, ok)

	// AssertConformsToSpec cannot catch an omitted optional field or a wrong
	// scalar, so the resolved names and the deep-link ids need explicit value
	// assertions — they are the point of the endpoint.
	assert.Equal(t, "Ada Lovelace", row["actor_name"])
	assert.Equal(t, "Platform", row["source_team_name"])
	assert.Equal(t, "model_provider", row["surface"])
	assert.Equal(t, "770e8400-e29b-41d4-a716-446655440002", row["actor_user_id"])
	assert.Equal(t, "880e8400-e29b-41d4-a716-446655440003", row["source_team_id"])
	assert.Equal(t, "990e8400-e29b-41d4-a716-446655440004", row["source_resource_id"])
	assert.Equal(t, "aa0e8400-e29b-41d4-a716-446655440005", row["created_resource_id"])

	detail, ok := row["detail"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "OpenAI", detail["source_name"])
	assert.Equal(t, true, detail["has_api_key"])
}

func TestListTeamSettingsAudit_HonoursPageAndLimit(t *testing.T) {
	svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)
	svc.EXPECT().
		ListAudit(mock.Anything, testTeamSettingsUserID, testTeamSettingsTeamID, 3, 5).
		Return(&models.TeamSettingsAuditPage{
			Entries: []*models.TeamSettingsAuditEntryView{}, TotalCount: 11, Page: 3, PerPage: 5,
		}, nil)

	w, body := doAuditGet(t, svc, teamSettingsAuditPath+"?page=3&limit=5")

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.EqualValues(t, 3, body["page"])
	assert.EqualValues(t, 5, body["per_page"])
	assert.EqualValues(t, 3, body["total_pages"])
}

// The required `entries` array must serialize as `[]`, never `null`.
// AssertConformsToSpec passes on a `null` here (measured on #829), so the wire
// form is asserted directly — that is the assertion that actually bites.
func TestListTeamSettingsAudit_EmptyPageSerializesAsArray(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries []*models.TeamSettingsAuditEntryView
	}{
		{"empty slice", []*models.TeamSettingsAuditEntryView{}},
		{"nil slice", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)
			svc.EXPECT().ListAudit(mock.Anything, mock.Anything, mock.Anything, 1, 20).
				Return(&models.TeamSettingsAuditPage{
					Entries: tt.entries, TotalCount: 0, Page: 1, PerPage: 20,
				}, nil)

			w, _ := doAuditGet(t, svc, teamSettingsAuditPath)

			require.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), `"entries":[]`)
			assert.NotContains(t, w.Body.String(), `"entries":null`)
			// An empty log is one empty page, not zero — so a client's
			// "page 1 of N" never reads "page 1 of 0".
			assert.Contains(t, w.Body.String(), `"total_pages":1`)
		})
	}
}

// A deleted actor and a deleted source team must render as null names beside
// the ids that survive them, not fail the page.
func TestListTeamSettingsAudit_NullNamesRenderWithoutErroring(t *testing.T) {
	view := auditView("cc0e8400-e29b-41d4-a716-446655440007", "", "")
	view.Entry.ActorUserID = nil // ON DELETE SET NULL
	view.Entry.SourceResourceID = nil
	view.Entry.CreatedResourceID = nil

	svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)
	svc.EXPECT().ListAudit(mock.Anything, mock.Anything, mock.Anything, 1, 20).
		Return(&models.TeamSettingsAuditPage{
			Entries:    []*models.TeamSettingsAuditEntryView{view},
			TotalCount: 1, Page: 1, PerPage: 20,
		}, nil)

	w, body := doAuditGet(t, svc, teamSettingsAuditPath)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	row := body["entries"].([]any)[0].(map[string]any)

	// Present-and-null, not absent: the fields are required, so a client can
	// read them unconditionally.
	for _, key := range []string{
		"actor_user_id", "actor_name", "source_team_name",
		"source_resource_id", "created_resource_id",
	} {
		require.Contains(t, row, key, "%s must be present even when null", key)
		assert.Nil(t, row[key], "%s must be null", key)
	}
	// The source team id outlives the team it names — that is why the column
	// carries no foreign key.
	assert.Equal(t, "880e8400-e29b-41d4-a716-446655440003", row["source_team_id"])
}

func TestListTeamSettingsAudit_ForbiddenWhenRoleLacksPermission(t *testing.T) {
	svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)
	svc.EXPECT().ListAudit(mock.Anything, mock.Anything, mock.Anything, 1, 20).
		Return(nil, services.ErrPermissionDenied)

	w, _ := doAuditGet(t, svc, teamSettingsAuditPath)

	assert.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	// The message must not describe a WRITE: this is a read, and a member who
	// listed the audit log should not be told they cannot change settings.
	assert.Contains(t, w.Body.String(), "manage this team's settings")
	assert.NotContains(t, w.Body.String(), "change this team's settings")
}

func TestListTeamSettingsAudit_RejectsOutOfRangePaging(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"page below the minimum", "?page=0"},
		{"page above the maximum", "?page=1000001"},
		{"limit below the minimum", "?limit=0"},
		{"limit above the maximum", "?limit=101"},
		{"negative limit", "?limit=-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No EXPECT: rejecting the parameters must happen before the
			// service is reached, so any call fails this test.
			svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)

			w, _ := doAuditGet(t, svc, teamSettingsAuditPath+tt.query)

			assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
		})
	}
}

func TestListTeamSettingsAudit_RejectsNonNumericPaging(t *testing.T) {
	svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)

	w, _ := doAuditGet(t, svc, teamSettingsAuditPath+"?page=not-a-number")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	// The team_id special-case must not leak onto a query parameter — telling
	// someone `page` must be a UUID sends them to the wrong place (#734).
	assert.NotContains(t, w.Body.String(), "must be a valid UUID")
}

func TestListTeamSettingsAudit_RequiresAuthenticatedUser(t *testing.T) {
	svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)
	srv := createTestTeamSettingsAuditServer(svc)

	// No user in the context — the audit read needs an actor to authorize.
	req := httptest.NewRequest(http.MethodGet, teamSettingsAuditPath, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "body: %s", w.Body.String())
}

func TestListTeamSettingsAudit_RejectsNonUUIDTeamID(t *testing.T) {
	svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)
	srv := createTestTeamSettingsAuditServer(svc)

	req := makeTeamSettingsRequest(http.MethodGet, "/api/v1/not-a-uuid/settings/audit", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "team_id must be a valid UUID")
}

// A row whose stored detail is empty must still render `{}` — `detail` is a
// required, non-nullable object and a nil Go map marshals to `null`.
func TestListTeamSettingsAudit_EmptyDetailRendersAsObject(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  json.RawMessage
	}{
		{"nil detail", nil},
		{"empty object", json.RawMessage(`{}`)},
		{"undecodable detail", json.RawMessage(`not json`)},
		{"json null", json.RawMessage(`null`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			view := auditView("dd0e8400-e29b-41d4-a716-446655440008", "Ada", "Platform")
			view.Entry.Detail = tt.raw

			svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)
			svc.EXPECT().ListAudit(mock.Anything, mock.Anything, mock.Anything, 1, 20).
				Return(&models.TeamSettingsAuditPage{
					Entries:    []*models.TeamSettingsAuditEntryView{view},
					TotalCount: 1, Page: 1, PerPage: 20,
				}, nil)

			w, body := doAuditGet(t, svc, teamSettingsAuditPath)

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			row := body["entries"].([]any)[0].(map[string]any)
			assert.NotNil(t, row["detail"])
			assert.Equal(t, map[string]any{}, row["detail"])
		})
	}
}

// A malformed id in a stored row is a data-integrity fault, not a client error:
// it must surface as a 500 rather than a half-rendered page.
func TestListTeamSettingsAudit_UnparseableStoredIDIsAnInternalError(t *testing.T) {
	view := auditView("not-a-uuid", "Ada", "Platform")

	svc := servicesmocks.NewMockTeamSettingsAuditServiceInterface(t)
	svc.EXPECT().ListAudit(mock.Anything, mock.Anything, mock.Anything, 1, 20).
		Return(&models.TeamSettingsAuditPage{
			Entries:    []*models.TeamSettingsAuditEntryView{view},
			TotalCount: 1, Page: 1, PerPage: 20,
		}, nil)

	w, _ := doAuditGet(t, svc, teamSettingsAuditPath)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

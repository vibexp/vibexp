package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	statusesgen "github.com/vibexp/vibexp/internal/server/gen/statuses"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	testStatusesTeamID = "660e8400-e29b-41d4-a716-446655440001"
	testStatusesUserID = "880e8400-e29b-41d4-a716-446655440003"
	testStatusesRowID  = "550e8400-e29b-41d4-a716-446655440000"
	testStatusesBase   = "/api/v1/" + testStatusesTeamID + "/statuses"
	testStatusesByID   = testStatusesBase + "/" + testStatusesRowID
)

var testStatusesNow = time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

// mockStatusesContainer overrides only the status management service.
type mockStatusesContainer struct {
	BaseMockContainer
	statusManagement services.StatusManagementServiceInterface
}

func (c *mockStatusesContainer) StatusManagementService() services.StatusManagementServiceInterface {
	return c.statusManagement
}

// newStatusesTestServer mounts the Statuses routes exactly as production does
// (mountStatusesHandlers: the body guard plus the generated router), without
// the team middleware, which has its own tests. The authenticated user is
// injected per request.
func newStatusesTestServer(svc services.StatusManagementServiceInterface) *Server {
	r := chi.NewRouter()
	srv := &Server{
		container: &mockStatusesContainer{statusManagement: svc},
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}
	srv.mountStatusesHandlers(r, statusesgen.NewStrictHandlerWithOptions(
		&statusesStrictServer{s: srv},
		nil,
		statusesgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.statusesBindErrorHandler,
			ResponseErrorHandlerFunc: srv.statusesResponseErrorHandler,
		},
	))
	return srv
}

// serveStatuses sends one request as the test user and asserts the response
// conforms to the spec, whatever its status.
func serveStatuses(t *testing.T, srv *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, testStatusesUserID))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)
	return w
}

func requireStatusesProblem(t *testing.T, w *httptest.ResponseRecorder, status int, code string) string {
	t.Helper()
	require.Equal(t, status, w.Code, w.Body.String())
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, code, problem.Code)
	return problem.Detail
}

func managedCustomStatus() *services.ManagedStatus {
	id := testStatusesRowID
	created, updated := testStatusesNow, testStatusesNow.Add(time.Hour)
	return &services.ManagedStatus{
		CatalogStatus: services.CatalogStatus{
			ID: &id, Slug: "in-review", Name: "In review", IsLive: true,
			CreatedAt: &created, UpdatedAt: &updated,
		},
		ResourceType: models.StatusResourceTypeArtifact,
		UsageCount:   3,
	}
}

func TestListStatuses_SystemAndCustom(t *testing.T) {
	svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
	svc.EXPECT().List(mock.Anything, testStatusesTeamID, models.StatusResourceTypeArtifact).
		Return([]services.ManagedStatus{
			{
				CatalogStatus: services.CatalogStatus{Slug: "active", Name: "Active", IsSystem: true, IsLive: true},
				ResourceType:  models.StatusResourceTypeArtifact, UsageCount: 7,
			},
			{
				CatalogStatus: services.CatalogStatus{Slug: "draft", Name: "Draft", IsSystem: true},
				ResourceType:  models.StatusResourceTypeArtifact,
			},
			*managedCustomStatus(),
		}, nil)

	w := serveStatuses(t, newStatusesTestServer(svc), http.MethodGet, testStatusesBase+"?resource_type=artifact", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Statuses []map[string]interface{} `json:"statuses"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Statuses, 3)

	// A system status has no id and no timestamps: the keys are absent.
	assert.Equal(t, map[string]interface{}{
		"resource_type": "artifact", "slug": "active", "name": "Active",
		"is_system": true, "is_live": true, "usage_count": float64(7),
	}, resp.Statuses[0])
	assert.Equal(t, false, resp.Statuses[1]["is_live"])
	assert.Equal(t, float64(0), resp.Statuses[1]["usage_count"])

	assert.Equal(t, map[string]interface{}{
		"id": testStatusesRowID, "resource_type": "artifact", "slug": "in-review", "name": "In review",
		"is_system": false, "is_live": true, "usage_count": float64(3),
		"created_at": "2026-10-10T09:00:00Z", "updated_at": "2026-10-10T10:00:00Z",
	}, resp.Statuses[2])
}

// A required array is `[]`, never null, even when the service returns nil.
func TestListStatuses_EmptyIsAnArray(t *testing.T) {
	svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
	svc.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	w := serveStatuses(t, newStatusesTestServer(svc), http.MethodGet, testStatusesBase+"?resource_type=memory", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"statuses":[]}`, w.Body.String())
}

func TestListStatuses_Errors(t *testing.T) {
	t.Run("a missing resource_type is a 400 from the binder", func(t *testing.T) {
		svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
		w := serveStatuses(t, newStatusesTestServer(svc), http.MethodGet, testStatusesBase, "")
		assert.Equal(t, "resource_type is required",
			requireStatusesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST"))
	})

	t.Run("an unsupported resource_type is a 400 from the service", func(t *testing.T) {
		svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
		svc.EXPECT().List(mock.Anything, testStatusesTeamID, "artifacts").
			Return(nil, services.ErrStatusResourceTypeUnsupported)
		w := serveStatuses(t, newStatusesTestServer(svc), http.MethodGet, testStatusesBase+"?resource_type=artifacts", "")
		assert.Contains(t, requireStatusesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST"), "prompt, artifact")
	})

	t.Run("a malformed team id is a 400", func(t *testing.T) {
		svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
		w := serveStatuses(t, newStatusesTestServer(svc), http.MethodGet,
			"/api/v1/not-a-uuid/statuses?resource_type=artifact", "")
		assert.Equal(t, "team_id must be a valid UUID",
			requireStatusesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST"))
	})

	t.Run("a service failure is a 500 that does not leak the cause", func(t *testing.T) {
		svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
		svc.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("pq: relation \"artifacts\" does not exist"))
		w := serveStatuses(t, newStatusesTestServer(svc), http.MethodGet, testStatusesBase+"?resource_type=artifact", "")
		assert.Equal(t, statusesMsgListFailed,
			requireStatusesProblem(t, w, http.StatusInternalServerError, "INTERNAL_ERROR"))
	})

	t.Run("a status whose id is not a UUID is a 500", func(t *testing.T) {
		svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
		bad := managedCustomStatus()
		id := "not-a-uuid"
		bad.ID = &id
		svc.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
			Return([]services.ManagedStatus{*bad}, nil)
		w := serveStatuses(t, newStatusesTestServer(svc), http.MethodGet, testStatusesBase+"?resource_type=artifact", "")
		requireStatusesProblem(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
	})
}

func TestCreateStatus_Created(t *testing.T) {
	svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
	svc.EXPECT().Create(mock.Anything, services.CreateStatusParams{
		TeamID: testStatusesTeamID, UserID: testStatusesUserID,
		ResourceType: models.StatusResourceTypeArtifact, Slug: "in-review", Name: "In review",
	}).Return(managedCustomStatus(), nil)

	w := serveStatuses(t, newStatusesTestServer(svc), http.MethodPost, testStatusesBase,
		`{"resource_type":"artifact","slug":"in-review","name":"In review"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, testStatusesRowID, created["id"])
	assert.Equal(t, "in-review", created["slug"])
	assert.Equal(t, false, created["is_system"])
}

func TestCreateStatus_ServiceErrorsMapToDocumentedResponses(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
		code   string
	}{
		"permission denied":         {services.ErrPermissionDenied, http.StatusForbidden, "FORBIDDEN"},
		"unsupported resource type": {services.ErrStatusResourceTypeUnsupported, http.StatusBadRequest, "BAD_REQUEST"},
		"invalid slug":              {services.ErrStatusSlugInvalid, http.StatusBadRequest, "BAD_REQUEST"},
		"slug too long":             {services.ErrStatusSlugTooLong, http.StatusBadRequest, "BAD_REQUEST"},
		"name required":             {services.ErrStatusNameRequired, http.StatusBadRequest, "BAD_REQUEST"},
		"name too long":             {services.ErrStatusNameTooLong, http.StatusBadRequest, "BAD_REQUEST"},
		"21st status":               {services.ErrStatusLimitReached, http.StatusBadRequest, "BAD_REQUEST"},
		"reserved slug":             {services.ErrStatusReserved, http.StatusConflict, "RESOURCE_EXISTS"},
		"duplicate slug":            {repositories.ErrTeamStatusExists, http.StatusConflict, "RESOURCE_EXISTS"},
		"storage failure":           {errors.New("connection reset"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
			svc.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, tc.err)

			w := serveStatuses(t, newStatusesTestServer(svc), http.MethodPost, testStatusesBase,
				`{"resource_type":"artifact","slug":"in-review","name":"In review"}`)
			detail := requireStatusesProblem(t, w, tc.status, tc.code)
			assert.NotContains(t, detail, "connection reset")
		})
	}
}

func TestCreateStatus_BodyIsGuardedBeforeTheService(t *testing.T) {
	cases := map[string]struct{ body, detail string }{
		"an unknown field": {
			`{"resource_type":"artifact","slug":"in-review","name":"In review","is_live":false}`,
			"Unknown field(s): is_live. Only resource_type, slug, name may be set.",
		},
		"malformed JSON": {`{"resource_type":`, msgInvalidBodyWellFormedJSON},
		"a JSON array":   {`["artifact"]`, msgInvalidBodyWellFormedJSON},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// No expectation: a rejected body never reaches the service.
			svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
			w := serveStatuses(t, newStatusesTestServer(svc), http.MethodPost, testStatusesBase, tc.body)
			assert.Equal(t, tc.detail, requireStatusesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST"))
		})
	}
}

func TestUpdateStatus_Renamed(t *testing.T) {
	svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
	renamed := managedCustomStatus()
	renamed.Name = "Under review"
	svc.EXPECT().Rename(mock.Anything, services.RenameStatusParams{
		TeamID: testStatusesTeamID, UserID: testStatusesUserID, ID: testStatusesRowID, Name: "Under review",
	}).Return(renamed, nil)

	w := serveStatuses(t, newStatusesTestServer(svc), http.MethodPatch, testStatusesByID, `{"name":"Under review"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "Under review", got["name"])
	assert.Equal(t, "in-review", got["slug"])
}

// The slug is immutable. A rename that carries one is refused outright rather
// than answered 200 with the field silently ignored, and the service is never
// called, so the stored slug cannot change.
func TestUpdateStatus_RejectsAnyFieldButName(t *testing.T) {
	for name, body := range map[string]string{
		"slug":          `{"name":"Under review","slug":"under-review"}`,
		"slug alone":    `{"slug":"under-review"}`,
		"resource_type": `{"name":"Under review","resource_type":"memory"}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
			w := serveStatuses(t, newStatusesTestServer(svc), http.MethodPatch, testStatusesByID, body)
			detail := requireStatusesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
			assert.Contains(t, detail, "Only name may be set.")
		})
	}
}

func TestUpdateStatus_Errors(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
		code   string
	}{
		"permission denied": {services.ErrPermissionDenied, http.StatusForbidden, "FORBIDDEN"},
		"unknown id":        {repositories.ErrTeamStatusNotFound, http.StatusNotFound, "RESOURCE_NOT_FOUND"},
		"empty name":        {services.ErrStatusNameRequired, http.StatusBadRequest, "BAD_REQUEST"},
		"name too long":     {services.ErrStatusNameTooLong, http.StatusBadRequest, "BAD_REQUEST"},
		"storage failure":   {errors.New("connection reset"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
			svc.EXPECT().Rename(mock.Anything, mock.Anything).Return(nil, tc.err)
			w := serveStatuses(t, newStatusesTestServer(svc), http.MethodPatch, testStatusesByID, `{"name":"x"}`)
			requireStatusesProblem(t, w, tc.status, tc.code)
		})
	}

	t.Run("a missing name reaches the service empty, which rejects it", func(t *testing.T) {
		svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
		svc.EXPECT().Rename(mock.Anything, mock.MatchedBy(func(p services.RenameStatusParams) bool {
			return p.Name == ""
		})).Return(nil, services.ErrStatusNameRequired)
		w := serveStatuses(t, newStatusesTestServer(svc), http.MethodPatch, testStatusesByID, `{}`)
		assert.Equal(t, "name is required", requireStatusesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST"))
	})

	t.Run("a malformed id is a 400 from the binder", func(t *testing.T) {
		svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
		w := serveStatuses(t, newStatusesTestServer(svc), http.MethodPatch, testStatusesBase+"/not-a-uuid", `{"name":"x"}`)
		assert.Equal(t, "status id must be a valid UUID",
			requireStatusesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST"))
	})
}

func TestDeleteStatus_ReportsTheReassignment(t *testing.T) {
	svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
	svc.EXPECT().Delete(mock.Anything, services.DeleteStatusParams{
		TeamID: testStatusesTeamID, UserID: testStatusesUserID, ID: testStatusesRowID,
	}).Return(&services.StatusDeletion{ReassignedCount: 12, ReassignedTo: "active"}, nil)

	w := serveStatuses(t, newStatusesTestServer(svc), http.MethodDelete, testStatusesByID, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"reassigned_count":12,"reassigned_to":"active"}`, w.Body.String())
}

func TestDeleteStatus_Errors(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
		code   string
	}{
		"permission denied": {services.ErrPermissionDenied, http.StatusForbidden, "FORBIDDEN"},
		"unknown id":        {repositories.ErrTeamStatusNotFound, http.StatusNotFound, "RESOURCE_NOT_FOUND"},
		"storage failure":   {errors.New("connection reset"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
			svc.EXPECT().Delete(mock.Anything, mock.Anything).Return(nil, tc.err)
			w := serveStatuses(t, newStatusesTestServer(svc), http.MethodDelete, testStatusesByID, "")
			requireStatusesProblem(t, w, tc.status, tc.code)
		})
	}

	t.Run("a malformed id is a 400 from the binder", func(t *testing.T) {
		svc := servicesmocks.NewMockStatusManagementServiceInterface(t)
		w := serveStatuses(t, newStatusesTestServer(svc), http.MethodDelete, testStatusesBase+"/not-a-uuid", "")
		requireStatusesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
	})
}

// The role matrix end to end, through the REAL status management,
// authorization and audit services: a member is refused on all three writes
// with nothing stored and nothing audited, while an owner and an admin succeed
// and leave exactly one audit entry each.
//
// The entry is stored by the status repository in the transaction of the
// change (#1333), so the mocked writes do what the real one does with the
// builder they are handed -- run it -- and the test counts the entries built.
// The audit repository mock carries no expectation: nothing appends on its own.
func TestStatusesWrites_RoleMatrixThroughTheRealServices(t *testing.T) {
	stored := models.TeamStatus{
		ID: testStatusesRowID, TeamID: testStatusesTeamID, ResourceType: models.StatusResourceTypeArtifact,
		Slug: "in-review", Name: "In review", CreatedAt: testStatusesNow, UpdatedAt: testStatusesNow,
	}
	type auditedWrite func(build repositories.TeamStatusAuditBuilder, change repositories.TeamStatusChange) error
	writes := []struct {
		name, method, path, body string
		want                     int
		arrange                  func(repo *repomocks.MockTeamStatusRepository, audited auditedWrite)
	}{
		{
			name: "create", method: http.MethodPost, path: testStatusesBase, want: http.StatusCreated,
			body: `{"resource_type":"artifact","slug":"in-review","name":"In review"}`,
			arrange: func(repo *repomocks.MockTeamStatusRepository, audited auditedWrite) {
				repo.EXPECT().Count(mock.Anything, testStatusesTeamID, models.StatusResourceTypeArtifact).Return(0, nil)
				repo.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(func(
						_ context.Context, row *models.TeamStatus, build repositories.TeamStatusAuditBuilder,
					) error {
						row.ID, row.CreatedAt, row.UpdatedAt = stored.ID, stored.CreatedAt, stored.UpdatedAt
						return audited(build, repositories.TeamStatusChange{Status: row})
					})
			},
		},
		{
			name: "rename", method: http.MethodPatch, path: testStatusesByID, want: http.StatusOK,
			body: `{"name":"Under review"}`,
			arrange: func(repo *repomocks.MockTeamStatusRepository, audited auditedWrite) {
				renamed := stored
				renamed.Name = "Under review"
				repo.EXPECT().
					UpdateName(mock.Anything, testStatusesTeamID, testStatusesRowID, "Under review", mock.Anything).
					RunAndReturn(func(
						_ context.Context, _, _, _ string, build repositories.TeamStatusAuditBuilder,
					) (*models.TeamStatus, error) {
						change := repositories.TeamStatusChange{Status: &renamed, PreviousName: stored.Name}
						return &renamed, audited(build, change)
					})
				repo.EXPECT().UsageCounts(mock.Anything, mock.Anything, mock.Anything).Return(map[string]int{}, nil)
			},
		},
		{
			name: "delete", method: http.MethodDelete, path: testStatusesByID, want: http.StatusOK,
			arrange: func(repo *repomocks.MockTeamStatusRepository, audited auditedWrite) {
				repo.EXPECT().GetByID(mock.Anything, testStatusesTeamID, testStatusesRowID).Return(&stored, nil)
				repo.EXPECT().
					DeleteAndReassign(mock.Anything, testStatusesTeamID, testStatusesRowID, "active", mock.Anything).
					RunAndReturn(func(
						_ context.Context, _, _, _ string, build repositories.TeamStatusAuditBuilder,
					) (*models.TeamStatus, int, error) {
						change := repositories.TeamStatusChange{Status: &stored, ReassignedCount: 4}
						return &stored, 4, audited(build, change)
					})
			},
		},
	}

	build := func(t *testing.T, role models.TeamMemberRole) (*Server, *repomocks.MockTeamStatusRepository) {
		t.Helper()
		logger := slog.New(slog.DiscardHandler)
		repo := repomocks.NewMockTeamStatusRepository(t)
		members := repomocks.NewMockTeamMemberRepository(t)
		members.EXPECT().GetByTeamAndUser(mock.Anything, testStatusesTeamID, testStatusesUserID).
			Return(&models.TeamMember{Role: role}, nil)

		authorizer := services.NewAuthorizationService(members, logger)
		audit := services.NewTeamSettingsAuditService(repomocks.NewMockTeamSettingsAuditRepository(t), authorizer,
			repomocks.NewMockUserRepository(t), repomocks.NewMockTeamRepository(t), logger)
		svc := services.NewStatusManagementService(
			repo, services.NewStatusCatalogService(repo), authorizer, audit, logger)
		return newStatusesTestServer(svc), repo
	}

	for _, write := range writes {
		t.Run("a member cannot "+write.name, func(t *testing.T) {
			// The status repository carries no expectation: nothing is read or
			// written -- and so nothing audited -- for a caller who may not
			// change team settings.
			srv, _ := build(t, models.TeamMemberRoleMember)
			w := serveStatuses(t, srv, write.method, write.path, write.body)
			assert.Equal(t, statusesMsgForbidden, requireStatusesProblem(t, w, http.StatusForbidden, "FORBIDDEN"))
		})

		for _, role := range []models.TeamMemberRole{models.TeamMemberRoleOwner, models.TeamMemberRoleAdmin} {
			t.Run("an "+string(role)+" can "+write.name, func(t *testing.T) {
				srv, repo := build(t, role)
				var entries []*models.TeamSettingsAudit
				write.arrange(repo, func(
					buildEntry repositories.TeamStatusAuditBuilder, change repositories.TeamStatusChange,
				) error {
					entry, err := buildEntry(change)
					if err != nil {
						return err
					}
					entries = append(entries, entry)
					return nil
				})

				w := serveStatuses(t, srv, write.method, write.path, write.body)
				require.Equal(t, write.want, w.Code, w.Body.String())
				require.Len(t, entries, 1, "each successful write stores exactly one audit entry")
				assert.Equal(t, models.SettingsAuditSurfaceCustomStatuses, entries[0].Surface)
				assert.Equal(t, testStatusesTeamID, entries[0].TeamID)
			})
		}
	}
}

// statusesResponseErrorHandler's fallback: an error that is not an APIError
// never reaches the client as anything but a generic 500.
func TestStatusesResponseErrorHandler_NonAPIErrorIsAGeneric500(t *testing.T) {
	srv := newStatusesTestServer(servicesmocks.NewMockStatusManagementServiceInterface(t))
	req := httptest.NewRequest(http.MethodGet, testStatusesBase, nil)
	w := httptest.NewRecorder()

	srv.statusesResponseErrorHandler(w, req, errors.New("pq: secret detail"))

	detail := requireStatusesProblem(t, w, http.StatusInternalServerError, "INTERNAL_ERROR")
	assert.NotContains(t, detail, "secret")
}

// errReader fails every read, standing in for a client that disconnects
// mid-body.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestPeekJSONObjectBody(t *testing.T) {
	peek := func(body string) (map[string]json.RawMessage, bool, *http.Request) {
		req := httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(body))
		fields, ok := peekJSONObjectBody(httptest.NewRecorder(), req)
		return fields, ok, req
	}

	t.Run("an object yields its keys and leaves the body readable", func(t *testing.T) {
		fields, ok, req := peek(`{"name":"x","slug":"y"}`)
		require.True(t, ok)
		assert.Len(t, fields, 2)
		rest, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"name":"x","slug":"y"}`, string(rest))
	})

	// The generated decoder accepts `null` as a zero-valued struct, so a guard
	// that skipped it would let an all-zero body through unjudged.
	t.Run("null is an object with no keys, not a body to skip", func(t *testing.T) {
		fields, ok, _ := peek(`null`)
		require.True(t, ok)
		assert.NotNil(t, fields)
		assert.Empty(t, fields)
	})

	for name, body := range map[string]string{"empty": "", "blank": " \n", "an array": `[1]`, "malformed": `{"a":`} {
		t.Run(name+" is left for the decoder", func(t *testing.T) {
			fields, ok, _ := peek(body)
			require.True(t, ok)
			assert.Nil(t, fields)
		})
	}

	t.Run("an unreadable body is a 400 and stops the request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/x", errReader{})
		w := httptest.NewRecorder()
		fields, ok := peekJSONObjectBody(w, req)
		assert.False(t, ok)
		assert.Nil(t, fields)
		requireStatusesProblem(t, w, http.StatusBadRequest, "BAD_REQUEST")
	})
}

func TestRejectUnknownStatusBodyFields_LeavesOtherRequestsAlone(t *testing.T) {
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})

	for name, req := range map[string]*http.Request{
		"a GET":              httptest.NewRequest(http.MethodGet, testStatusesBase+"?resource_type=artifact", nil),
		"a DELETE":           httptest.NewRequest(http.MethodDelete, testStatusesByID, strings.NewReader(`{"slug":"x"}`)),
		"an empty POST body": httptest.NewRequest(http.MethodPost, testStatusesBase, strings.NewReader("  ")),
	} {
		t.Run(name, func(t *testing.T) {
			reached = false
			w := httptest.NewRecorder()
			rejectUnknownStatusBodyFields(next).ServeHTTP(w, req)
			assert.True(t, reached)
			assert.Equal(t, http.StatusNoContent, w.Code)
		})
	}

	t.Run("an unreadable body is answered 400 without reaching the handler", func(t *testing.T) {
		reached = false
		w := httptest.NewRecorder()
		rejectUnknownStatusBodyFields(next).ServeHTTP(w,
			httptest.NewRequest(http.MethodPost, testStatusesBase, errReader{}))
		assert.False(t, reached)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("the body is restored for the decoder after a pass", func(t *testing.T) {
		var seen []byte
		read := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			seen = body
		})
		req := httptest.NewRequest(http.MethodPatch, testStatusesByID, strings.NewReader(`{"name":"x"}`))
		rejectUnknownStatusBodyFields(read).ServeHTTP(httptest.NewRecorder(), req)
		assert.JSONEq(t, `{"name":"x"}`, string(seen))
	})
}

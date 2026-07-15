package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// The project handlers must turn services.ErrPermissionDenied into a 403.
//
// This is not a formality: the handlers map errors by matching substrings of the
// message, and ErrPermissionDenied's text ("permission denied: user is not a
// member of the team") matches none of their branches — notably NOT the existing
// "user is not a member of the specified team" check. Without an explicit
// errors.Is branch ordered before that chain, a denial renders as a 500.

const (
	projRBACHandlerUser = "user-caller"
	projRBACHandlerTeam = "team-123"
)

// MockProjectRBACContainer overrides only the project service.
type MockProjectRBACContainer struct {
	BaseMockContainer
	projectService *svcmocks.MockProjectServiceInterface
}

func (m *MockProjectRBACContainer) ProjectService() services.ProjectServiceInterface {
	return m.projectService
}

// createTestProjectRBACServer wires the three mutating project routes. The team
// validation middleware is deliberately not mounted — it enforces tenancy, and
// the role decision under test happens in the service.
func createTestProjectRBACServer(svc *svcmocks.MockProjectServiceInterface) *Server {
	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: &MockProjectRBACContainer{projectService: svc},
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}

	r.Route("/api/v1/{team_id}/projects", func(r chi.Router) {
		r.Post("/", srv.handleCreateProject)
		r.Put("/{slug}", srv.handleUpdateProject)
		r.Delete("/{slug}", srv.handleDeleteProject)
	})

	return srv
}

func projRBACRequest(method, path, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, projRBACHandlerUser))
}

func TestProjectHandlers_PermissionDeniedIsForbidden(t *testing.T) {
	base := "/api/v1/" + projRBACHandlerTeam + "/projects"

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		stub   func(*svcmocks.MockProjectServiceInterface)
		detail string
	}{
		{
			name:   "create",
			method: http.MethodPost,
			path:   base,
			body:   `{"name":"P","slug":"p"}`,
			stub: func(m *svcmocks.MockProjectServiceInterface) {
				m.EXPECT().CreateProject(projRBACHandlerUser, projRBACHandlerTeam, mock.Anything).
					Return(nil, services.ErrPermissionDenied).Once()
			},
			detail: "Only team owners and admins can create projects",
		},
		{
			name:   "update",
			method: http.MethodPut,
			path:   base + "/my-project",
			body:   `{"name":"Renamed"}`,
			stub: func(m *svcmocks.MockProjectServiceInterface) {
				m.EXPECT().UpdateProject(projRBACHandlerTeam, projRBACHandlerUser, "my-project", mock.Anything).
					Return(nil, services.ErrPermissionDenied).Once()
			},
			detail: "Only team owners and admins can update projects",
		},
		{
			name:   "delete",
			method: http.MethodDelete,
			path:   base + "/my-project",
			stub: func(m *svcmocks.MockProjectServiceInterface) {
				m.EXPECT().DeleteProject(projRBACHandlerTeam, projRBACHandlerUser, "my-project").
					Return(services.ErrPermissionDenied).Once()
			},
			detail: "Only team owners and admins can delete projects",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := svcmocks.NewMockProjectServiceInterface(t)
			tc.stub(svc)

			srv := createTestProjectRBACServer(svc)
			req := projRBACRequest(tc.method, tc.path, tc.body)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusForbidden, w.Code, "denial must be 403, not 500: %s", w.Body.String())
			// #235 AC: project 403s are RFC 9457 FORBIDDEN.
			assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
			assert.Contains(t, w.Body.String(), `"code":"FORBIDDEN"`)
			assert.Contains(t, w.Body.String(), tc.detail)
		})
	}
}

// TestProjectHandlers_NonMemberDenialIsForbidden guards the exact trap above: a
// non-member's denial message contains "not a member of the team", which is one
// word away from the handlers' pre-existing "not a member of the specified team"
// branch and matches nothing else. It must still be a 403.
func TestProjectHandlers_NonMemberDenialIsForbidden(t *testing.T) {
	nonMemberErr := services.ErrPermissionDenied

	svc := svcmocks.NewMockProjectServiceInterface(t)
	svc.EXPECT().DeleteProject(projRBACHandlerTeam, projRBACHandlerUser, "my-project").
		Return(nonMemberErr).Once()

	srv := createTestProjectRBACServer(svc)
	req := projRBACRequest(http.MethodDelete, "/api/v1/"+projRBACHandlerTeam+"/projects/my-project", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

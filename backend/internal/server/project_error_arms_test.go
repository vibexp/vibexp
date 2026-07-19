package server

// Error-arm + validation coverage for project_handlers.go (coverage epic #358 /
// issue #393). The handleXProjectError helpers and the field-length validators
// are pure functions over (ResponseWriter, error) — they only touch s.logger and
// writeErrorResponse — so they are driven directly against a recorder, one row
// per branch, asserting the HTTP status each error class maps to.

import (
	stderrors "errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

func projectErrArmsServer() *Server {
	return &Server{logger: slog.New(slog.DiscardHandler)}
}

func TestHandleCreateProjectError_Arms(t *testing.T) {
	srv := projectErrArmsServer()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"permission denied → 403", services.ErrPermissionDenied, http.StatusForbidden},
		{"already exists → 409", stderrors.New("project already exists"), http.StatusConflict},
		{"not a team member → 403", stderrors.New("user is not a member of the specified team"), http.StatusForbidden},
		{"unknown → 500", stderrors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/team/projects", nil)
			srv.handleCreateProjectError(rr, req, "user-1", tt.err)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandleGetProjectError_Arms(t *testing.T) {
	srv := projectErrArmsServer()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not found → 404", stderrors.New("project not found"), http.StatusNotFound},
		{"unknown → 500", stderrors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.handleGetProjectError(rr, "user-1", "slug", tt.err)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandleUpdateProjectError_Arms(t *testing.T) {
	srv := projectErrArmsServer()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"permission denied → 403", services.ErrPermissionDenied, http.StatusForbidden},
		{"not found → 404", stderrors.New("project not found"), http.StatusNotFound},
		{"already exists → 409", stderrors.New("slug already exists"), http.StatusConflict},
		{"version mismatch → 409", stderrors.New("version mismatch detected"), http.StatusConflict},
		{"not a team member → 403", stderrors.New("user is not a member of the specified team"), http.StatusForbidden},
		{"cross-team move → 400", stderrors.New("resources cannot be moved between teams"), http.StatusBadRequest},
		{"unknown → 500", stderrors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/team/projects/slug", nil)
			srv.handleUpdateProjectError(rr, req, "user-1", "slug", tt.err)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandleDeleteProjectError_Arms(t *testing.T) {
	srv := projectErrArmsServer()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"permission denied → 403", services.ErrPermissionDenied, http.StatusForbidden},
		{"not found → 404", stderrors.New("project not found"), http.StatusNotFound},
		{"last project → 400", services.NewCannotDeleteLastProjectError("team-1", "slug"), http.StatusBadRequest},
		{"unknown → 500", stderrors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/team/projects/slug", nil)
			srv.handleDeleteProjectError(rr, req, "user-1", "slug", tt.err)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestValidateProjectFieldLengths_Arms(t *testing.T) {
	srv := projectErrArmsServer()

	t.Run("all within limits passes", func(t *testing.T) {
		rr := httptest.NewRecorder()
		ok := srv.validateProjectFieldLengths(rr, "name", "slug", "desc", "https://x", "https://y")
		assert.True(t, ok)
	})

	tooLong := strings.Repeat("a", 300)
	fields := []struct {
		name              string
		name_, slug, desc string
		git, home         string
	}{
		{"name too long", tooLong, "", "", "", ""},
		{"slug too long", "n", strings.Repeat("s", 200), "", "", ""},
		{"description too long", "n", "", strings.Repeat("d", 1100), "", ""},
		{"git url too long", "n", "", "", strings.Repeat("g", 600), ""},
		{"homepage too long", "n", "", "", "", strings.Repeat("h", 600)},
	}
	for _, f := range fields {
		t.Run(f.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			ok := srv.validateProjectFieldLengths(rr, f.name_, f.slug, f.desc, f.git, f.home)
			assert.False(t, ok)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestValidateUpdateProjectRequest_DelegatesToLengthCheck(t *testing.T) {
	srv := projectErrArmsServer()
	tooLong := strings.Repeat("a", 300)

	t.Run("nil pointers pass (nothing to validate)", func(t *testing.T) {
		rr := httptest.NewRecorder()
		ok := srv.validateUpdateProjectRequest(rr, &models.UpdateProjectRequest{})
		assert.True(t, ok)
	})

	t.Run("over-long name fails", func(t *testing.T) {
		rr := httptest.NewRecorder()
		ok := srv.validateUpdateProjectRequest(rr, &models.UpdateProjectRequest{Name: &tooLong})
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

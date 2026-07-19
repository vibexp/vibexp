package server

// Coverage for the pure helpers of prompt_handlers.go (coverage epic #358 /
// issue #393): parseIntParam bounds, the create/update request validators, and
// the handleUpdatePromptError / handlePromptVersionError mappers. All are pure
// over their inputs (validators/mappers touch only s.logger + writeErrorResponse),
// so they are driven directly, one row per branch.

import (
	stderrors "errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

func promptErrArmsServer() *Server {
	return &Server{logger: slog.New(slog.DiscardHandler)}
}

func TestParseIntParam(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		min, max, want int
	}{
		{"empty is zero", "", 1, 100, 0},
		{"non-numeric is zero", "abc", 1, 100, 0},
		{"below min is zero", "0", 1, 100, 0},
		{"above max is zero", "150", 1, 100, 0},
		{"within range passes through", "42", 1, 100, 42},
		{"max=0 disables the upper bound", "9999", 1, 0, 9999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseIntParam(tt.query, tt.min, tt.max))
		})
	}
}

func TestValidateCreatePromptRequest_Arms(t *testing.T) {
	valid := func() *models.CreatePromptRequest {
		return &models.CreatePromptRequest{Name: "N", Slug: "s", Body: "b", ProjectID: "p"}
	}
	tests := []struct {
		name   string
		mutate func(*models.CreatePromptRequest)
		wantOK bool
	}{
		{"valid", func(*models.CreatePromptRequest) {}, true},
		{"missing name", func(r *models.CreatePromptRequest) { r.Name = "" }, false},
		{"missing slug", func(r *models.CreatePromptRequest) { r.Slug = "" }, false},
		{"missing body", func(r *models.CreatePromptRequest) { r.Body = "" }, false},
		{"missing project_id", func(r *models.CreatePromptRequest) { r.ProjectID = "" }, false},
		{"description too long", func(r *models.CreatePromptRequest) { r.Description = strings.Repeat("d", 201) }, false},
		{"name too long", func(r *models.CreatePromptRequest) { r.Name = strings.Repeat("n", 51) }, false},
		{"slug too long", func(r *models.CreatePromptRequest) { r.Slug = strings.Repeat("s", 256) }, false},
		{"invalid status", func(r *models.CreatePromptRequest) { r.Status = "archived" }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid()
			tt.mutate(req)
			rr := httptest.NewRecorder()
			ok := validateCreatePromptRequest(req, rr)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				assert.Equal(t, http.StatusBadRequest, rr.Code)
			}
		})
	}
}

func TestValidateUpdatePromptRequest_Arms(t *testing.T) {
	srv := promptErrArmsServer()
	strp := func(s string) *string { return &s }
	tests := []struct {
		name   string
		req    *models.UpdatePromptRequest
		wantOK bool
	}{
		{"all nil passes", &models.UpdatePromptRequest{}, true},
		{"valid status passes", &models.UpdatePromptRequest{Status: strp("published")}, true},
		{"name too long", &models.UpdatePromptRequest{Name: strp(strings.Repeat("n", 51))}, false},
		{"slug too long", &models.UpdatePromptRequest{Slug: strp(strings.Repeat("s", 256))}, false},
		{"description too long", &models.UpdatePromptRequest{Description: strp(strings.Repeat("d", 201))}, false},
		{"invalid status", &models.UpdatePromptRequest{Status: strp("archived")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			ok := srv.validateUpdatePromptRequest(tt.req, rr)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				assert.Equal(t, http.StatusBadRequest, rr.Code)
			}
		})
	}
}

func TestHandleUpdatePromptError_Arms(t *testing.T) {
	srv := promptErrArmsServer()
	slug := "my-slug"
	req := &models.UpdatePromptRequest{Slug: &slug}
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"permission denied → 403", services.ErrPermissionDenied, http.StatusForbidden},
		{"not found → 404", repositories.ErrPromptNotFound, http.StatusNotFound},
		{"version mismatch → 409", stderrors.New("version mismatch"), http.StatusConflict},
		{"slug conflict → 409", stderrors.New("prompt with slug 'my-slug' already exists for this user"), http.StatusConflict},
		{"project not found → 400", stderrors.New("project does not belong to user"), http.StatusBadRequest},
		{"not a team member → 403", stderrors.New("user is not a member of the specified team"), http.StatusForbidden},
		{"cross-team move → 400", stderrors.New("resources cannot be moved between teams"), http.StatusBadRequest},
		{"unknown → 500", stderrors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.handleUpdatePromptError(tt.err, req, rr)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandlePromptVersionError_Arms(t *testing.T) {
	srv := promptErrArmsServer()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"sentinel not-found → 404", repositories.ErrPromptNotFound, http.StatusNotFound},
		{"not-found fragment → 404", stderrors.New("version not found"), http.StatusNotFound},
		{"unknown → 500", stderrors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.handlePromptVersionError(rr, "user-1", "slug", tt.err)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

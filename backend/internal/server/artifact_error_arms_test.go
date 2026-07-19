package server

// Error-arm + validation coverage for artifact_handlers.go (coverage epic #358
// / issue #393). The handleXArtifactError helpers are pure over
// (ResponseWriter, error); the request validators short-circuit on the first
// bad field before any TypeService call, so both are driven directly against a
// recorder, one row per branch.

import (
	"context"
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

func artifactErrArmsServer() *Server {
	return &Server{logger: slog.New(slog.DiscardHandler)}
}

func TestHandleCreateArtifactError_Arms(t *testing.T) {
	srv := artifactErrArmsServer()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"permission denied → 403", services.ErrPermissionDenied, http.StatusForbidden},
		{"already exists → 409", stderrors.New("slug already exists"), http.StatusConflict},
		{"project not found → 400", stderrors.New("project not found"), http.StatusBadRequest},
		{"not a team member → 403", stderrors.New("user is not a member of the specified team"), http.StatusForbidden},
		{"cross-team move → 400", stderrors.New("resources cannot be moved between teams"), http.StatusBadRequest},
		{"unknown → 500", stderrors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.handleCreateArtifactError(rr, "user-1", tt.err)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandleGetArtifactError_Arms(t *testing.T) {
	srv := artifactErrArmsServer()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not found → 404", stderrors.New("artifact not found"), http.StatusNotFound},
		{"unknown → 500", stderrors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.handleGetArtifactError(rr, "user-1", "proj-1", "slug", tt.err)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandleUpdateArtifactError_Arms(t *testing.T) {
	srv := artifactErrArmsServer()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"permission denied → 403", services.ErrPermissionDenied, http.StatusForbidden},
		{"not found → 404", stderrors.New("artifact not found"), http.StatusNotFound},
		{"already exists → 409", stderrors.New("slug already exists"), http.StatusConflict},
		// Note: a literal "project not found" hits the errNotFoundFragment ("not
		// found") branch first (→ 404), so the dedicated 400 arm below it is
		// unreachable and intentionally not asserted.
		{"not a team member → 403", stderrors.New("user is not a member of the specified team"), http.StatusForbidden},
		{"cross-team move → 400", stderrors.New("resources cannot be moved between teams"), http.StatusBadRequest},
		{"unknown → 500", stderrors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.handleUpdateArtifactError(rr, "user-1", "proj-1", "slug", tt.err)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

// TestValidateCreateArtifactRequest_RequiredFields drives every required-field
// branch that returns before any TypeService lookup.
func TestValidateCreateArtifactRequest_RequiredFields(t *testing.T) {
	srv := artifactErrArmsServer()
	const validUUID = "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name string
		req  *models.CreateArtifactRequest
	}{
		{"missing project_id", &models.CreateArtifactRequest{}},
		{"invalid project_id", &models.CreateArtifactRequest{ProjectID: "not-a-uuid"}},
		{"missing slug", &models.CreateArtifactRequest{ProjectID: validUUID}},
		{"missing title", &models.CreateArtifactRequest{ProjectID: validUUID, Slug: "s"}},
		{"missing content", &models.CreateArtifactRequest{ProjectID: validUUID, Slug: "s", Title: "T"}},
		{
			"over-long slug",
			&models.CreateArtifactRequest{
				ProjectID: validUUID, Slug: strings.Repeat("s", 300), Title: "T", Content: "c",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			ok := srv.validateCreateArtifactRequest(context.Background(), rr, "team-1", tt.req)
			assert.False(t, ok)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

// TestValidateUpdateArtifactRequest_BadFields covers the invalid-UUID and
// over-long-field branches, both of which return before a TypeService call.
func TestValidateUpdateArtifactRequest_BadFields(t *testing.T) {
	srv := artifactErrArmsServer()
	badUUID := "not-a-uuid"
	longTitle := strings.Repeat("t", 300)

	t.Run("invalid project_id", func(t *testing.T) {
		rr := httptest.NewRecorder()
		ok := srv.validateUpdateArtifactRequest(context.Background(), rr, "team-1",
			&models.UpdateArtifactRequest{ProjectID: &badUUID})
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("over-long title", func(t *testing.T) {
		rr := httptest.NewRecorder()
		ok := srv.validateUpdateArtifactRequest(context.Background(), rr, "team-1",
			&models.UpdateArtifactRequest{Title: &longTitle})
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

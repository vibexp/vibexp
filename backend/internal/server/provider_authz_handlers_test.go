package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// Regression tests for #464 at the HTTP boundary: a service-level permission
// denial must surface as 403, not as the generic "create/update failed" the
// provider handlers otherwise return. Without this the SPA cannot tell a role
// problem from a broken config, and the 500 hides the denial from operators.

const authzTestUserID = "user-provider-authz"

// assertProviderForbidden drives one request and asserts the 403 mapping.
func assertProviderForbidden(t *testing.T, srv *Server, req *http.Request) {
	t.Helper()

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var body map[string]interface{}
	assert.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "FORBIDDEN", body["code"])
}

func TestEmbeddingProviderHandlers_PermissionDeniedIsForbidden(t *testing.T) {
	const base = "/api/v1/team-123/embedding-providers"

	tests := []struct {
		name   string
		expect func(*MockEmbeddingProviderContainer)
		req    func() *http.Request
	}{
		{
			name: "create",
			expect: func(c *MockEmbeddingProviderContainer) {
				c.embeddingProviderService.On("CreateEmbeddingProvider",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrPermissionDenied)
			},
			req: func() *http.Request {
				return makeAuthenticatedEmbeddingProviderRequest(http.MethodPost, base,
					models.CreateEmbeddingProviderRequest{
						Name: "n", ProviderType: "openai_compatible", Model: "m",
					}, authzTestUserID)
			},
		},
		{
			name: "update",
			expect: func(c *MockEmbeddingProviderContainer) {
				c.embeddingProviderService.On("GetEmbeddingProvider",
					mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrProviderNotFound).Maybe()
				c.embeddingProviderService.On("UpdateEmbeddingProvider",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrPermissionDenied)
			},
			req: func() *http.Request {
				return makeAuthenticatedEmbeddingProviderRequest(http.MethodPut, base+"/provider-1",
					models.UpdateEmbeddingProviderRequest{}, authzTestUserID)
			},
		},
		{
			name: "delete",
			expect: func(c *MockEmbeddingProviderContainer) {
				c.embeddingProviderService.On("DeleteEmbeddingProvider",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(services.ErrPermissionDenied)
			},
			req: func() *http.Request {
				return makeAuthenticatedEmbeddingProviderRequest(http.MethodDelete, base+"/provider-1",
					nil, authzTestUserID)
			},
		},
		{
			name: "validate",
			expect: func(c *MockEmbeddingProviderContainer) {
				c.embeddingProviderService.On("ValidateEmbeddingProvider",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrPermissionDenied)
			},
			req: func() *http.Request {
				return makeAuthenticatedEmbeddingProviderRequest(http.MethodPost, base+"/validate",
					models.ValidateEmbeddingProviderRequest{
						ProviderType: "openai_compatible",
						BaseURL:      "https://api.openai.com/v1",
						Model:        "m",
					}, authzTestUserID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := newMockEmbeddingProviderContainer(t)
			tt.expect(container)
			assertProviderForbidden(t, createTestEmbeddingProviderServer(container), tt.req())
		})
	}
}

func TestModelProviderHandlers_PermissionDeniedIsForbidden(t *testing.T) {
	const base = "/api/v1/team-123/model-providers"

	tests := []struct {
		name   string
		expect func(*MockModelProviderContainer)
		req    func() *http.Request
	}{
		{
			name: "create",
			expect: func(c *MockModelProviderContainer) {
				c.modelProviderService.On("CreateModelProvider",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrPermissionDenied)
			},
			req: func() *http.Request {
				return makeAuthenticatedModelProviderRequest(http.MethodPost, base,
					models.CreateModelProviderRequest{
						Name: "n", ProviderType: "openai_compatible", Model: "m",
					}, authzTestUserID)
			},
		},
		{
			name: "update",
			expect: func(c *MockModelProviderContainer) {
				c.modelProviderService.On("UpdateModelProvider",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrPermissionDenied)
			},
			req: func() *http.Request {
				return makeAuthenticatedModelProviderRequest(http.MethodPut, base+"/provider-1",
					models.UpdateModelProviderRequest{}, authzTestUserID)
			},
		},
		{
			name: "delete",
			expect: func(c *MockModelProviderContainer) {
				c.modelProviderService.On("DeleteModelProvider",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(services.ErrPermissionDenied)
			},
			req: func() *http.Request {
				return makeAuthenticatedModelProviderRequest(http.MethodDelete, base+"/provider-1",
					nil, authzTestUserID)
			},
		},
		{
			name: "validate",
			expect: func(c *MockModelProviderContainer) {
				c.modelProviderService.On("ValidateModelProvider",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrPermissionDenied)
			},
			req: func() *http.Request {
				return makeAuthenticatedModelProviderRequest(http.MethodPost, base+"/validate",
					models.ValidateModelProviderRequest{
						ProviderType: "openai_compatible",
						BaseURL:      "https://api.openai.com/v1",
						Model:        "m",
					}, authzTestUserID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := newMockModelProviderContainer(t)
			tt.expect(container)
			assertProviderForbidden(t, createTestModelProviderServer(container), tt.req())
		})
	}
}

// TestEmbeddingMaintenance_DeniedForMember covers the two destructive endpoints
// that do not pass through a provider service, so they are gated in the handler:
// reprocess (spends the team's provider budget on a full re-embed) and
// clear-embeddings (deletes the team's entire embedding index). Both were
// reachable by any team member before #464.
func TestEmbeddingMaintenance_DeniedForMember(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "reprocess",
			method: http.MethodPost,
			path:   "/api/v1/team-123/embedding-providers/provider-1/reprocess",
		},
		{
			name:   "clear embeddings",
			method: http.MethodDelete,
			path:   "/api/v1/team-123/settings/embedding-providers/embeddings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := newMockEmbeddingProviderContainer(t)
			// Override the permissive default with an explicit denial.
			container.authzService.ExpectedCalls = nil
			container.authzService.On("Can",
				mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(services.ErrPermissionDenied)

			srv := createTestEmbeddingProviderServer(container)
			req := makeAuthenticatedEmbeddingProviderRequest(tt.method, tt.path, nil, authzTestUserID)

			assertProviderForbidden(t, srv, req)

			// The destructive work must not have started.
			container.embeddingRepository.AssertNotCalled(t, "DeleteByTeam")
			container.embeddingProviderService.AssertNotCalled(t, "GetEmbeddingProvider")
		})
	}
}

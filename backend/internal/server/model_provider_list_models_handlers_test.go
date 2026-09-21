package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Handler tests for the model listing op (#1070), mounted under both prefixes.

var modelProviderPrefixes = []string{"model-providers", "settings/model-providers"}

func listModelsPath(prefix string) string {
	return "/api/v1/" + testModelProviderTeamID + "/" + prefix + "/models"
}

// TestHandleListProviderModels_SpecConformance covers a successful listing and a
// supported:false outcome on both prefixes, each validated against the spec.
func TestHandleListProviderModels_SpecConformance(t *testing.T) {
	outcomes := map[string]*models.ProviderModelList{
		"listed": {
			Supported: true,
			Models: []models.ProviderModel{
				{ID: "gpt-4o", OwnedBy: "openai"},
				{ID: "llama3"},
			},
		},
		"unsupported": {Supported: false, Message: "unauthorized"},
	}

	for _, prefix := range modelProviderPrefixes {
		for name, outcome := range outcomes {
			t.Run(prefix+"/"+name, func(t *testing.T) {
				mockContainer := newMockModelProviderContainer(t)

				apiKey := "sk-test"
				reqBody := models.ListProviderModelsRequest{
					ProviderType: "openai_compatible",
					BaseURL:      "https://api.openai.com/v1",
					APIKey:       &apiKey,
				}
				mockContainer.modelProviderService.
					On("ListProviderModels", mock.Anything, testModelProviderTeamID, "user-123", reqBody).
					Return(outcome, nil)

				srv := createTestModelProviderServer(mockContainer)
				req := makeAuthenticatedModelProviderRequest("POST", listModelsPath(prefix), reqBody, "user-123")
				w := httptest.NewRecorder()

				srv.ServeHTTP(w, req)

				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				specconformance.AssertConformsToSpec(t, req, w)

				var raw map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
				if !outcome.Supported {
					// AssertConformsToSpec accepts null for a required array, so
					// only a literal check proves the nil list marshals as [] (#125).
					assert.JSONEq(t, `[]`, string(raw["models"]))
					assert.JSONEq(t, `"unauthorized"`, string(raw["message"]))
				} else {
					_, hasMessage := raw["message"]
					assert.False(t, hasMessage, "an empty message is omitted")
				}
			})
		}
	}
}

// TestHandleListProviderModels_ProviderIDIsPassedThrough pins that a blank key
// plus provider_id reaches the service, which owns the stored-key reuse.
func TestHandleListProviderModels_ProviderIDIsPassedThrough(t *testing.T) {
	mockContainer := newMockModelProviderContainer(t)
	providerID := "550e8400-e29b-41d4-a716-446655440000"

	mockContainer.modelProviderService.
		On("ListProviderModels", mock.Anything, testModelProviderTeamID, "user-123",
			models.ListProviderModelsRequest{
				ProviderType: "openai_compatible",
				BaseURL:      "https://api.openai.com/v1",
				ProviderID:   providerID,
			}).
		Return(&models.ProviderModelList{Supported: true}, nil)

	srv := createTestModelProviderServer(mockContainer)
	req := makeAuthenticatedModelProviderRequest("POST", listModelsPath("model-providers"), map[string]any{
		"provider_type": "openai_compatible",
		"base_url":      "https://api.openai.com/v1",
		"provider_id":   providerID,
	}, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"supported":true,"models":[]}`, w.Body.String())
}

func TestHandleListProviderModels_ValidationError(t *testing.T) {
	tests := []struct {
		name    string
		reqBody map[string]any
	}{
		{"missing provider_type", map[string]any{"base_url": "https://api.openai.com/v1"}},
		{"missing base_url", map[string]any{"provider_type": "openai_compatible"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := createTestModelProviderServer(newMockModelProviderContainer(t))
			req := makeAuthenticatedModelProviderRequest("POST", listModelsPath("model-providers"), tt.reqBody, "user-123")
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

func TestHandleListProviderModels_ServiceErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"member is forbidden", errors.Join(services.ErrPermissionDenied, errors.New("member")), http.StatusForbidden},
		{"unknown provider_id", services.ErrModelProviderNotFound, http.StatusNotFound},
		{"internal failure", errors.New("decrypt failed"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)
			mockContainer.modelProviderService.
				On("ListProviderModels", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return((*models.ProviderModelList)(nil), tt.err)

			srv := createTestModelProviderServer(mockContainer)
			req := makeAuthenticatedModelProviderRequest("POST", listModelsPath("settings/model-providers"),
				map[string]any{"provider_type": "openai_compatible", "base_url": "https://api.openai.com/v1"},
				"user-123")
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, tt.status, w.Code)
			assert.NotContains(t, w.Body.String(), "decrypt failed", "the raw error never reaches the client")
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

package server

import (
	"bytes"
	"context"
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
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// githubAppPrefixes are the two mounts every operation is served on. Each test
// loops over both, so a route registered on only one prefix fails here rather
// than in production for whichever client happens to use the other.
var githubAppPrefixes = []string{
	"integrations/github/app",
	"settings/github-app",
}

// Fixture secrets. These strings must never appear in a response body; the
// no-leak test greps for them explicitly rather than trusting struct tags,
// because a tag typo is exactly the failure that would otherwise ship silently.
const (
	githubAppTestPrivateKey     = "PRIVATE-KEY-FIXTURE-must-never-be-returned"
	githubAppTestClientSecret   = "CLIENT-SECRET-FIXTURE-must-never-be-returned"
	githubAppTestRoutingSegment = "test-routing-segment"
)

type MockGitHubAppConfigContainer struct {
	BaseMockContainer
	mock.Mock
	githubAppConfigService *svcmocks.MockGitHubAppConfigServiceInterface
}

func (m *MockGitHubAppConfigContainer) GitHubAppConfigService() services.GitHubAppConfigServiceInterface {
	return m.githubAppConfigService
}

func newMockGitHubAppConfigContainer(t *testing.T) *MockGitHubAppConfigContainer {
	return &MockGitHubAppConfigContainer{
		githubAppConfigService: svcmocks.NewMockGitHubAppConfigServiceInterface(t),
	}
}

// createTestGitHubAppConfigServer wires the PRODUCTION route setup, so the tests
// exercise the same route tree the server mounts (including both prefixes).
func createTestGitHubAppConfigServer(container *MockGitHubAppConfigContainer) *Server {
	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: container,
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}

	// Mount the PRODUCTION route function under each prefix. Registering the
	// verbs by hand here would make these tests pass even if a route were
	// missing from production or registered on only one prefix — the exact
	// regressions this file exists to catch. teamValidationMiddleware is applied
	// by mountGitHubAppConfigRoutes in production and skipped here, so these
	// tests cover the handlers and the spec contract rather than re-testing team
	// membership.
	for _, prefix := range githubAppPrefixes {
		r.Route("/api/v1/{team_id}/"+prefix, srv.setupGitHubAppConfigRoutes)
	}

	return srv
}

func makeGitHubAppRequest(method, path string, body any) *http.Request {
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			panic(err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
}

func sampleGitHubAppConfigResponse() *models.GitHubAppConfigResponse {
	userID := "user-123"
	return &models.GitHubAppConfigResponse{
		GitHubAppConfig: models.GitHubAppConfig{
			ID:                     "b1234567-89ab-cdef-0123-456789abcdef",
			TeamID:                 "team-123",
			UserID:                 &userID,
			AppID:                  "123456",
			AppSlug:                "acme-vibexp",
			ClientID:               "Iv1.a1b2c3d4",
			PrivateKeyEncrypted:    githubAppTestPrivateKey,
			ClientSecretEncrypted:  githubAppTestClientSecret,
			WebhookSecretEncrypted: "encrypted-webhook-secret",
			WebhookToken:           githubAppTestRoutingSegment,
			CreatedAt:              time.Now(),
			UpdatedAt:              time.Now(),
			Version:                1,
		},
		HasPrivateKey:    true,
		HasClientSecret:  true,
		HasWebhookSecret: true,
		WebhookURL:       "https://vibexp.example.com/api/v1/webhooks/github/" + githubAppTestRoutingSegment,
	}
}

func validGitHubAppCreateRequest() models.CreateGitHubAppConfigRequest {
	return models.CreateGitHubAppConfigRequest{
		AppID:        "123456",
		AppSlug:      "acme-vibexp",
		ClientID:     "Iv1.a1b2c3d4",
		PrivateKey:   githubAppTestPrivateKey,
		ClientSecret: githubAppTestClientSecret,
	}
}

// --- Spec-conformance coverage: one conforming response per documented
// operation, on both prefixes, so every operation maps to a covered payload
// and the ledger gains nothing (#122). ---

func TestHandleGetGitHubAppConfig_SpecConformance(t *testing.T) {
	for _, prefix := range githubAppPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockGitHubAppConfigContainer(t)
			c.githubAppConfigService.EXPECT().
				GetAppConfig(mock.Anything, "team-123").
				Return(sampleGitHubAppConfigResponse(), nil)

			req := makeGitHubAppRequest(http.MethodGet, "/api/v1/team-123/"+prefix, nil)
			w := httptest.NewRecorder()
			createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var got models.GitHubAppConfigResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.True(t, got.HasPrivateKey)
			assert.True(t, got.HasClientSecret)
			assert.True(t, got.HasWebhookSecret)
			assert.Contains(t, got.WebhookURL, githubAppTestRoutingSegment)
		})
	}
}

func TestHandleCreateGitHubAppConfig_SpecConformance(t *testing.T) {
	for _, prefix := range githubAppPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockGitHubAppConfigContainer(t)
			created := &models.GitHubAppConfigCreated{
				GitHubAppConfigResponse: *sampleGitHubAppConfigResponse(),
				WebhookSecret:           "generated-webhook-secret",
			}
			c.githubAppConfigService.EXPECT().
				CreateAppConfig(mock.Anything, "team-123", "user-123", validGitHubAppCreateRequest()).
				Return(created, nil)

			req := makeGitHubAppRequest(
				http.MethodPost, "/api/v1/team-123/"+prefix, validGitHubAppCreateRequest())
			w := httptest.NewRecorder()
			createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			// Create is the ONE response that carries the plaintext secret.
			var got models.GitHubAppConfigCreated
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, "generated-webhook-secret", got.WebhookSecret)
			assert.NotEmpty(t, got.WebhookURL)
		})
	}
}

func TestHandleUpdateGitHubAppConfig_SpecConformance(t *testing.T) {
	for _, prefix := range githubAppPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockGitHubAppConfigContainer(t)
			slug := "renamed-app"
			reqBody := models.UpdateGitHubAppConfigRequest{AppSlug: &slug}
			c.githubAppConfigService.EXPECT().
				UpdateAppConfig(mock.Anything, "team-123", "user-123", reqBody).
				Return(sampleGitHubAppConfigResponse(), nil)

			req := makeGitHubAppRequest(http.MethodPut, "/api/v1/team-123/"+prefix, reqBody)
			w := httptest.NewRecorder()
			createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

func TestHandleDeleteGitHubAppConfig_SpecConformance(t *testing.T) {
	for _, prefix := range githubAppPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockGitHubAppConfigContainer(t)
			c.githubAppConfigService.EXPECT().
				DeleteAppConfig(mock.Anything, "team-123", "user-123").
				Return(nil)

			req := makeGitHubAppRequest(http.MethodDelete, "/api/v1/team-123/"+prefix, nil)
			w := httptest.NewRecorder()
			createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusNoContent, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)
			assert.Empty(t, w.Body.Bytes(), "204 must carry no body")
		})
	}
}

func TestHandleValidateGitHubAppConfig_SpecConformance(t *testing.T) {
	for _, prefix := range githubAppPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockGitHubAppConfigContainer(t)
			c.githubAppConfigService.EXPECT().
				ValidateAppConfig(mock.Anything, "team-123", "user-123").
				Return(&models.ValidateGitHubAppResponse{
					IsValid:     true,
					Message:     "GitHub App configuration is valid",
					AppSlug:     "acme-vibexp",
					Permissions: map[string]string{"contents": "read", "metadata": "read"},
					Details: models.ValidateGitHubAppDetails{
						ResponseTime: 214,
						StatusCode:   http.StatusOK,
					},
				}, nil)

			req := makeGitHubAppRequest(
				http.MethodPost, "/api/v1/team-123/"+prefix+"/validate", nil)
			w := httptest.NewRecorder()
			createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)
		})
	}
}

// A failed probe is a 200 body, not an HTTP error, and its error_details must be
// one of the spec's enum values — the conformance assertion is what pins that.
func TestHandleValidateGitHubAppConfig_FailedProbeIsStill200(t *testing.T) {
	for _, prefix := range githubAppPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockGitHubAppConfigContainer(t)
			c.githubAppConfigService.EXPECT().
				ValidateAppConfig(mock.Anything, "team-123", "user-123").
				Return(&models.ValidateGitHubAppResponse{
					IsValid: false,
					Message: "GitHub rejected these credentials",
					Details: models.ValidateGitHubAppDetails{
						ResponseTime: 88,
						StatusCode:   http.StatusUnauthorized,
						ErrorDetails: "invalid_credentials",
					},
				}, nil)

			req := makeGitHubAppRequest(
				http.MethodPost, "/api/v1/team-123/"+prefix+"/validate", nil)
			w := httptest.NewRecorder()
			createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var got models.ValidateGitHubAppResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.False(t, got.IsValid)
			assert.Equal(t, "invalid_credentials", got.Details.ErrorDetails)
		})
	}
}

func TestHandleRotateGitHubAppWebhookToken_SpecConformance(t *testing.T) {
	for _, prefix := range githubAppPrefixes {
		t.Run(prefix, func(t *testing.T) {
			c := newMockGitHubAppConfigContainer(t)
			rotated := sampleGitHubAppConfigResponse()
			rotated.WebhookToken = "NEWtoken123"
			rotated.WebhookURL = "https://vibexp.example.com/api/v1/webhooks/github/NEWtoken123"
			c.githubAppConfigService.EXPECT().
				RotateWebhookToken(mock.Anything, "team-123", "user-123").
				Return(rotated, nil)

			req := makeGitHubAppRequest(
				http.MethodPost, "/api/v1/team-123/"+prefix+"/rotate-webhook-token", nil)
			w := httptest.NewRecorder()
			createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var got models.GitHubAppConfigResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Contains(t, got.WebhookURL, "NEWtoken123")
			assert.NotContains(t, got.WebhookURL, githubAppTestRoutingSegment,
				"the rotated URL must not still carry the old token")
		})
	}
}

// TestGitHubAppConfig_NoSecretEverLeaks greps the SERIALIZED response for the
// fixture secrets on every read path. The struct tags are supposed to prevent
// this, but a tag typo is silent — and the whole point of the has_* booleans is
// that a leak here hands over a team's GitHub credentials.
func TestGitHubAppConfig_NoSecretEverLeaks(t *testing.T) {
	leaks := []string{githubAppTestPrivateKey, githubAppTestClientSecret, "encrypted-webhook-secret"}

	assertNoLeak := func(t *testing.T, body []byte) {
		t.Helper()
		for _, secret := range leaks {
			assert.NotContains(t, string(body), secret, "a secret reached the response body")
		}
	}

	for _, prefix := range githubAppPrefixes {
		t.Run(prefix+"/get", func(t *testing.T) {
			c := newMockGitHubAppConfigContainer(t)
			c.githubAppConfigService.EXPECT().
				GetAppConfig(mock.Anything, "team-123").
				Return(sampleGitHubAppConfigResponse(), nil)

			req := makeGitHubAppRequest(http.MethodGet, "/api/v1/team-123/"+prefix, nil)
			w := httptest.NewRecorder()
			createTestGitHubAppConfigServer(c).ServeHTTP(w, req)
			assertNoLeak(t, w.Body.Bytes())
			// The routing token is exposed only inside webhook_url, never as a
			// field of its own.
			assert.NotContains(t, w.Body.String(), `"webhook_token"`)
		})

		t.Run(prefix+"/create", func(t *testing.T) {
			c := newMockGitHubAppConfigContainer(t)
			c.githubAppConfigService.EXPECT().
				CreateAppConfig(mock.Anything, "team-123", "user-123", validGitHubAppCreateRequest()).
				Return(&models.GitHubAppConfigCreated{
					GitHubAppConfigResponse: *sampleGitHubAppConfigResponse(),
					WebhookSecret:           "generated-webhook-secret",
				}, nil)

			req := makeGitHubAppRequest(
				http.MethodPost, "/api/v1/team-123/"+prefix, validGitHubAppCreateRequest())
			w := httptest.NewRecorder()
			createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

			// Create echoes back the request's private key nowhere, even though
			// the request carried it.
			assertNoLeak(t, w.Body.Bytes())
			// ...but it DOES carry the generated webhook secret, exactly once.
			assert.Contains(t, w.Body.String(), "generated-webhook-secret")
		})
	}
}

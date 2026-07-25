package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// apiValidationErrorForPrivateKey mirrors what the service returns for an
// unparseable PEM: a field-level APIError, not a bare error.
func apiValidationErrorForPrivateKey() error {
	return apierrors.NewValidationError(
		"The private key could not be parsed",
		[]apierrors.ValidationError{{
			Field:   "private_key",
			Message: "must be an RSA private key in PEM format (raw or base64-encoded)",
		}},
	)
}

// A GitHub App registration holds a team's GitHub credentials, so every mutating
// operation is owner/admin surface. These tests pin the boundary at the HTTP
// layer: the service's ErrPermissionDenied must surface as 403 FORBIDDEN, never
// as a generic 500 — the caller needs to know it is a role problem, and an
// operator needs the distinction in logs.
//
// Read is deliberately absent: it is gated by route-level team membership and
// returns no secret values, matching the provider convention.
func TestGitHubAppConfigHandlers_PermissionDeniedIsForbidden(t *testing.T) {
	tests := []struct {
		name   string
		expect func(*MockGitHubAppConfigContainer)
		method string
		suffix string
		body   any
	}{
		{
			name: "create",
			expect: func(c *MockGitHubAppConfigContainer) {
				c.githubAppConfigService.EXPECT().
					CreateAppConfig(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrPermissionDenied)
			},
			method: http.MethodPost,
			body:   validGitHubAppCreateRequest(),
		},
		{
			name: "update",
			expect: func(c *MockGitHubAppConfigContainer) {
				c.githubAppConfigService.EXPECT().
					UpdateAppConfig(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrPermissionDenied)
			},
			method: http.MethodPut,
			body:   models.UpdateGitHubAppConfigRequest{},
		},
		{
			name: "delete",
			expect: func(c *MockGitHubAppConfigContainer) {
				c.githubAppConfigService.EXPECT().
					DeleteAppConfig(mock.Anything, mock.Anything, mock.Anything).
					Return(services.ErrPermissionDenied)
			},
			method: http.MethodDelete,
		},
		{
			// Gated like a mutation: it makes the server perform an authenticated
			// outbound call with the team's credentials.
			name: "validate",
			expect: func(c *MockGitHubAppConfigContainer) {
				c.githubAppConfigService.EXPECT().
					ValidateAppConfig(mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrPermissionDenied)
			},
			method: http.MethodPost,
			suffix: "/validate",
		},
		{
			name: "rotate webhook token",
			expect: func(c *MockGitHubAppConfigContainer) {
				c.githubAppConfigService.EXPECT().
					RotateWebhookToken(mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrPermissionDenied)
			},
			method: http.MethodPost,
			suffix: "/rotate-webhook-token",
		},
	}

	for _, prefix := range githubAppPrefixes {
		for _, tt := range tests {
			t.Run(prefix+"/"+tt.name, func(t *testing.T) {
				c := newMockGitHubAppConfigContainer(t)
				tt.expect(c)

				req := makeGitHubAppRequest(
					tt.method, "/api/v1/team-123/"+prefix+tt.suffix, tt.body)
				w := httptest.NewRecorder()
				createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

				assert.Equal(t, http.StatusForbidden, w.Code)

				var body map[string]any
				require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
				assert.Equal(t, "FORBIDDEN", body["code"])
			})
		}
	}
}

// The conflict sentinels each carry their own code so the SPA can tell "you have
// no App yet" from "someone else owns this App" — both are 409, and collapsing
// them would make the UI unable to say anything useful.
func TestGitHubAppConfigHandlers_ConflictSentinels(t *testing.T) {
	tests := []struct {
		name     string
		expect   func(*MockGitHubAppConfigContainer)
		method   string
		suffix   string
		body     any
		wantCode string
	}{
		{
			name: "no app configured",
			expect: func(c *MockGitHubAppConfigContainer) {
				c.githubAppConfigService.EXPECT().
					GetAppConfig(mock.Anything, mock.Anything).
					Return(nil, services.ErrGitHubAppNotConfigured)
			},
			method:   http.MethodGet,
			wantCode: "GITHUB_APP_NOT_CONFIGURED",
		},
		{
			name: "app owned by another team",
			expect: func(c *MockGitHubAppConfigContainer) {
				c.githubAppConfigService.EXPECT().
					CreateAppConfig(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrGitHubAppAlreadyRegistered)
			},
			method:   http.MethodPost,
			body:     validGitHubAppCreateRequest(),
			wantCode: "GITHUB_APP_ALREADY_REGISTERED",
		},
		{
			name: "team already has an app",
			expect: func(c *MockGitHubAppConfigContainer) {
				c.githubAppConfigService.EXPECT().
					CreateAppConfig(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrGitHubAppConfigExists)
			},
			method:   http.MethodPost,
			body:     validGitHubAppCreateRequest(),
			wantCode: "GITHUB_APP_CONFIG_EXISTS",
		},
		{
			name: "concurrent modification",
			expect: func(c *MockGitHubAppConfigContainer) {
				c.githubAppConfigService.EXPECT().
					UpdateAppConfig(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, services.ErrGitHubAppConfigConflict)
			},
			method:   http.MethodPut,
			body:     models.UpdateGitHubAppConfigRequest{},
			wantCode: "GITHUB_APP_CONFIG_CONFLICT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newMockGitHubAppConfigContainer(t)
			tt.expect(c)

			req := makeGitHubAppRequest(
				tt.method, "/api/v1/team-123/"+githubAppPrefixes[0]+tt.suffix, tt.body)
			w := httptest.NewRecorder()
			createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

			assert.Equal(t, http.StatusConflict, w.Code,
				"the request is well-formed and the team addressable — 409, not 404")

			var body map[string]any
			require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
			assert.Equal(t, tt.wantCode, body["code"])
		})
	}
}

// A missing required field must be a 400 with a field list, so the UI can point
// at the offending input instead of showing a generic failure.
func TestGitHubAppConfigHandlers_ValidationErrors(t *testing.T) {
	t.Run("missing required fields", func(t *testing.T) {
		c := newMockGitHubAppConfigContainer(t)

		req := makeGitHubAppRequest(http.MethodPost,
			"/api/v1/team-123/"+githubAppPrefixes[0],
			models.CreateGitHubAppConfigRequest{AppID: "123456"})
		w := httptest.NewRecorder()
		createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var body map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "VALIDATION_FAILED", body["code"])
		// The service is never reached, so mockery fails the test if it were.
		assert.NotEmpty(t, body["validation_errors"])
	})

	// An unparseable private key reaches the service, which returns a field-level
	// APIError. It must pass through as a 400 rather than being flattened into a
	// 500 — it is user input, not a server fault.
	t.Run("unparseable private key is a field error, not a 500", func(t *testing.T) {
		c := newMockGitHubAppConfigContainer(t)
		c.githubAppConfigService.EXPECT().
			CreateAppConfig(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, apiValidationErrorForPrivateKey())

		req := makeGitHubAppRequest(http.MethodPost,
			"/api/v1/team-123/"+githubAppPrefixes[0], validGitHubAppCreateRequest())
		w := httptest.NewRecorder()
		createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var body map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "VALIDATION_FAILED", body["code"])
	})

	t.Run("malformed JSON body", func(t *testing.T) {
		c := newMockGitHubAppConfigContainer(t)

		req := makeGitHubAppRequest(http.MethodPost,
			"/api/v1/team-123/"+githubAppPrefixes[0], nil)
		req.Body = http.NoBody
		w := httptest.NewRecorder()
		createTestGitHubAppConfigServer(c).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

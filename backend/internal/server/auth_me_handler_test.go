package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// getMeMockContainer exposes only the mocked AuthService so handleGetMe can be
// exercised directly (middleware/routing bypassed).
type getMeMockContainer struct {
	BaseMockContainer
	authService services.AuthServiceInterface
}

func (c *getMeMockContainer) AuthService() services.AuthServiceInterface { return c.authService }

// TestHandleGetMe_IsInstanceAdmin verifies GET /auth/me reports is_instance_admin
// from config.IsInstanceAdmin (case-insensitively) and that the response conforms
// to the CurrentUser schema.
func TestHandleGetMe_IsInstanceAdmin(t *testing.T) {
	tests := []struct {
		name           string
		instanceAdmins config.EnvStringSlice
		email          string
		wantAdmin      bool
	}{
		{"admin email matches case-insensitively", config.EnvStringSlice{"admin@example.com"}, "Admin@Example.com", true},
		{"non-admin email is false", config.EnvStringSlice{"admin@example.com"}, "user@example.com", false},
		{"empty list is dormant", nil, "admin@example.com", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			user := &models.User{
				ID:                  "user-123",
				Email:               tc.email,
				Name:                "Test User",
				SubscriptionStatus:  "active",
				OnboardingCompleted: true,
				CreatedAt:           time.Now(),
				UpdatedAt:           time.Now(),
				Version:             1,
			}

			mockAuth := servicesmocks.NewMockAuthServiceInterface(t)
			mockAuth.On("GetUserByID", mock.Anything, "user-123").Return(user, nil)

			cfg := &config.Config{Auth: config.AuthConfig{InstanceAdmins: tc.instanceAdmins}}
			srv := New("8080", nil, "test-api-key", cfg, slog.New(slog.DiscardHandler))
			srv.container = &getMeMockContainer{authService: mockAuth}

			req := createAuthenticatedRequest("GET", "/api/v1/auth/me", "", "user-123")
			rr := httptest.NewRecorder()

			srv.handleGetMe(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)

			var resp models.CurrentUserResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			assert.Equal(t, tc.wantAdmin, resp.IsInstanceAdmin)
			require.NotNil(t, resp.User)
			assert.Equal(t, tc.email, resp.Email)

			specconformance.AssertConformsToSpec(t, req, rr)
			mockAuth.AssertExpectations(t)
		})
	}
}

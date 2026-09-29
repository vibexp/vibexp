package server

import (
	"context"
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
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// getMeMockContainer exposes the mocked AuthService and an instance admin
// resolver so handleGetMe can be exercised directly (middleware/routing
// bypassed).
type getMeMockContainer struct {
	BaseMockContainer
	authService services.AuthServiceInterface
	resolver    services.InstanceAdminResolver
}

func (c *getMeMockContainer) AuthService() services.AuthServiceInterface { return c.authService }

func (c *getMeMockContainer) InstanceAdminResolver() services.InstanceAdminResolver {
	return c.resolver
}

// failingInstanceAdminGrants fails every grant lookup.
type failingInstanceAdminGrants struct {
	repositories.InstanceAdminRepository
}

func (failingInstanceAdminGrants) IsGranted(context.Context, string) (bool, error) {
	return false, assert.AnError
}

// TestHandleGetMe_IsInstanceAdmin verifies GET /auth/me reports
// is_instance_admin (root OR DB-granted) and is_root_instance_admin from the
// instance admin resolver, and that the response conforms to the CurrentUser
// schema. A grant lookup failure degrades the flag to false; it never fails /me.
func TestHandleGetMe_IsInstanceAdmin(t *testing.T) {
	const userID = "user-123"
	tests := []struct {
		name           string
		instanceAdmins config.EnvStringSlice
		email          string
		grants         repositories.InstanceAdminRepository
		wantAdmin      bool
		wantRoot       bool
	}{
		{"root admin matches case-insensitively", config.EnvStringSlice{"admin@example.com"}, "Admin@Example.com",
			noInstanceAdminGrants{}, true, true},
		{"DB-granted admin is admin but not root", config.EnvStringSlice{"admin@example.com"}, "user@example.com",
			&memInstanceAdminGrants{granted: map[string]bool{userID: true}}, true, false},
		{"non-admin is neither", config.EnvStringSlice{"admin@example.com"}, "user@example.com",
			noInstanceAdminGrants{}, false, false},
		{"empty list has no root admin", nil, "admin@example.com", noInstanceAdminGrants{}, false, false},
		{"grant lookup failure degrades to false", nil, "user@example.com", failingInstanceAdminGrants{},
			false, false},
		{"root admin survives a grant lookup failure", config.EnvStringSlice{"admin@example.com"},
			"admin@example.com", failingInstanceAdminGrants{}, true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			user := &models.User{
				ID:                  userID,
				Email:               tc.email,
				Name:                "Test User",
				Status:              models.UserStatusActive,
				OnboardingCompleted: true,
				CreatedAt:           time.Now(),
				UpdatedAt:           time.Now(),
				Version:             1,
			}

			mockAuth := servicesmocks.NewMockAuthServiceInterface(t)
			mockAuth.On("GetUserByID", mock.Anything, userID).Return(user, nil)

			cfg := &config.Config{Auth: config.AuthConfig{InstanceAdmins: tc.instanceAdmins}}
			srv := New("8080", nil, "test-api-key", cfg, slog.New(slog.DiscardHandler))
			srv.container = &getMeMockContainer{
				authService: mockAuth,
				resolver: services.NewInstanceAdminService(tc.instanceAdmins, tc.grants,
					alwaysActiveUserRepository{}, slog.New(slog.DiscardHandler)),
			}

			req := createAuthenticatedRequest("GET", "/api/v1/auth/me", "", userID)
			rr := httptest.NewRecorder()

			srv.handleGetMe(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)

			var resp models.CurrentUserResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			assert.Equal(t, tc.wantAdmin, resp.IsInstanceAdmin)
			assert.Equal(t, tc.wantRoot, resp.IsRootInstanceAdmin)
			assert.Contains(t, rr.Body.String(), `"is_root_instance_admin":`)
			require.NotNil(t, resp.User)
			assert.Equal(t, tc.email, resp.Email)

			specconformance.AssertConformsToSpec(t, req, rr)
			mockAuth.AssertExpectations(t)
		})
	}
}

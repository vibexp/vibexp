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
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
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
	accessRepo  repositories.ResourceAccessRepository
}

// ResourceAccessRepository returns the injected repository, or nil like the
// base container when a test sets none.
func (c *getMeMockContainer) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return c.accessRepo
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
		unverified     bool
	}{
		{"root admin matches case-insensitively", config.EnvStringSlice{"admin@example.com"}, "Admin@Example.com",
			noInstanceAdminGrants{}, true, true, false},
		{"DB-granted admin is admin but not root", config.EnvStringSlice{"admin@example.com"}, "user@example.com",
			&memInstanceAdminGrants{granted: map[string]bool{userID: true}}, true, false, false},
		{"non-admin is neither", config.EnvStringSlice{"admin@example.com"}, "user@example.com",
			noInstanceAdminGrants{}, false, false, false},
		{"empty list has no root admin", nil, "admin@example.com", noInstanceAdminGrants{}, false, false, false},
		{"grant lookup failure degrades to false", nil, "user@example.com", failingInstanceAdminGrants{},
			false, false, false},
		{"root admin survives a grant lookup failure", config.EnvStringSlice{"admin@example.com"},
			"admin@example.com", failingInstanceAdminGrants{}, true, true, false},
		// #1467: the address alone grants no power; a DB grant still applies.
		{"unverified root address is neither", config.EnvStringSlice{"admin@example.com"},
			"admin@example.com", noInstanceAdminGrants{}, false, false, true},
		{"unverified root address with a DB grant is admin but not root", config.EnvStringSlice{"admin@example.com"},
			"admin@example.com", &memInstanceAdminGrants{granted: map[string]bool{userID: true}}, true, false, true},
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
			if !tc.unverified {
				verifiedAt := time.Now()
				user.EmailVerifiedAt = &verifiedAt
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

// TestHandleGetMe_KnowledgeLoop verifies GET /auth/me reports the user's first
// AI-tool read and write (#1508) as knowledge_loop, conforming to the
// CurrentUser schema: timestamps when they exist, nulls when they do not, and
// nulls (never a failed /me) when the lookup errors.
func TestHandleGetMe_KnowledgeLoop(t *testing.T) {
	const userID = "user-123"
	firstRead := time.Date(2026, 10, 15, 18, 4, 50, 0, time.UTC)
	firstWrite := time.Date(2026, 10, 15, 18, 12, 3, 0, time.UTC)

	tests := []struct {
		name      string
		loop      models.KnowledgeLoop
		err       error
		wantRead  *time.Time
		wantWrite *time.Time
	}{
		{name: "not started", loop: models.KnowledgeLoop{}},
		{name: "read only", loop: models.KnowledgeLoop{FirstReadAt: &firstRead}, wantRead: &firstRead},
		{
			name:      "running",
			loop:      models.KnowledgeLoop{FirstReadAt: &firstRead, FirstWriteAt: &firstWrite},
			wantRead:  &firstRead,
			wantWrite: &firstWrite,
		},
		{name: "lookup failure reports not started", err: assert.AnError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			user := &models.User{
				ID:        userID,
				Email:     "user@example.com",
				Name:      "Test User",
				Status:    models.UserStatusActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Version:   1,
			}
			mockAuth := servicesmocks.NewMockAuthServiceInterface(t)
			mockAuth.On("GetUserByID", mock.Anything, userID).Return(user, nil)
			accessRepo := repomocks.NewMockResourceAccessRepository(t)
			accessRepo.On("GetUserKnowledgeLoop", mock.Anything, userID).Return(tc.loop, tc.err)

			srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
			srv.container = &getMeMockContainer{
				authService: mockAuth,
				resolver: services.NewInstanceAdminService(nil, noInstanceAdminGrants{},
					alwaysActiveUserRepository{}, slog.New(slog.DiscardHandler)),
				accessRepo: accessRepo,
			}

			req := createAuthenticatedRequest("GET", "/api/v1/auth/me", "", userID)
			rr := httptest.NewRecorder()
			srv.handleGetMe(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			var resp models.CurrentUserResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			assertSameTime(t, tc.wantRead, resp.KnowledgeLoop.FirstReadAt)
			assertSameTime(t, tc.wantWrite, resp.KnowledgeLoop.FirstWriteAt)
			assert.Contains(t, rr.Body.String(), `"knowledge_loop":{"first_read_at":`)

			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func assertSameTime(t *testing.T, want, got *time.Time) {
	t.Helper()
	if want == nil {
		assert.Nil(t, got)
		return
	}
	require.NotNil(t, got)
	assert.True(t, want.Equal(*got), "want %v, got %v", want, got)
}

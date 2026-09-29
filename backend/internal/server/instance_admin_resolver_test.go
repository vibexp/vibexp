package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// memInstanceAdminGrants is an in-memory InstanceAdminRepository, so the real
// resolver can be driven end to end without a database.
type memInstanceAdminGrants struct {
	repositories.InstanceAdminRepository
	granted map[string]bool
}

func (m *memInstanceAdminGrants) IsGranted(_ context.Context, userID string) (bool, error) {
	return m.granted[userID], nil
}

func (m *memInstanceAdminGrants) Grant(_ context.Context, userID string, _ *string) (bool, error) {
	if m.granted[userID] {
		return false, nil
	}
	m.granted[userID] = true
	return true, nil
}

func (m *memInstanceAdminGrants) Revoke(_ context.Context, userID string, _ *string) error {
	if !m.granted[userID] {
		return repositories.ErrInstanceAdminNotFound
	}
	delete(m.granted, userID)
	return nil
}

// memUsers is a UserRepository answering GetByID from a map.
type memUsers struct {
	repositories.UserRepository
	users map[string]*models.User
}

func (m memUsers) GetByID(_ context.Context, userID string) (*models.User, error) {
	if u, ok := m.users[userID]; ok {
		return u, nil
	}
	return nil, repositories.ErrUserNotFound
}

// serveAdminGate runs one request as userID through instanceAdminMiddleware
// and reports the status.
func serveAdminGate(t *testing.T, srv *Server, userID string) int {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, userID))
	rr := httptest.NewRecorder()
	srv.instanceAdminMiddleware(next).ServeHTTP(rr, req)
	return rr.Code
}

// TestInstanceAdminMiddleware_DBGrantThenRevoke is the #1233 acceptance
// criterion: a DB-granted admin reaches the admin surface, and after a root
// admin revokes the grant the NEXT request gets 404 (no cache in between). A
// suspended DB admin is denied while still granted.
func TestInstanceAdminMiddleware_DBGrantThenRevoke(t *testing.T) {
	root := &models.User{ID: uuid.NewString(), Email: "root@example.com", Status: models.UserStatusActive}
	delegate := &models.User{ID: uuid.NewString(), Email: "delegate@example.com", Status: models.UserStatusActive}
	suspended := &models.User{ID: uuid.NewString(), Email: "gone@example.com", Status: models.UserStatusSuspended}
	users := memUsers{users: map[string]*models.User{root.ID: root, delegate.ID: delegate, suspended.ID: suspended}}

	grants := &memInstanceAdminGrants{granted: map[string]bool{}}
	resolver := services.NewInstanceAdminService([]string{"root@example.com"}, grants, users,
		slog.New(slog.DiscardHandler))

	mockAuth := servicesmocks.NewMockAuthServiceInterface(t)
	for _, u := range []*models.User{root, delegate, suspended} {
		mockAuth.On("GetUserByID", mock.Anything, u.ID).Return(u, nil)
	}
	cfg := &config.Config{Auth: config.AuthConfig{InstanceAdmins: config.EnvStringSlice{"root@example.com"}}}
	srv := newAdminTestServer(cfg, &adminMockContainer{authService: mockAuth, instanceAdminResolver: resolver})

	ctx := context.Background()
	assert.Equal(t, http.StatusOK, serveAdminGate(t, srv, root.ID), "root admin")
	assert.Equal(t, http.StatusNotFound, serveAdminGate(t, srv, delegate.ID), "not yet granted")

	granted, err := resolver.GrantInstanceAdmin(ctx, root.ID, delegate.ID)
	require.NoError(t, err)
	require.True(t, granted)
	assert.Equal(t, http.StatusOK, serveAdminGate(t, srv, delegate.ID), "DB-granted admin")

	grants.granted[suspended.ID] = true
	assert.Equal(t, http.StatusNotFound, serveAdminGate(t, srv, suspended.ID), "suspended DB admin")

	require.NoError(t, resolver.RevokeInstanceAdmin(ctx, root.ID, delegate.ID))
	assert.Equal(t, http.StatusNotFound, serveAdminGate(t, srv, delegate.ID), "revoked on the next request")
	assert.Equal(t, http.StatusOK, serveAdminGate(t, srv, root.ID), "root admin unaffected")
}

// TestInstanceAdminMiddleware_ResolverOutcome pins the middleware against the
// resolver's answer: allowed passes through, denied and a resolver error both
// 404 (fail closed).
func TestInstanceAdminMiddleware_ResolverOutcome(t *testing.T) {
	user := &models.User{ID: "user-1", Email: "delegate@example.com"}

	tests := []struct {
		name       string
		isAdmin    bool
		err        error
		wantStatus int
	}{
		{"allowed", true, nil, http.StatusOK},
		{"denied", false, nil, http.StatusNotFound},
		{"resolver error fails closed", false, assert.AnError, http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockAuth := servicesmocks.NewMockAuthServiceInterface(t)
			mockAuth.On("GetUserByID", mock.Anything, "user-1").Return(user, nil)
			resolver := servicesmocks.NewMockInstanceAdminResolver(t)
			resolver.On("IsInstanceAdmin", mock.Anything, user).Return(tc.isAdmin, tc.err)

			srv := newAdminTestServer(&config.Config{},
				&adminMockContainer{authService: mockAuth, instanceAdminResolver: resolver})

			assert.Equal(t, tc.wantStatus, serveAdminGate(t, srv, "user-1"))
		})
	}
}

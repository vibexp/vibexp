package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

func adminUserDetailNamed(id, name string) *models.AdminUserDetail {
	return &models.AdminUserDetail{
		ID:          id,
		Email:       "target@example.com",
		Name:        name,
		Status:      models.UserStatusActive,
		CreatedAt:   time.Now(),
		Memberships: []models.AdminTeamMembership{},
	}
}

func patchUserRequest(t *testing.T, id string, body any) *http.Request {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest("PATCH", "/api/v1/admin/users/"+id, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestUpdateAdminUser_Success renames a user and asserts the RESULT, not just a
// 200 — a converter that dropped the field would still return 200.
func TestUpdateAdminUser_Success(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("UpdateUserName", mock.Anything, id, "Renamed").
		Return(adminUserDetailNamed(id, "Renamed"), nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := patchUserRequest(t, id, map[string]string{"name": "Renamed"})
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminUserDetail
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "Renamed", resp.Name)

	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestUpdateAdminUser_UnknownIDReturns404(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("UpdateUserName", mock.Anything, id, "Renamed").Return(nil, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := patchUserRequest(t, id, map[string]string{"name": "Renamed"})
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestUpdateAdminUser_ServiceErrorReturns500(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("UpdateUserName", mock.Anything, id, "Renamed").Return(nil, errors.New("db down"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := patchUserRequest(t, id, map[string]string{"name": "Renamed"})
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestDeleteAdminUser_Success is the allowed path: 204 and no body.
func TestDeleteAdminUser_Success(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, id).Return(adminUserDetailNamed(id, "Target"), nil)
	mockAdmin.On("DeleteUser", mock.Anything, mock.Anything, id, mock.Anything).Return(true, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/users/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
	assert.Empty(t, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestDeleteAdminUser_BlockedReturnsStructuredBlockers is the acceptance
// criterion that matters most. A refused delete must return the documented
// blocker payload — team id, name and member count — because the SPA renders it
// so the admin knows exactly which teams to transfer. A bare problem document
// would leave them guessing.
func TestDeleteAdminUser_BlockedReturnsStructuredBlockers(t *testing.T) {
	id := uuid.NewString()
	teamA, teamB := uuid.NewString(), uuid.NewString()

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, id).Return(adminUserDetailNamed(id, "Target"), nil)
	mockAdmin.On("DeleteUser", mock.Anything, mock.Anything, id, mock.Anything).
		Return(false, &services.ErrAdminDeleteBlocked{Blockers: []models.AdminDeleteBlocker{
			{TeamID: teamA, TeamName: "Acme Engineering", MemberCount: 4},
			{TeamID: teamB, TeamName: "Beta Squad", MemberCount: 2},
		}})
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/users/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)

	var resp admingen.AdminUserDeleteBlockedResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Blockers, 2)
	assert.Equal(t, teamA, resp.Blockers[0].TeamId.String())
	assert.Equal(t, "Acme Engineering", resp.Blockers[0].TeamName)
	assert.Equal(t, int64(4), resp.Blockers[0].MemberCount)
	assert.Equal(t, "Beta Squad", resp.Blockers[1].TeamName)
	assert.NotEmpty(t, resp.Message)

	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestDeleteAdminUser_BlockedEmptyBlockersSerializeAsArray covers #125 for the
// new required array.
func TestDeleteAdminUser_BlockedEmptyBlockersSerializeAsArray(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, id).Return(adminUserDetailNamed(id, "Target"), nil)
	mockAdmin.On("DeleteUser", mock.Anything, mock.Anything, id, mock.Anything).
		Return(false, &services.ErrAdminDeleteBlocked{})
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/users/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), `"blockers":[]`)
	assert.NotContains(t, rr.Body.String(), `"blockers":null`)
}

// TestDeleteAdminUser_LockoutGuardsReturn409 covers the self and config-admin
// refusals, which are problem documents rather than blocker payloads.
func TestDeleteAdminUser_LockoutGuardsReturn409(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"self-delete", &services.ErrAdminDeleteSelf{}, "cannot delete their own account"},
		{
			"config-listed instance admin",
			&services.ErrAdminDeleteInstanceAdmin{Email: "root@example.com"},
			"configured instance admin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id := uuid.NewString()
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			mockAdmin.On("GetUserDetail", mock.Anything, id).Return(adminUserDetailNamed(id, "Target"), nil)
			mockAdmin.On("DeleteUser", mock.Anything, mock.Anything, id, mock.Anything).
				Return(false, tc.err)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("DELETE", "/api/v1/admin/users/"+id, nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusConflict, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.wantMsg)
		})
	}
}

func TestDeleteAdminUser_UnknownIDReturns404(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, id).Return(nil, nil)
	mockAdmin.On("DeleteUser", mock.Anything, mock.Anything, id, mock.Anything).Return(false, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/users/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestDeleteAdminUser_ServiceErrorReturns500(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, id).Return(adminUserDetailNamed(id, "Target"), nil)
	mockAdmin.On("DeleteUser", mock.Anything, mock.Anything, id, mock.Anything).
		Return(false, errors.New("db down"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/users/"+id, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestAdminUserMutations_RecordActivity asserts both mutations write an audit
// row attributed to the ACTING ADMIN. That attribution is load-bearing for the
// delete: activities.user_id is ON DELETE CASCADE, so a row owned by the target
// would be erased by the very delete it records.
func TestAdminUserMutations_RecordActivity(t *testing.T) {
	tests := []struct {
		name     string
		wantType string
		run      func(*testing.T, *servicesmocks.MockAdminServiceInterface, string) *http.Request
	}{
		{
			name:     "update",
			wantType: activities.ActivityTypeAdminUserUpdated,
			run: func(t *testing.T, m *servicesmocks.MockAdminServiceInterface, id string) *http.Request {
				m.On("UpdateUserName", mock.Anything, id, "Renamed").
					Return(adminUserDetailNamed(id, "Renamed"), nil)
				return patchUserRequest(t, id, map[string]string{"name": "Renamed"})
			},
		},
		{
			name:     "delete",
			wantType: activities.ActivityTypeAdminUserDeleted,
			run: func(t *testing.T, m *servicesmocks.MockAdminServiceInterface, id string) *http.Request {
				m.On("GetUserDetail", mock.Anything, id).Return(adminUserDetailNamed(id, "Target"), nil)
				m.On("DeleteUser", mock.Anything, mock.Anything, id, mock.Anything).Return(true, nil)
				return httptest.NewRequest("DELETE", "/api/v1/admin/users/"+id, nil)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			targetID := uuid.NewString()
			actingID := uuid.NewString()

			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			req := tc.run(t, mockAdmin, targetID)

			activitySvc := &MockActivityService{}
			activitySvc.On("RecordResourceActivity",
				mock.Anything,
				actingID,
				tc.wantType,
				activities.EntityTypeUser,
				mock.MatchedBy(func(entityID *string) bool {
					return entityID != nil && *entityID == targetID
				}),
				mock.Anything,
				mock.MatchedBy(func(md map[string]interface{}) bool {
					// The email must be captured BEFORE the delete, or the audit
					// row names nobody.
					return md["target_user_id"] == targetID && md["target_email"] == "target@example.com"
				}),
			).Return(nil).Once()

			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				adminService:    mockAdmin,
				activityService: activitySvc,
			})

			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, actingID))
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Contains(t, []int{http.StatusOK, http.StatusNoContent}, rr.Code)
			activitySvc.AssertExpectations(t)
		})
	}
}

// TestDeleteAdminUser_PassesActingAdminAndPredicate proves the handler supplies
// the two things the guards need. A stubbed-out predicate would silently disable
// the config-admin protection on the epic's only destructive endpoint.
func TestDeleteAdminUser_PassesActingAdminAndPredicate(t *testing.T) {
	targetID := uuid.NewString()
	actingID := uuid.NewString()

	cfg := &config.Config{}
	cfg.Auth.InstanceAdmins = []string{"root@example.com"}

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("GetUserDetail", mock.Anything, targetID).
		Return(adminUserDetailNamed(targetID, "Target"), nil)
	mockAdmin.On("DeleteUser",
		mock.Anything,
		actingID,
		targetID,
		mock.MatchedBy(func(p services.InstanceAdminPredicate) bool {
			return p != nil && p("root@example.com") && !p("someone@example.com")
		}),
	).Return(true, nil)

	srv := newAdminTestServer(cfg, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/users/"+targetID, nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, actingID))
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
}

// TestAdminDeleteBlockedError_Message: the wrapper carries the service's detail
// through the strict server's error path unchanged, so the 409 body's `message`
// is the same explanation the service produced.
func TestAdminDeleteBlockedError_Message(t *testing.T) {
	err := &adminDeleteBlockedError{detail: "owns shared teams"}
	assert.Equal(t, "owns shared teams", err.Error())
}

// TestToGenDeleteBlockedResponse_SkipsUnparseableTeamID covers the defensive
// branch: a non-UUID team id cannot be rendered, but dropping the whole response
// would turn a SAFE refusal into a 500 that looks like the delete might have
// happened. The refusal must stand.
func TestToGenDeleteBlockedResponse_SkipsUnparseableTeamID(t *testing.T) {
	good := uuid.NewString()
	got := toGenDeleteBlockedResponse(&adminDeleteBlockedError{
		detail: "blocked",
		blockers: []models.AdminDeleteBlocker{
			{TeamID: "not-a-uuid", TeamName: "Broken", MemberCount: 2},
			{TeamID: good, TeamName: "Acme", MemberCount: 3},
		},
	})

	require.Len(t, got.Blockers, 1, "the unparseable row is skipped, not fatal")
	assert.Equal(t, good, got.Blockers[0].TeamId.String())
	assert.Equal(t, "blocked", got.Message)
}

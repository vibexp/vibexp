package server

import (
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

// adminUserDetailWithStatus is the fixture both operations return.
func adminUserDetailWithStatus(id, status string) *models.AdminUserDetail {
	return &models.AdminUserDetail{
		ID:          id,
		Email:       "target@example.com",
		Name:        "Target",
		Status:      status,
		CreatedAt:   time.Now(),
		Memberships: []models.AdminTeamMembership{},
	}
}

// TestSuspendAdminUser_Success asserts the happy path returns the UPDATED
// detail (status suspended) and conforms to the spec.
func TestSuspendAdminUser_Success(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("SuspendUser", mock.Anything, mock.Anything, id, mock.Anything).
		Return(adminUserDetailWithStatus(id, models.UserStatusSuspended), nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("POST", "/api/v1/admin/users/"+id+"/suspend", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminUserDetail
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, id, resp.Id.String())
	// The whole point of the operation — assert the resulting state, not just 200.
	assert.Equal(t, admingen.AdminUserDetailStatus("suspended"), resp.Status)

	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestReactivateAdminUser_Success mirrors the above for reactivation.
func TestReactivateAdminUser_Success(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("ReactivateUser", mock.Anything, id).
		Return(adminUserDetailWithStatus(id, models.UserStatusActive), nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("POST", "/api/v1/admin/users/"+id+"/reactivate", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminUserDetail
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, admingen.AdminUserDetailStatus("active"), resp.Status)

	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestSuspendAdminUser_LockoutGuardsReturn409 pins the two guards that stop an
// instance suspending its way out of its own admin surface. Both must be 409,
// and both must be distinguishable from a 404.
func TestSuspendAdminUser_LockoutGuardsReturn409(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "acting admin suspends themselves",
			err:     &services.ErrAdminSuspendSelf{},
			wantMsg: "cannot suspend their own account",
		},
		{
			name:    "target is a config-listed instance admin",
			err:     &services.ErrAdminSuspendInstanceAdmin{Email: "root@example.com"},
			wantMsg: "configured instance admin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id := uuid.NewString()
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			mockAdmin.On("SuspendUser", mock.Anything, mock.Anything, id, mock.Anything).
				Return(nil, tc.err)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("POST", "/api/v1/admin/users/"+id+"/suspend", nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusConflict, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.wantMsg)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestAdminSuspension_UnknownIDReturns404 covers both operations: a nil detail
// from the service is an unknown id, not a 500.
func TestAdminSuspension_UnknownIDReturns404(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		expect func(*servicesmocks.MockAdminServiceInterface, string)
	}{
		{
			name: "suspend",
			path: "suspend",
			expect: func(m *servicesmocks.MockAdminServiceInterface, id string) {
				m.On("SuspendUser", mock.Anything, mock.Anything, id, mock.Anything).Return(nil, nil)
			},
		},
		{
			name: "reactivate",
			path: "reactivate",
			expect: func(m *servicesmocks.MockAdminServiceInterface, id string) {
				m.On("ReactivateUser", mock.Anything, id).Return(nil, nil)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id := uuid.NewString()
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			tc.expect(mockAdmin, id)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := httptest.NewRequest("POST", "/api/v1/admin/users/"+id+"/"+tc.path, nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusNotFound, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestAdminSuspension_ServiceErrorReturns500 keeps a genuine failure distinct
// from the 409 guards.
func TestAdminSuspension_ServiceErrorReturns500(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("SuspendUser", mock.Anything, mock.Anything, id, mock.Anything).
		Return(nil, errors.New("db down"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("POST", "/api/v1/admin/users/"+id+"/suspend", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestAdminSuspension_InvalidUUIDReturns400 exercises the generated binder.
func TestAdminSuspension_InvalidUUIDReturns400(t *testing.T) {
	for _, action := range []string{"suspend", "reactivate"} {
		t.Run(action, func(t *testing.T) {
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				adminService: servicesmocks.NewMockAdminServiceInterface(t),
			})

			req := httptest.NewRequest("POST", "/api/v1/admin/users/not-a-uuid/"+action, nil)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestSuspendAdminUser_PassesActingAdminAndPredicate proves the handler hands
// the service the two things its guards need: the CALLER's id (for the
// self-suspension check) and the config instance-admin predicate. Getting either
// wrong silently disables a lockout guard.
func TestSuspendAdminUser_PassesActingAdminAndPredicate(t *testing.T) {
	targetID := uuid.NewString()
	actingID := uuid.NewString()

	cfg := &config.Config{}
	cfg.Auth.InstanceAdmins = []string{"root@example.com"}

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("SuspendUser",
		mock.Anything,
		actingID,
		targetID,
		mock.MatchedBy(func(p services.InstanceAdminPredicate) bool {
			// The predicate must be the real config allowlist, not a stub that
			// always says false (which would disable the config-admin guard).
			return p != nil && p("root@example.com") && !p("someone@example.com")
		}),
	).Return(adminUserDetailWithStatus(targetID, models.UserStatusSuspended), nil)

	srv := newAdminTestServer(cfg, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("POST", "/api/v1/admin/users/"+targetID+"/suspend", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, actingID))
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

// TestAdminSuspension_RecordsActivity is the audit acceptance criterion: both
// transitions must write an activities row whose user_id is the ACTING ADMIN and
// whose entity_id is the AFFECTED account, so "who did this to whom" is
// answerable afterwards. Getting those two round the wrong way would still pass
// a mere "an activity was recorded" assertion, so both are asserted explicitly.
func TestAdminSuspension_RecordsActivity(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		wantType   string
		wantStatus string
		expectSvc  func(*servicesmocks.MockAdminServiceInterface, string)
	}{
		{
			name:       "suspend",
			action:     "suspend",
			wantType:   activities.ActivityTypeAdminUserSuspended,
			wantStatus: models.UserStatusSuspended,
			expectSvc: func(m *servicesmocks.MockAdminServiceInterface, id string) {
				m.On("SuspendUser", mock.Anything, mock.Anything, id, mock.Anything).
					Return(adminUserDetailWithStatus(id, models.UserStatusSuspended), nil)
			},
		},
		{
			name:       "reactivate",
			action:     "reactivate",
			wantType:   activities.ActivityTypeAdminUserReactivated,
			wantStatus: models.UserStatusActive,
			expectSvc: func(m *servicesmocks.MockAdminServiceInterface, id string) {
				m.On("ReactivateUser", mock.Anything, id).
					Return(adminUserDetailWithStatus(id, models.UserStatusActive), nil)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			targetID := uuid.NewString()
			actingID := uuid.NewString()

			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			tc.expectSvc(mockAdmin, targetID)

			activitySvc := &MockActivityService{}
			activitySvc.On("RecordResourceActivity",
				mock.Anything,
				actingID, // the ACTING ADMIN is the activity's user_id
				tc.wantType,
				activities.EntityTypeUser,
				mock.MatchedBy(func(entityID *string) bool {
					// the AFFECTED account is the entity_id
					return entityID != nil && *entityID == targetID
				}),
				mock.Anything,
				mock.MatchedBy(func(md map[string]interface{}) bool {
					return md["target_user_id"] == targetID &&
						md["target_email"] == "target@example.com" &&
						md["new_status"] == tc.wantStatus
				}),
			).Return(nil).Once()

			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				adminService:    mockAdmin,
				activityService: activitySvc,
			})

			req := httptest.NewRequest("POST", "/api/v1/admin/users/"+targetID+"/"+tc.action, nil)
			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, actingID))
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			activitySvc.AssertExpectations(t)
		})
	}
}

// TestAdminSuspension_ActivityFailureDoesNotFailTheTransition: the status change
// is already committed by the time the audit row is written, so a recording
// failure must be logged and dropped rather than turning a successful
// suspension into a 500 the caller would retry.
func TestAdminSuspension_ActivityFailureDoesNotFailTheTransition(t *testing.T) {
	targetID := uuid.NewString()

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("SuspendUser", mock.Anything, mock.Anything, targetID, mock.Anything).
		Return(adminUserDetailWithStatus(targetID, models.UserStatusSuspended), nil)

	activitySvc := &MockActivityService{}
	activitySvc.On("RecordResourceActivity",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything,
	).Return(errors.New("activities table is full"))

	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		adminService:    mockAdmin,
		activityService: activitySvc,
	})

	req := httptest.NewRequest("POST", "/api/v1/admin/users/"+targetID+"/suspend", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "a failed audit write must not undo a committed suspension")
}

// TestAdminSuspension_ConversionErrorReturns500 covers the defensive path where
// the store hands back a non-UUID id: that is a server bug, not a client error,
// so it must be a logged 500 rather than a malformed 200.
func TestAdminSuspension_ConversionErrorReturns500(t *testing.T) {
	id := uuid.NewString()
	bad := adminUserDetailWithStatus("not-a-uuid", models.UserStatusSuspended)

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("SuspendUser", mock.Anything, mock.Anything, id, mock.Anything).Return(bad, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := httptest.NewRequest("POST", "/api/v1/admin/users/"+id+"/suspend", nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

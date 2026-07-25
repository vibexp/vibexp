package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
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

func postUserRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/admin/users", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func createdUserDetail(id string) *models.AdminUserDetail {
	return &models.AdminUserDetail{
		ID:          id,
		Email:       "new.user@example.com",
		Name:        "New User",
		Status:      models.UserStatusActive,
		Memberships: []models.AdminTeamMembership{},
	}
}

// TestCreateAdminUser_Success asserts 201, the created payload, and spec
// conformance.
func TestCreateAdminUser_Success(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("CreateUser", mock.Anything, mock.MatchedBy(func(r services.AdminUserCreateRequest) bool {
		return r.Email == "new.user@example.com" && r.Name == "New User"
	})).Return(createdUserDetail(id), nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := postUserRequest(t, `{"email":"New.User@Example.com","name":"New User"}`)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var resp admingen.AdminUserDetail
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, id, resp.Id.String())
	assert.Equal(t, "New User", resp.Name)
	assert.Equal(t, admingen.AdminUserDetailStatus("active"), resp.Status)

	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestCreateAdminUser_PassesIDPProviderThrough keeps the optional label from
// being silently dropped.
func TestCreateAdminUser_PassesIDPProviderThrough(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("CreateUser", mock.Anything, mock.MatchedBy(func(r services.AdminUserCreateRequest) bool {
		return r.IDPProvider != nil && *r.IDPProvider == "oidc"
	})).Return(createdUserDetail(id), nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := postUserRequest(t, `{"email":"new.user@example.com","name":"New User","idp_provider":"oidc"}`)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
}

// TestCreateAdminUser_InvalidBodyReturns400 covers both validation layers.
//
// `format: email` IS enforced by the generated code — oapi-codegen types the
// field as openapi_types.Email, whose UnmarshalJSON applies a regex — so those
// cases are rejected by the binder with its own message. `minLength` on `name` is
// NOT enforced, so a blank or whitespace-only name is caught by the handler's own
// check; without it an account would be created with no display name.
//
// Either way the service mock has no expectations, so anything reaching it fails
// the test rather than passing quietly.
func TestCreateAdminUser_InvalidBodyReturns400(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string // "" = only the status matters (rejected by the generated binder)
	}{
		{"malformed email (binder)", `{"email":"not-an-email","name":"New User"}`, ""},
		{"empty email (binder)", `{"email":"","name":"New User"}`, ""},
		{"whitespace-only email (binder)", `{"email":"   ","name":"New User"}`, ""},
		{"empty name (handler)", `{"email":"new.user@example.com","name":""}`, "name is required"},
		{"whitespace-only name (handler)", `{"email":"new.user@example.com","name":"   "}`, "name is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := postUserRequest(t, tc.body)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			if tc.want != "" {
				assert.Contains(t, rr.Body.String(), tc.want)
			}
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// TestCreateAdminUser_DuplicateEmailReturns409.
func TestCreateAdminUser_DuplicateEmailReturns409(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("CreateUser", mock.Anything, mock.Anything).
		Return(nil, &services.ErrAdminUserEmailTaken{Email: "taken@example.com"})
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := postUserRequest(t, `{"email":"taken@example.com","name":"Someone"}`)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), "taken@example.com")
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestCreateAdminUser_ServiceErrorReturns500 — including the provisioning
// failure, which is a 500 precisely so no unprovisioned account is reported as
// created.
func TestCreateAdminUser_ServiceErrorReturns500(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("CreateUser", mock.Anything, mock.Anything).
		Return(nil, errors.New("failed to provision the new user: event channel full"))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := postUserRequest(t, `{"email":"new.user@example.com","name":"New User"}`)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestCreateAdminUser_UnreadableAfterCreateIs500NotFound: a nil detail here means
// the row could not be read back after being written. Reporting 404 for something
// just created would be actively misleading.
func TestCreateAdminUser_UnreadableAfterCreateIs500NotFound(t *testing.T) {
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("CreateUser", mock.Anything, mock.Anything).Return(nil, nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := postUserRequest(t, `{"email":"new.user@example.com","name":"New User"}`)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.NotEqual(t, http.StatusNotFound, rr.Code)
}

// TestCreateAdminUser_RecordsActivity: the acting admin is the actor and the new
// account is the entity, so "who created whom" survives.
func TestCreateAdminUser_RecordsActivity(t *testing.T) {
	id := uuid.NewString()
	actingID := uuid.NewString()

	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("CreateUser", mock.Anything, mock.Anything).Return(createdUserDetail(id), nil)

	activitySvc := &MockActivityService{}
	activitySvc.On("RecordResourceActivity",
		mock.Anything,
		actingID,
		activities.ActivityTypeAdminUserCreated,
		activities.EntityTypeUser,
		mock.MatchedBy(func(entityID *string) bool { return entityID != nil && *entityID == id }),
		mock.Anything,
		mock.MatchedBy(func(md map[string]interface{}) bool {
			return md["target_user_id"] == id && md["target_email"] == "new.user@example.com"
		}),
	).Return(nil).Once()

	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		adminService:    mockAdmin,
		activityService: activitySvc,
	})

	req := postUserRequest(t, `{"email":"new.user@example.com","name":"New User"}`)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, actingID))
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	activitySvc.AssertExpectations(t)
}

// TestCreateAdminUser_RejectsUnknownFields is the enforcement behind
// `additionalProperties: false` on AdminUserCreateRequest.
//
// Without the body-guard entry this returned 201 while silently discarding the
// extra fields — and the two most likely ones are exactly the dangerous ones:
// `status` belongs to the suspend/reactivate endpoints (#454) and
// `is_instance_admin` is config-only by the epic's decision. An admin setting
// either and receiving 201 would reasonably believe it applied.
//
// The service mock has no expectations, so anything reaching it fails the test.
func TestCreateAdminUser_RejectsUnknownFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "status belongs to suspend/reactivate",
			body: `{"email":"a@example.com","name":"A","status":"suspended"}`,
			want: "status",
		},
		{
			name: "instance-admin rights are config-only",
			body: `{"email":"a@example.com","name":"A","is_instance_admin":true}`,
			want: "is_instance_admin",
		},
		{
			name: "identity fields are IdP-owned",
			body: `{"email":"a@example.com","name":"A","idp_subject":"sub-1"}`,
			want: "idp_subject",
		},
		{
			name: "several at once are all named",
			body: `{"email":"a@example.com","name":"A","status":"active","idp_subject":"s"}`,
			want: "idp_subject, status",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

			req := postUserRequest(t, tc.body)
			rr := httptest.NewRecorder()
			mountAdminStrictRouter(srv).ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.want)
		})
	}
}

// TestCreateAdminUser_AcceptsAllDeclaredFields is the control: the guard must not
// reject a body using every property the schema declares.
func TestCreateAdminUser_AcceptsAllDeclaredFields(t *testing.T) {
	id := uuid.NewString()
	mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
	mockAdmin.On("CreateUser", mock.Anything, mock.Anything).Return(createdUserDetail(id), nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: mockAdmin})

	req := postUserRequest(t, `{"email":"a@example.com","name":"A","idp_provider":"oidc"}`)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
}

// TestValidateAdminUserCreate_DefenceInDepth exercises the handler's own checks
// directly. Over HTTP the generated Email type rejects a malformed address before
// these run, so they are unreachable that way — but they are the safety net if the
// spec ever drops `format: email`, and an untested safety net is not one.
func TestValidateAdminUserCreate_DefenceInDepth(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		reqName string
		wantErr string
	}{
		{"trims and lowercases", "  Mixed@Example.COM ", " A ", ""},
		{"empty email", "", "A", "email is required"},
		{"whitespace email", "   ", "A", "email is required"},
		{"malformed email", "not-an-email", "A", "valid address"},
		{"empty name", "a@example.com", "", "name is required"},
		{"whitespace name", "a@example.com", "   ", "name is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			email, name, err := validateAdminUserCreate(admingen.AdminUserCreateRequest{
				Email: openapi_types.Email(tc.email),
				Name:  tc.reqName,
			})
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "mixed@example.com", email)
			assert.Equal(t, "A", name)
		})
	}
}

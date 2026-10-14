package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Setup links through the admin API (#1436).

var setupExpiry = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

// createWithSetup creates a user through the router with the given setup
// outcome and returns the decoded response.
func createWithSetup(t *testing.T, setup services.AdminUserSetup) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	id := uuid.NewString()
	admin := servicesmocks.NewMockAdminServiceInterface(t)
	admin.On("CreateUser", mock.Anything, mock.Anything).Return(createdUserDetail(id), nil)
	account := servicesmocks.NewMockAdminAccountServiceInterface(t)
	account.On("SetUpNewUser", mock.Anything, mock.Anything, id).Return(setup)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: admin, adminAccount: account})

	req := postUserRequest(t, `{"email":"new.user@example.com"}`)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	specconformance.AssertConformsToSpec(t, req, rr)

	var resp map[string]any
	if rr.Code == http.StatusCreated {
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	}
	return rr, resp
}

func TestCreateAdminUser_ReportsTheSetup(t *testing.T) {
	t.Run("emailed: no link in the response", func(t *testing.T) {
		rr, resp := createWithSetup(t, services.AdminUserSetup{Delivery: services.AdminLinkByEmail, ExpiresAt: setupExpiry})

		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		assert.Equal(t, map[string]any{"delivery": "email", "expires_at": "2026-10-21T09:00:00Z"}, resp["setup"])
		assert.NotContains(t, rr.Body.String(), "#token=")
	})

	t.Run("no email: the link is returned once", func(t *testing.T) {
		link := "https://app.example.com/account/setup#token=COPIED"
		rr, resp := createWithSetup(t, services.AdminUserSetup{
			Delivery: services.AdminLinkByCopy, LinkURL: link, ExpiresAt: setupExpiry,
		})

		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		assert.Equal(t, map[string]any{"delivery": "link", "link_url": link, "expires_at": "2026-10-21T09:00:00Z"},
			resp["setup"])
	})

	t.Run("no link, with the reason", func(t *testing.T) {
		for _, reason := range []string{services.AdminSetupReasonRootAdmin, services.AdminSetupReasonLocalSignInOff,
			services.AdminSetupReasonEmailFailed} {
			rr, resp := createWithSetup(t, services.AdminUserSetup{Delivery: "none", Reason: reason})

			require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
			assert.Equal(t, map[string]any{"delivery": "none", "reason": reason}, resp["setup"], reason)
		}
	})

	t.Run("the user's detail and sign-in state come with it", func(t *testing.T) {
		rr, resp := createWithSetup(t, services.AdminUserSetup{Delivery: "none", Reason: services.AdminSetupReasonLocalSignInOff})

		require.Equal(t, http.StatusCreated, rr.Code)
		assert.Equal(t, "new.user@example.com", resp["email"])
		assert.Equal(t, []any{}, resp["sign_in_methods"])
		assert.Equal(t, false, resp["has_password"])
	})
}

func TestCreateAdminUser_PassesTeamsAndAnOptionalName(t *testing.T) {
	id := uuid.NewString()
	team := uuid.NewString()
	admin := servicesmocks.NewMockAdminServiceInterface(t)
	admin.On("CreateUser", mock.Anything, services.AdminUserCreateRequest{
		Email: "new.user@example.com", Name: "",
		Teams: []services.AdminTeamAssignment{{TeamID: team, Role: models.TeamMemberRoleAdmin}},
	}).Return(createdUserDetail(id), nil)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: admin})

	req := postUserRequest(t, `{"email":"new.user@example.com","teams":[{"team_id":"`+team+`","role":"admin"}]}`)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
}

func TestCreateAdminUser_BadTeamIs400(t *testing.T) {
	admin := servicesmocks.NewMockAdminServiceInterface(t)
	admin.On("CreateUser", mock.Anything, mock.Anything).
		Return(nil, &services.ErrAdminUserTeamInvalid{Index: 1, Reason: "a personal workspace cannot be joined"})
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminService: admin})

	req := postUserRequest(t, `{"email":"new.user@example.com"}`)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), `"field":"teams[1]"`)
	assert.Contains(t, rr.Body.String(), "personal workspace")
	specconformance.AssertConformsToSpec(t, req, rr)
}

func postSetupLink(t *testing.T, account services.AdminAccountServiceInterface, userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminAccount: account})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+userID+"/setup-link", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	specconformance.AssertConformsToSpec(t, req, rr)
	return rr
}

func TestIssueAdminUserSetupLink(t *testing.T) {
	userID := uuid.NewString()

	t.Run("a link to copy", func(t *testing.T) {
		account := servicesmocks.NewMockAdminAccountServiceInterface(t)
		account.On("IssueSetupLink", mock.Anything, mock.Anything, userID, services.AdminLinkByCopy).
			Return(&services.AdminSetupLink{Delivery: services.AdminLinkByCopy,
				LinkURL: "https://app.example.com/account/setup#token=COPIED", ExpiresAt: setupExpiry}, nil)

		rr := postSetupLink(t, account, userID, `{"delivery":"link"}`)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.JSONEq(t, `{"delivery":"link","link_url":"https://app.example.com/account/setup#token=COPIED",`+
			`"expires_at":"2026-10-21T09:00:00Z"}`, rr.Body.String())
	})

	t.Run("by email", func(t *testing.T) {
		account := servicesmocks.NewMockAdminAccountServiceInterface(t)
		account.On("IssueSetupLink", mock.Anything, mock.Anything, userID, services.AdminLinkByEmail).
			Return(&services.AdminSetupLink{Delivery: services.AdminLinkByEmail, ExpiresAt: setupExpiry}, nil)

		rr := postSetupLink(t, account, userID, `{"delivery":"email"}`)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.JSONEq(t, `{"delivery":"email","expires_at":"2026-10-21T09:00:00Z"}`, rr.Body.String())
	})

	for code, err := range map[string]error{
		"own_account":             services.ErrAdminOwnAccount,
		"password_login_disabled": services.ErrAdminLocalSignInOff,
		"has_sign_in_method":      services.ErrAdminAccountHasSignInMethod,
		"account_suspended":       services.ErrAdminAccountSuspended,
		"email_unavailable":       services.ErrAccountEmailUnavailable,
		"root_admin_link_refused": services.ErrAdminRootAdminLinkRefused,
	} {
		t.Run("409 "+code, func(t *testing.T) {
			account := servicesmocks.NewMockAdminAccountServiceInterface(t)
			account.On("IssueSetupLink", mock.Anything, mock.Anything, userID, mock.Anything).Return(nil, err)

			rr := postSetupLink(t, account, userID, `{"delivery":"link"}`)

			require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
			var problem struct {
				Code   string `json:"code"`
				Detail string `json:"detail"`
			}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &problem))
			assert.Equal(t, code, problem.Code)
			assert.Equal(t, err.Error(), problem.Detail, "the UI can show why")
		})
	}

	t.Run("404 for an unknown user", func(t *testing.T) {
		account := servicesmocks.NewMockAdminAccountServiceInterface(t)
		account.On("IssueSetupLink", mock.Anything, mock.Anything, userID, mock.Anything).
			Return(nil, services.ErrAdminUserNotFound)

		rr := postSetupLink(t, account, userID, `{"delivery":"link"}`)

		assert.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	})

	t.Run("500 that says nothing for anything else", func(t *testing.T) {
		account := servicesmocks.NewMockAdminAccountServiceInterface(t)
		account.On("IssueSetupLink", mock.Anything, mock.Anything, userID, mock.Anything).
			Return(nil, errors.New("pq: connection refused"))

		rr := postSetupLink(t, account, userID, `{"delivery":"link"}`)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.NotContains(t, rr.Body.String(), "connection refused")
	})

	t.Run("the issue is audited with the delivery and never the link", func(t *testing.T) {
		actingID := uuid.NewString()
		account := servicesmocks.NewMockAdminAccountServiceInterface(t)
		account.On("IssueSetupLink", mock.Anything, actingID, userID, services.AdminLinkByCopy).
			Return(&services.AdminSetupLink{Email: "pending@example.com", Delivery: services.AdminLinkByCopy,
				LinkURL: "https://app.example.com/account/setup#token=COPIED", ExpiresAt: setupExpiry}, nil)
		activitySvc := expectSetupLinkAudit(t, actingID, userID, "pending@example.com", "link")
		srv := newAdminTestServer(&config.Config{}, &adminMockContainer{adminAccount: account, activityService: activitySvc})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+userID+"/setup-link",
			bytes.NewBufferString(`{"delivery":"link"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, actingID))
		rr := httptest.NewRecorder()
		mountAdminStrictRouter(srv).ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		activitySvc.AssertExpectations(t)
	})

	t.Run("400 for a bad delivery", func(t *testing.T) {
		for _, body := range []string{`{"delivery":"sms"}`, `{}`, `{not json`, `{"delivery":"link","extra":1}`} {
			rr := postSetupLink(t, servicesmocks.NewMockAdminAccountServiceInterface(t), userID, body)
			assert.Equal(t, http.StatusBadRequest, rr.Code, body)
		}
	})
}

// expectSetupLinkAudit expects exactly one setup-link activity for the user,
// carrying the delivery and the address and no link or token.
func expectSetupLinkAudit(t *testing.T, actingID, userID, email, delivery string) *MockActivityService {
	t.Helper()
	activitySvc := &MockActivityService{}
	activitySvc.On("RecordResourceActivity",
		mock.Anything, actingID, activities.ActivityTypeAdminUserSetupLinkIssued, activities.EntityTypeUser,
		mock.MatchedBy(func(entityID *string) bool { return entityID != nil && *entityID == userID }),
		mock.Anything,
		mock.MatchedBy(func(md map[string]interface{}) bool {
			raw, err := json.Marshal(md)
			require.NoError(t, err)
			return md["delivery"] == delivery && md["target_email"] == email && md["target_user_id"] == userID &&
				!strings.Contains(string(raw), "token")
		}),
	).Return(nil).Once()
	activitySvc.On("RecordResourceActivity", mock.Anything, mock.Anything, activities.ActivityTypeAdminUserCreated,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	return activitySvc
}

func TestCreateAdminUser_AuditsTheSetupLink(t *testing.T) {
	id := uuid.NewString()
	actingID := uuid.NewString()
	admin := servicesmocks.NewMockAdminServiceInterface(t)
	admin.On("CreateUser", mock.Anything, mock.Anything).Return(createdUserDetail(id), nil)
	account := servicesmocks.NewMockAdminAccountServiceInterface(t)
	account.On("SetUpNewUser", mock.Anything, actingID, id).Return(services.AdminUserSetup{
		Delivery: services.AdminLinkByCopy, LinkURL: "https://app.example.com/account/setup#token=COPIED", ExpiresAt: setupExpiry,
	})
	activitySvc := expectSetupLinkAudit(t, actingID, id, "new.user@example.com", "link")
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		adminService: admin, adminAccount: account, activityService: activitySvc,
	})

	req := postUserRequest(t, `{"email":"new.user@example.com"}`)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, actingID))
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	activitySvc.AssertExpectations(t)
}

func adminAccountRequest(
	t *testing.T, account services.AdminAccountServiceInterface, activity *MockActivityService,
	actingID, method, target, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	container := &adminMockContainer{adminAccount: account}
	if activity != nil {
		container.activityService = activity
	}
	srv := newAdminTestServer(&config.Config{}, container)
	var reader *bytes.Buffer
	if body != "" {
		reader = bytes.NewBufferString(body)
	} else {
		reader = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, actingID))
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	specconformance.AssertConformsToSpec(t, req, rr)
	return rr
}

func TestIssueAdminUserResetLink(t *testing.T) {
	userID := uuid.NewString()
	actingID := uuid.NewString()
	target := "/api/v1/admin/users/" + userID + "/reset-link"

	t.Run("a copied link with invalidate now, audited without the link", func(t *testing.T) {
		account := servicesmocks.NewMockAdminAccountServiceInterface(t)
		account.On("IssueResetLink", mock.Anything, actingID, userID,
			services.AdminResetLinkRequest{Delivery: services.AdminLinkByCopy, InvalidateNow: true}).
			Return(&services.AdminResetLink{AdminSetupLink: services.AdminSetupLink{
				Email: "ada@example.com", Delivery: services.AdminLinkByCopy,
				LinkURL: "https://app.example.com/account/reset#token=COPIED", ExpiresAt: setupExpiry,
			}, Invalidated: true}, nil)
		activitySvc := &MockActivityService{}
		activitySvc.On("RecordResourceActivity",
			mock.Anything, actingID, activities.ActivityTypeAdminUserResetLinkIssued, activities.EntityTypeUser,
			mock.MatchedBy(func(id *string) bool { return id != nil && *id == userID }), mock.Anything,
			mock.MatchedBy(func(md map[string]interface{}) bool {
				raw, err := json.Marshal(md)
				require.NoError(t, err)
				return md["delivery"] == "link" && md["invalidated"] == true && md["target_email"] == "ada@example.com" &&
					!strings.Contains(string(raw), "token")
			}),
		).Return(nil).Once()

		rr := adminAccountRequest(t, account, activitySvc, actingID, http.MethodPost, target,
			`{"delivery":"link","invalidate_now":true}`)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.JSONEq(t, `{"delivery":"link","link_url":"https://app.example.com/account/reset#token=COPIED",`+
			`"expires_at":"2026-10-21T09:00:00Z","invalidated":true}`, rr.Body.String())
		activitySvc.AssertExpectations(t)
	})

	t.Run("by email; invalidate now defaults to false", func(t *testing.T) {
		account := servicesmocks.NewMockAdminAccountServiceInterface(t)
		account.On("IssueResetLink", mock.Anything, actingID, userID,
			services.AdminResetLinkRequest{Delivery: services.AdminLinkByEmail}).
			Return(&services.AdminResetLink{AdminSetupLink: services.AdminSetupLink{
				Delivery: services.AdminLinkByEmail, ExpiresAt: setupExpiry,
			}}, nil)

		rr := adminAccountRequest(t, account, nil, actingID, http.MethodPost, target, `{"delivery":"email"}`)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.JSONEq(t, `{"delivery":"email","expires_at":"2026-10-21T09:00:00Z","invalidated":false}`, rr.Body.String())
	})

	for code, err := range map[string]error{
		"own_account":             services.ErrAdminOwnAccount,
		"password_login_disabled": services.ErrAdminLocalSignInOff,
		"account_suspended":       services.ErrAdminAccountSuspended,
		"email_unavailable":       services.ErrAccountEmailUnavailable,
		"email_unverified_shared": services.ErrAccountEmailUnverifiedShared,
		"root_admin_link_refused": services.ErrAdminRootAdminLinkRefused,
	} {
		t.Run("409 "+code, func(t *testing.T) {
			account := servicesmocks.NewMockAdminAccountServiceInterface(t)
			account.On("IssueResetLink", mock.Anything, mock.Anything, userID, mock.Anything).Return(nil, err)

			rr := adminAccountRequest(t, account, nil, actingID, http.MethodPost, target, `{"delivery":"link"}`)

			require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), `"code":"`+code+`"`)
		})
	}

	t.Run("400 for a bad delivery", func(t *testing.T) {
		for _, body := range []string{`{"delivery":"sms"}`, `{}`, `{"delivery":"link","extra":1}`} {
			rr := adminAccountRequest(t, servicesmocks.NewMockAdminAccountServiceInterface(t), nil, actingID,
				http.MethodPost, target, body)
			assert.Equal(t, http.StatusBadRequest, rr.Code, body)
		}
	})
}

func TestDeleteAdminUserPassword(t *testing.T) {
	userID := uuid.NewString()
	actingID := uuid.NewString()
	target := "/api/v1/admin/users/" + userID + "/password"

	t.Run("removed, confirmed as the last method, audited", func(t *testing.T) {
		account := servicesmocks.NewMockAdminAccountServiceInterface(t)
		account.On("RemovePassword", mock.Anything, actingID, userID, true).Return("ada@example.com", nil)
		activitySvc := &MockActivityService{}
		activitySvc.On("RecordResourceActivity",
			mock.Anything, actingID, activities.ActivityTypeAdminUserLocalSignInRemoved, activities.EntityTypeUser,
			mock.Anything, mock.Anything,
			mock.MatchedBy(func(md map[string]interface{}) bool {
				return md["target_email"] == "ada@example.com" && md["confirmed_last_method"] == true
			}),
		).Return(nil).Once()

		rr := adminAccountRequest(t, account, activitySvc, actingID, http.MethodDelete,
			target+"?confirm_last_method=true", "")

		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		activitySvc.AssertExpectations(t)
	})

	for name, tc := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"the last way in": {services.ErrLastSignInMethod, http.StatusConflict, apierrors.CodeLastSignInMethod},
		"own account":     {services.ErrAdminOwnAccount, http.StatusConflict, apierrors.CodeOwnAccount},
		"no password":     {services.ErrAccountPasswordNotSet, http.StatusNotFound, apierrors.CodeResourceNotFound},
		"no such user":    {services.ErrAdminUserNotFound, http.StatusNotFound, apierrors.CodeResourceNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			account := servicesmocks.NewMockAdminAccountServiceInterface(t)
			account.On("RemovePassword", mock.Anything, actingID, userID, false).Return("", tc.err)

			rr := adminAccountRequest(t, account, nil, actingID, http.MethodDelete, target, "")

			require.Equal(t, tc.status, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), `"code":"`+tc.code+`"`)
		})
	}
}

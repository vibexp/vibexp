package server

import (
	"bytes"
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
	"github.com/vibexp/vibexp/internal/services"
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

	t.Run("400 for a bad delivery", func(t *testing.T) {
		for _, body := range []string{`{"delivery":"sms"}`, `{}`, `{not json`} {
			rr := postSetupLink(t, servicesmocks.NewMockAdminAccountServiceInterface(t), userID, body)
			assert.Equal(t, http.StatusBadRequest, rr.Code, body)
		}
	})
}

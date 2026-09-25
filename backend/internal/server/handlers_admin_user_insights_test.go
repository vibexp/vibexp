package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
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

// adminInsightsForbiddenKeys are JSON keys that would mean resource content
// leaked into a counts-only payload (#1135, epic #1131 "counts only").
var adminInsightsForbiddenKeys = []string{`"title"`, `"slug"`, `"content"`, `"body"`, `"text"`}

// assertNoResourceContent fails when any content-bearing key appears.
func assertNoResourceContent(t *testing.T, body string) {
	t.Helper()
	for _, key := range adminInsightsForbiddenKeys {
		assert.NotContains(t, body, key)
	}
}

// serveAdminInsights runs one request through the admin strict router.
func serveAdminInsights(
	t *testing.T, container *adminMockContainer, path string,
) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	srv := newAdminTestServer(&config.Config{}, container)
	req := httptest.NewRequest("GET", path, nil)
	rr := httptest.NewRecorder()
	mountAdminStrictRouter(srv).ServeHTTP(rr, req)
	return req, rr
}

func TestGetAdminUserInsights(t *testing.T) {
	userID, teamID, projectID := uuid.NewString(), uuid.NewString(), uuid.NewString()

	t.Run("200 conforms and carries counts only", func(t *testing.T) {
		insights := &models.AdminUserInsights{
			UserID: userID,
			Totals: models.AdminResourceCounts{Prompts: 2, FeedItems: 1, Total: 3},
			Teams: []models.AdminUserTeamResourceCounts{{
				TeamID: teamID, TeamName: "Acme", IsMember: true,
				Counts: models.AdminResourceCounts{Prompts: 2, FeedItems: 1, Total: 3},
				Projects: []models.AdminUserProjectResourceCounts{{
					ProjectID: projectID, ProjectName: "Platform",
					Counts: models.AdminProjectResourceCounts{Prompts: 2},
				}},
			}},
		}
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserInsights", mock.Anything, userID).Return(insights, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin},
			"/api/v1/admin/users/"+userID+"/insights")

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assertNoResourceContent(t, rr.Body.String())

		var resp struct {
			UserID string `json:"user_id"`
			Totals struct {
				Prompts, FeedItems, Total int64
			} `json:"totals"`
			Teams []struct {
				TeamName string `json:"team_name"`
				IsMember bool   `json:"is_member"`
				Projects []struct {
					ProjectName string `json:"project_name"`
				} `json:"projects"`
			} `json:"teams"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Equal(t, userID, resp.UserID)
		assert.Equal(t, int64(3), resp.Totals.Total)
		require.Len(t, resp.Teams, 1)
		assert.Equal(t, "Acme", resp.Teams[0].TeamName)
		assert.True(t, resp.Teams[0].IsMember)
		require.Len(t, resp.Teams[0].Projects, 1)
		assert.Equal(t, "Platform", resp.Teams[0].Projects[0].ProjectName)
	})

	t.Run("a user with no resources serializes teams as []", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserInsights", mock.Anything, userID).Return(&models.AdminUserInsights{UserID: userID}, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin},
			"/api/v1/admin/users/"+userID+"/insights")

		require.Equal(t, http.StatusOK, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.Contains(t, rr.Body.String(), `"teams":[]`)
	})

	t.Run("404 for an unknown user", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserInsights", mock.Anything, userID).Return(nil, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin},
			"/api/v1/admin/users/"+userID+"/insights")

		require.Equal(t, http.StatusNotFound, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("400 for a malformed id", func(t *testing.T) {
		req, rr := serveAdminInsights(t, &adminMockContainer{}, "/api/v1/admin/users/not-a-uuid/insights")
		require.Equal(t, http.StatusBadRequest, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("500 when the service fails", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserInsights", mock.Anything, userID).Return(nil, errors.New("db down"))

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin},
			"/api/v1/admin/users/"+userID+"/insights")

		require.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})
}

func TestGetAdminUserResourceCreationMetrics(t *testing.T) {
	userID := uuid.NewString()
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	path := "/api/v1/admin/users/" + userID + "/resource-creation-metrics"

	t.Run("200 passes the range through and conforms", func(t *testing.T) {
		metrics := &models.AdminUserCreationMetrics{
			From: from, To: to, Granularity: "day",
			Series: []models.AdminUserCreationPoint{
				{Bucket: from, Prompts: 2, Memories: 1},
				{Bucket: from.AddDate(0, 0, 1)},
			},
		}
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserCreationMetrics", mock.Anything, userID,
			mock.MatchedBy(func(q services.AdminTimeseriesQuery) bool {
				return q.Granularity == "week" && q.From != nil && q.From.Equal(from) && q.To != nil && q.To.Equal(to)
			}),
		).Return(metrics, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin},
			path+"?from=2026-09-01T00:00:00Z&to=2026-09-03T00:00:00Z&granularity=week")

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assertNoResourceContent(t, rr.Body.String())
		assert.Contains(t, rr.Body.String(), `"prompts":2`)
	})

	t.Run("400 for an out-of-enum granularity", func(t *testing.T) {
		req, rr := serveAdminInsights(t, &adminMockContainer{}, path+"?granularity=hour")
		require.Equal(t, http.StatusBadRequest, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("400 for an invalid range", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserCreationMetrics", mock.Anything, userID, mock.Anything).
			Return(nil, &services.ErrAdminTimeseriesRange{Detail: "invalid range"})

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("404 for an unknown user", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserCreationMetrics", mock.Anything, userID, mock.Anything).Return(nil, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusNotFound, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("500 when the service fails", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserCreationMetrics", mock.Anything, userID, mock.Anything).Return(nil, errors.New("boom"))

		_, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestGetAdminUserTimeline(t *testing.T) {
	userID, teamID, projectID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	resourceID := uuid.NewString()
	path := "/api/v1/admin/users/" + userID + "/timeline"
	projectName := "Platform"
	next := "opaque-next"

	t.Run("200 rows carry exactly the opaque field set", func(t *testing.T) {
		page := &models.AdminUserTimelinePage{
			Items: []models.AdminUserTimelineEvent{
				{
					ResourceType: "prompt", Action: "updated", ResourceID: resourceID,
					TeamID: teamID, TeamName: "Acme", ProjectID: &projectID, ProjectName: &projectName,
					OccurredAt: time.Now().UTC(),
				},
				{
					ResourceType: "agent", Action: "created", ResourceID: uuid.NewString(),
					TeamID: teamID, TeamName: "Acme", OccurredAt: time.Now().UTC().Add(-time.Hour),
				},
			},
			NextCursor: &next,
		}
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTimeline", mock.Anything, userID, "abc", 2).Return(page, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path+"?cursor=abc&limit=2")

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assertNoResourceContent(t, rr.Body.String())
		assert.NotContains(t, rr.Body.String(), resourceID, "the full resource id must not leave the server")

		var resp struct {
			Items      []map[string]json.RawMessage `json:"items"`
			NextCursor *string                      `json:"next_cursor"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Len(t, resp.Items, 2)
		for _, item := range resp.Items {
			keys := make([]string, 0, len(item))
			for k := range item {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			assert.Equal(t, []string{
				"action", "occurred_at", "project_id", "project_name",
				"resource_short_id", "resource_type", "team_id", "team_name",
			}, keys)
		}
		assert.JSONEq(t, `"`+resourceID[:8]+`"`, string(resp.Items[0]["resource_short_id"]))
		assert.JSONEq(t, `null`, string(resp.Items[1]["project_id"]))
		require.NotNil(t, resp.NextCursor)
		assert.Equal(t, next, *resp.NextCursor)
	})

	t.Run("last page has a null cursor and an empty page is []", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTimeline", mock.Anything, userID, "", 0).Return(&models.AdminUserTimelinePage{}, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusOK, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.JSONEq(t, `{"items":[],"next_cursor":null}`, rr.Body.String())
	})

	for _, limit := range []string{"0", "101", "-1"} {
		t.Run("400 for limit "+limit, func(t *testing.T) {
			req, rr := serveAdminInsights(t, &adminMockContainer{}, path+"?limit="+limit)
			require.Equal(t, http.StatusBadRequest, rr.Code)
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}

	t.Run("400 for a malformed cursor", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTimeline", mock.Anything, userID, "garbage", 0).
			Return(nil, &services.ErrAdminInvalidCursor{Detail: "invalid cursor"})

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path+"?cursor=garbage")

		require.Equal(t, http.StatusBadRequest, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("404 for an unknown user", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTimeline", mock.Anything, userID, "", 0).Return(nil, nil)

		req, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)

		require.Equal(t, http.StatusNotFound, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})

	t.Run("500 when the service fails", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("GetUserTimeline", mock.Anything, userID, "", 0).Return(nil, errors.New("boom"))

		_, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestGetAdminUserNotificationPreferences(t *testing.T) {
	userID := uuid.NewString()
	path := "/api/v1/admin/users/" + userID + "/notification-preferences"

	t.Run("defaults for a user with no stored row", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("UserExists", mock.Anything, userID).Return(true, nil)
		mockPrefs := servicesmocks.NewMockUserPreferencesServiceInterface(t)
		mockPrefs.On("GetPreferences", mock.Anything, userID).
			Return(&models.PreferencesResponse{Preferences: models.DefaultPreferences()}, nil)

		req, rr := serveAdminInsights(t,
			&adminMockContainer{adminService: mockAdmin, prefsService: mockPrefs}, path)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)

		var resp struct {
			IsDefault bool       `json:"is_default"`
			UpdatedAt *time.Time `json:"updated_at"`
			Email     struct {
				AccountSecurity bool `json:"account_security"`
			} `json:"email_notification"`
			Notifications struct {
				Types map[string]json.RawMessage `json:"types"`
			} `json:"notifications"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.True(t, resp.IsDefault)
		assert.Nil(t, resp.UpdatedAt)
		defaults := models.DefaultPreferences()
		assert.Equal(t, defaults.EmailNotification.AccountSecurity, resp.Email.AccountSecurity)
		assert.Len(t, resp.Notifications.Types, len(defaults.Notifications.Types))
	})

	t.Run("stored values for a user who saved preferences", func(t *testing.T) {
		saved := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
		prefs := models.DefaultPreferences()
		prefs.EmailNotification.MarketingPromotional = true
		prefs.Notifications.Channels.Email = false

		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("UserExists", mock.Anything, userID).Return(true, nil)
		mockPrefs := servicesmocks.NewMockUserPreferencesServiceInterface(t)
		mockPrefs.On("GetPreferences", mock.Anything, userID).
			Return(&models.PreferencesResponse{Preferences: prefs, UpdatedAt: saved}, nil)

		req, rr := serveAdminInsights(t,
			&adminMockContainer{adminService: mockAdmin, prefsService: mockPrefs}, path)

		require.Equal(t, http.StatusOK, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		body := rr.Body.String()
		assert.Contains(t, body, `"is_default":false`)
		assert.Contains(t, body, `"updated_at":"2026-09-20T10:00:00Z"`)
		assert.Contains(t, body, `"marketing_promotional":true`)
		assert.True(t, strings.Contains(body, `"channels":{"email":false`), body)
	})

	t.Run("404 for an unknown user, without reading preferences", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("UserExists", mock.Anything, userID).Return(false, nil)
		mockPrefs := servicesmocks.NewMockUserPreferencesServiceInterface(t)

		req, rr := serveAdminInsights(t,
			&adminMockContainer{adminService: mockAdmin, prefsService: mockPrefs}, path)

		require.Equal(t, http.StatusNotFound, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
		mockPrefs.AssertNotCalled(t, "GetPreferences", mock.Anything, mock.Anything)
	})

	t.Run("500 when the existence check fails", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("UserExists", mock.Anything, userID).Return(false, errors.New("boom"))

		_, rr := serveAdminInsights(t, &adminMockContainer{adminService: mockAdmin}, path)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})

	t.Run("500 when reading preferences fails", func(t *testing.T) {
		mockAdmin := servicesmocks.NewMockAdminServiceInterface(t)
		mockAdmin.On("UserExists", mock.Anything, userID).Return(true, nil)
		mockPrefs := servicesmocks.NewMockUserPreferencesServiceInterface(t)
		mockPrefs.On("GetPreferences", mock.Anything, userID).Return(nil, errors.New("boom"))

		_, rr := serveAdminInsights(t,
			&adminMockContainer{adminService: mockAdmin, prefsService: mockPrefs}, path)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

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

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// MockUserPreferencesService is a mock implementation of UserPreferencesServiceInterface
type MockUserPreferencesService struct {
	mock.Mock
}

func (m *MockUserPreferencesService) GetPreferences(
	ctx context.Context, userID string,
) (*models.PreferencesResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.PreferencesResponse), args.Error(1)
}

func (m *MockUserPreferencesService) UpdatePreferences(
	ctx context.Context, userID string, req models.UpdatePreferencesRequest,
) (*models.PreferencesResponse, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.PreferencesResponse), args.Error(1)
}

// MockPreferencesContainer implements Container interface for preferences handler tests
type MockPreferencesContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	preferencesService *MockUserPreferencesService
}

func (m *MockPreferencesContainer) UserPreferencesService() services.UserPreferencesServiceInterface {
	return m.preferencesService
}

func newMockPreferencesContainer(_ *testing.T) *MockPreferencesContainer {
	return &MockPreferencesContainer{
		preferencesService: &MockUserPreferencesService{},
	}
}

func createTestPreferencesServer(container *MockPreferencesContainer) *Server {
	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during test

	// Initialize router manually for testing
	r := chi.NewRouter()

	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	// Register routes manually for testing
	r.Route("/api/v1/preferences", func(r chi.Router) {
		r.Get("/", srv.handleGetPreferences)
		r.Put("/", srv.handleUpdatePreferences)
	})

	return srv
}

const preferencesTestPath = "/api/v1/preferences"

func makeAuthenticatedPreferencesRequest(method string, body interface{}, userID string) *http.Request {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			panic(err)
		}
	}

	req := httptest.NewRequest(method, preferencesTestPath, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, userID))

	return req
}

// TestHandleGetPreferences_Success tests successful retrieval of existing user preferences
func TestHandleGetPreferences_Success(t *testing.T) {
	mockContainer := newMockPreferencesContainer(t)

	userID := "user-123"
	updatedAt := time.Now()

	expectedResponse := &models.PreferencesResponse{
		Preferences: models.Preferences{
			EmailNotification: models.EmailNotificationPreferences{
				PlatformAnnouncement: true,
				AccountSecurity:      true,
				NewFeature:           false,
				MarketingPromotional: true,
			},
		},
		UpdatedAt: updatedAt,
	}

	mockContainer.preferencesService.On("GetPreferences", mock.Anything, userID).
		Return(expectedResponse, nil)

	srv := createTestPreferencesServer(mockContainer)
	req := makeAuthenticatedPreferencesRequest("GET", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PreferencesResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.True(t, response.Preferences.EmailNotification.PlatformAnnouncement)
	assert.True(t, response.Preferences.EmailNotification.AccountSecurity)
	assert.False(t, response.Preferences.EmailNotification.NewFeature)
	assert.True(t, response.Preferences.EmailNotification.MarketingPromotional)

	mockContainer.preferencesService.AssertExpectations(t)
}

// TestHandleGetPreferences_NewUser tests retrieval of default preferences for a new user
func TestHandleGetPreferences_NewUser(t *testing.T) {
	mockContainer := newMockPreferencesContainer(t)

	userID := "new-user-456"

	// Return default preferences (no UpdatedAt since user has no preferences yet)
	expectedResponse := &models.PreferencesResponse{
		Preferences: models.DefaultPreferences(),
	}

	mockContainer.preferencesService.On("GetPreferences", mock.Anything, userID).
		Return(expectedResponse, nil)

	srv := createTestPreferencesServer(mockContainer)
	req := makeAuthenticatedPreferencesRequest("GET", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PreferencesResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Verify default values
	assert.True(t, response.Preferences.EmailNotification.PlatformAnnouncement)
	assert.True(t, response.Preferences.EmailNotification.AccountSecurity)
	assert.True(t, response.Preferences.EmailNotification.NewFeature)
	assert.False(t, response.Preferences.EmailNotification.MarketingPromotional)

	mockContainer.preferencesService.AssertExpectations(t)
}

// TestHandleGetPreferences_ServiceError tests error handling when service fails
func TestHandleGetPreferences_ServiceError(t *testing.T) {
	mockContainer := newMockPreferencesContainer(t)

	userID := "user-123"

	mockContainer.preferencesService.On("GetPreferences", mock.Anything, userID).
		Return((*models.PreferencesResponse)(nil), errors.New("database connection failed"))

	srv := createTestPreferencesServer(mockContainer)
	req := makeAuthenticatedPreferencesRequest("GET", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// RFC 9457 error format
	assert.Equal(t, "DATABASE_ERROR", response["code"])
	assert.Equal(t, float64(500), response["status"])
	assert.Contains(t, response["detail"], "Failed to retrieve user preferences")

	mockContainer.preferencesService.AssertExpectations(t)
}

// TestHandleUpdatePreferences_Success tests successful update of all preferences
func TestHandleUpdatePreferences_Success(t *testing.T) {
	mockContainer := newMockPreferencesContainer(t)

	userID := "user-123"
	updatedAt := time.Now()

	updateReq := models.UpdatePreferencesRequest{
		EmailNotification: &models.EmailNotificationPreferences{
			PlatformAnnouncement: false,
			AccountSecurity:      false, // Will be enforced to true by service
			NewFeature:           true,
			MarketingPromotional: true,
		},
	}

	expectedResponse := &models.PreferencesResponse{
		Preferences: models.Preferences{
			EmailNotification: models.EmailNotificationPreferences{
				PlatformAnnouncement: false,
				AccountSecurity:      true, // Always true
				NewFeature:           true,
				MarketingPromotional: true,
			},
		},
		UpdatedAt: updatedAt,
	}

	mockContainer.preferencesService.On("UpdatePreferences", mock.Anything, userID, updateReq).
		Return(expectedResponse, nil)

	srv := createTestPreferencesServer(mockContainer)
	req := makeAuthenticatedPreferencesRequest("PUT", updateReq, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PreferencesResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.False(t, response.Preferences.EmailNotification.PlatformAnnouncement)
	assert.True(t, response.Preferences.EmailNotification.AccountSecurity) // Always true
	assert.True(t, response.Preferences.EmailNotification.NewFeature)
	assert.True(t, response.Preferences.EmailNotification.MarketingPromotional)

	mockContainer.preferencesService.AssertExpectations(t)
}

// TestHandleUpdatePreferences_PartialUpdate tests updating only some preference fields
func TestHandleUpdatePreferences_PartialUpdate(t *testing.T) {
	mockContainer := newMockPreferencesContainer(t)

	userID := "user-123"
	updatedAt := time.Now()

	// Only updating email notification preferences
	updateReq := models.UpdatePreferencesRequest{
		EmailNotification: &models.EmailNotificationPreferences{
			PlatformAnnouncement: true,
			AccountSecurity:      true,
			NewFeature:           false,
			MarketingPromotional: false,
		},
	}

	expectedResponse := &models.PreferencesResponse{
		Preferences: models.Preferences{
			EmailNotification: models.EmailNotificationPreferences{
				PlatformAnnouncement: true,
				AccountSecurity:      true,
				NewFeature:           false,
				MarketingPromotional: false,
			},
		},
		UpdatedAt: updatedAt,
	}

	mockContainer.preferencesService.On("UpdatePreferences", mock.Anything, userID, updateReq).
		Return(expectedResponse, nil)

	srv := createTestPreferencesServer(mockContainer)
	req := makeAuthenticatedPreferencesRequest("PUT", updateReq, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PreferencesResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.True(t, response.Preferences.EmailNotification.PlatformAnnouncement)
	assert.True(t, response.Preferences.EmailNotification.AccountSecurity)
	assert.False(t, response.Preferences.EmailNotification.NewFeature)
	assert.False(t, response.Preferences.EmailNotification.MarketingPromotional)

	mockContainer.preferencesService.AssertExpectations(t)
}

// TestHandleUpdatePreferences_EmptyBody tests update with empty request body
func TestHandleUpdatePreferences_EmptyBody(t *testing.T) {
	mockContainer := newMockPreferencesContainer(t)

	userID := "user-123"
	updatedAt := time.Now()

	// Empty update request (no email notification provided)
	updateReq := models.UpdatePreferencesRequest{}

	expectedResponse := &models.PreferencesResponse{
		Preferences: models.DefaultPreferences(),
		UpdatedAt:   updatedAt,
	}

	mockContainer.preferencesService.On("UpdatePreferences", mock.Anything, userID, updateReq).
		Return(expectedResponse, nil)

	srv := createTestPreferencesServer(mockContainer)
	req := makeAuthenticatedPreferencesRequest("PUT", updateReq, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockContainer.preferencesService.AssertExpectations(t)
}

// TestHandleUpdatePreferences_InvalidJSON tests rejection of malformed JSON
func TestHandleUpdatePreferences_InvalidJSON(t *testing.T) {
	mockContainer := newMockPreferencesContainer(t)

	userID := "user-123"

	srv := createTestPreferencesServer(mockContainer)

	// Create request with invalid JSON
	req := httptest.NewRequest("PUT", "/api/v1/preferences", bytes.NewReader([]byte(`{invalid json}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, userID))

	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// RFC 9457 error format
	assert.Equal(t, "BAD_REQUEST", response["code"])
	assert.Equal(t, float64(400), response["status"])
	assert.Contains(t, response["detail"], "Invalid request body")
}

// TestHandleUpdatePreferences_ServiceError tests error handling when service fails
func TestHandleUpdatePreferences_ServiceError(t *testing.T) {
	mockContainer := newMockPreferencesContainer(t)

	userID := "user-123"

	updateReq := models.UpdatePreferencesRequest{
		EmailNotification: &models.EmailNotificationPreferences{
			PlatformAnnouncement: true,
			AccountSecurity:      true,
			NewFeature:           true,
			MarketingPromotional: false,
		},
	}

	mockContainer.preferencesService.On("UpdatePreferences", mock.Anything, userID, updateReq).
		Return((*models.PreferencesResponse)(nil), errors.New("database error"))

	srv := createTestPreferencesServer(mockContainer)
	req := makeAuthenticatedPreferencesRequest("PUT", updateReq, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// RFC 9457 error format
	assert.Equal(t, "PREFERENCES_UPDATE_FAILED", response["code"])
	assert.Equal(t, float64(500), response["status"])
	assert.Contains(t, response["detail"], "Unable to update preferences")

	mockContainer.preferencesService.AssertExpectations(t)
}

// TestHandleUpdatePreferences_AccountSecurityAlwaysTrue tests that account_security cannot be disabled
func TestHandleUpdatePreferences_AccountSecurityAlwaysTrue(t *testing.T) {
	mockContainer := newMockPreferencesContainer(t)

	userID := "user-123"
	updatedAt := time.Now()

	// User tries to disable account security
	updateReq := models.UpdatePreferencesRequest{
		EmailNotification: &models.EmailNotificationPreferences{
			PlatformAnnouncement: true,
			AccountSecurity:      false, // User tries to disable this
			NewFeature:           true,
			MarketingPromotional: false,
		},
	}

	// Service enforces account_security as true
	expectedResponse := &models.PreferencesResponse{
		Preferences: models.Preferences{
			EmailNotification: models.EmailNotificationPreferences{
				PlatformAnnouncement: true,
				AccountSecurity:      true, // Always enforced as true
				NewFeature:           true,
				MarketingPromotional: false,
			},
		},
		UpdatedAt: updatedAt,
	}

	mockContainer.preferencesService.On("UpdatePreferences", mock.Anything, userID, updateReq).
		Return(expectedResponse, nil)

	srv := createTestPreferencesServer(mockContainer)
	req := makeAuthenticatedPreferencesRequest("PUT", updateReq, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PreferencesResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Verify account_security is always true regardless of input
	assert.True(t, response.Preferences.EmailNotification.AccountSecurity)

	mockContainer.preferencesService.AssertExpectations(t)
}

// TestHandleGetPreferences_ResponseFormat tests the response JSON structure
func TestHandleGetPreferences_ResponseFormat(t *testing.T) {
	mockContainer := newMockPreferencesContainer(t)

	userID := "user-123"
	updatedAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	expectedResponse := &models.PreferencesResponse{
		Preferences: models.Preferences{
			EmailNotification: models.EmailNotificationPreferences{
				PlatformAnnouncement: true,
				AccountSecurity:      true,
				NewFeature:           true,
				MarketingPromotional: false,
			},
		},
		UpdatedAt: updatedAt,
	}

	mockContainer.preferencesService.On("GetPreferences", mock.Anything, userID).
		Return(expectedResponse, nil)

	srv := createTestPreferencesServer(mockContainer)
	req := makeAuthenticatedPreferencesRequest("GET", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Verify JSON structure
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Check top-level keys
	assert.Contains(t, response, "preferences")
	assert.Contains(t, response, "updated_at")

	// Check preferences structure
	prefs := response["preferences"].(map[string]interface{})
	assert.Contains(t, prefs, "email_notification")

	// Check email notification structure
	emailNotif := prefs["email_notification"].(map[string]interface{})
	assert.Contains(t, emailNotif, "platform_announcement")
	assert.Contains(t, emailNotif, "account_security")
	assert.Contains(t, emailNotif, "new_feature")
	assert.Contains(t, emailNotif, "marketing_promotional")

	mockContainer.preferencesService.AssertExpectations(t)
}

// runUpdatePreferencesSubTest executes a single update preferences test case
func runUpdatePreferencesSubTest(t *testing.T, emailPrefs models.EmailNotificationPreferences) {
	mockContainer := newMockPreferencesContainer(t)
	userID := "user-123"

	updateReq := models.UpdatePreferencesRequest{EmailNotification: &emailPrefs}

	// Response always has AccountSecurity as true
	responsePrefs := emailPrefs
	responsePrefs.AccountSecurity = true

	expectedResponse := &models.PreferencesResponse{
		Preferences: models.Preferences{EmailNotification: responsePrefs},
		UpdatedAt:   time.Now(),
	}

	mockContainer.preferencesService.On("UpdatePreferences", mock.Anything, userID, updateReq).
		Return(expectedResponse, nil)

	srv := createTestPreferencesServer(mockContainer)
	req := makeAuthenticatedPreferencesRequest("PUT", updateReq, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PreferencesResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.Preferences.EmailNotification.AccountSecurity)

	mockContainer.preferencesService.AssertExpectations(t)
}

// TestHandleUpdatePreferences_AllFieldsCombinations tests various combinations of preference fields
func TestHandleUpdatePreferences_AllFieldsCombinations(t *testing.T) {
	tests := []struct {
		name       string
		emailPrefs models.EmailNotificationPreferences
	}{
		{
			name: "all_enabled",
			emailPrefs: models.EmailNotificationPreferences{
				PlatformAnnouncement: true, AccountSecurity: true,
				NewFeature: true, MarketingPromotional: true,
			},
		},
		{
			name: "all_disabled_except_security",
			emailPrefs: models.EmailNotificationPreferences{
				PlatformAnnouncement: false, AccountSecurity: false,
				NewFeature: false, MarketingPromotional: false,
			},
		},
		{
			name: "mixed_preferences",
			emailPrefs: models.EmailNotificationPreferences{
				PlatformAnnouncement: true, AccountSecurity: true,
				NewFeature: false, MarketingPromotional: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runUpdatePreferencesSubTest(t, tt.emailPrefs)
		})
	}
}

// TestHandleUpdatePreferences_WithNotifications tests that the handler decodes
// the notifications block correctly and passes it to the service unchanged.
//
//nolint:funlen // table-driven test with multiple notification test cases
func TestHandleUpdatePreferences_WithNotifications(t *testing.T) {
	tests := []struct {
		name          string
		updateReq     models.UpdatePreferencesRequest
		serviceResult *models.PreferencesResponse
		serviceError  error
		wantStatus    int
		checkResponse func(t *testing.T, resp models.PreferencesResponse)
	}{
		{
			name: "notifications_block_decoded_and_forwarded",
			updateReq: models.UpdatePreferencesRequest{
				Notifications: &models.NotificationPreferences{
					Channels: models.NotificationChannelPreferences{
						InApp:   true,
						Email:   false,
						WebPush: true,
					},
					Types: map[string]models.NotificationTypePreference{
						"feed.item.created": {InApp: true, Email: "digest", WebPush: false},
						"team.invitation":   {InApp: true, Email: "instant", WebPush: false},
					},
				},
			},
			serviceResult: &models.PreferencesResponse{
				Preferences: models.Preferences{
					EmailNotification: models.DefaultPreferences().EmailNotification,
					Notifications: models.NotificationPreferences{
						Channels: models.NotificationChannelPreferences{
							InApp:   true,
							Email:   false,
							WebPush: true,
						},
						Types: map[string]models.NotificationTypePreference{
							"feed.item.created": {InApp: true, Email: "digest", WebPush: false},
							"team.invitation":   {InApp: true, Email: "instant", WebPush: false},
						},
					},
				},
			},
			serviceError: nil,
			wantStatus:   http.StatusOK,
			checkResponse: func(t *testing.T, resp models.PreferencesResponse) {
				t.Helper()
				assert.False(t, resp.Preferences.Notifications.Channels.Email)
				assert.True(t, resp.Preferences.Notifications.Channels.InApp)
				assert.True(t, resp.Preferences.Notifications.Channels.WebPush)
				assert.Equal(t, "digest", resp.Preferences.Notifications.Types["feed.item.created"].Email)
				assert.Equal(t, "instant", resp.Preferences.Notifications.Types["team.invitation"].Email)
			},
		},
		{
			name: "notifications_and_email_notification_together",
			updateReq: models.UpdatePreferencesRequest{
				EmailNotification: &models.EmailNotificationPreferences{
					PlatformAnnouncement: true,
					AccountSecurity:      true,
					NewFeature:           false,
					MarketingPromotional: false,
				},
				Notifications: &models.NotificationPreferences{
					Channels: models.NotificationChannelPreferences{
						InApp:   true,
						Email:   true,
						WebPush: false,
					},
					Types: map[string]models.NotificationTypePreference{
						"feed.reply.created": {InApp: false, Email: "none", WebPush: false},
					},
				},
			},
			serviceResult: &models.PreferencesResponse{
				Preferences: models.Preferences{
					EmailNotification: models.EmailNotificationPreferences{
						PlatformAnnouncement: true,
						AccountSecurity:      true,
						NewFeature:           false,
						MarketingPromotional: false,
					},
					Notifications: models.NotificationPreferences{
						Channels: models.NotificationChannelPreferences{
							InApp:   true,
							Email:   true,
							WebPush: false,
						},
						Types: map[string]models.NotificationTypePreference{
							"feed.reply.created": {InApp: false, Email: "none", WebPush: false},
						},
					},
				},
			},
			serviceError: nil,
			wantStatus:   http.StatusOK,
			checkResponse: func(t *testing.T, resp models.PreferencesResponse) {
				t.Helper()
				assert.True(t, resp.Preferences.EmailNotification.AccountSecurity)
				assert.False(t, resp.Preferences.EmailNotification.NewFeature)
				assert.True(t, resp.Preferences.Notifications.Channels.Email)
				assert.Equal(t, "none", resp.Preferences.Notifications.Types["feed.reply.created"].Email)
			},
		},
		{
			name: "notifications_only_no_email_notification",
			updateReq: models.UpdatePreferencesRequest{
				Notifications: &models.NotificationPreferences{
					Channels: models.NotificationChannelPreferences{
						InApp:   false,
						Email:   false,
						WebPush: false,
					},
					Types: map[string]models.NotificationTypePreference{},
				},
			},
			serviceResult: &models.PreferencesResponse{
				Preferences: models.Preferences{
					EmailNotification: models.DefaultPreferences().EmailNotification,
					Notifications: models.NotificationPreferences{
						Channels: models.NotificationChannelPreferences{
							InApp:   false,
							Email:   false,
							WebPush: false,
						},
						Types: map[string]models.NotificationTypePreference{},
					},
				},
			},
			serviceError: nil,
			wantStatus:   http.StatusOK,
			checkResponse: func(t *testing.T, resp models.PreferencesResponse) {
				t.Helper()
				assert.False(t, resp.Preferences.Notifications.Channels.InApp)
				assert.False(t, resp.Preferences.Notifications.Channels.Email)
				assert.False(t, resp.Preferences.Notifications.Channels.WebPush)
				assert.NotNil(t, resp.Preferences.Notifications.Types)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockPreferencesContainer(t)
			userID := "user-notif-test"

			mockContainer.preferencesService.On(
				"UpdatePreferences", mock.Anything, userID, tt.updateReq,
			).Return(tt.serviceResult, tt.serviceError)

			srv := createTestPreferencesServer(mockContainer)
			req := makeAuthenticatedPreferencesRequest("PUT", tt.updateReq, userID)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.checkResponse != nil && tt.wantStatus == http.StatusOK {
				var response models.PreferencesResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				tt.checkResponse(t, response)
			}

			mockContainer.preferencesService.AssertExpectations(t)
		})
	}
}

// TestHandleGetPreferences_DifferentUsers tests preferences retrieval for different users
func TestHandleGetPreferences_DifferentUsers(t *testing.T) {
	tests := []struct {
		name   string
		userID string
	}{
		{
			name:   "regular_user",
			userID: "user-123",
		},
		{
			name:   "uuid_user",
			userID: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:   "long_user_id",
			userID: "user-with-very-long-identifier-123456789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockPreferencesContainer(t)

			expectedResponse := &models.PreferencesResponse{
				Preferences: models.DefaultPreferences(),
				UpdatedAt:   time.Now(),
			}

			mockContainer.preferencesService.On("GetPreferences", mock.Anything, tt.userID).
				Return(expectedResponse, nil)

			srv := createTestPreferencesServer(mockContainer)
			req := makeAuthenticatedPreferencesRequest("GET", nil, tt.userID)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			mockContainer.preferencesService.AssertExpectations(t)
		})
	}
}

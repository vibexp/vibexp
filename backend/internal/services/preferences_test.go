package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
)

// MockUserPreferencesRepository is a mock implementation of UserPreferencesRepository
type MockUserPreferencesRepository struct {
	mock.Mock
}

func (m *MockUserPreferencesRepository) GetByUserID(
	ctx context.Context,
	userID string,
) (*models.UserPreferences, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserPreferences), args.Error(1)
}

func (m *MockUserPreferencesRepository) Upsert(
	ctx context.Context,
	prefs *models.UserPreferences,
) error {
	args := m.Called(ctx, prefs)
	return args.Error(0)
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserPreferencesService_GetPreferences(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		setupMock      func(*MockUserPreferencesRepository)
		expectedResult *models.PreferencesResponse
		expectedError  bool
	}{
		{
			name:   "returns existing preferences with notifications merged from defaults",
			userID: "user-123",
			setupMock: func(m *MockUserPreferencesRepository) {
				m.On("GetByUserID", mock.Anything, "user-123").Return(&models.UserPreferences{
					ID:     "pref-123",
					UserID: "user-123",
					Preferences: models.Preferences{
						EmailNotification: models.EmailNotificationPreferences{
							PlatformAnnouncement: false,
							AccountSecurity:      true,
							NewFeature:           true,
							MarketingPromotional: false,
						},
						// Notifications is zero-value; defaults must be merged in
					},
					UpdatedAt: time.Now(),
				}, nil)
			},
			expectedResult: &models.PreferencesResponse{
				Preferences: models.Preferences{
					EmailNotification: models.EmailNotificationPreferences{
						PlatformAnnouncement: false,
						AccountSecurity:      true,
						NewFeature:           true,
						MarketingPromotional: false,
					},
					Notifications: models.DefaultNotificationPreferences(),
				},
			},
			expectedError: false,
		},
		{
			name:   "returns default preferences when none exist",
			userID: "user-456",
			setupMock: func(m *MockUserPreferencesRepository) {
				m.On("GetByUserID", mock.Anything, "user-456").Return(nil, nil)
			},
			expectedResult: &models.PreferencesResponse{
				Preferences: models.DefaultPreferences(),
			},
			expectedError: false,
		},
		{
			name:   "returns error on repository failure",
			userID: "user-789",
			setupMock: func(m *MockUserPreferencesRepository) {
				m.On("GetByUserID", mock.Anything, "user-789").
					Return(nil, errors.New("database error"))
			},
			expectedResult: nil,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockUserPreferencesRepository)
			tt.setupMock(mockRepo)

			service := NewUserPreferencesService(mockRepo)
			result, err := service.GetPreferences(context.Background(), tt.userID)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.Preferences, result.Preferences)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestUserPreferencesService_UpdatePreferences(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		request        models.UpdatePreferencesRequest
		setupMock      func(*MockUserPreferencesRepository)
		expectedResult *models.PreferencesResponse
		expectedError  bool
	}{
		{
			name:   "updates existing preferences",
			userID: "user-123",
			request: models.UpdatePreferencesRequest{
				EmailNotification: &models.EmailNotificationPreferences{
					PlatformAnnouncement: false,
					AccountSecurity:      false, // Should be overridden to true
					NewFeature:           false,
					MarketingPromotional: true,
				},
			},
			setupMock: func(m *MockUserPreferencesRepository) {
				m.On("GetByUserID", mock.Anything, "user-123").Return(&models.UserPreferences{
					ID:     "pref-123",
					UserID: "user-123",
					Preferences: models.Preferences{
						EmailNotification: models.EmailNotificationPreferences{
							PlatformAnnouncement: true,
							AccountSecurity:      true,
							NewFeature:           true,
							MarketingPromotional: false,
						},
					},
				}, nil)
				m.On("Upsert", mock.Anything, mock.MatchedBy(func(p *models.UserPreferences) bool {
					// Verify AccountSecurity is always true
					return p.Preferences.EmailNotification.AccountSecurity &&
						!p.Preferences.EmailNotification.PlatformAnnouncement &&
						!p.Preferences.EmailNotification.NewFeature &&
						p.Preferences.EmailNotification.MarketingPromotional
				})).Return(nil)
			},
			expectedResult: &models.PreferencesResponse{
				Preferences: models.Preferences{
					EmailNotification: models.EmailNotificationPreferences{
						PlatformAnnouncement: false,
						AccountSecurity:      true, // Always true
						NewFeature:           false,
						MarketingPromotional: true,
					},
				},
			},
			expectedError: false,
		},
		{
			name:   "creates new preferences with defaults when none exist",
			userID: "user-new",
			request: models.UpdatePreferencesRequest{
				EmailNotification: &models.EmailNotificationPreferences{
					PlatformAnnouncement: true,
					AccountSecurity:      true,
					NewFeature:           false,
					MarketingPromotional: true,
				},
			},
			setupMock: func(m *MockUserPreferencesRepository) {
				m.On("GetByUserID", mock.Anything, "user-new").Return(nil, nil)
				m.On("Upsert", mock.Anything, mock.MatchedBy(func(p *models.UserPreferences) bool {
					return p.UserID == "user-new" &&
						p.Preferences.EmailNotification.AccountSecurity
				})).Return(nil)
			},
			expectedResult: &models.PreferencesResponse{
				Preferences: models.Preferences{
					EmailNotification: models.EmailNotificationPreferences{
						PlatformAnnouncement: true,
						AccountSecurity:      true,
						NewFeature:           false,
						MarketingPromotional: true,
					},
					Notifications: models.DefaultNotificationPreferences(),
				},
			},
			expectedError: false,
		},
		{
			name:   "returns error on get failure",
			userID: "user-error",
			request: models.UpdatePreferencesRequest{
				EmailNotification: &models.EmailNotificationPreferences{},
			},
			setupMock: func(m *MockUserPreferencesRepository) {
				m.On("GetByUserID", mock.Anything, "user-error").
					Return(nil, errors.New("get error"))
			},
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name:   "returns error on upsert failure",
			userID: "user-upsert-error",
			request: models.UpdatePreferencesRequest{
				EmailNotification: &models.EmailNotificationPreferences{},
			},
			setupMock: func(m *MockUserPreferencesRepository) {
				m.On("GetByUserID", mock.Anything, "user-upsert-error").Return(nil, nil)
				m.On("Upsert", mock.Anything, mock.Anything).
					Return(errors.New("upsert error"))
			},
			expectedResult: nil,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockUserPreferencesRepository)
			tt.setupMock(mockRepo)

			service := NewUserPreferencesService(mockRepo)
			result, err := service.UpdatePreferences(
				context.Background(),
				tt.userID,
				tt.request,
			)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.Preferences, result.Preferences)
				// Always verify AccountSecurity is true
				assert.True(t, result.Preferences.EmailNotification.AccountSecurity)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestUserPreferencesService_AccountSecurityAlwaysEnabled(t *testing.T) {
	// This test verifies the business rule that AccountSecurity cannot be disabled
	mockRepo := new(MockUserPreferencesRepository)

	mockRepo.On("GetByUserID", mock.Anything, "user-test").Return(nil, nil)
	mockRepo.On("Upsert", mock.Anything, mock.MatchedBy(func(p *models.UserPreferences) bool {
		// AccountSecurity MUST always be true
		return p.Preferences.EmailNotification.AccountSecurity
	})).Return(nil)

	service := NewUserPreferencesService(mockRepo)

	// Try to disable AccountSecurity
	result, err := service.UpdatePreferences(
		context.Background(),
		"user-test",
		models.UpdatePreferencesRequest{
			EmailNotification: &models.EmailNotificationPreferences{
				PlatformAnnouncement: false,
				AccountSecurity:      false, // Attempting to disable
				NewFeature:           false,
				MarketingPromotional: false,
			},
		},
	)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Verify AccountSecurity is STILL true despite the request to disable it
	assert.True(t, result.Preferences.EmailNotification.AccountSecurity)

	mockRepo.AssertExpectations(t)
}

func TestUserPreferencesService_GetPreferences_MergesDefaultNotifications(t *testing.T) {
	// Users with existing rows that lack the notifications sub-tree must receive
	// defaults instead of zero/nil values, preventing frontend crashes.
	mockRepo := new(MockUserPreferencesRepository)

	mockRepo.On("GetByUserID", mock.Anything, "user-legacy").Return(&models.UserPreferences{
		ID:     "pref-legacy",
		UserID: "user-legacy",
		Preferences: models.Preferences{
			EmailNotification: models.EmailNotificationPreferences{
				PlatformAnnouncement: true,
				AccountSecurity:      true,
				NewFeature:           true,
				MarketingPromotional: false,
			},
			// Notifications is zero-value: Channels is empty struct, Types is nil
		},
		UpdatedAt: time.Now(),
	}, nil)

	service := NewUserPreferencesService(mockRepo)
	result, err := service.GetPreferences(context.Background(), "user-legacy")

	assert.NoError(t, err)
	assert.NotNil(t, result)

	defaults := models.DefaultNotificationPreferences()

	// Channels must be merged from defaults
	assert.Equal(t, defaults.Channels, result.Preferences.Notifications.Channels)
	// Types must not be nil
	assert.NotNil(t, result.Preferences.Notifications.Types)
	assert.Equal(t, defaults.Types, result.Preferences.Notifications.Types)

	mockRepo.AssertExpectations(t)
}

func TestUserPreferencesService_UpdatePreferences_PersistsNotifications(t *testing.T) {
	// Verifies that the Notifications body sent by the frontend is stored correctly.
	mockRepo := new(MockUserPreferencesRepository)

	customTypes := map[string]models.NotificationTypePreference{
		"feed.item.created": {InApp: true, Email: "none", WebPush: true},
	}
	reqNotifications := &models.NotificationPreferences{
		Channels: models.NotificationChannelPreferences{
			InApp:   true,
			Email:   false,
			WebPush: true,
		},
		Types: customTypes,
	}

	mockRepo.On("GetByUserID", mock.Anything, "user-notif").Return(nil, nil)
	mockRepo.On("Upsert", mock.Anything, mock.MatchedBy(func(p *models.UserPreferences) bool {
		n := p.Preferences.Notifications
		return n.Channels.WebPush && !n.Channels.Email && n.Channels.InApp &&
			n.Types != nil
	})).Return(nil)

	service := NewUserPreferencesService(mockRepo)
	result, err := service.UpdatePreferences(
		context.Background(),
		"user-notif",
		models.UpdatePreferencesRequest{
			Notifications: reqNotifications,
		},
	)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, reqNotifications.Channels, result.Preferences.Notifications.Channels)
	assert.Equal(t, customTypes, result.Preferences.Notifications.Types)

	mockRepo.AssertExpectations(t)
}

func TestUserPreferencesService_UpdatePreferences_InvalidEmailFrequencyRejected(t *testing.T) {
	// UpdatePreferences must reject notification types with an invalid Email frequency.
	mockRepo := new(MockUserPreferencesRepository)

	mockRepo.On("GetByUserID", mock.Anything, "user-invalid-freq").Return(nil, nil)

	service := NewUserPreferencesService(mockRepo)
	result, err := service.UpdatePreferences(
		context.Background(),
		"user-invalid-freq",
		models.UpdatePreferencesRequest{
			Notifications: &models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{InApp: true},
				Types: map[string]models.NotificationTypePreference{
					"feed.item.created": {InApp: true, Email: "weekly", WebPush: false},
				},
			},
		},
	)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid email frequency")
	assert.Contains(t, err.Error(), "weekly")
	assert.Contains(t, err.Error(), "feed.item.created")

	// Upsert must NOT be called when validation fails
	mockRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

func TestUserPreferencesService_UpdatePreferences_ValidEmailFrequenciesAccepted(t *testing.T) {
	// All three valid Email frequency values must be accepted without error.
	validFreqs := []string{"instant", "digest", "none"}

	for _, freq := range validFreqs {
		t.Run(freq, func(t *testing.T) {
			mockRepo := new(MockUserPreferencesRepository)

			mockRepo.On("GetByUserID", mock.Anything, "user-valid-freq").Return(nil, nil)
			mockRepo.On("Upsert", mock.Anything, mock.Anything).Return(nil)

			service := NewUserPreferencesService(mockRepo)
			result, err := service.UpdatePreferences(
				context.Background(),
				"user-valid-freq",
				models.UpdatePreferencesRequest{
					Notifications: &models.NotificationPreferences{
						Channels: models.NotificationChannelPreferences{InApp: true},
						Types: map[string]models.NotificationTypePreference{
							"feed.item.created": {InApp: true, Email: freq, WebPush: false},
						},
					},
				},
			)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestUserPreferencesService_UpdatePreferences_EmptyEmailFrequencyAccepted(t *testing.T) {
	// An empty Email string must be accepted (zero-value, no frequency set).
	mockRepo := new(MockUserPreferencesRepository)

	mockRepo.On("GetByUserID", mock.Anything, "user-empty-freq").Return(nil, nil)
	mockRepo.On("Upsert", mock.Anything, mock.Anything).Return(nil)

	service := NewUserPreferencesService(mockRepo)
	result, err := service.UpdatePreferences(
		context.Background(),
		"user-empty-freq",
		models.UpdatePreferencesRequest{
			Notifications: &models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{InApp: true},
				Types: map[string]models.NotificationTypePreference{
					"feed.item.created": {InApp: true, Email: "", WebPush: false},
				},
			},
		},
	)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	mockRepo.AssertExpectations(t)
}

func TestUserPreferencesService_UpdatePreferences_NilTypesDefaultsToDefaults(t *testing.T) {
	// When caller sends Notifications with nil Types, service must default Types
	// to DefaultNotificationPreferences().Types to prevent nil map access.
	mockRepo := new(MockUserPreferencesRepository)

	defaults := models.DefaultNotificationPreferences()

	mockRepo.On("GetByUserID", mock.Anything, "user-nil-types").Return(nil, nil)
	mockRepo.On("Upsert", mock.Anything, mock.MatchedBy(func(p *models.UserPreferences) bool {
		return p.Preferences.Notifications.Types != nil
	})).Return(nil)

	service := NewUserPreferencesService(mockRepo)
	result, err := service.UpdatePreferences(
		context.Background(),
		"user-nil-types",
		models.UpdatePreferencesRequest{
			Notifications: &models.NotificationPreferences{
				Channels: models.NotificationChannelPreferences{
					InApp:   true,
					Email:   true,
					WebPush: true,
				},
				Types: nil, // Caller sends nil — service must fill defaults
			},
		},
	)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Preferences.Notifications.Types)
	assert.Equal(t, defaults.Types, result.Preferences.Notifications.Types)

	mockRepo.AssertExpectations(t)
}

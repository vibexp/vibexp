package services

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/auth/idp"
	idpmocks "github.com/vibexp/vibexp/internal/auth/idp/mocks"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/feature_flags"
	"github.com/vibexp/vibexp/pkg/events"
	event_mocks "github.com/vibexp/vibexp/pkg/events/mocks"
)

// Test configuration constants
const testOAuthClientID = "test-client-id"

func createTestAuthServiceNew(
	userRepo *repo_mocks.MockUserRepository,
	identityProvider *idpmocks.MockIdentityProvider,
	allowedEmails []string,
) *AuthService {
	logger := func() *slog.Logger { l, _ := logtest.New(); return l }()

	featureFlagSvc := feature_flags.NewFeatureFlagService(logger)

	userSignInAllowlist := feature_flags.NewUserSignInAllowlistFlag(logger, allowedEmails)
	featureFlagSvc.RegisterFlag(userSignInAllowlist)

	return NewAuthService(userRepo, newTestRegistry(identityProvider), nil, logger, featureFlagSvc)
}

// newTestRegistry wraps a mock identity provider in a registry. It defaults the
// provider's Name() to WorkOS (the registry keys by Name() at build time) when
// the test has not already stubbed it.
func newTestRegistry(p *idpmocks.MockIdentityProvider) *idp.Registry {
	p.On("Name").Return(idp.ProviderWorkOS).Maybe()
	return idp.NewRegistry(p)
}

func createTestClaims() *idp.Claims {
	return &idp.Claims{
		Subject: "workos-sub-123",
		Email:   "test@example.com",
		Name:    "Test User",
		Picture: "https://example.com/avatar.jpg",
	}
}

func strPtr(s string) *string {
	return &s
}

func TestAuthService_GetUserByID_New(t *testing.T) {
	googleID := "google-123"
	testUser := &models.User{
		ID:                 "user-123",
		GoogleID:           &googleID,
		Email:              "test@example.com",
		Name:               "Test User",
		SubscriptionStatus: "free",
		SubscriptionPlan:   func() *string { s := "free"; return &s }(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	tests := []struct {
		name         string
		userID       string
		setupMocks   func(*repo_mocks.MockUserRepository)
		expectError  bool
		expectedUser *models.User
	}{
		{
			name:   "successful retrieval",
			userID: "user-123",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				mockRepo.On("GetByID", context.Background(), "user-123").Return(testUser, nil)
			},
			expectError:  false,
			expectedUser: testUser,
		},
		{
			name:   "user not found",
			userID: "user-456",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				mockRepo.On("GetByID", context.Background(), "user-456").Return(nil, assert.AnError)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &repo_mocks.MockUserRepository{}
			mockIDP := &idpmocks.MockIdentityProvider{}
			service := createTestAuthServiceNew(mockRepo, mockIDP, []string{"test@example.com"})
			tt.setupMocks(mockRepo)

			ctx := context.Background()
			user, err := service.GetUserByID(ctx, tt.userID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUser, user)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

//nolint:funlen,gocyclo // Test function requires comprehensive setup and assertions
func TestAuthService_CreateOrUpdateUserFromClaims(t *testing.T) {
	claims := createTestClaims()
	workosProvider := string(idp.ProviderWorkOS)
	existingUser := &models.User{
		ID:                 "user-123",
		GoogleID:           nil, // WorkOS users have no google_id
		Email:              "old@example.com",
		Name:               "Old Name",
		IDPProvider:        strPtr(workosProvider),
		IDPSubject:         strPtr("workos-sub-123"),
		SubscriptionStatus: "free",
		SubscriptionPlan:   func() *string { s := "free"; return &s }(),
		CreatedAt:          time.Now().Add(-time.Hour),
		UpdatedAt:          time.Now().Add(-time.Hour),
	}

	tests := []struct {
		name         string
		setupMocks   func(*repo_mocks.MockUserRepository)
		expectError  bool
		validateUser func(*testing.T, *models.User)
	}{
		{
			name: "create new WorkOS user when not found",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				mockRepo.On("GetByIDPSubject", context.Background(), workosProvider, "workos-sub-123").
					Return(nil, repositories.ErrUserNotFound)
				// WorkOS provider does NOT fall back to GetByGoogleID
				// Email-fallback for legacy Google-row migration: also returns "not found" here
				mockRepo.On("GetByEmail", context.Background(), "test@example.com").
					Return(nil, repositories.ErrUserNotFound)

				mockRepo.On("Create", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					return user.GoogleID == nil && // WorkOS: no google_id
						user.Email == "test@example.com" &&
						user.Name == "Test User" &&
						user.IDPProvider != nil && *user.IDPProvider == workosProvider &&
						user.IDPSubject != nil && *user.IDPSubject == "workos-sub-123"
				})).Return(nil).Run(func(args mock.Arguments) {
					user := args.Get(1).(*models.User)
					user.ID = "user-new-123"
					user.SubscriptionStatus = "free"
					user.SubscriptionPlan = func() *string { s := "free"; return &s }()
				})
			},
			expectError: false,
			validateUser: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-new-123", user.ID)
				assert.Nil(t, user.GoogleID, "WorkOS users should not have google_id")
				assert.Equal(t, "test@example.com", user.Email)
				assert.Equal(t, "Test User", user.Name)
				assert.NotNil(t, user.IDPProvider)
				assert.Equal(t, workosProvider, *user.IDPProvider)
			},
		},
		{
			name: "adopt legacy Google user via email match on first WorkOS sign-in",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				// IDP-tuple lookup misses (no row with idp_provider='workos' yet)
				mockRepo.On("GetByIDPSubject", context.Background(), workosProvider, "workos-sub-123").
					Return(nil, repositories.ErrUserNotFound)

				// Email fallback finds the legacy Google row
				googleStr := "google"
				googleSubStr := "google-sub-old"
				googleIDStr := "google-sub-old"
				legacy := &models.User{
					ID:          "user-legacy-1",
					Email:       "test@example.com",
					Name:        "Old Name",
					IDPProvider: &googleStr,
					IDPSubject:  &googleSubStr,
					GoogleID:    &googleIDStr,
				}
				mockRepo.On("GetByEmail", context.Background(), "test@example.com").
					Return(legacy, nil)

				// Update is called: idp_provider/idp_subject overwritten to WorkOS
				mockRepo.On("Update", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					return user.ID == "user-legacy-1" &&
						user.Email == "test@example.com" &&
						user.IDPProvider != nil && *user.IDPProvider == workosProvider &&
						user.IDPSubject != nil && *user.IDPSubject == "workos-sub-123"
				})).Return(nil)
			},
			expectError: false,
			validateUser: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-legacy-1", user.ID, "should adopt legacy row, not create new")
				assert.NotNil(t, user.IDPProvider)
				assert.Equal(t, workosProvider, *user.IDPProvider)
				assert.NotNil(t, user.IDPSubject)
				assert.Equal(t, "workos-sub-123", *user.IDPSubject)
			},
		},
		{
			name: "update user matched via IDP subject tuple",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				mockRepo.On("GetByIDPSubject", context.Background(), workosProvider, "workos-sub-123").
					Return(existingUser, nil)
				mockRepo.On("Update", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					return user.ID == "user-123" &&
						user.Email == "test@example.com" &&
						user.Name == "Test User" &&
						user.IDPProvider != nil && *user.IDPProvider == workosProvider
				})).Return(nil)
			},
			expectError: false,
			validateUser: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-123", user.ID)
				assert.Equal(t, "test@example.com", user.Email)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &repo_mocks.MockUserRepository{}
			mockIDP := &idpmocks.MockIdentityProvider{}
			mockIDP.On("Name").Return(idp.ProviderWorkOS)
			service := createTestAuthServiceNew(mockRepo, mockIDP, []string{"test@example.com"})
			tt.setupMocks(mockRepo)

			ctx := context.Background()
			user, _, err := service.createOrUpdateUserFromClaims(ctx, workosProvider, claims)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				if tt.validateUser != nil {
					tt.validateUser(t, user)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestAuthService_GetLoginURL(t *testing.T) {
	mockRepo := &repo_mocks.MockUserRepository{}
	mockIDP := &idpmocks.MockIdentityProvider{}
	service := createTestAuthServiceNew(mockRepo, mockIDP, []string{"test@example.com"})

	expectedURL := "https://sso.workos.com/authorize?state=test-state&client_id=" + testOAuthClientID
	// The registry keys the mock under "workos"; GetLoginURL resolves it and
	// forwards the provider name to AuthorizeURL. The redirect override is empty
	// so the provider uses its own configured redirect URI.
	mockIDP.On("AuthorizeURL", "test-state", "", "workos").Return(expectedURL)

	url := service.GetLoginURL("test-state", "workos")
	assert.NotEmpty(t, url)
	assert.Equal(t, expectedURL, url)

	mockIDP.AssertExpectations(t)
}

func TestAuthService_GetLoginURL_UnknownProvider_ReturnsEmpty(t *testing.T) {
	mockRepo := &repo_mocks.MockUserRepository{}
	mockIDP := &idpmocks.MockIdentityProvider{}
	service := createTestAuthServiceNew(mockRepo, mockIDP, []string{})

	// "github" is not in the registry (only "workos" is) → empty URL, and
	// AuthorizeURL is never called.
	got := service.GetLoginURL("some-state", "github")
	assert.Empty(t, got)
	mockIDP.AssertNotCalled(t, "AuthorizeURL", mock.Anything, mock.Anything, mock.Anything)
}

func TestAuthService_EnabledProviders(t *testing.T) {
	mockRepo := &repo_mocks.MockUserRepository{}
	mockIDP := &idpmocks.MockIdentityProvider{}
	service := createTestAuthServiceNew(mockRepo, mockIDP, []string{})

	assert.Equal(t, []string{"workos"}, service.EnabledProviders())
}

func TestAuthService_RefreshTokens(t *testing.T) {
	refreshed := &idp.Tokens{AccessToken: "new-access", RefreshToken: "new-refresh"}

	t.Run("routes to the named provider", func(t *testing.T) {
		mockRepo := &repo_mocks.MockUserRepository{}
		mockIDP := &idpmocks.MockIdentityProvider{}
		service := createTestAuthServiceNew(mockRepo, mockIDP, []string{})
		mockIDP.On("Refresh", context.Background(), "old-refresh").Return(refreshed, nil)

		got, err := service.RefreshTokens(context.Background(), "workos", "old-refresh")
		assert.NoError(t, err)
		assert.Equal(t, refreshed, got)
		mockIDP.AssertExpectations(t)
	})

	t.Run("empty provider falls back to the sole enabled provider", func(t *testing.T) {
		mockRepo := &repo_mocks.MockUserRepository{}
		mockIDP := &idpmocks.MockIdentityProvider{}
		service := createTestAuthServiceNew(mockRepo, mockIDP, []string{})
		mockIDP.On("Refresh", context.Background(), "legacy-refresh").Return(refreshed, nil)

		// Legacy session with no provider name: single-provider deployment routes to it.
		got, err := service.RefreshTokens(context.Background(), "", "legacy-refresh")
		assert.NoError(t, err)
		assert.Equal(t, refreshed, got)
		mockIDP.AssertExpectations(t)
	})

	t.Run("unknown provider errors", func(t *testing.T) {
		mockRepo := &repo_mocks.MockUserRepository{}
		mockIDP := &idpmocks.MockIdentityProvider{}
		service := createTestAuthServiceNew(mockRepo, mockIDP, []string{})

		_, err := service.RefreshTokens(context.Background(), "github", "tok")
		assert.Error(t, err)
		mockIDP.AssertNotCalled(t, "Refresh", mock.Anything, mock.Anything)
	})
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAuthService_HandleCallback(t *testing.T) {
	testTokens := &idp.Tokens{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	testClaims := createTestClaims()
	workosProvider := string(idp.ProviderWorkOS)

	tests := []struct {
		name           string
		code           string
		allowedEmails  []string
		setupMocks     func(*repo_mocks.MockUserRepository, *idpmocks.MockIdentityProvider)
		expectError    bool
		validateResult func(*testing.T, *models.User, *idp.Tokens)
	}{
		{
			name:          "successful callback with new user creation",
			code:          "test-auth-code",
			allowedEmails: []string{"test@example.com"},
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository, mockIDP *idpmocks.MockIdentityProvider) {
				mockIDP.On("Name").Return(idp.ProviderWorkOS)
				mockIDP.On("ExchangeCode", context.Background(), "test-auth-code", "").
					Return(testTokens, testClaims, nil)

				mockRepo.On("GetByIDPSubject", context.Background(), workosProvider, "workos-sub-123").
					Return(nil, repositories.ErrUserNotFound)
				// Email-fallback also misses for a brand-new user
				mockRepo.On("GetByEmail", context.Background(), "test@example.com").
					Return(nil, repositories.ErrUserNotFound)

				mockRepo.On("Create", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					return user.GoogleID == nil &&
						user.Email == "test@example.com" &&
						user.Name == "Test User"
				})).Return(nil).Run(func(args mock.Arguments) {
					user := args.Get(1).(*models.User)
					user.ID = "user-new-123"
					user.SubscriptionStatus = "free"
				})
			},
			expectError: false,
			validateResult: func(t *testing.T, user *models.User, tokens *idp.Tokens) {
				assert.Equal(t, "user-new-123", user.ID)
				assert.Nil(t, user.GoogleID)
				assert.NotNil(t, tokens)
				assert.Equal(t, "test-access-token", tokens.AccessToken)
			},
		},
		{
			name:          "successful callback with existing user update",
			code:          "test-auth-code",
			allowedEmails: []string{"test@example.com"},
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository, mockIDP *idpmocks.MockIdentityProvider) {
				existingUser := &models.User{
					ID:                 "user-existing-123",
					GoogleID:           nil,
					IDPProvider:        strPtr(workosProvider),
					IDPSubject:         strPtr("workos-sub-123"),
					Email:              "old@example.com",
					Name:               "Old Name",
					SubscriptionStatus: "premium",
					CreatedAt:          time.Now().Add(-time.Hour),
					UpdatedAt:          time.Now().Add(-time.Hour),
				}

				mockIDP.On("Name").Return(idp.ProviderWorkOS)
				mockIDP.On("ExchangeCode", context.Background(), "test-auth-code", "").
					Return(testTokens, testClaims, nil)

				mockRepo.On("GetByIDPSubject", context.Background(), workosProvider, "workos-sub-123").
					Return(existingUser, nil)

				mockRepo.On("Update", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					return user.ID == "user-existing-123" &&
						user.Email == "test@example.com"
				})).Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, user *models.User, tokens *idp.Tokens) {
				assert.Equal(t, "user-existing-123", user.ID)
				assert.NotNil(t, tokens)
			},
		},
		{
			name:          "OAuth token exchange failure",
			code:          "invalid-code",
			allowedEmails: []string{"test@example.com"},
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository, mockIDP *idpmocks.MockIdentityProvider) {
				mockIDP.On("ExchangeCode", context.Background(), "invalid-code", "").
					Return(nil, nil, fmt.Errorf("invalid authorization code"))
			},
			expectError: true,
		},
		{
			name:          "user creation failure",
			code:          "test-auth-code",
			allowedEmails: []string{"test@example.com"},
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository, mockIDP *idpmocks.MockIdentityProvider) {
				mockIDP.On("Name").Return(idp.ProviderWorkOS)
				mockIDP.On("ExchangeCode", context.Background(), "test-auth-code", "").
					Return(testTokens, testClaims, nil)

				mockRepo.On("GetByIDPSubject", context.Background(), workosProvider, "workos-sub-123").
					Return(nil, repositories.ErrUserNotFound)
				mockRepo.On("GetByEmail", context.Background(), "test@example.com").
					Return(nil, repositories.ErrUserNotFound)
				mockRepo.On("Create", context.Background(), mock.AnythingOfType("*models.User")).
					Return(fmt.Errorf("database error"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &repo_mocks.MockUserRepository{}
			mockIDP := &idpmocks.MockIdentityProvider{}
			service := createTestAuthServiceNew(mockRepo, mockIDP, tt.allowedEmails)

			tt.setupMocks(mockRepo, mockIDP)

			ctx := context.Background()
			user, tokens, _, err := service.HandleCallback(ctx, tt.code, string(idp.ProviderWorkOS))

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.NotNil(t, tokens)
				if tt.validateResult != nil {
					tt.validateResult(t, user, tokens)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAuthService_HandleDevLogin(t *testing.T) {
	tests := []struct {
		name           string
		email          string
		userName       string
		setupMocks     func(*repo_mocks.MockUserRepository)
		expectError    bool
		validateResult func(*testing.T, *models.User)
	}{
		{
			name:     "successful dev login with new user creation",
			email:    "dev@example.com",
			userName: "Dev User",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				mockRepo.On("GetByEmail", context.Background(), "dev@example.com").
					Return(nil, repositories.ErrUserNotFound)
				mockRepo.On("Create", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					// WorkOS dev users have no google_id
					return user.GoogleID == nil &&
						user.Email == "dev@example.com" &&
						user.Name == "Dev User"
				})).Return(nil).Run(func(args mock.Arguments) {
					user := args.Get(1).(*models.User)
					user.ID = "user-dev-123"
					user.SubscriptionStatus = "free"
				})
			},
			expectError: false,
			validateResult: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-dev-123", user.ID)
				assert.Nil(t, user.GoogleID, "Dev users (WorkOS) should not have google_id")
				assert.Equal(t, "dev@example.com", user.Email)
			},
		},
		{
			name:     "successful dev login with existing user",
			email:    "existing@example.com",
			userName: "Existing User",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				existingUser := &models.User{
					ID:                 "user-existing-456",
					GoogleID:           nil,
					Email:              "existing@example.com",
					Name:               "Existing User",
					SubscriptionStatus: "premium",
					CreatedAt:          time.Now().Add(-time.Hour),
					UpdatedAt:          time.Now().Add(-time.Hour),
				}
				mockRepo.On("GetByEmail", context.Background(), "existing@example.com").
					Return(existingUser, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-existing-456", user.ID)
			},
		},
		{
			name:     "failed to query user",
			email:    "error@example.com",
			userName: "Error User",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				mockRepo.On("GetByEmail", context.Background(), "error@example.com").
					Return(nil, fmt.Errorf("database connection error"))
			},
			expectError: true,
		},
		{
			name:     "failed to create new user",
			email:    "newuser@example.com",
			userName: "New User",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				mockRepo.On("GetByEmail", context.Background(), "newuser@example.com").
					Return(nil, repositories.ErrUserNotFound)
				mockRepo.On("Create", context.Background(), mock.AnythingOfType("*models.User")).
					Return(fmt.Errorf("failed to create user"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &repo_mocks.MockUserRepository{}
			mockIDP := &idpmocks.MockIdentityProvider{}
			service := createTestAuthServiceNew(mockRepo, mockIDP, []string{})

			tt.setupMocks(mockRepo)

			ctx := context.Background()
			user, err := service.HandleDevLogin(ctx, tt.email, tt.userName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				if tt.validateResult != nil {
					tt.validateResult(t, user)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestAuthService_PublishesUserCreatedEvent(t *testing.T) {
	testClaims := &idp.Claims{
		Subject: "workos-sub-123",
		Email:   "test@example.com",
		Name:    "Test User",
		Picture: "https://example.com/avatar.jpg",
	}
	workosProvider := string(idp.ProviderWorkOS)

	tests := []struct {
		name             string
		setupMocks       func(*repo_mocks.MockUserRepository, *event_mocks.MockEventPublisher)
		runScenario      string
		expectEventCalls int
	}{
		{
			name:        "publishes event when creating new user via OIDC callback",
			runScenario: "callback",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository, mockEventManager *event_mocks.MockEventPublisher) {
				mockRepo.On("GetByIDPSubject", mock.Anything, workosProvider, "workos-sub-123").
					Return(nil, repositories.ErrUserNotFound)
				mockRepo.On("GetByEmail", mock.Anything, "test@example.com").
					Return(nil, repositories.ErrUserNotFound)

				mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(user *models.User) bool {
					return user.GoogleID == nil && user.Email == "test@example.com"
				})).Return(nil).Run(func(args mock.Arguments) {
					user := args.Get(1).(*models.User)
					user.ID = "user-new-123"
				})

				mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
					return event.Type() == events.EventTypeUserCreated
				})).Return(nil).Once()
			},
			expectEventCalls: 1,
		},
		{
			name:        "publishes event when creating new user via dev login",
			runScenario: "dev",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository, mockEventManager *event_mocks.MockEventPublisher) {
				mockRepo.On("GetByEmail", mock.Anything, "dev@example.com").
					Return(nil, repositories.ErrUserNotFound)
				mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(user *models.User) bool {
					return user.Email == "dev@example.com"
				})).Return(nil).Run(func(args mock.Arguments) {
					user := args.Get(1).(*models.User)
					user.ID = "user-dev-123"
				})
				mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
					return event.Type() == events.EventTypeUserCreated
				})).Return(nil).Once()
			},
			expectEventCalls: 1,
		},
		{
			name:        "does not publish event when updating existing user",
			runScenario: "callback",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository, mockEventManager *event_mocks.MockEventPublisher) {
				existingUser := &models.User{
					ID:          "user-existing-123",
					GoogleID:    nil,
					IDPProvider: strPtr(workosProvider),
					IDPSubject:  strPtr("workos-sub-123"),
					Email:       "old@example.com",
					Name:        "Old Name",
				}
				mockRepo.On("GetByIDPSubject", mock.Anything, workosProvider, "workos-sub-123").
					Return(existingUser, nil)
				mockRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.User")).
					Return(nil)
			},
			expectEventCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &repo_mocks.MockUserRepository{}
			mockIDP := &idpmocks.MockIdentityProvider{}
			mockIDP.On("Name").Return(idp.ProviderWorkOS).Maybe()
			mockEventManager := &event_mocks.MockEventPublisher{}

			logger := func() *slog.Logger { l, _ := logtest.New(); return l }()

			featureFlagSvc := feature_flags.NewFeatureFlagService(logger)
			userSignInAllowlist := feature_flags.NewUserSignInAllowlistFlag(
				logger, []string{"test@example.com", "dev@example.com"})
			featureFlagSvc.RegisterFlag(userSignInAllowlist)

			service := NewAuthService(mockRepo, idp.NewRegistry(mockIDP), mockEventManager, logger, featureFlagSvc)

			tt.setupMocks(mockRepo, mockEventManager)

			ctx := context.Background()

			if tt.runScenario == "dev" {
				_, err := service.HandleDevLogin(ctx, "dev@example.com", "Dev User")
				assert.NoError(t, err)
			} else {
				_, _, err := service.createOrUpdateUserFromClaims(ctx, workosProvider, testClaims)
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
			mockEventManager.AssertExpectations(t)

			if tt.expectEventCalls > 0 {
				mockEventManager.AssertNumberOfCalls(t, "Publish", tt.expectEventCalls)
			} else {
				mockEventManager.AssertNotCalled(t, "Publish")
			}
		})
	}
}

// TestAuthService_GoogleLegacyFallback verifies the legacy google_id lookup still
// works for Google-signed-in users (backwards compatibility test).
func TestAuthService_GoogleLegacyFallback(t *testing.T) {
	googleProvider := string(idp.ProviderGoogle)
	testClaims := &idp.Claims{
		Subject: "google-123",
		Email:   "google@example.com",
		Name:    "Google User",
	}

	mockRepo := &repo_mocks.MockUserRepository{}
	mockIDP := &idpmocks.MockIdentityProvider{}
	mockIDP.On("Name").Return(idp.ProviderGoogle)

	// Not found by IDP subject → fall back to google_id
	mockRepo.On("GetByIDPSubject", mock.Anything, googleProvider, "google-123").
		Return(nil, repositories.ErrUserNotFound)

	legacyGoogleID := "google-123"
	legacyUser := &models.User{
		ID:       "legacy-1",
		GoogleID: &legacyGoogleID,
		Email:    "legacy@example.com",
	}
	mockRepo.On("GetByGoogleID", mock.Anything, "google-123").Return(legacyUser, nil)
	mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
		return u.ID == "legacy-1"
	})).Return(nil)

	logger := func() *slog.Logger { l, _ := logtest.New(); return l }()
	featureFlagSvc := feature_flags.NewFeatureFlagService(logger)

	service := NewAuthService(mockRepo, idp.NewRegistry(mockIDP), nil, logger, featureFlagSvc)

	user, _, err := service.createOrUpdateUserFromClaims(context.Background(), googleProvider, testClaims)
	assert.NoError(t, err)
	assert.Equal(t, "legacy-1", user.ID)

	mockRepo.AssertExpectations(t)
}

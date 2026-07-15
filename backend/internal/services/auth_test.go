package services

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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

	userSignInAllowlist := feature_flags.NewUserSignInAllowlistFlag(logger, nil, allowedEmails)
	featureFlagSvc.RegisterFlag(userSignInAllowlist)

	return NewAuthService(userRepo, newTestRegistry(identityProvider), nil, logger, featureFlagSvc)
}

// newTestRegistry wraps a mock identity provider in a registry. It defaults the
// provider's Name() to the generic OIDC provider (the registry keys by Name()
// at build time) when the test has not already stubbed it.
func newTestRegistry(p *idpmocks.MockIdentityProvider) *idp.Registry {
	p.On("Name").Return(idp.ProviderOIDC).Maybe()
	return idp.NewRegistry(p)
}

// createTestClaims builds the claims a real identity provider returns. Email is
// EmailVerified because every enabled provider (Google/GitHub/generic OIDC)
// verifies it provider-side — tests that care about the unverified case set the
// field explicitly (see TestAuthService_HandleCallback_EmailVerifiedWithAllowlist).
func createTestClaims() *idp.Claims {
	return &idp.Claims{
		Subject:       "oidc-sub-123",
		Email:         "test@example.com",
		EmailVerified: true,
		Name:          "Test User",
		Picture:       "https://example.com/avatar.jpg",
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
	testProvider := string(idp.ProviderOIDC)
	existingUser := &models.User{
		ID:                 "user-123",
		GoogleID:           nil, // non-Google users have no google_id
		Email:              "old@example.com",
		Name:               "Old Name",
		IDPProvider:        strPtr(testProvider),
		IDPSubject:         strPtr("oidc-sub-123"),
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
			name: "create new user when not found",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				mockRepo.On("GetByIDPSubject", context.Background(), testProvider, "oidc-sub-123").
					Return(nil, repositories.ErrUserNotFound)
				// A non-Google provider does NOT fall back to GetByGoogleID.

				mockRepo.On("Create", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					return user.GoogleID == nil && // non-Google: no google_id
						user.Email == "test@example.com" &&
						user.Name == "Test User" &&
						user.IDPProvider != nil && *user.IDPProvider == testProvider &&
						user.IDPSubject != nil && *user.IDPSubject == "oidc-sub-123"
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
				assert.Nil(t, user.GoogleID, "non-Google users should not have google_id")
				assert.Equal(t, "test@example.com", user.Email)
				assert.Equal(t, "Test User", user.Name)
				assert.NotNil(t, user.IDPProvider)
				assert.Equal(t, testProvider, *user.IDPProvider)
			},
		},
		{
			name: "update user matched via IDP subject tuple",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				mockRepo.On("GetByIDPSubject", context.Background(), testProvider, "oidc-sub-123").
					Return(existingUser, nil)
				mockRepo.On("Update", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					return user.ID == "user-123" &&
						user.Email == "test@example.com" &&
						user.Name == "Test User" &&
						user.IDPProvider != nil && *user.IDPProvider == testProvider
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
			mockIDP.On("Name").Return(idp.ProviderOIDC)
			service := createTestAuthServiceNew(mockRepo, mockIDP, []string{"test@example.com"})
			tt.setupMocks(mockRepo)

			ctx := context.Background()
			user, _, err := service.createOrUpdateUserFromClaims(ctx, testProvider, claims)

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

	expectedURL := "https://idp.example.com/authorize?state=test-state&client_id=" + testOAuthClientID
	// The registry keys the mock under "oidc"; GetLoginURL resolves it and
	// forwards the provider name to AuthorizeURL. The redirect override is empty
	// so the provider uses its own configured redirect URI.
	mockIDP.On("AuthorizeURL", "test-state", "", "oidc").Return(expectedURL)

	url := service.GetLoginURL("test-state", "oidc")
	assert.NotEmpty(t, url)
	assert.Equal(t, expectedURL, url)

	mockIDP.AssertExpectations(t)
}

func TestAuthService_GetLoginURL_UnknownProvider_ReturnsEmpty(t *testing.T) {
	mockRepo := &repo_mocks.MockUserRepository{}
	mockIDP := &idpmocks.MockIdentityProvider{}
	service := createTestAuthServiceNew(mockRepo, mockIDP, []string{})

	// "github" is not in the registry (only "oidc" is) → empty URL, and
	// AuthorizeURL is never called.
	got := service.GetLoginURL("some-state", "github")
	assert.Empty(t, got)
	mockIDP.AssertNotCalled(t, "AuthorizeURL", mock.Anything, mock.Anything, mock.Anything)
}

func TestAuthService_EnabledProviders(t *testing.T) {
	mockRepo := &repo_mocks.MockUserRepository{}
	mockIDP := &idpmocks.MockIdentityProvider{}
	service := createTestAuthServiceNew(mockRepo, mockIDP, []string{})

	assert.Equal(t, []string{"oidc"}, service.EnabledProviders())
}

func TestAuthService_RefreshTokens(t *testing.T) {
	refreshed := &idp.Tokens{AccessToken: "new-access", RefreshToken: "new-refresh"}

	t.Run("routes to the named provider", func(t *testing.T) {
		mockRepo := &repo_mocks.MockUserRepository{}
		mockIDP := &idpmocks.MockIdentityProvider{}
		service := createTestAuthServiceNew(mockRepo, mockIDP, []string{})
		mockIDP.On("Refresh", context.Background(), "old-refresh").Return(refreshed, nil)

		got, err := service.RefreshTokens(context.Background(), "oidc", "old-refresh")
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
	testProvider := string(idp.ProviderOIDC)

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
				mockIDP.On("Name").Return(idp.ProviderOIDC)
				mockIDP.On("ExchangeCode", context.Background(), "test-auth-code", "").
					Return(testTokens, testClaims, nil)

				mockRepo.On("GetByIDPSubject", context.Background(), testProvider, "oidc-sub-123").
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
					IDPProvider:        strPtr(testProvider),
					IDPSubject:         strPtr("oidc-sub-123"),
					Email:              "old@example.com",
					Name:               "Old Name",
					SubscriptionStatus: "premium",
					CreatedAt:          time.Now().Add(-time.Hour),
					UpdatedAt:          time.Now().Add(-time.Hour),
				}

				mockIDP.On("Name").Return(idp.ProviderOIDC)
				mockIDP.On("ExchangeCode", context.Background(), "test-auth-code", "").
					Return(testTokens, testClaims, nil)

				mockRepo.On("GetByIDPSubject", context.Background(), testProvider, "oidc-sub-123").
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
				mockIDP.On("Name").Return(idp.ProviderOIDC)
				mockIDP.On("ExchangeCode", context.Background(), "test-auth-code", "").
					Return(testTokens, testClaims, nil)

				mockRepo.On("GetByIDPSubject", context.Background(), testProvider, "oidc-sub-123").
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
			user, tokens, _, err := service.HandleCallback(ctx, tt.code, string(idp.ProviderOIDC))

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

// TestAuthService_HandleCallback_AccessRestricted verifies the security gate:
// when the authenticated email is not on the allowlist, HandleCallback returns
// ErrAccessRestricted AFTER exchanging the code but BEFORE any user lookup or
// creation — leaving zero database residue (and therefore no user.created event).
func TestAuthService_HandleCallback_AccessRestricted(t *testing.T) {
	mockRepo := &repo_mocks.MockUserRepository{}
	mockIDP := &idpmocks.MockIdentityProvider{}
	// Allowlist that does NOT include the claims email ("test@example.com").
	service := createTestAuthServiceNew(mockRepo, mockIDP, []string{"allowed@example.com"})

	testTokens := &idp.Tokens{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	mockIDP.On("Name").Return(idp.ProviderOIDC)
	mockIDP.On("ExchangeCode", context.Background(), "test-auth-code", "").
		Return(testTokens, createTestClaims(), nil)

	user, tokens, isNewUser, err := service.HandleCallback(
		context.Background(), "test-auth-code", string(idp.ProviderOIDC))

	assert.ErrorIs(t, err, ErrAccessRestricted)
	assert.Nil(t, user)
	assert.Nil(t, tokens)
	assert.False(t, isNewUser)
	// No user row was touched: the repo was never asked to look up or create.
	mockRepo.AssertNotCalled(t, "GetByIDPSubject", mock.Anything, mock.Anything, mock.Anything)
	mockRepo.AssertNotCalled(t, "GetByGoogleID", mock.Anything, mock.Anything)
	mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

// TestAuthService_HandleCallback_EmailVerifiedWithAllowlist covers #218: when an
// access allowlist is active, the allowlisted address must also be one the
// identity provider VERIFIED. Otherwise a provider that relays an unverified
// claim (generic OIDC) lets an attacker assert an allowlisted address and sign in.
func TestAuthService_HandleCallback_EmailVerifiedWithAllowlist(t *testing.T) {
	tests := []struct {
		name          string
		allowedEmails []string
		emailVerified bool
		expectDenied  bool
		rationale     string
	}{
		{
			name:          "allowlist active, allowlisted email, unverified",
			allowedEmails: []string{"test@example.com"},
			emailVerified: false,
			expectDenied:  true,
			rationale:     "an unverified claim to an allowlisted address must not sign in",
		},
		{
			name:          "allowlist active, allowlisted email, verified",
			allowedEmails: []string{"test@example.com"},
			emailVerified: true,
			expectDenied:  false,
			rationale:     "a verified, allowlisted address signs in normally",
		},
		{
			name:          "no allowlist, unverified",
			allowedEmails: nil,
			emailVerified: false,
			expectDenied:  false,
			rationale:     "open instances must behave exactly as before #218",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &repo_mocks.MockUserRepository{}
			mockIDP := &idpmocks.MockIdentityProvider{}
			service := createTestAuthServiceNew(mockRepo, mockIDP, tt.allowedEmails)

			claims := createTestClaims()
			claims.EmailVerified = tt.emailVerified

			testTokens := &idp.Tokens{
				AccessToken:  "test-access-token",
				RefreshToken: "test-refresh-token",
				ExpiresAt:    time.Now().Add(time.Hour),
			}
			mockIDP.On("Name").Return(idp.ProviderOIDC)
			mockIDP.On("ExchangeCode", context.Background(), "test-auth-code", "").
				Return(testTokens, claims, nil)

			if !tt.expectDenied {
				existing := &models.User{ID: "user-1", Email: claims.Email, Name: claims.Name}
				mockRepo.On("GetByIDPSubject", context.Background(), string(idp.ProviderOIDC), claims.Subject).
					Return(existing, nil)
				mockRepo.On("Update", context.Background(), mock.Anything).Return(nil)
			}

			user, _, _, err := service.HandleCallback(
				context.Background(), "test-auth-code", string(idp.ProviderOIDC))

			if tt.expectDenied {
				assert.ErrorIs(t, err, ErrAccessRestricted, tt.rationale)
				assert.Nil(t, user)
				// Denied before any row is touched: no residue, no user.created event.
				mockRepo.AssertNotCalled(t, "GetByIDPSubject", mock.Anything, mock.Anything, mock.Anything)
				mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			} else {
				require.NoError(t, err, tt.rationale)
				assert.NotNil(t, user)
			}
			mockRepo.AssertExpectations(t)
		})
	}
}

// TestAuthService_HandleDevLogin_AccessRestricted verifies the same gate on the
// dev-login path: a non-allowlisted email is denied before any user lookup.
func TestAuthService_HandleDevLogin_AccessRestricted(t *testing.T) {
	mockRepo := &repo_mocks.MockUserRepository{}
	mockIDP := &idpmocks.MockIdentityProvider{}
	service := createTestAuthServiceNew(mockRepo, mockIDP, []string{"allowed@example.com"})

	user, err := service.HandleDevLogin(context.Background(), "test@example.com", "Test User")

	assert.ErrorIs(t, err, ErrAccessRestricted)
	assert.Nil(t, user)
	mockRepo.AssertNotCalled(t, "GetByEmail", mock.Anything, mock.Anything)
	mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

// TestAuthService_HandleDevLogin_AllowlistedEmailIsNotBlockedByVerification pins
// that #218's unverified-email rule does NOT reach dev login: it has no identity
// provider and so no verification concept, so an allowlisted address must still
// sign in. Without this, treating dev login as unverified would break every local
// dev loop on an allowlisted instance.
func TestAuthService_HandleDevLogin_AllowlistedEmailIsNotBlockedByVerification(t *testing.T) {
	mockRepo := &repo_mocks.MockUserRepository{}
	mockIDP := &idpmocks.MockIdentityProvider{}
	service := createTestAuthServiceNew(mockRepo, mockIDP, []string{"dev@example.com"})

	existing := &models.User{ID: "user-1", Email: "dev@example.com", Name: "Dev User"}
	mockRepo.On("GetByEmail", context.Background(), "dev@example.com").Return(existing, nil)

	user, err := service.HandleDevLogin(context.Background(), "dev@example.com", "Dev User")

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "dev@example.com", user.Email)
	mockRepo.AssertExpectations(t)
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
					// dev users have no google_id
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
				assert.Nil(t, user.GoogleID, "dev users should not have google_id")
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
		Subject: "oidc-sub-123",
		Email:   "test@example.com",
		Name:    "Test User",
		Picture: "https://example.com/avatar.jpg",
	}
	testProvider := string(idp.ProviderOIDC)

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
				mockRepo.On("GetByIDPSubject", mock.Anything, testProvider, "oidc-sub-123").
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
					IDPProvider: strPtr(testProvider),
					IDPSubject:  strPtr("oidc-sub-123"),
					Email:       "old@example.com",
					Name:        "Old Name",
				}
				mockRepo.On("GetByIDPSubject", mock.Anything, testProvider, "oidc-sub-123").
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
			mockIDP.On("Name").Return(idp.ProviderOIDC).Maybe()
			mockEventManager := &event_mocks.MockEventPublisher{}

			logger := func() *slog.Logger { l, _ := logtest.New(); return l }()

			featureFlagSvc := feature_flags.NewFeatureFlagService(logger)
			userSignInAllowlist := feature_flags.NewUserSignInAllowlistFlag(
				logger, nil, []string{"test@example.com", "dev@example.com"})
			featureFlagSvc.RegisterFlag(userSignInAllowlist)

			service := NewAuthService(mockRepo, idp.NewRegistry(mockIDP), mockEventManager, logger, featureFlagSvc)

			tt.setupMocks(mockRepo, mockEventManager)

			ctx := context.Background()

			if tt.runScenario == "dev" {
				_, err := service.HandleDevLogin(ctx, "dev@example.com", "Dev User")
				assert.NoError(t, err)
			} else {
				_, _, err := service.createOrUpdateUserFromClaims(ctx, testProvider, testClaims)
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

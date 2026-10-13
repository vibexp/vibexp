package services

import (
	"context"
	"errors"
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
	service, _ := createTestAuthServiceWithRecorder(userRepo, identityProvider, allowedEmails)
	return service
}

// createTestAuthServiceWithRecorder is createTestAuthServiceNew plus the log
// recorder, for tests asserting the audit trail — an allowlist denial looks
// identical to the user, so the log is the operator's only diagnostic.
func createTestAuthServiceWithRecorder(
	userRepo *repo_mocks.MockUserRepository,
	identityProvider *idpmocks.MockIdentityProvider,
	allowedEmails []string,
) (*AuthService, *logtest.Recorder) {
	logger, recorder := logtest.New()

	allowlist := newStaticAllowlistResolver(logger, allowedEmails)

	service := NewAuthService(userRepo, permissiveIdentityRepo(), newTestRegistry(identityProvider), nil, logger, allowlist)
	return service, recorder
}

// permissiveIdentityRepo is an identity repository that accepts any Upsert, for
// tests that are not about the identity a sign-in records.
func permissiveIdentityRepo() *repo_mocks.MockUserIdentityRepository {
	identities := &repo_mocks.MockUserIdentityRepository{}
	identities.On("Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	return identities
}

// denialReason returns the `reason` attribute of the sign-in-denied audit log.
func denialReason(t *testing.T, recorder *logtest.Recorder) string {
	t.Helper()
	for _, entry := range recorder.AllEntries() {
		if entry.Message == "Sign-in denied by access allowlist" {
			reason, ok := entry.Data["reason"].(string)
			require.True(t, ok, "the denial log must carry a string `reason` attribute")
			return reason
		}
	}
	t.Fatal("no sign-in denial was logged")
	return ""
}

// newTestRegistry wraps a mock identity provider in a registry. It defaults the
// provider's Name() to the generic OIDC provider (the registry keys by Name()
// at build time) when the test has not already stubbed it.
func newTestRegistry(p *idpmocks.MockIdentityProvider) IdentityProviderResolver {
	p.On("Name").Return(idp.ProviderOIDC).Maybe()
	return staticIDPResolver{reg: idp.NewRegistry(p)}
}

// staticIDPResolver is an IdentityProviderResolver serving a fixed registry (or
// a fixed error). The services tests cannot import services/mocks (cycle).
type staticIDPResolver struct {
	reg    *idp.Registry
	err    error
	health map[string]ProviderHealth
}

func (s staticIDPResolver) Snapshot(context.Context) (*idp.Registry, error) { return s.reg, s.err }

func (s staticIDPResolver) Provider(_ context.Context, slug string) (idp.IdentityProvider, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	p, ok := s.reg.Get(idp.ProviderName(slug))
	return p, ok, nil
}

func (s staticIDPResolver) Health() map[string]ProviderHealth { return s.health }

func (s staticIDPResolver) TestProvider(context.Context, models.InstanceAuthProvider, *string) error {
	return nil
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
		ID:        "user-123",
		GoogleID:  &googleID,
		Email:     "test@example.com",
		Name:      "Test User",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
		ID:          "user-123",
		GoogleID:    nil, // non-Google users have no google_id
		Email:       "old@example.com",
		Name:        "Old Name",
		IDPProvider: strPtr(testProvider),
		IDPSubject:  strPtr("oidc-sub-123"),
		CreatedAt:   time.Now().Add(-time.Hour),
		UpdatedAt:   time.Now().Add(-time.Hour),
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
				// A non-Google provider does NOT fall back to the legacy Google
				// identity: AssertExpectations fails on the unexpected lookup.

				mockRepo.On("Create", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					return user.GoogleID == nil && // non-Google: no google_id
						user.Email == "test@example.com" &&
						user.Name == "Test User" &&
						user.IDPProvider != nil && *user.IDPProvider == testProvider &&
						user.IDPSubject != nil && *user.IDPSubject == "oidc-sub-123"
				})).Return(nil).Run(func(args mock.Arguments) {
					user := args.Get(1).(*models.User)
					user.ID = "user-new-123"
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
			user, _, err := service.createOrUpdateUserFromClaims(ctx, testProvider, idp.ProviderName(testProvider), claims)

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

	url, err := service.GetLoginURL(context.Background(), "test-state", "oidc")
	require.NoError(t, err)
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
	got, err := service.GetLoginURL(context.Background(), "some-state", "github")
	require.NoError(t, err)
	assert.Empty(t, got)
	mockIDP.AssertNotCalled(t, "AuthorizeURL", mock.Anything, mock.Anything, mock.Anything)
}

func TestAuthService_EnabledProviders(t *testing.T) {
	mockRepo := &repo_mocks.MockUserRepository{}
	mockIDP := &idpmocks.MockIdentityProvider{}
	service := createTestAuthServiceNew(mockRepo, mockIDP, []string{})

	got, err := service.EnabledProviders(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []ProviderInfo{{Slug: "oidc", DisplayName: "Single Sign-On", Type: "oidc"}}, got)
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
					ID:          "user-existing-123",
					GoogleID:    nil,
					IDPProvider: strPtr(testProvider),
					IDPSubject:  strPtr("oidc-sub-123"),
					Email:       "old@example.com",
					Name:        "Old Name",
					CreatedAt:   time.Now().Add(-time.Hour),
					UpdatedAt:   time.Now().Add(-time.Hour),
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

// TestAuthService_HandleCallback_DenialLogsDistinctReason covers #218's audit
// requirement. Both denials look identical to the user (?error=access_restricted),
// so this log line is the ONLY way an operator can tell "not on the list" from
// "the provider never verified this address" — which is exactly the diagnostic
// needed if a generic OIDC provider omits email_verified and locks an instance out.
func TestAuthService_HandleCallback_DenialLogsDistinctReason(t *testing.T) {
	tests := []struct {
		name           string
		claimsEmail    string
		emailVerified  bool
		expectedReason string
	}{
		{
			name:           "unverified email on the allowlist",
			claimsEmail:    "test@example.com",
			emailVerified:  false,
			expectedReason: "unverified_email",
		},
		{
			name:           "verified email not on the allowlist",
			claimsEmail:    "outsider@elsewhere.test",
			emailVerified:  true,
			expectedReason: "not_on_allowlist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &repo_mocks.MockUserRepository{}
			mockIDP := &idpmocks.MockIdentityProvider{}
			service, recorder := createTestAuthServiceWithRecorder(
				mockRepo, mockIDP, []string{"test@example.com"})

			claims := createTestClaims()
			claims.Email = tt.claimsEmail
			claims.EmailVerified = tt.emailVerified

			mockIDP.On("Name").Return(idp.ProviderOIDC)
			mockIDP.On("ExchangeCode", context.Background(), "test-auth-code", "").
				Return(&idp.Tokens{AccessToken: "test-access-token"}, claims, nil)

			_, _, _, err := service.HandleCallback(
				context.Background(), "test-auth-code", string(idp.ProviderOIDC))

			require.ErrorIs(t, err, ErrAccessRestricted)
			assert.Equal(t, tt.expectedReason, denialReason(t, recorder))
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
					ID:        "user-existing-456",
					GoogleID:  nil,
					Email:     "existing@example.com",
					Name:      "Existing User",
					CreatedAt: time.Now().Add(-time.Hour),
					UpdatedAt: time.Now().Add(-time.Hour),
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

			allowlist := newStaticAllowlistResolver(logger, []string{"test@example.com", "dev@example.com"})

			service := NewAuthService(mockRepo, permissiveIdentityRepo(), staticIDPResolver{reg: idp.NewRegistry(mockIDP)}, mockEventManager, logger, allowlist)

			tt.setupMocks(mockRepo, mockEventManager)

			ctx := context.Background()

			if tt.runScenario == "dev" {
				_, err := service.HandleDevLogin(ctx, "dev@example.com", "Dev User")
				assert.NoError(t, err)
			} else {
				_, _, err := service.createOrUpdateUserFromClaims(ctx, testProvider, idp.ProviderName(testProvider), testClaims)
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
	mockRepo.On("GetByIDPSubject", mock.Anything, models.LegacyGoogleIdentityProvider, "google-123").
		Return(legacyUser, nil)
	mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
		return u.ID == "legacy-1"
	})).Return(nil)

	logger := func() *slog.Logger { l, _ := logtest.New(); return l }()
	service := NewAuthService(mockRepo, permissiveIdentityRepo(), staticIDPResolver{reg: idp.NewRegistry(mockIDP)}, nil, logger,
		newStaticAllowlistResolver(logger, nil))

	user, _, err := service.createOrUpdateUserFromClaims(context.Background(), googleProvider, idp.ProviderGoogle, testClaims)
	assert.NoError(t, err)
	assert.Equal(t, "legacy-1", user.ID)

	mockRepo.AssertExpectations(t)
}

// slugProvider returns a mock identity provider named slug.
func slugProvider(slug string) *idpmocks.MockIdentityProvider {
	p := &idpmocks.MockIdentityProvider{}
	p.On("Name").Return(idp.ProviderName(slug)).Maybe()
	return p
}

// newResolverAuthService builds an AuthService with open access over resolver.
func newResolverAuthService(userRepo *repo_mocks.MockUserRepository, resolver IdentityProviderResolver) *AuthService {
	logger := func() *slog.Logger { l, _ := logtest.New(); return l }()
	return NewAuthService(userRepo, permissiveIdentityRepo(), resolver, nil, logger, newStaticAllowlistResolver(logger, nil))
}

func TestAuthService_ResolverFailureIsUnresolvable(t *testing.T) {
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{}, staticIDPResolver{err: errors.New("db down")})
	ctx := context.Background()

	_, err := svc.EnabledProviders(ctx)
	assert.ErrorIs(t, err, ErrIdentityProvidersUnresolvable)
	_, err = svc.GetLoginURL(ctx, "s", "corp-sso")
	assert.ErrorIs(t, err, ErrIdentityProvidersUnresolvable)
	_, _, _, err = svc.HandleCallback(ctx, "code", "corp-sso")
	assert.ErrorIs(t, err, ErrIdentityProvidersUnresolvable)
	_, err = svc.RefreshTokens(ctx, "corp-sso", "tok")
	assert.ErrorIs(t, err, ErrIdentityProvidersUnresolvable)
}

func TestAuthService_UnknownSlugIsUnavailable(t *testing.T) {
	p := slugProvider("corp-sso")
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{}, staticIDPResolver{reg: idp.NewRegistry(p)})
	ctx := context.Background()

	_, _, _, err := svc.HandleCallback(ctx, "code", "partner-sso")
	assert.ErrorIs(t, err, ErrIdentityProviderUnavailable)
	_, err = svc.RefreshTokens(ctx, "partner-sso", "tok")
	assert.ErrorIs(t, err, ErrIdentityProviderUnavailable)
	p.AssertNotCalled(t, "ExchangeCode", mock.Anything, mock.Anything, mock.Anything)
}

// TestAuthService_HandleCallback_PersistsSlug covers multi-OIDC identity: the
// user row is keyed by the provider SLUG, and an OIDC provider never takes the
// Google-only google_id fallback even when an admin slugged it "google".
func TestAuthService_HandleCallback_PersistsSlug(t *testing.T) {
	claims := &idp.Claims{Subject: "sub-1", Email: "u@example.com", EmailVerified: true, Name: "U"}
	for _, slug := range []string{"corp-sso", "google"} {
		t.Run(slug, func(t *testing.T) {
			p := slugProvider(slug)
			p.On("ExchangeCode", mock.Anything, "code", "").Return(&idp.Tokens{AccessToken: "a"}, claims, nil)
			repo := &repo_mocks.MockUserRepository{}
			repo.On("GetByIDPSubject", mock.Anything, slug, "sub-1").Return(nil, repositories.ErrUserNotFound)
			repo.On("Create", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
				return u.IDPProvider != nil && *u.IDPProvider == slug && u.GoogleID == nil
			})).Return(nil)
			reg := idp.NewRegistryFromEntries(idp.Entry{Provider: p, Type: idp.ProviderOIDC, DisplayName: "SSO"})
			svc := newResolverAuthService(repo, staticIDPResolver{reg: reg})

			user, _, isNew, err := svc.HandleCallback(context.Background(), "code", slug)
			require.NoError(t, err)
			assert.True(t, isNew)
			assert.Equal(t, slug, *user.IDPProvider)
			repo.AssertNotCalled(t, "GetByIDPSubject", mock.Anything, models.LegacyGoogleIdentityProvider, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}

// TestAuthService_HandleCallback_GoogleTypeKeepsLegacyFallback pins that a
// google-TYPE provider still matches a legacy google_id user and stamps new
// users with google_id, whatever its slug.
func TestAuthService_HandleCallback_GoogleTypeKeepsLegacyFallback(t *testing.T) {
	claims := &idp.Claims{Subject: "g-1", Email: "g@example.com", EmailVerified: true}
	p := slugProvider("google-workspace")
	p.On("ExchangeCode", mock.Anything, "code", "").Return(&idp.Tokens{AccessToken: "a"}, claims, nil)
	repo := &repo_mocks.MockUserRepository{}
	repo.On("GetByIDPSubject", mock.Anything, "google-workspace", "g-1").Return(nil, repositories.ErrUserNotFound)
	repo.On("GetByIDPSubject", mock.Anything, models.LegacyGoogleIdentityProvider, "g-1").
		Return(nil, repositories.ErrUserNotFound)
	repo.On("Create", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
		return u.GoogleID != nil && *u.GoogleID == "g-1" && *u.IDPProvider == "google-workspace"
	})).Return(nil)
	reg := idp.NewRegistryFromEntries(idp.Entry{Provider: p, Type: idp.ProviderGoogle, DisplayName: "Google"})
	svc := newResolverAuthService(repo, staticIDPResolver{reg: reg})

	_, _, _, err := svc.HandleCallback(context.Background(), "code", "google-workspace")
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestAuthService_RefreshTokens_EmptySlugWithSeveralProvidersIsUnavailable(t *testing.T) {
	reg := idp.NewRegistry(slugProvider("corp-sso"), slugProvider("partner-sso"))
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{}, staticIDPResolver{reg: reg})
	_, err := svc.RefreshTokens(context.Background(), "", "tok")
	assert.ErrorIs(t, err, ErrIdentityProviderUnavailable, "no silent pick among several providers")
}

func TestAuthService_RefreshTokens_UnhealthyProviderIsTemporarilyUnavailable(t *testing.T) {
	reg := idp.NewRegistry(slugProvider("github"))
	health := map[string]ProviderHealth{"corp-sso": {Healthy: false, LastError: "discovery timed out"}}
	svc := newResolverAuthService(&repo_mocks.MockUserRepository{}, staticIDPResolver{reg: reg, health: health})

	_, err := svc.RefreshTokens(context.Background(), "corp-sso", "tok")
	assert.ErrorIs(t, err, ErrIdentityProviderTemporarilyUnavailable, "enabled but unhealthy is transient")
	assert.NotErrorIs(t, err, ErrIdentityProviderUnavailable)

	_, err = svc.RefreshTokens(context.Background(), "partner-sso", "tok")
	assert.ErrorIs(t, err, ErrIdentityProviderUnavailable, "not enabled at all is permanent")
}

// identityAuthService builds an open-access AuthService over a strict identity
// repository, for tests about the identity a sign-in resolves and records.
func identityAuthService(
	t *testing.T, users *repo_mocks.MockUserRepository, identities *repo_mocks.MockUserIdentityRepository,
) *AuthService {
	t.Helper()
	logger := func() *slog.Logger { l, _ := logtest.New(); return l }()
	return NewAuthService(users, identities, staticIDPResolver{reg: idp.NewRegistry()}, nil, logger,
		newStaticAllowlistResolver(logger, nil))
}

// A returning user is resolved by identity and the sign-in is recorded against
// that identity (#1427).
func TestAuthService_SignIn_RecordsTheIdentityUsed(t *testing.T) {
	ctx := context.Background()
	claims := &idp.Claims{Subject: "sub-1", Email: "u@example.com", Name: "U", EmailVerified: true}
	users := &repo_mocks.MockUserRepository{}
	identities := &repo_mocks.MockUserIdentityRepository{}
	users.On("GetByIDPSubject", ctx, "corp-sso", "sub-1").
		Return(&models.User{ID: "user-1", Email: "u@example.com"}, nil)
	users.On("Update", ctx, mock.MatchedBy(func(u *models.User) bool {
		return u.ID == "user-1" && *u.IDPProvider == "corp-sso" && *u.IDPSubject == "sub-1"
	})).Return(nil)
	identities.On("Upsert", ctx, "user-1", "corp-sso", "sub-1").Return(nil)

	user, isNew, err := identityAuthService(t, users, identities).
		createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC, claims)

	require.NoError(t, err)
	assert.False(t, isNew)
	assert.Equal(t, "user-1", user.ID)
	users.AssertExpectations(t)
	identities.AssertExpectations(t)
}

// A user matched through the legacy Google identity gets the slug identity of
// the provider they signed in with (#1427).
func TestAuthService_SignIn_LegacyGoogleUserGetsSlugIdentity(t *testing.T) {
	ctx := context.Background()
	claims := &idp.Claims{Subject: "g-1", Email: "g@example.com", EmailVerified: true}
	users := &repo_mocks.MockUserRepository{}
	identities := &repo_mocks.MockUserIdentityRepository{}
	users.On("GetByIDPSubject", ctx, "google-workspace", "g-1").Return(nil, repositories.ErrUserNotFound)
	users.On("GetByIDPSubject", ctx, models.LegacyGoogleIdentityProvider, "g-1").
		Return(&models.User{ID: "legacy-1", Email: "g@example.com", GoogleID: strPtr("g-1")}, nil)
	users.On("Update", ctx, mock.Anything).Return(nil)
	identities.On("Upsert", ctx, "legacy-1", "google-workspace", "g-1").Return(nil)

	user, _, err := identityAuthService(t, users, identities).
		createOrUpdateUserFromClaims(ctx, "google-workspace", idp.ProviderGoogle, claims)

	require.NoError(t, err)
	assert.Equal(t, "legacy-1", user.ID)
	users.AssertExpectations(t)
	identities.AssertExpectations(t)
}

// A new user's identity is written by UserRepository.Create in the same
// statement as the row, so the service must not record it a second time.
func TestAuthService_SignIn_NewUserIdentityIsWrittenByCreate(t *testing.T) {
	ctx := context.Background()
	claims := &idp.Claims{Subject: "sub-new", Email: "New@Example.com", Name: "N", EmailVerified: true}
	users := &repo_mocks.MockUserRepository{}
	identities := &repo_mocks.MockUserIdentityRepository{}
	users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
	users.On("Create", ctx, mock.MatchedBy(func(u *models.User) bool {
		return *u.IDPProvider == "corp-sso" && *u.IDPSubject == "sub-new" && u.EmailVerifiedAt != nil
	})).Return(nil)

	_, isNew, err := identityAuthService(t, users, identities).
		createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC, claims)

	require.NoError(t, err)
	assert.True(t, isNew)
	identities.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	users.AssertExpectations(t)
}

// A sign-in whose identity cannot be recorded fails instead of succeeding with
// an account that has no identity row.
func TestAuthService_SignIn_FailsWhenTheIdentityCannotBeRecorded(t *testing.T) {
	ctx := context.Background()
	claims := &idp.Claims{Subject: "sub-1", Email: "u@example.com"}
	users := &repo_mocks.MockUserRepository{}
	identities := &repo_mocks.MockUserIdentityRepository{}
	users.On("GetByIDPSubject", ctx, "corp-sso", "sub-1").
		Return(&models.User{ID: "user-1", Email: "u@example.com"}, nil)
	users.On("Update", ctx, mock.Anything).Return(nil)
	identities.On("Upsert", ctx, "user-1", "corp-sso", "sub-1").Return(repositories.ErrUserIdentityTaken)

	user, _, err := identityAuthService(t, users, identities).
		createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC, claims)

	require.ErrorIs(t, err, repositories.ErrUserIdentityTaken)
	assert.Nil(t, user)
}

// email_verified_at describes the STORED address (#1427).
func TestEmailVerifiedAtAfterSignIn(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-24 * time.Hour)

	tests := []struct {
		name     string
		stored   *time.Time
		oldEmail string
		claims   idp.Claims
		want     *time.Time
	}{
		{"first verification is stamped", nil, "u@example.com",
			idp.Claims{Email: "u@example.com", EmailVerified: true}, &now},
		{"an already verified address keeps its first timestamp", &earlier, "u@example.com",
			idp.Claims{Email: "U@Example.com", EmailVerified: true}, &earlier},
		{"an unverified sign-in does not unverify an unchanged address", &earlier, "u@example.com",
			idp.Claims{Email: "u@example.com", EmailVerified: false}, &earlier},
		{"an unverified sign-in leaves a never-verified address unverified", nil, "u@example.com",
			idp.Claims{Email: "u@example.com", EmailVerified: false}, nil},
		{"a changed, verified address restarts from this sign-in", &earlier, "old@example.com",
			idp.Claims{Email: "new@example.com", EmailVerified: true}, &now},
		{"a changed, unverified address is cleared", &earlier, "old@example.com",
			idp.Claims{Email: "new@example.com", EmailVerified: false}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &models.User{Email: tt.oldEmail, EmailVerifiedAt: tt.stored}
			got := emailVerifiedAtAfterSignIn(user, &tt.claims, now)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.True(t, tt.want.Equal(*got))
		})
	}
}

// Dev login finds an existing account whatever casing is typed, and a new dev
// account's identity subject uses the normalized address (#1427).
func TestAuthService_HandleDevLogin_NormalizesTheSubject(t *testing.T) {
	ctx := context.Background()
	users := &repo_mocks.MockUserRepository{}
	users.On("GetByEmail", ctx, "Dev@Example.com").Return(nil, repositories.ErrUserNotFound)
	users.On("Create", ctx, mock.MatchedBy(func(u *models.User) bool {
		return *u.IDPProvider == devIDPProvider && *u.IDPSubject == "dev_dev@example.com" &&
			u.Email == "dev@example.com" && u.EmailVerifiedAt == nil
	})).Return(nil)

	_, err := identityAuthService(t, users, &repo_mocks.MockUserIdentityRepository{}).
		HandleDevLogin(ctx, "Dev@Example.com", "Dev")

	require.NoError(t, err)
	users.AssertExpectations(t)
}

// email_verified_at is what #1430's account-linking rule trusts, so pin what a
// whole sign-in writes, not only the helper: a verified sign-in sets it, and an
// unverified one that changes the address clears it.
func TestAuthService_SignIn_WritesEmailVerifiedAt(t *testing.T) {
	ctx := context.Background()
	verifiedLongAgo := time.Now().Add(-48 * time.Hour)

	tests := []struct {
		name   string
		stored models.User
		claims idp.Claims
		check  func(*testing.T, *models.User)
	}{
		{
			name:   "a verified sign-in sets it",
			stored: models.User{ID: "user-1", Email: "u@example.com"},
			claims: idp.Claims{Subject: "sub-1", Email: "u@example.com", EmailVerified: true},
			check: func(t *testing.T, u *models.User) {
				require.NotNil(t, u.EmailVerifiedAt)
				assert.WithinDuration(t, time.Now(), *u.EmailVerifiedAt, time.Minute)
			},
		},
		{
			name:   "an unverified sign-in that changes the email clears it",
			stored: models.User{ID: "user-1", Email: "old@example.com", EmailVerifiedAt: &verifiedLongAgo},
			claims: idp.Claims{Subject: "sub-1", Email: "new@example.com", EmailVerified: false},
			check: func(t *testing.T, u *models.User) {
				assert.Equal(t, "new@example.com", u.Email)
				assert.Nil(t, u.EmailVerifiedAt, "the new address was never verified")
			},
		},
		{
			name:   "an unverified sign-in with the same email keeps it",
			stored: models.User{ID: "user-1", Email: "u@example.com", EmailVerifiedAt: &verifiedLongAgo},
			claims: idp.Claims{Subject: "sub-1", Email: "u@example.com", EmailVerified: false},
			check: func(t *testing.T, u *models.User) {
				require.NotNil(t, u.EmailVerifiedAt)
				assert.True(t, verifiedLongAgo.Equal(*u.EmailVerifiedAt))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored := tt.stored
			users := &repo_mocks.MockUserRepository{}
			identities := &repo_mocks.MockUserIdentityRepository{}
			users.On("GetByIDPSubject", ctx, "corp-sso", "sub-1").Return(&stored, nil)
			var written *models.User
			users.On("Update", ctx, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				written = args.Get(1).(*models.User)
			})
			identities.On("Upsert", ctx, "user-1", "corp-sso", "sub-1").Return(nil)

			claims := tt.claims
			_, _, err := identityAuthService(t, users, identities).
				createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC, &claims)

			require.NoError(t, err)
			require.NotNil(t, written, "the user row must be written")
			tt.check(t, written)
		})
	}
}

// A new account is verified from its first sign-in only when the provider says so.
func TestAuthService_SignIn_NewUserEmailVerifiedAt(t *testing.T) {
	ctx := context.Background()
	for _, verified := range []bool{true, false} {
		users := &repo_mocks.MockUserRepository{}
		users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
		var created *models.User
		users.On("Create", ctx, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			created = args.Get(1).(*models.User)
		})
		claims := &idp.Claims{Subject: "sub-new", Email: "n@example.com", EmailVerified: verified}

		_, _, err := identityAuthService(t, users, &repo_mocks.MockUserIdentityRepository{}).
			createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC, claims)

		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, verified, created.EmailVerifiedAt != nil, "claims verified=%v", verified)
	}
}

func TestAuthService_RevokeUserSessions(t *testing.T) {
	t.Run("returns the new epoch", func(t *testing.T) {
		mockRepo := &repo_mocks.MockUserRepository{}
		service := createTestAuthServiceNew(mockRepo, &idpmocks.MockIdentityProvider{}, nil)
		mockRepo.On("BumpSessionEpoch", mock.Anything, "user-1").Return(int64(3), nil)

		epoch, err := service.RevokeUserSessions(context.Background(), "user-1")

		assert.NoError(t, err)
		assert.Equal(t, int64(3), epoch)
		mockRepo.AssertExpectations(t)
	})

	t.Run("an unknown user stays distinguishable", func(t *testing.T) {
		mockRepo := &repo_mocks.MockUserRepository{}
		service := createTestAuthServiceNew(mockRepo, &idpmocks.MockIdentityProvider{}, nil)
		mockRepo.On("BumpSessionEpoch", mock.Anything, "ghost").Return(int64(0), repositories.ErrUserNotFound)

		_, err := service.RevokeUserSessions(context.Background(), "ghost")

		assert.ErrorIs(t, err, repositories.ErrUserNotFound)
	})
}

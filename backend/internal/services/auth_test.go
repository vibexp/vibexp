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

// noAccountYet makes repo answer "no such account" to both sign-in lookups, by
// identity and by email. Since #1434 a sign-in is resolved to its account
// before the allowlist decides (an existing account is judged by its stored
// email), so a denial test must let the lookups run; what it asserts is that
// nothing was WRITTEN.
func noAccountYet(repo *repo_mocks.MockUserRepository) {
	repo.On("GetByIDPSubject", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, repositories.ErrUserNotFound).Maybe()
	repo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()
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
				mockRepo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()
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
				// The account keeps its own email and name: neither follows the
				// provider's claims after the account exists (#1430).
				mockRepo.On("Update", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					return user.ID == "user-123" &&
						user.Email == "old@example.com" &&
						user.Name == "Old Name" &&
						user.IDPProvider != nil && *user.IDPProvider == testProvider
				})).Return(nil)
			},
			expectError: false,
			validateUser: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-123", user.ID)
				assert.Equal(t, "old@example.com", user.Email)
				assert.Equal(t, "Old Name", user.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &repo_mocks.MockUserRepository{}
			mockIDP := &idpmocks.MockIdentityProvider{}
			mockIDP.On("Name").Return(idp.ProviderOIDC)
			// An open instance: these cases are about resolving and writing the
			// account. Which email the allowlist judges is covered by
			// TestAuthService_AllowlistSubject.
			service := createTestAuthServiceNew(mockRepo, mockIDP, nil)
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
				mockRepo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()

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
			name: "successful callback with existing user update",
			code: "test-auth-code",
			// The allowlist judges an existing account by its STORED email
			// (#1434), which here differs from the claim email.
			allowedEmails: []string{"old@example.com"},
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository, mockIDP *idpmocks.MockIdentityProvider) {
				verifiedAt := time.Now().Add(-24 * time.Hour)
				existingUser := &models.User{
					ID:          "user-existing-123",
					GoogleID:    nil,
					IDPProvider: strPtr(testProvider),
					IDPSubject:  strPtr("oidc-sub-123"),
					Email:       "old@example.com",
					// Verified earlier, so the stored address satisfies the
					// allowlist's verification rule although today's claim is for
					// another address.
					EmailVerifiedAt: &verifiedAt,
					Name:            "Old Name",
					CreatedAt:       time.Now().Add(-time.Hour),
					UpdatedAt:       time.Now().Add(-time.Hour),
				}

				mockIDP.On("Name").Return(idp.ProviderOIDC)
				mockIDP.On("ExchangeCode", context.Background(), "test-auth-code", "").
					Return(testTokens, testClaims, nil)

				mockRepo.On("GetByIDPSubject", context.Background(), testProvider, "oidc-sub-123").
					Return(existingUser, nil)

				mockRepo.On("Update", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					// The stored email is kept even though the provider now
					// reports test@example.com (#1430).
					return user.ID == "user-existing-123" &&
						user.Email == "old@example.com"
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
				mockRepo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()
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
	noAccountYet(mockRepo)

	user, tokens, isNewUser, err := service.HandleCallback(
		context.Background(), "test-auth-code", string(idp.ProviderOIDC))

	assert.ErrorIs(t, err, ErrAccessRestricted)
	assert.Equal(t, ErrAccessRestricted, err, "a denial is returned as is, not wrapped as a failure")
	assert.Nil(t, user)
	assert.Nil(t, tokens)
	assert.False(t, isNewUser)
	// No user row was touched: nothing was created or updated.
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

			if tt.expectDenied {
				noAccountYet(mockRepo)
			} else {
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
				mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
				mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
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
			noAccountYet(mockRepo)

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
	mockRepo.On("Update", context.Background(), existing).Return(nil)

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
						user.Name == "Dev User" &&
						user.EmailVerifiedAt != nil // dev login vouches for the address (#1467)
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
				// An existing account from before email verification was recorded is
				// marked verified, so a dev root admin keeps its power (#1467).
				mockRepo.On("Update", context.Background(), mock.MatchedBy(func(user *models.User) bool {
					return user.ID == "user-existing-456" && user.EmailVerifiedAt != nil
				})).Return(nil).Once()
			},
			expectError: false,
			validateResult: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-existing-456", user.ID)
				assert.NotNil(t, user.EmailVerifiedAt)
			},
		},
		{
			name:     "dev login with an already verified user writes nothing",
			email:    "verified@example.com",
			userName: "Verified User",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				verifiedAt := time.Now().Add(-time.Hour)
				mockRepo.On("GetByEmail", context.Background(), "verified@example.com").
					Return(&models.User{ID: "user-verified-789", Email: "verified@example.com", EmailVerifiedAt: &verifiedAt}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, user *models.User) {
				assert.Equal(t, "user-verified-789", user.ID)
			},
		},
		{
			name:     "failed to mark an existing user verified",
			email:    "unmarked@example.com",
			userName: "Unmarked User",
			setupMocks: func(mockRepo *repo_mocks.MockUserRepository) {
				mockRepo.On("GetByEmail", context.Background(), "unmarked@example.com").
					Return(&models.User{ID: "user-unmarked", Email: "unmarked@example.com"}, nil)
				mockRepo.On("Update", context.Background(), mock.Anything).Return(fmt.Errorf("db down"))
			},
			expectError: true,
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
				mockRepo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()

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

			// An open instance: this test is about the user.created event, not
			// about who the allowlist admits.
			allowlist := newStaticAllowlistResolver(logger, nil)

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
			repo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()
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
	repo.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()
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
	users.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()
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

// email_verified_at describes the STORED address, which no longer follows the
// claims (#1427, #1430).
func TestEmailVerifiedAtAfterSignIn(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-24 * time.Hour)

	tests := []struct {
		name   string
		stored *time.Time
		claims idp.Claims
		want   *time.Time
	}{
		{"first verification of the stored address is stamped", nil,
			idp.Claims{Email: "U@Example.com", EmailVerified: true}, &now},
		{"an already verified address keeps its first timestamp", &earlier,
			idp.Claims{Email: "u@example.com", EmailVerified: true}, &earlier},
		{"an unverified sign-in does not unverify the address", &earlier,
			idp.Claims{Email: "u@example.com", EmailVerified: false}, &earlier},
		{"an unverified sign-in leaves a never-verified address unverified", nil,
			idp.Claims{Email: "u@example.com", EmailVerified: false}, nil},
		{"a provider verifying a DIFFERENT address does not verify the stored one", nil,
			idp.Claims{Email: "other@example.com", EmailVerified: true}, nil},
		{"a different address at the provider does not unverify the stored one", &earlier,
			idp.Claims{Email: "other@example.com", EmailVerified: false}, &earlier},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &models.User{Email: "u@example.com", EmailVerifiedAt: tt.stored}
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
			u.Email == "dev@example.com" && u.EmailVerifiedAt != nil
	})).Return(nil)

	_, err := identityAuthService(t, users, &repo_mocks.MockUserIdentityRepository{}).
		HandleDevLogin(ctx, "Dev@Example.com", "Dev")

	require.NoError(t, err)
	users.AssertExpectations(t)
}

// returningSignIn runs a sign-in for an account matched by its own identity
// (corp-sso, sub-1) and returns the row the service wrote.
func returningSignIn(t *testing.T, stored models.User, claims idp.Claims) *models.User {
	t.Helper()
	ctx := context.Background()
	users := &repo_mocks.MockUserRepository{}
	identities := &repo_mocks.MockUserIdentityRepository{}
	users.On("GetByIDPSubject", ctx, "corp-sso", "sub-1").Return(&stored, nil)
	var written *models.User
	users.On("Update", ctx, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		written = args.Get(1).(*models.User)
	})
	identities.On("Upsert", ctx, stored.ID, "corp-sso", "sub-1").Return(nil)

	_, isNew, err := identityAuthService(t, users, identities).
		createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC, &claims)

	require.NoError(t, err)
	assert.False(t, isNew)
	require.NotNil(t, written, "the user row must be written")
	users.AssertNotCalled(t, "GetByEmail", mock.Anything, mock.Anything)
	return written
}

// The account email is stable: a returning user keeps it whatever the provider
// reports, and their name and avatar are only filled while empty (#1430).
func TestAuthService_ReturningSignIn_KeepsEmailAndProfile(t *testing.T) {
	avatar := "https://cdn.example.com/mine.png"

	t.Run("a different email, name and picture at the provider change nothing", func(t *testing.T) {
		written := returningSignIn(t,
			models.User{ID: "user-1", Email: "me@example.com", Name: "My Name", AvatarURL: &avatar},
			idp.Claims{
				Subject: "sub-1", Email: "Changed@Provider.example", Name: "Provider Name",
				Picture: "https://provider.example/p.png", EmailVerified: true,
			})

		assert.Equal(t, "me@example.com", written.Email)
		assert.Equal(t, "My Name", written.Name)
		require.NotNil(t, written.AvatarURL)
		assert.Equal(t, avatar, *written.AvatarURL)
		assert.Nil(t, written.EmailVerifiedAt, "the provider verified another address, not the stored one")
	})

	t.Run("an empty name and avatar are filled from the claims", func(t *testing.T) {
		written := returningSignIn(t,
			models.User{ID: "user-1", Email: "me@example.com", Name: "  "},
			idp.Claims{Subject: "sub-1", Email: "me@example.com", Name: "Provider Name", Picture: "https://provider.example/p.png"})

		assert.Equal(t, "Provider Name", written.Name)
		require.NotNil(t, written.AvatarURL)
		assert.Equal(t, "https://provider.example/p.png", *written.AvatarURL)
	})

	t.Run("a verified sign-in with the stored email sets email_verified_at", func(t *testing.T) {
		written := returningSignIn(t,
			models.User{ID: "user-1", Email: "me@example.com", Name: "Me"},
			idp.Claims{Subject: "sub-1", Email: "me@example.com", EmailVerified: true})

		require.NotNil(t, written.EmailVerifiedAt)
		assert.WithinDuration(t, time.Now(), *written.EmailVerifiedAt, time.Minute)
	})
}

// recordingClaimHook records the accounts it was asked to secure, in a shared
// call log so its order relative to the repository writes can be asserted.
type recordingClaimHook struct {
	log *[]string
	err error
	// providers, when set, records the provider name each call was given.
	providers *[]string
}

func (h recordingClaimHook) OnUnverifiedAccountClaimed(_ context.Context, userID, provider string) error {
	*h.log = append(*h.log, "hook:"+userID)
	if h.providers != nil {
		*h.providers = append(*h.providers, provider)
	}
	return h.err
}

// linkFixture is an AuthService whose repositories record the order of writes.
type linkFixture struct {
	svc        *AuthService
	users      *repo_mocks.MockUserRepository
	identities *repo_mocks.MockUserIdentityRepository
	log        []string
	providers  []string
	written    *models.User
	held       []models.UserIdentity
}

// newLinkFixture prepares a sign-in through corp-sso/sub-new that matches no
// identity and finds account by email. hookErr, upsertErr are what the hook and
// the identity write answer.
func newLinkFixture(t *testing.T, account *models.User, hookErr, upsertErr error) *linkFixture {
	t.Helper()
	f := &linkFixture{users: &repo_mocks.MockUserRepository{}, identities: &repo_mocks.MockUserIdentityRepository{}}
	f.users.On("GetByIDPSubject", mock.Anything, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
	f.users.On("GetByEmail", mock.Anything, mock.Anything).Return(account, nil)
	f.users.On("Update", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		f.written = args.Get(1).(*models.User)
		f.log = append(f.log, "update")
	}).Maybe()
	f.identities.On("Upsert", mock.Anything, account.ID, "corp-sso", "sub-new").Return(upsertErr).
		Run(func(mock.Arguments) { f.log = append(f.log, "upsert") }).Maybe()
	// The account holds no sign-in identity unless a test says otherwise
	// (holdsIdentity), and bumping its session epoch yields 7.
	f.identities.On("ListByUserID", mock.Anything, account.ID).
		Return(func(context.Context, string) ([]models.UserIdentity, error) { return f.held, nil }).Maybe()
	f.users.On("BumpSessionEpoch", mock.Anything, account.ID).Return(int64(7), nil).
		Run(func(mock.Arguments) { f.log = append(f.log, "epoch") }).Maybe()

	logger := func() *slog.Logger { l, _ := logtest.New(); return l }()
	f.svc = NewAuthService(f.users, f.identities, staticIDPResolver{reg: idp.NewRegistry()}, nil, logger,
		newStaticAllowlistResolver(logger, nil),
		WithUnverifiedAccountClaimHook(recordingClaimHook{log: &f.log, err: hookErr, providers: &f.providers}))
	return f
}

// holdsIdentity makes the account already own a sign-in identity.
func (f *linkFixture) holdsIdentity(provider, subject string) *linkFixture {
	f.held = append(f.held, models.UserIdentity{Provider: provider, Subject: subject})
	return f
}

func (f *linkFixture) signIn(claims idp.Claims) (*models.User, bool, error) {
	return f.svc.createOrUpdateUserFromClaims(context.Background(), "corp-sso", idp.ProviderOIDC, &claims)
}

// An admin-created account (no identity, never verified) completes its first
// provider sign-in: the sign-in lands in that account, the hook secures it
// first, and nothing is created (#1430).
func TestAuthService_LinkByVerifiedEmail_AdminCreatedAccount(t *testing.T) {
	account := &models.User{ID: "pre-1", Email: "ada@example.com", Name: "Ada", IDPProvider: strPtr("corp-sso")}
	f := newLinkFixture(t, account, nil, nil)

	user, isNew, err := f.signIn(idp.Claims{
		Subject: "sub-new", Email: "Ada@Example.com", Name: "Provider Ada", EmailVerified: true,
	})

	require.NoError(t, err)
	assert.False(t, isNew, "linking is not a registration")
	assert.Equal(t, "pre-1", user.ID)
	assert.Equal(t, []string{"hook:pre-1", "epoch", "update", "upsert"}, f.log,
		"the account is secured and its sessions ended before it is marked verified, and all before the identity is attached")
	assert.EqualValues(t, 7, user.SessionEpoch, "the session minted for this sign-in must carry the new epoch")
	require.NotNil(t, f.written.EmailVerifiedAt)
	assert.Equal(t, "ada@example.com", f.written.Email)
	assert.Equal(t, "Ada", f.written.Name, "the admin-given name is kept")
	assert.Equal(t, "sub-new", *f.written.IDPSubject)
	f.users.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	f.users.AssertCalled(t, "GetByEmail", mock.Anything, "Ada@Example.com")
}

// A second provider with the same verified email joins the same account. The
// account's email is already verified, so there is nothing to secure.
func TestAuthService_LinkByVerifiedEmail_SecondProvider(t *testing.T) {
	verifiedAt := time.Now().Add(-time.Hour)
	account := &models.User{
		ID: "user-1", Email: "ada@example.com", Name: "Ada", EmailVerifiedAt: &verifiedAt,
		IDPProvider: strPtr("google"), IDPSubject: strPtr("g-1"),
	}
	f := newLinkFixture(t, account, nil, nil).holdsIdentity("google", "g-1")

	user, isNew, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

	require.NoError(t, err)
	assert.False(t, isNew)
	assert.Equal(t, "user-1", user.ID)
	assert.Equal(t, []string{"update", "upsert"}, f.log,
		"a verified account is not claimed: no hook, no session-epoch bump")
	f.identities.AssertNotCalled(t, "ListByUserID", mock.Anything, mock.Anything)
	assert.True(t, verifiedAt.Equal(*f.written.EmailVerifiedAt), "the first verification is kept")
	f.users.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// An unverified email matching an existing account is refused and writes
// nothing: no identity, no account change, no new account.
func TestAuthService_LinkByEmail_UnverifiedIsRefused(t *testing.T) {
	account := &models.User{ID: "user-1", Email: "ada@example.com", Name: "Ada"}
	f := newLinkFixture(t, account, nil, nil)

	user, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: false})

	require.ErrorIs(t, err, ErrEmailNotVerified)
	assert.Nil(t, user)
	assert.Empty(t, f.log, "no hook, no update, no identity")
	f.users.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	assert.Nil(t, account.EmailVerifiedAt)
}

// A suspended account gains no identity: it is returned untouched so the
// caller's suspension gate refuses the sign-in.
func TestAuthService_LinkByEmail_SuspendedAccountGainsNoIdentity(t *testing.T) {
	for _, verified := range []bool{true, false} {
		account := &models.User{ID: "user-1", Email: "ada@example.com", Status: models.UserStatusSuspended}
		f := newLinkFixture(t, account, nil, nil)

		user, isNew, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: verified})

		require.NoError(t, err, "verified=%v", verified)
		assert.False(t, isNew)
		require.NotNil(t, user)
		assert.True(t, user.IsSuspended(), "the caller must see the suspended account to refuse it")
		assert.Empty(t, f.log, "verified=%v: no hook, no update, no identity", verified)
		f.identities.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		assert.Nil(t, account.EmailVerifiedAt)
		assert.Nil(t, account.IDPSubject)
	}
}

// A hook that fails aborts the sign-in before anything is written: no identity
// and no email_verified_at change, so the next attempt runs the hook again.
func TestAuthService_LinkByVerifiedEmail_HookErrorLinksNothing(t *testing.T) {
	account := &models.User{ID: "pre-1", Email: "ada@example.com", Name: "Ada"}
	hookErr := errors.New("credential store unavailable")
	f := newLinkFixture(t, account, hookErr, nil)

	user, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

	require.ErrorIs(t, err, hookErr)
	assert.Nil(t, user)
	assert.Equal(t, []string{"hook:pre-1"}, f.log, "neither the account nor the identity is written")
	f.users.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	f.identities.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	assert.Nil(t, account.EmailVerifiedAt, "email_verified_at is not set on the model either")
	assert.Nil(t, account.IDPSubject)
}

// The link decision for a VERIFIED claim email, over the two facts about the
// account that decide it (#1430):
//
//	                     | no identity yet       | already has an identity
//	email never verified | claimed and linked    | REFUSED (may be someone else's)
//	email verified       | linked                | linked
//
// An open instance creates an account from an UNVERIFIED claim, so a
// never-verified account that someone can already sign in to may not belong to
// the mailbox's owner. Linking the owner into it would put two people in one
// account, so that cell is refused and writes nothing.
func TestAuthService_LinkByVerifiedEmail_DecisionMatrix(t *testing.T) {
	verifiedAt := time.Now().Add(-time.Hour)
	claims := idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true}

	tests := []struct {
		name        string
		verified    bool
		hasIdentity bool
		wantErr     error
		wantLog     []string
	}{
		{"never verified, no identity: claimed and linked", false, false, nil,
			[]string{"hook:user-1", "epoch", "update", "upsert"}},
		{"never verified, has an identity: refused", false, true, ErrExistingSignInRequired, nil},
		{"verified, no identity: linked", true, false, nil, []string{"update", "upsert"}},
		{"verified, has an identity: linked", true, true, nil, []string{"update", "upsert"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &models.User{ID: "user-1", Email: "ada@example.com", Name: "Ada"}
			if tt.verified {
				account.EmailVerifiedAt = &verifiedAt
			}
			f := newLinkFixture(t, account, nil, nil)
			if tt.hasIdentity {
				f.holdsIdentity("other-sso", "someone")
			}

			user, isNew, err := f.signIn(claims)

			assert.Equal(t, tt.wantLog, f.log)
			assert.False(t, isNew)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, user)
				assert.Nil(t, account.EmailVerifiedAt, "a refused link does not verify the account")
				assert.Nil(t, account.IDPSubject)
				f.users.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "user-1", user.ID)
			require.NotNil(t, f.written.EmailVerifiedAt)
		})
	}
}

// The path of a user from before email verification was recorded (migration
// 022_consolidated left email_verified_at NULL): a second provider is refused
// at first, one sign-in with their own provider verifies the account, and the
// second provider then links.
func TestAuthService_LinkByVerifiedEmail_PreVerificationUserPath(t *testing.T) {
	ctx := context.Background()
	account := &models.User{ID: "old-1", Email: "ada@example.com", Name: "Ada"}
	held := []models.UserIdentity{{Provider: "google", Subject: "g-1"}}
	users := &repo_mocks.MockUserRepository{}
	identities := &repo_mocks.MockUserIdentityRepository{}
	users.On("GetByIDPSubject", ctx, "google", "g-1").Return(account, nil)
	users.On("GetByIDPSubject", ctx, models.LegacyGoogleIdentityProvider, mock.Anything).
		Return(nil, repositories.ErrUserNotFound).Maybe()
	users.On("GetByIDPSubject", ctx, "github", "gh-1").Return(nil, repositories.ErrUserNotFound)
	users.On("GetByEmail", ctx, "ada@example.com").Return(account, nil)
	users.On("Update", ctx, mock.Anything).Return(nil)
	identities.On("ListByUserID", ctx, "old-1").Return(held, nil)
	identities.On("Upsert", ctx, "old-1", mock.Anything, mock.Anything).Return(nil)
	svc := identityAuthService(t, users, identities)
	github := &idp.Claims{Subject: "gh-1", Email: "ada@example.com", EmailVerified: true}

	_, _, err := svc.createOrUpdateUserFromClaims(ctx, "github", idp.ProviderGitHub, github)
	require.ErrorIs(t, err, ErrExistingSignInRequired, "a second provider is refused while the account is unverified")
	identities.AssertNotCalled(t, "Upsert", ctx, "old-1", "github", "gh-1")

	_, _, err = svc.createOrUpdateUserFromClaims(ctx, "google", idp.ProviderGoogle,
		&idp.Claims{Subject: "g-1", Email: "ada@example.com", EmailVerified: true})
	require.NoError(t, err)
	require.NotNil(t, account.EmailVerifiedAt, "signing in with their own provider verifies the account")

	user, _, err := svc.createOrUpdateUserFromClaims(ctx, "github", idp.ProviderGitHub, github)
	require.NoError(t, err, "the second provider now links")
	assert.Equal(t, "old-1", user.ID)
	identities.AssertCalled(t, "Upsert", ctx, "old-1", "github", "gh-1")
	users.AssertNotCalled(t, "BumpSessionEpoch", mock.Anything, mock.Anything)
}

// A failure to read the account's identities or to end its sessions aborts the
// claim: the account is not marked verified and nothing is linked.
func TestAuthService_LinkByVerifiedEmail_ClaimStepFailuresLinkNothing(t *testing.T) {
	ctx := context.Background()
	claims := &idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true}
	boom := errors.New("database unavailable")

	for name, arrange := range map[string]func(*repo_mocks.MockUserRepository, *repo_mocks.MockUserIdentityRepository){
		"identities unreadable": func(_ *repo_mocks.MockUserRepository, i *repo_mocks.MockUserIdentityRepository) {
			i.On("ListByUserID", ctx, "pre-1").Return(nil, boom)
		},
		"sessions cannot be ended": func(u *repo_mocks.MockUserRepository, i *repo_mocks.MockUserIdentityRepository) {
			i.On("ListByUserID", ctx, "pre-1").Return([]models.UserIdentity{}, nil)
			u.On("BumpSessionEpoch", ctx, "pre-1").Return(int64(0), boom)
		},
	} {
		t.Run(name, func(t *testing.T) {
			account := &models.User{ID: "pre-1", Email: "ada@example.com"}
			users := &repo_mocks.MockUserRepository{}
			identities := &repo_mocks.MockUserIdentityRepository{}
			users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
			users.On("GetByEmail", ctx, "ada@example.com").Return(account, nil)
			arrange(users, identities)

			user, _, err := identityAuthService(t, users, identities).
				createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC, claims)

			require.ErrorIs(t, err, boom)
			assert.Nil(t, user)
			users.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
			identities.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			assert.Nil(t, account.EmailVerifiedAt)
		})
	}
}

// When attaching the identity fails after the account was secured and marked
// verified, the sign-in fails; the retry links without a second hook run being
// required for safety (the hook already ran, and is idempotent anyway).
func TestAuthService_LinkByVerifiedEmail_IdentityWriteFailureFailsTheSignIn(t *testing.T) {
	account := &models.User{ID: "pre-1", Email: "ada@example.com"}
	f := newLinkFixture(t, account, nil, repositories.ErrUserIdentityTaken)

	user, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

	require.ErrorIs(t, err, repositories.ErrUserIdentityTaken)
	assert.Nil(t, user)
	assert.Equal(t, []string{"hook:pre-1", "epoch", "update", "upsert"}, f.log)
}

// Claims with no email cannot match an account by email: the lookup is skipped
// and a new account is attempted, as before.
func TestAuthService_SignIn_EmptyClaimEmailNeverLinks(t *testing.T) {
	ctx := context.Background()
	users := &repo_mocks.MockUserRepository{}
	users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
	users.On("Create", ctx, mock.Anything).Return(nil)

	_, isNew, err := identityAuthService(t, users, &repo_mocks.MockUserIdentityRepository{}).
		createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC,
			&idp.Claims{Subject: "sub-new", Email: "  ", EmailVerified: true})

	require.NoError(t, err)
	assert.True(t, isNew)
	users.AssertNotCalled(t, "GetByEmail", mock.Anything, mock.Anything)
}

// A failing email lookup is an error, never treated as "no such account".
func TestAuthService_SignIn_EmailLookupErrorIsNotAMiss(t *testing.T) {
	ctx := context.Background()
	users := &repo_mocks.MockUserRepository{}
	users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
	users.On("GetByEmail", ctx, "ada@example.com").Return(nil, errors.New("connection reset"))

	user, _, err := identityAuthService(t, users, &repo_mocks.MockUserIdentityRepository{}).
		createOrUpdateUserFromClaims(ctx, "corp-sso", idp.ProviderOIDC,
			&idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

	require.Error(t, err)
	assert.Nil(t, user)
	users.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// ProvisionFromClaims shares the same resolution, so it links too.
func TestAuthService_ProvisionFromClaims_LinksByVerifiedEmail(t *testing.T) {
	account := &models.User{ID: "pre-1", Email: "ada@example.com"}
	f := newLinkFixture(t, account, nil, nil)

	user, err := f.svc.ProvisionFromClaims(context.Background(), "corp-sso",
		&idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

	require.NoError(t, err)
	assert.Equal(t, "pre-1", user.ID)
	assert.Equal(t, []string{"hook:pre-1", "epoch", "update", "upsert"}, f.log)

	_, err = newLinkFixture(t, &models.User{ID: "pre-2", Email: "bob@example.com"}, nil, nil).svc.
		ProvisionFromClaims(context.Background(), "corp-sso",
			&idp.Claims{Subject: "sub-new", Email: "bob@example.com", EmailVerified: false})
	require.ErrorIs(t, err, ErrEmailNotVerified)
}

// The default hook does nothing, and a nil option keeps it.
func TestAuthService_DefaultClaimHookIsANoop(t *testing.T) {
	logger := func() *slog.Logger { l, _ := logtest.New(); return l }()
	svc := NewAuthService(nil, nil, staticIDPResolver{reg: idp.NewRegistry()}, nil, logger,
		newStaticAllowlistResolver(logger, nil), WithUnverifiedAccountClaimHook(nil))

	require.NotNil(t, svc.claimHook)
	require.NoError(t, svc.claimHook.OnUnverifiedAccountClaimed(context.Background(), "user-1", "SSO"))
}

// The claim hook is told the provider by the name the sign-in page shows, so
// the owner's notice names it (#1493); the slug stands in when the provider
// cannot be resolved.
func TestAuthService_ClaimHookIsGivenTheProviderDisplayName(t *testing.T) {
	signIn := func(t *testing.T, resolver IdentityProviderResolver) []string {
		f := newLinkFixture(t, &models.User{ID: "pre-1", Email: "ada@example.com"}, nil, nil)
		f.svc.resolver = resolver
		_, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})
		require.NoError(t, err)
		return f.providers
	}

	t.Run("the display name of an enabled provider", func(t *testing.T) {
		reg := idp.NewRegistryFromEntries(
			idp.Entry{Provider: slugProvider("corp-sso"), Type: idp.ProviderOIDC, DisplayName: "Corp SSO"})
		assert.Equal(t, []string{"Corp SSO"}, signIn(t, staticIDPResolver{reg: reg}))
	})
	t.Run("the slug when the provider is not in the snapshot", func(t *testing.T) {
		assert.Equal(t, []string{"corp-sso"}, signIn(t, staticIDPResolver{reg: idp.NewRegistry()}))
	})
	t.Run("the slug when the service has no resolver", func(t *testing.T) {
		assert.Equal(t, []string{"corp-sso"}, signIn(t, nil))
	})
	t.Run("the slug when the providers cannot be resolved", func(t *testing.T) {
		assert.Equal(t, []string{"corp-sso"}, signIn(t, staticIDPResolver{err: errors.New("db down")}))
	})
}

// A new account is verified from its first sign-in only when the provider says so.
func TestAuthService_SignIn_NewUserEmailVerifiedAt(t *testing.T) {
	ctx := context.Background()
	for _, verified := range []bool{true, false} {
		users := &repo_mocks.MockUserRepository{}
		users.On("GetByIDPSubject", ctx, "corp-sso", "sub-new").Return(nil, repositories.ErrUserNotFound)
		users.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()
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

// The unverified-email refusal is a policy outcome: HandleCallback returns it
// recognizably and does not report it as a failure in the error log.
func TestAuthService_HandleCallback_EmailNotVerifiedIsNotLoggedAsAnError(t *testing.T) {
	ctx := context.Background()
	claims := &idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: false}
	mockIDP := &idpmocks.MockIdentityProvider{}
	mockIDP.On("ExchangeCode", ctx, "code", "").Return(&idp.Tokens{AccessToken: "a"}, claims, nil)
	users := &repo_mocks.MockUserRepository{}
	users.On("GetByIDPSubject", ctx, string(idp.ProviderOIDC), "sub-new").Return(nil, repositories.ErrUserNotFound)
	users.On("GetByEmail", ctx, "ada@example.com").Return(&models.User{ID: "user-1", Email: "ada@example.com"}, nil)
	service, recorder := createTestAuthServiceWithRecorder(users, mockIDP, nil)

	user, tokens, isNew, err := service.HandleCallback(ctx, "code", string(idp.ProviderOIDC))

	require.ErrorIs(t, err, ErrEmailNotVerified)
	assert.Equal(t, ErrEmailNotVerified, err, "returned as is, not wrapped as a failure")
	assert.Nil(t, user)
	assert.Nil(t, tokens)
	assert.False(t, isNew)
	var refusals int
	for _, entry := range recorder.AllEntries() {
		assert.NotEqual(t, slog.LevelError, entry.Level, "unexpected error log: %s", entry.Message)
		if entry.Data["reason"] == "unverified_email" {
			refusals++
		}
	}
	assert.Equal(t, 1, refusals, "the refusal is logged once, with its reason")
}

// #1466: the claim decides under the user's lock. When it finds the account
// changed since the sign-in read it, the sign-in re-reads the account and
// carries on from what is true now.
func TestAuthService_LinkByVerifiedEmail_AccountChangedBeforeTheClaim(t *testing.T) {
	stale := func(err error) error {
		return fmt.Errorf("remove the one-time links and the password of a claimed account: %w", err)
	}

	t.Run("verified meanwhile: linked as a verified account, on the fresh row", func(t *testing.T) {
		account := &models.User{ID: "pre-1", Email: "ada@example.com", Version: 3}
		f := newLinkFixture(t, account, stale(repositories.ErrAccountNoLongerClaimable), nil)
		verifiedAt := time.Now().Add(-time.Second)
		f.users.On("GetByID", mock.Anything, "pre-1").Return(&models.User{
			ID: "pre-1", Email: "ada@example.com", Name: "Ada", Version: 4, EmailVerifiedAt: &verifiedAt,
		}, nil).Run(func(mock.Arguments) { f.log = append(f.log, "reload") })

		user, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

		require.NoError(t, err)
		assert.Equal(t, "pre-1", user.ID)
		assert.Equal(t, []string{"hook:pre-1", "reload", "update", "upsert"}, f.log,
			"no session bump for a claim that did not happen")
		assert.EqualValues(t, 4, f.written.Version, "the update carries the version the completion left")
		assert.True(t, f.written.EmailVerifiedAt.Equal(verifiedAt), "the completion's verification is kept")
		assert.Equal(t, "Ada", f.written.Name, "the name the owner set is kept")
	})

	t.Run("given an identity meanwhile: refused like any such account", func(t *testing.T) {
		account := &models.User{ID: "pre-1", Email: "ada@example.com"}
		f := newLinkFixture(t, account, stale(repositories.ErrAccountNoLongerClaimable), nil)
		f.users.On("GetByID", mock.Anything, "pre-1").Return(&models.User{ID: "pre-1", Email: "ada@example.com"}, nil)

		user, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

		require.ErrorIs(t, err, ErrExistingSignInRequired)
		assert.Nil(t, user)
		f.users.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		f.identities.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("suspended meanwhile: returned untouched, nothing linked", func(t *testing.T) {
		account := &models.User{ID: "pre-1", Email: "ada@example.com"}
		f := newLinkFixture(t, account, stale(repositories.ErrAccountNoLongerClaimable), nil)
		verifiedAt := time.Now()
		f.users.On("GetByID", mock.Anything, "pre-1").Return(&models.User{
			ID: "pre-1", Email: "ada@example.com", EmailVerifiedAt: &verifiedAt, Status: models.UserStatusSuspended,
		}, nil)

		user, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

		require.NoError(t, err)
		assert.True(t, user.IsSuspended(), "the caller's suspension gate refuses it")
		f.users.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		f.identities.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("email changed meanwhile: refused, nothing linked", func(t *testing.T) {
		account := &models.User{ID: "pre-1", Email: "ada@example.com"}
		f := newLinkFixture(t, account, stale(repositories.ErrAccountNoLongerClaimable), nil)
		verifiedAt := time.Now()
		f.users.On("GetByID", mock.Anything, "pre-1").Return(&models.User{
			ID: "pre-1", Email: "someone.else@example.com", EmailVerifiedAt: &verifiedAt,
		}, nil)

		_, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

		require.ErrorIs(t, err, ErrExistingSignInRequired)
		f.users.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("a failed re-read fails the sign-in and links nothing", func(t *testing.T) {
		account := &models.User{ID: "pre-1", Email: "ada@example.com"}
		f := newLinkFixture(t, account, stale(repositories.ErrAccountNoLongerClaimable), nil)
		f.users.On("GetByID", mock.Anything, "pre-1").Return(nil, errors.New("database unavailable"))

		_, _, err := f.signIn(idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true})

		require.Error(t, err)
		f.users.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})
}

// fakeDisconnected answers IsDisconnected from a fixed set of
// "user|provider|subject" keys.
type fakeDisconnected struct {
	keys map[string]bool
	err  error
}

func (f fakeDisconnected) IsDisconnected(_ context.Context, userID, provider, subject string) (bool, error) {
	return f.keys[userID+"|"+provider+"|"+subject], f.err
}

// #1460: a provider account the owner disconnected is not linked by email
// again; everything else links as before.
func TestAuthService_LinkByVerifiedEmail_DisconnectedIdentity(t *testing.T) {
	verifiedAt := time.Now().Add(-time.Hour)
	claims := idp.Claims{Subject: "sub-new", Email: "ada@example.com", EmailVerified: true}

	t.Run("refused, nothing written", func(t *testing.T) {
		account := &models.User{ID: "user-1", Email: "ada@example.com", EmailVerifiedAt: &verifiedAt}
		f := newLinkFixture(t, account, nil, nil)
		f.svc.disconnected = fakeDisconnected{keys: map[string]bool{"user-1|corp-sso|sub-new": true}}

		user, _, err := f.signIn(claims)

		require.ErrorIs(t, err, ErrIdentityDisconnected)
		assert.True(t, isSignInRefusal(err), "a policy outcome the callback explains")
		assert.Nil(t, user)
		assert.Empty(t, f.log, "nothing is claimed, updated or linked")
	})

	t.Run("another provider account of the same provider still links", func(t *testing.T) {
		account := &models.User{ID: "user-1", Email: "ada@example.com", EmailVerifiedAt: &verifiedAt}
		f := newLinkFixture(t, account, nil, nil)
		f.svc.disconnected = fakeDisconnected{keys: map[string]bool{"user-1|corp-sso|sub-old": true}}

		user, _, err := f.signIn(claims)

		require.NoError(t, err)
		assert.Equal(t, "user-1", user.ID)
	})

	t.Run("an admin-created account's first sign-in is unaffected: nothing is recorded for it", func(t *testing.T) {
		account := &models.User{ID: "pre-1", Email: "ada@example.com"}
		f := newLinkFixture(t, account, nil, nil)
		f.svc.disconnected = fakeDisconnected{keys: map[string]bool{}}

		_, _, err := f.signIn(claims)

		require.NoError(t, err)
		assert.Equal(t, []string{"hook:pre-1", "epoch", "update", "upsert"}, f.log)
	})

	t.Run("a failed read fails the sign-in and links nothing", func(t *testing.T) {
		account := &models.User{ID: "user-1", Email: "ada@example.com", EmailVerifiedAt: &verifiedAt}
		f := newLinkFixture(t, account, nil, nil)
		f.svc.disconnected = fakeDisconnected{err: errors.New("database unavailable")}

		_, _, err := f.signIn(claims)

		require.Error(t, err)
		require.NotErrorIs(t, err, ErrIdentityDisconnected)
		assert.Empty(t, f.log)
	})
}

// #1460, the legacy Google path: a Google account linked under both the
// provider slug and the legacy alias is disconnected (RemoveIdentity deletes
// both rows, TestIntegrationSignInMethod_DisconnectIsRemembered). Its next
// sign-in finds neither, falls through to the email match, and is refused
// there by the record of the slug, not re-linked.
func TestAuthService_DisconnectedGoogleAccountIsRefusedOnTheLegacyPathToo(t *testing.T) {
	ctx := context.Background()
	verifiedAt := time.Now().Add(-time.Hour)
	account := &models.User{ID: "user-1", Email: "ada@example.com", EmailVerifiedAt: &verifiedAt}
	users := &repo_mocks.MockUserRepository{}
	identities := &repo_mocks.MockUserIdentityRepository{}
	users.On("GetByIDPSubject", ctx, "google", "g-sub").Return(nil, repositories.ErrUserNotFound).Once()
	users.On("GetByIDPSubject", ctx, models.LegacyGoogleIdentityProvider, "g-sub").
		Return(nil, repositories.ErrUserNotFound).Once()
	users.On("GetByEmail", ctx, "ada@example.com").Return(account, nil)
	svc := identityAuthService(t, users, identities)
	svc.disconnected = fakeDisconnected{keys: map[string]bool{"user-1|google|g-sub": true}}

	user, _, err := svc.createOrUpdateUserFromClaims(ctx, "google", idp.ProviderGoogle,
		&idp.Claims{Subject: "g-sub", Email: "ada@example.com", EmailVerified: true})

	require.ErrorIs(t, err, ErrIdentityDisconnected)
	assert.Nil(t, user)
	users.AssertExpectations(t)
	users.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	identities.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

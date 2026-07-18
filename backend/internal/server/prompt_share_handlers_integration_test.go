package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// MockPromptShareContainer implements Container interface for prompt share handler tests
type MockPromptShareContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	promptShareService   *svcmocks.MockPromptShareServiceInterface
	resourceUsageService *MockResourceUsageServiceForHandlers
	userRepository       *mockUserRepository
	teamService          *svcmocks.MockTeamServiceInterface
}

func (m *MockPromptShareContainer) PromptShareService() services.PromptShareServiceInterface {
	return m.promptShareService
}

func (m *MockPromptShareContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.resourceUsageService
}

func (m *MockPromptShareContainer) UserRepository() repositories.UserRepository {
	return m.userRepository
}

func (m *MockPromptShareContainer) TeamService() services.TeamServiceInterface { return m.teamService }

// mockUserRepository implements UserRepository for testing
type mockUserRepository struct {
	mock.Mock
}

func (m *mockUserRepository) GetByID(ctx context.Context, userID string) (*models.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *mockUserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *mockUserRepository) GetByGoogleID(ctx context.Context, googleID string) (*models.User, error) {
	return nil, nil
}

func (m *mockUserRepository) GetByIDPSubject(ctx context.Context, provider, subject string) (*models.User, error) {
	return nil, nil
}

func (m *mockUserRepository) GetByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*models.User, error) {
	return nil, nil
}

func (m *mockUserRepository) Create(ctx context.Context, user *models.User) error {
	return nil
}

func (m *mockUserRepository) Update(ctx context.Context, user *models.User) error {
	return nil
}

func (m *mockUserRepository) UpdateSubscriptionStatus(ctx context.Context, userID, status string, plan *string) error {
	return nil
}

func (m *mockUserRepository) UpdateSubscriptionStatusWithTrial(
	ctx context.Context,
	userID, status string,
	plan *string,
	trialEnd *time.Time,
) error {
	return nil
}

func (m *mockUserRepository) UpdateSubscriptionWithCancellation(
	ctx context.Context,
	userID, status string,
	plan *string,
	trialEnd *time.Time,
	canceledAt *time.Time,
) error {
	return nil
}

func (m *mockUserRepository) UpdateStripeCustomerID(ctx context.Context, userID, customerID string) error {
	return nil
}

func (m *mockUserRepository) UpdateTrialEndsAt(ctx context.Context, userID string, trialEndsAt *time.Time) error {
	return nil
}

func (m *mockUserRepository) UpdateDefaultTeamID(ctx context.Context, userID, teamID string) error {
	return nil
}

func (m *mockUserRepository) MarkOnboardingCompleted(ctx context.Context, userID string) error {
	return nil
}

func (m *mockUserRepository) GetNamesByIDs(_ context.Context, _ []string) (map[string]string, error) {
	return map[string]string{}, nil
}

// promptShareTestParams describes one prompt-share handler request/assertion
// round-trip executed by runPromptShareTest.
type promptShareTestParams struct {
	method, url    string
	slug           string
	body           interface{}
	userID         string
	mockSetup      func(*svcmocks.MockPromptShareServiceInterface)
	expectedStatus int
	checkResponse  func(*testing.T, *httptest.ResponseRecorder)
}

func runPromptShareTest(t *testing.T, p promptShareTestParams) {
	t.Helper()
	mockService := svcmocks.NewMockPromptShareServiceInterface(t)
	mockResourceUsage := &MockResourceUsageServiceForHandlers{}
	mockResourceUsage.On("CheckAndIncrementUsage", mock.Anything, mock.Anything, mock.Anything).
		Return(true, nil).Maybe()

	mockTeamService := svcmocks.NewMockTeamServiceInterface(t)
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, mock.Anything, mock.Anything).
		Return(true, nil).Maybe()

	mockContainer := &MockPromptShareContainer{
		promptShareService:   mockService,
		resourceUsageService: mockResourceUsage,
		teamService:          mockTeamService,
	}

	p.mockSetup(mockService)

	cfg := &config.Config{}
	srv := newTestServer(t, mockContainer, cfg)

	var reqBody []byte
	if p.body != nil {
		var err error
		reqBody, err = json.Marshal(p.body)
		assert.NoError(t, err)
	}

	req := httptest.NewRequest(p.method, p.url, bytes.NewReader(reqBody))
	if p.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("team_id", "550e8400-e29b-41d4-a716-446655440000")
	rctx.URLParams.Add("slug", p.slug)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	if p.userID != "" {
		req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, p.userID))
	}

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, p.expectedStatus, rr.Code)
	if p.checkResponse != nil {
		p.checkResponse(t, rr)
	}
}

var createPromptShareTests = []struct {
	name           string
	request        map[string]interface{}
	setupMocks     func(*svcmocks.MockPromptShareServiceInterface)
	expectedStatus int
	checkResponse  func(*testing.T, *httptest.ResponseRecorder)
}{
	{
		name: "Successfully create public share",
		request: map[string]interface{}{
			"share_type": "public",
		},
		setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface) {
			now := time.Now()
			mockService.EXPECT().
				CreateShare("test-user", "test-prompt", mock.MatchedBy(func(req *models.CreateShareRequest) bool {
					return req.ShareType == "public"
				})).
				Return(&models.ShareResponse{
					ShareToken: "abc123xyz789",
					ShareURL:   "/shared/prompts/abc123xyz789",
					ShareType:  "public",
					CreatedAt:  now,
				}, nil)
		},
		expectedStatus: http.StatusOK,
		checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
			var resp models.ShareResponse
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, "public", resp.ShareType)
			assert.NotEmpty(t, resp.ShareToken)
			assert.NotEmpty(t, resp.ShareURL)
		},
	},
	{
		name: "Successfully create restricted share with emails",
		request: map[string]interface{}{
			"share_type": "restricted",
			"emails":     []string{"alice@example.com", "bob@example.com"},
		},
		setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface) {
			now := time.Now()
			mockService.EXPECT().
				CreateShare("test-user", "test-prompt", mock.MatchedBy(func(req *models.CreateShareRequest) bool {
					return req.ShareType == "restricted" && len(req.Emails) == 2
				})).
				Return(&models.ShareResponse{
					ShareToken: "abc123xyz789",
					ShareURL:   "/shared/prompts/abc123xyz789",
					ShareType:  "restricted",
					Emails:     []string{"alice@example.com", "bob@example.com"},
					CreatedAt:  now,
				}, nil)
		},
		expectedStatus: http.StatusOK,
		checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
			var resp models.ShareResponse
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, "restricted", resp.ShareType)
			assert.Equal(t, 2, len(resp.Emails))
		},
	},
	{
		name: "Fail when prompt not found",
		request: map[string]interface{}{
			"share_type": "public",
		},
		setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface) {
			mockService.EXPECT().
				CreateShare("test-user", "test-prompt", mock.Anything).
				Return(nil, fmt.Errorf("prompt not found"))
		},
		expectedStatus: http.StatusNotFound,
	},
}

func TestCreatePromptShare_Integration(t *testing.T) {
	for _, tt := range createPromptShareTests {
		t.Run(tt.name, func(t *testing.T) {
			runPromptShareTest(t, promptShareTestParams{
				method:         "POST",
				url:            "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-prompt/share",
				slug:           "test-prompt",
				body:           tt.request,
				userID:         "test-user",
				mockSetup:      tt.setupMocks,
				expectedStatus: tt.expectedStatus,
				checkResponse:  tt.checkResponse,
			})
		})
	}
}

func TestGetPromptShare_Integration(t *testing.T) {
	tests := []struct {
		name           string
		slug           string
		setupMocks     func(*svcmocks.MockPromptShareServiceInterface)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "Successfully get share",
			slug: "test-prompt",
			setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface) {
				now := time.Now()
				mockService.EXPECT().
					GetShare("test-user", "test-prompt").
					Return(&models.ShareResponse{
						ShareToken: "abc123xyz789",
						ShareURL:   "/shared/prompts/abc123xyz789",
						ShareType:  "public",
						CreatedAt:  now,
					}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp models.ShareResponse
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				assert.NoError(t, err)
				assert.NotEmpty(t, resp.ShareToken)
			},
		},
		{
			name: "Fail when share not found",
			slug: "nonexistent",
			setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface) {
				mockService.EXPECT().
					GetShare("test-user", "nonexistent").
					Return(nil, fmt.Errorf("share not found"))
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runPromptShareTest(t, promptShareTestParams{
				method:         "GET",
				url:            fmt.Sprintf("/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/%s/share", tt.slug),
				slug:           tt.slug,
				userID:         "test-user",
				mockSetup:      tt.setupMocks,
				expectedStatus: tt.expectedStatus,
				checkResponse:  tt.checkResponse,
			})
		})
	}
}

func TestDeletePromptShare_Integration(t *testing.T) {
	tests := []struct {
		name           string
		slug           string
		setupMocks     func(*svcmocks.MockPromptShareServiceInterface)
		expectedStatus int
	}{
		{
			name: "Successfully delete share",
			slug: "test-prompt",
			setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface) {
				mockService.EXPECT().
					DeleteShare("test-user", "test-prompt").
					Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name: "Fail when share not found",
			slug: "nonexistent",
			setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface) {
				mockService.EXPECT().
					DeleteShare("test-user", "nonexistent").
					Return(fmt.Errorf("share not found"))
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runPromptShareTest(t, promptShareTestParams{
				method:         "DELETE",
				url:            fmt.Sprintf("/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/%s/share", tt.slug),
				slug:           tt.slug,
				userID:         "test-user",
				mockSetup:      tt.setupMocks,
				expectedStatus: tt.expectedStatus,
			})
		})
	}
}

func runSharedPromptTest(
	t *testing.T,
	token string,
	addAuth bool,
	setupMocks func(*svcmocks.MockPromptShareServiceInterface, *mockUserRepository),
	expectedStatus int,
	checkResponse func(*testing.T, *httptest.ResponseRecorder),
) {
	t.Helper()
	mockService := svcmocks.NewMockPromptShareServiceInterface(t)
	mockUserRepo := &mockUserRepository{}
	mockContainer := &MockPromptShareContainer{
		promptShareService: mockService,
		userRepository:     mockUserRepo,
	}

	setupMocks(mockService, mockUserRepo)

	cfg := &config.Config{}
	srv := newTestServer(t, mockContainer, cfg)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/shared/prompts/%s", token), nil)

	// Add chi URL params
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", token)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Add user context if auth is required
	if addAuth {
		req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "test-user"))
	}

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, expectedStatus, rr.Code)
	if checkResponse != nil {
		checkResponse(t, rr)
	}
}

var sharedPromptTests = []struct {
	name           string
	token          string
	addAuth        bool
	setupMocks     func(*svcmocks.MockPromptShareServiceInterface, *mockUserRepository)
	expectedStatus int
	checkResponse  func(*testing.T, *httptest.ResponseRecorder)
}{
	{
		name:    "Successfully get public shared prompt without auth",
		token:   "abc123xyz789",
		addAuth: false,
		setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface, userRepo *mockUserRepository) {
			mockService.EXPECT().
				GetSharedPrompt("abc123xyz789", (*string)(nil)).
				Return(&models.SharedPromptResponse{
					Prompt: models.Prompt{
						ID:   "prompt-123",
						Name: "Test Prompt",
						Slug: "test-prompt",
						Body: "Hello {{name}}",
					},
					ShareType:    "public",
					RenderedBody: "Hello {{name}}",
				}, nil)
		},
		expectedStatus: http.StatusOK,
		checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
			var resp models.SharedPromptResponse
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, "public", resp.ShareType)
			assert.Equal(t, "Test Prompt", resp.Prompt.Name)
		},
	},
	{
		name:    "Successfully get restricted shared prompt with auth",
		token:   "restricted123",
		addAuth: true,
		setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface, userRepo *mockUserRepository) {
			userRepo.On("GetByID", mock.Anything, "test-user").
				Return(&models.User{
					ID:    "test-user",
					Email: "test@example.com",
				}, nil)

			mockService.EXPECT().
				GetSharedPrompt("restricted123", mock.MatchedBy(func(email *string) bool {
					return email != nil && *email == "test@example.com"
				})).
				Return(&models.SharedPromptResponse{
					Prompt: models.Prompt{
						ID:   "prompt-123",
						Name: "Restricted Prompt",
						Slug: "restricted-prompt",
						Body: "Secret content",
					},
					ShareType:    "restricted",
					RenderedBody: "Secret content",
				}, nil)
		},
		expectedStatus: http.StatusOK,
		checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
			var resp models.SharedPromptResponse
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, "restricted", resp.ShareType)
			assert.Equal(t, "Restricted Prompt", resp.Prompt.Name)
		},
	},
	{
		name:    "Fail when share not found",
		token:   "nonexistent",
		addAuth: false,
		setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface, userRepo *mockUserRepository) {
			mockService.EXPECT().
				GetSharedPrompt("nonexistent", (*string)(nil)).
				Return(nil, fmt.Errorf("shared prompt not found"))
		},
		expectedStatus: http.StatusNotFound,
	},
	{
		name:    "Fail when access denied",
		token:   "restricted123",
		addAuth: false,
		setupMocks: func(mockService *svcmocks.MockPromptShareServiceInterface, userRepo *mockUserRepository) {
			mockService.EXPECT().
				GetSharedPrompt("restricted123", (*string)(nil)).
				Return(nil, fmt.Errorf("authentication required"))
		},
		expectedStatus: http.StatusUnauthorized,
	},
}

func TestGetSharedPrompt_Integration(t *testing.T) {
	for _, tt := range sharedPromptTests {
		t.Run(tt.name, func(t *testing.T) {
			runSharedPromptTest(
				t,
				tt.token,
				tt.addAuth,
				tt.setupMocks,
				tt.expectedStatus,
				tt.checkResponse,
			)
		})
	}
}

// newTestServer creates a test server with the provided mock container
func newTestServer(t *testing.T, mockContainer *MockPromptShareContainer, cfg *config.Config) *Server {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)

	// Initialize router manually for testing
	r := chi.NewRouter()

	srv := &Server{
		port:      "8080",
		container: mockContainer,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	// Register prompt sharing routes manually for testing
	r.Route(
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
		func(r chi.Router) {
			r.Post("/{slug}/share", srv.handleCreatePromptShare)
			r.Get("/{slug}/share", srv.handleGetPromptShare)
			r.Delete("/{slug}/share", srv.handleDeletePromptShare)
		},
	)

	// Register public shared prompt route
	r.Route("/api/v1/shared/prompts", func(r chi.Router) {
		r.Get("/{token}", srv.handleGetSharedPrompt)
	})

	return srv
}

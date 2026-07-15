package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// MockTeamContainer implements Container interface for team handler tests
type MockTeamContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	teamService          *svcmocks.MockTeamServiceInterface
	resourceUsageService *svcmocks.MockResourceUsageServiceInterface
	projectService       *svcmocks.MockProjectServiceInterface
	activityService      *MockActivityServiceForTeamHandlers
	environmentService   *services.EnvironmentService
	userRepository       *MockUserRepository
}

func (m *MockTeamContainer) ProjectService() services.ProjectServiceInterface {
	if m.projectService == nil {
		return nil
	}
	return m.projectService
}

func (m *MockTeamContainer) TeamService() services.TeamServiceInterface {
	return m.teamService
}

func (m *MockTeamContainer) TeamInvitationService() *services.TeamInvitationService {
	// Note: TeamInvitationService is a concrete type, not an interface.
	// Team handlers (handleCreateTeam, handleListTeams, etc.) do not use this service,
	// only invitation-specific handlers use it. Returning nil is safe for these tests.
	// If future tests require invitation functionality, initialize in newMockTeamContainer.
	return nil
}

func (m *MockTeamContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.resourceUsageService
}

func (m *MockTeamContainer) ActivityService() activities.ActivityService {
	return m.activityService
}

func (m *MockTeamContainer) EnvironmentService() *services.EnvironmentService {
	return m.environmentService
}

// MockUserRepository is a mock implementation for UserRepository
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) Create(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) GetByGoogleID(ctx context.Context, googleID string) (*models.User, error) {
	args := m.Called(ctx, googleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) Update(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// MockActivityServiceForTeamHandlers is a mock implementation for ActivityService
type MockActivityServiceForTeamHandlers struct {
	mock.Mock
}

func (m *MockActivityServiceForTeamHandlers) DeleteActivity(ctx context.Context, activityID string) error {
	args := m.Called(ctx, activityID)
	return args.Error(0)
}

func (m *MockActivityServiceForTeamHandlers) GetActivityTypes() []string {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]string)
}

func (m *MockActivityServiceForTeamHandlers) GetEntityTypes() []string {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]string)
}

func (m *MockActivityServiceForTeamHandlers) GetAllTypes() *activities.ActivityTypesResponse {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*activities.ActivityTypesResponse)
}

func (m *MockActivityServiceForTeamHandlers) GetActivities(
	ctx context.Context, filters activities.ActivityFilters,
) (*activities.ActivityListResponse, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.ActivityListResponse), args.Error(1)
}

func (m *MockActivityServiceForTeamHandlers) GetActivityStats(
	ctx context.Context, userID string,
) (*activities.ActivityStatsResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.ActivityStatsResponse), args.Error(1)
}

func (m *MockActivityServiceForTeamHandlers) GetActivityByID(
	ctx context.Context, userID string, activityID string,
) (*activities.Activity, error) {
	args := m.Called(ctx, userID, activityID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.Activity), args.Error(1)
}

func (m *MockActivityServiceForTeamHandlers) RecordActivity(
	ctx context.Context, userID string, req activities.CreateActivityRequest,
) (*activities.Activity, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.Activity), args.Error(1)
}

func (m *MockActivityServiceForTeamHandlers) RecordAuthActivity(
	ctx context.Context, userID string, activityType string, sessionID *string,
	metadata map[string]interface{}, sourceIP *string, userAgent *string,
) error {
	args := m.Called(ctx, userID, activityType, sessionID, metadata, sourceIP, userAgent)
	return args.Error(0)
}

func (m *MockActivityServiceForTeamHandlers) RecordResourceActivity(
	ctx context.Context, userID string, activityType string, entityType string,
	entityID *string, description string, metadata map[string]interface{},
) error {
	args := m.Called(ctx, userID, activityType, entityType, entityID, description, metadata)
	return args.Error(0)
}

func (m *MockActivityServiceForTeamHandlers) RecordClaudeCodeActivity(
	ctx context.Context, userID string, sessionID string, toolName *string,
	hookEventName string, metadata map[string]interface{},
) error {
	args := m.Called(ctx, userID, sessionID, toolName, hookEventName, metadata)
	return args.Error(0)
}

func (m *MockActivityServiceForTeamHandlers) RunRetentionJob(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func newMockTeamContainer(t *testing.T) *MockTeamContainer {
	cfg := &config.Config{}
	return &MockTeamContainer{
		teamService:          svcmocks.NewMockTeamServiceInterface(t),
		resourceUsageService: svcmocks.NewMockResourceUsageServiceInterface(t),
		projectService:       svcmocks.NewMockProjectServiceInterface(t),
		activityService:      &MockActivityServiceForTeamHandlers{},
		environmentService:   services.NewEnvironmentService(cfg),
		userRepository:       &MockUserRepository{},
	}
}

func createTestTeamServer(container *MockTeamContainer) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	// Initialize router manually for testing
	r := chi.NewRouter()

	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	// Register team routes
	r.Route("/api/v1/teams", func(r chi.Router) {
		r.Post("/", srv.handleCreateTeam)
		r.Get("/", srv.handleListTeams)
		r.Get("/{id}", srv.handleGetTeam)
		r.Put("/{id}", srv.handleUpdateTeam)
		r.Delete("/{id}", srv.handleDeleteTeam)

		// Member management
		r.Get("/{id}/members", srv.handleGetTeamMembers)
		r.Delete("/{id}/members/{userId}", srv.handleRemoveTeamMember)
	})

	return srv
}

// TestHandleListTeams_Success tests successful team listing
func TestHandleListTeams_Success(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	teams := []models.Team{
		{
			ID:          "team-1",
			Name:        "Team Alpha",
			Description: "First team",
			IsPersonal:  false,
			Role:        "owner",
			MemberCount: 5,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          "team-2",
			Name:        "Team Beta",
			Description: "Second team",
			IsPersonal:  false,
			Role:        "member",
			MemberCount: 3,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	}

	expectedResponse := &models.TeamListResponse{
		Teams:      teams,
		TotalCount: 2,
		Page:       1,
		PageSize:   20,
	}

	mockContainer.teamService.On("ListTeams", mock.Anything, "user-123", 1, 20).
		Return(expectedResponse, nil)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/teams", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.TeamListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.TotalCount)
	assert.Len(t, response.Teams, 2)
	assert.Equal(t, "Team Alpha", response.Teams[0].Name)
	assert.Equal(t, "Team Beta", response.Teams[1].Name)

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleListTeams_EmptyList tests listing when user has no teams
func TestHandleListTeams_EmptyList(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	expectedResponse := &models.TeamListResponse{
		Teams:      []models.Team{},
		TotalCount: 0,
		Page:       1,
		PageSize:   20,
	}

	mockContainer.teamService.On("ListTeams", mock.Anything, "user-456", 1, 20).
		Return(expectedResponse, nil)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/teams", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-456"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.TeamListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 0, response.TotalCount)
	assert.Empty(t, response.Teams)

	mockContainer.teamService.AssertExpectations(t)
}

// TestBootstrapDefaultProject_NilService verifies the bootstrap is a no-op
// (logs and returns) when no project service is configured.
func TestBootstrapDefaultProject_NilService(t *testing.T) {
	mockContainer := newMockTeamContainer(t)
	mockContainer.projectService = nil

	srv := createTestTeamServer(mockContainer)

	srv.bootstrapDefaultProject("user-789", &models.Team{ID: "team-new-123"})
}

// TestHandleCreateTeam_Success tests successful team creation
func TestHandleCreateTeam_Success(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	createReq := models.CreateTeamRequest{
		Name:        "New Team",
		Description: "Team description",
	}

	expectedTeam := &models.Team{
		ID:          "team-new-123",
		Name:        "New Team",
		Description: "Team description",
		IsPersonal:  false,
		Role:        "owner",
		MemberCount: 1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-789", mock.Anything).
		Return(true, nil)

	mockContainer.teamService.On("CreateTeam", mock.Anything, "user-789", &createReq).
		Return(expectedTeam, nil)

	mockContainer.projectService.On("CreateProject", "user-789", "team-new-123", models.DefaultProjectRequest()).
		Return(&models.Project{ID: "project-new-123", Slug: "project-1"}, nil)

	srv := createTestTeamServer(mockContainer)

	reqBody, err := json.Marshal(createReq)
	assert.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/v1/teams", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-789"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response models.Team
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "team-new-123", response.ID)
	assert.Equal(t, "New Team", response.Name)
	assert.Equal(t, "Team description", response.Description)

	mockContainer.teamService.AssertExpectations(t)
	mockContainer.resourceUsageService.AssertExpectations(t)
	mockContainer.projectService.AssertExpectations(t)
}

// TestHandleCreateTeam_DefaultProjectFailure verifies team creation still
// returns 201 with an unchanged response shape when default-project bootstrap
// fails (non-blocking, same semantics as the signup listener).
func TestHandleCreateTeam_DefaultProjectFailure(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	createReq := models.CreateTeamRequest{
		Name:        "New Team",
		Description: "Team description",
	}

	expectedTeam := &models.Team{
		ID:          "team-new-123",
		Name:        "New Team",
		Description: "Team description",
		IsPersonal:  false,
		Role:        "owner",
		MemberCount: 1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-789", mock.Anything).
		Return(true, nil)

	mockContainer.teamService.On("CreateTeam", mock.Anything, "user-789", &createReq).
		Return(expectedTeam, nil)

	mockContainer.projectService.On("CreateProject", "user-789", "team-new-123", models.DefaultProjectRequest()).
		Return(nil, errors.New("project creation failed"))

	srv := createTestTeamServer(mockContainer)

	reqBody, err := json.Marshal(createReq)
	assert.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/v1/teams", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-789"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response models.Team
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "team-new-123", response.ID)
	assert.Equal(t, "New Team", response.Name)
	assert.Equal(t, "Team description", response.Description)

	mockContainer.teamService.AssertExpectations(t)
	mockContainer.resourceUsageService.AssertExpectations(t)
	mockContainer.projectService.AssertExpectations(t)
}

// TestHandleCreateTeam_ValidationError tests validation errors during team creation
func TestHandleCreateTeam_ValidationError(t *testing.T) {
	tests := []struct {
		name          string
		request       models.CreateTeamRequest
		expectedError string
	}{
		{
			name: "Empty name",
			request: models.CreateTeamRequest{
				Name:        "",
				Description: "Valid description",
			},
			expectedError: "Name is required",
		},
		{
			name: "Name too long",
			request: models.CreateTeamRequest{
				Name:        strings.Repeat("a", 101), // 101 characters
				Description: "Valid description",
			},
			expectedError: "Name cannot be longer than 100 characters",
		},
		{
			name: "Description too long",
			request: models.CreateTeamRequest{
				Name:        "Valid Name",
				Description: strings.Repeat("a", 501), // 501 characters
			},
			expectedError: "Description cannot be longer than 500 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockTeamContainer(t)
			srv := createTestTeamServer(mockContainer)

			reqBody, err := json.Marshal(tt.request)
			assert.NoError(t, err)

			req := httptest.NewRequest("POST", "/api/v1/teams", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedError)
		})
	}
}

// TestHandleGetTeam_Success tests successful team retrieval
func TestHandleGetTeam_Success(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	expectedTeam := &models.Team{
		ID:          "team-456",
		Name:        "Existing Team",
		Description: "Team details",
		IsPersonal:  false,
		Role:        "owner",
		MemberCount: 10,
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now(),
	}

	mockContainer.teamService.On("GetTeam", mock.Anything, "user-999", "team-456").
		Return(expectedTeam, nil)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/teams/team-456", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-999"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.Team
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "team-456", response.ID)
	assert.Equal(t, "Existing Team", response.Name)
	assert.Equal(t, 10, response.MemberCount)

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleGetTeam_NotFound tests team retrieval when team doesn't exist
func TestHandleGetTeam_NotFound(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On("GetTeam", mock.Anything, "user-123", "non-existent-team").
		Return((*models.Team)(nil), errors.New("team not found"))

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/teams/non-existent-team", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Team not found")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleUpdateTeam_Success tests successful team update
func TestHandleUpdateTeam_Success(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	newName := "Updated Team Name"
	newDesc := "Updated description"
	updateReq := models.UpdateTeamRequest{
		Name:        &newName,
		Description: &newDesc,
	}

	updatedTeam := &models.Team{
		ID:          "team-789",
		Name:        newName,
		Description: newDesc,
		IsPersonal:  false,
		Role:        "owner",
		MemberCount: 5,
		CreatedAt:   time.Now().Add(-48 * time.Hour),
		UpdatedAt:   time.Now(),
	}

	mockContainer.teamService.On("UpdateTeam", mock.Anything, "user-555", "team-789", &updateReq).
		Return(updatedTeam, nil)

	srv := createTestTeamServer(mockContainer)

	reqBody, err := json.Marshal(updateReq)
	assert.NoError(t, err)

	req := httptest.NewRequest("PUT", "/api/v1/teams/team-789", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-555"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.Team
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "team-789", response.ID)
	assert.Equal(t, newName, response.Name)
	assert.Equal(t, newDesc, response.Description)

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleDeleteTeam_Success tests successful team deletion
func TestHandleDeleteTeam_Success(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On("DeleteTeam", mock.Anything, "user-111", "team-222").
		Return(nil)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-222", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-111"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleGetTeamMembers_Success tests successful team members retrieval
func TestHandleGetTeamMembers_Success(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	members := []models.TeamMemberDetail{
		{
			UserID:   "user-1",
			Email:    "user1@example.com",
			Name:     "User One",
			Role:     "owner",
			JoinedAt: time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02T15:04:05Z"),
		},
		{
			UserID:   "user-2",
			Email:    "user2@example.com",
			Name:     "User Two",
			Role:     "member",
			JoinedAt: time.Now().Add(-15 * 24 * time.Hour).Format("2006-01-02T15:04:05Z"),
		},
	}

	expectedResponse := &models.TeamMembersListResponse{
		Members:    members,
		TotalCount: 2,
		Page:       1,
		PageSize:   100,
	}

	mockContainer.teamService.On("GetTeamMembers", mock.Anything, "user-owner", "team-444", 1, 100).
		Return(expectedResponse, nil)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/teams/team-444/members", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.TeamMembersListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.TotalCount)
	assert.Len(t, response.Members, 2)
	assert.Equal(t, "user1@example.com", response.Members[0].Email)
	assert.Equal(t, "owner", response.Members[0].Role)

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleRemoveTeamMember_Success tests successful team member removal
func TestHandleRemoveTeamMember_Success(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On("RemoveTeamMember", mock.Anything, "user-owner", "team-555", "user-member").
		Return(nil)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-555/members/user-member", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleCreateTeam_ResourceLimitExceeded tests team creation when resource limit is exceeded
func TestHandleCreateTeam_ResourceLimitExceeded(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	createReq := models.CreateTeamRequest{
		Name:        "New Team",
		Description: "Team description",
	}

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-limit", mock.Anything).
		Return(false, nil)

	srv := createTestTeamServer(mockContainer)

	reqBody, err := json.Marshal(createReq)
	assert.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/v1/teams", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-limit"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "RESOURCE_LIMIT_EXCEEDED")

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestHandleListTeams_ServiceError tests error handling when service fails
func TestHandleListTeams_ServiceError(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On("ListTeams", mock.Anything, "user-error", 1, 20).
		Return((*models.TeamListResponse)(nil), errors.New("database connection failed"))

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/teams", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-error"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to list teams")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleGetTeam_ServiceError tests error handling when service returns generic error
func TestHandleGetTeam_ServiceError(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On("GetTeam", mock.Anything, "user-123", "team-error").
		Return((*models.Team)(nil), errors.New("database connection failed"))

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("GET", "/api/v1/teams/team-error", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to get team")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleUpdateTeam_ServiceError tests error handling when service fails
func TestHandleUpdateTeam_ServiceError(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	newName := "Updated Team Name"
	updateReq := models.UpdateTeamRequest{
		Name: &newName,
	}

	mockContainer.teamService.On("UpdateTeam", mock.Anything, "user-123", "team-error", &updateReq).
		Return((*models.Team)(nil), errors.New("database connection failed"))

	srv := createTestTeamServer(mockContainer)

	reqBody, err := json.Marshal(updateReq)
	assert.NoError(t, err)

	req := httptest.NewRequest("PUT", "/api/v1/teams/team-error", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to update team")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleUpdateTeam_Unauthorized tests update when team is not found (unauthorized)
func TestHandleUpdateTeam_Unauthorized(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	newName := "Updated Team Name"
	updateReq := models.UpdateTeamRequest{
		Name: &newName,
	}

	mockContainer.teamService.On("UpdateTeam", mock.Anything, "user-not-owner", "team-789", &updateReq).
		Return((*models.Team)(nil), errors.New("team not found"))

	srv := createTestTeamServer(mockContainer)

	reqBody, err := json.Marshal(updateReq)
	assert.NoError(t, err)

	req := httptest.NewRequest("PUT", "/api/v1/teams/team-789", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-not-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Team not found")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleDeleteTeam_ServiceError tests error handling when service fails with generic error
func TestHandleDeleteTeam_ServiceError(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On("DeleteTeam", mock.Anything, "user-123", "team-error").
		Return(errors.New("database connection failed"))

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-error", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to delete team")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleDeleteTeam_Unauthorized tests delete when user is not authorized
func TestHandleDeleteTeam_Unauthorized(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On("DeleteTeam", mock.Anything, "user-not-owner", "team-789").
		Return(errors.New("team not found"))

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-789", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-not-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Team not found")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleDeleteTeam_HasActiveSubscription tests delete when team has active subscription
func TestHandleDeleteTeam_HasActiveSubscription(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	activeSubErr := &services.ActiveSubscriptionError{
		SubscriptionID:   "sub-123",
		SubscriptionTier: "Professional",
		BillingPortalURL: "https://billing.example.com",
		HelpText:         "Please cancel your subscription first",
	}

	mockContainer.teamService.On("DeleteTeam", mock.Anything, "user-owner", "team-with-sub").
		Return(activeSubErr)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-with-sub", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var body struct {
		Code     string         `json:"code"`
		Detail   string         `json:"detail"`
		Metadata map[string]any `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ACTIVE_SUBSCRIPTION_EXISTS", body.Code)
	assert.NotEmpty(t, body.Detail)
	assert.Equal(t, "sub-123", body.Metadata["subscription_id"])
	assert.IsType(t, "", body.Metadata["billing_portal_url"])
	assert.Equal(t, "https://billing.example.com", body.Metadata["billing_portal_url"])

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleDeleteTeam_SubscriptionCanceling tests delete when the subscription is
// scheduled to cancel — the response must carry cancel_at as a string in metadata.
func TestHandleDeleteTeam_SubscriptionCanceling(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	cancelingErr := services.NewSubscriptionCancelingError("team-canceling", "Jul 4, 2026")

	mockContainer.teamService.On("DeleteTeam", mock.Anything, "user-owner", "team-canceling").
		Return(cancelingErr)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-canceling", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var body struct {
		Code     string         `json:"code"`
		Detail   string         `json:"detail"`
		Metadata map[string]any `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "SUBSCRIPTION_CANCELING", body.Code)
	assert.NotEmpty(t, body.Detail)
	assert.IsType(t, "", body.Metadata["cancel_at"])
	assert.Equal(t, "Jul 4, 2026", body.Metadata["cancel_at"])

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleDeleteTeam_HasMembers tests delete when the team still has members —
// the response must carry member_count as a string in metadata.
func TestHandleDeleteTeam_HasMembers(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	membersErr := services.NewTeamHasMembersError("team-members", 3)

	mockContainer.teamService.On("DeleteTeam", mock.Anything, "user-owner", "team-members").
		Return(membersErr)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-members", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var body struct {
		Code     string         `json:"code"`
		Detail   string         `json:"detail"`
		Metadata map[string]any `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "TEAM_HAS_MEMBERS", body.Code)
	assert.NotEmpty(t, body.Detail)
	assert.Equal(t, "3", body.Metadata["member_count"])

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleDeleteTeam_CannotDeletePersonalWorkspace tests that personal workspace cannot be deleted
func TestHandleDeleteTeam_CannotDeletePersonalWorkspace(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	cannotDeleteErr := &services.CannotDeletePersonalWorkspaceError{
		TeamID: "team-personal",
	}

	mockContainer.teamService.On("DeleteTeam", mock.Anything, "user-owner", "team-personal").
		Return(cannotDeleteErr)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-personal", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Cannot delete personal workspace")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleRemoveTeamMember_NotFound tests member removal when team or member not found
func TestHandleRemoveTeamMember_NotFound(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On("RemoveTeamMember", mock.Anything, "user-owner", "team-nonexistent", "user-member").
		Return(errors.New("team not found"))

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-nonexistent/members/user-member", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Team or member not found")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleRemoveTeamMember_CannotRemoveOwner tests that team owner cannot be removed
func TestHandleRemoveTeamMember_CannotRemoveOwner(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	// Stub the sentinel the service actually returns: the handler matches it with
	// errors.Is rather than sniffing the message, so a bare error would 500.
	mockContainer.teamService.On("RemoveTeamMember", mock.Anything, "user-admin", "team-555", "user-owner").
		Return(services.ErrCannotRemoveTeamOwner)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-555/members/user-owner", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-admin"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Cannot remove team owner")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleUpdateTeam_NonOwner_Returns403 tests that a non-owner gets 403 on update
func TestHandleUpdateTeam_NonOwner_Returns403(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	newName := "Attempted Update"
	updateReq := models.UpdateTeamRequest{
		Name: &newName,
	}

	mockContainer.teamService.On("UpdateTeam", mock.Anything, "user-non-owner", "team-owned-by-other", &updateReq).
		Return((*models.Team)(nil), services.ErrPermissionDenied)

	srv := createTestTeamServer(mockContainer)

	reqBody, err := json.Marshal(updateReq)
	assert.NoError(t, err)

	req := httptest.NewRequest("PUT", "/api/v1/teams/team-owned-by-other", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-non-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Only team owners and admins can update a team")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleDeleteTeam_NonOwner_Returns403 tests that a non-owner gets 403 on delete
func TestHandleDeleteTeam_NonOwner_Returns403(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On("DeleteTeam", mock.Anything, "user-non-owner", "team-owned-by-other").
		Return(services.ErrPermissionDenied)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-owned-by-other", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-non-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Only the team owner can delete a team")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleRemoveTeamMember_NonOwner_Returns403 tests that a non-owner gets 403 when removing a member
func TestHandleRemoveTeamMember_NonOwner_Returns403(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	mockContainer.teamService.On(
		"RemoveTeamMember", mock.Anything, "user-non-owner", "team-owned-by-other", "user-member",
	).Return(services.ErrPermissionDenied)

	srv := createTestTeamServer(mockContainer)
	req := httptest.NewRequest("DELETE", "/api/v1/teams/team-owned-by-other/members/user-member", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-non-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Only team owners and admins can remove members")

	mockContainer.teamService.AssertExpectations(t)
}

// TestHandleUpdateTeam_RepoFailure_Returns500 tests that a genuine repo failure returns 500
func TestHandleUpdateTeam_RepoFailure_Returns500(t *testing.T) {
	mockContainer := newMockTeamContainer(t)

	newName := "Some Name"
	updateReq := models.UpdateTeamRequest{
		Name: &newName,
	}

	mockContainer.teamService.On("UpdateTeam", mock.Anything, "user-owner", "team-repo-fail", &updateReq).
		Return((*models.Team)(nil), errors.New("pq: connection refused"))

	srv := createTestTeamServer(mockContainer)

	reqBody, err := json.Marshal(updateReq)
	assert.NoError(t, err)

	req := httptest.NewRequest("PUT", "/api/v1/teams/team-repo-fail", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-owner"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to update team")

	mockContainer.teamService.AssertExpectations(t)
}

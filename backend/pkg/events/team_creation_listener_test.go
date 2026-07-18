package events

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// MockTeamCreatorService is a mock implementation of DefaultTeamCreator
type MockTeamCreatorService struct {
	mock.Mock
}

func (m *MockTeamCreatorService) CreateDefaultTeam(ctx context.Context, userID string) (*models.Team, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

// MockProjectCreatorService is a mock implementation of ProjectCreator
type MockProjectCreatorService struct {
	mock.Mock
}

func (m *MockProjectCreatorService) CreateProject(
	userID, teamID string, req *models.CreateProjectRequest,
) (*models.Project, error) {
	args := m.Called(userID, teamID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Project), args.Error(1)
}

func TestNewTeamCreationListener(t *testing.T) {
	tests := []struct {
		name           string
		teamService    DefaultTeamCreator
		projectService ProjectCreator
		logger         *slog.Logger
	}{
		{
			name:           "with all dependencies",
			teamService:    &MockTeamCreatorService{},
			projectService: &MockProjectCreatorService{},
			logger:         slog.New(slog.DiscardHandler),
		},
		{
			name:           "with nil logger creates default",
			teamService:    &MockTeamCreatorService{},
			projectService: &MockProjectCreatorService{},
			logger:         nil,
		},
		{
			name:           "with nil project service",
			teamService:    &MockTeamCreatorService{},
			projectService: nil,
			logger:         slog.New(slog.DiscardHandler),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener := NewTeamCreationListener(tt.teamService, tt.projectService, tt.logger)

			assert.NotNil(t, listener)
			assert.NotNil(t, listener.logger)
			assert.Equal(t, tt.teamService, listener.teamService)
			assert.Equal(t, tt.projectService, listener.projectService)
		})
	}
}

func TestTeamCreationListener_EventTypes(t *testing.T) {
	listener := NewTeamCreationListener(
		&MockTeamCreatorService{}, &MockProjectCreatorService{}, slog.New(slog.DiscardHandler),
	)

	eventTypes := listener.EventTypes()

	assert.Len(t, eventTypes, 1)
	assert.Contains(t, eventTypes, EventTypeUserCreated)
}

func TestTeamCreationListener_Handle_SuccessfulCreation(t *testing.T) {
	mockTeamSvc := &MockTeamCreatorService{}
	mockProjectSvc := &MockProjectCreatorService{}
	logger := slog.New(slog.DiscardHandler)

	team := &models.Team{
		ID:      "team-456",
		OwnerID: "user-123",
		Name:    "Private Workspace",
		Slug:    "private-workspace",
	}
	mockTeamSvc.On("CreateDefaultTeam", mock.Anything, "user-123").Return(team, nil)

	project := &models.Project{
		ID:          "project-789",
		UserID:      "user-123",
		TeamID:      "team-456",
		Name:        "Project 1",
		Slug:        "project-1",
		Description: "Your first project - rename or customize as needed",
	}
	mockProjectSvc.On("CreateProject", "user-123", "team-456", mock.MatchedBy(func(req *models.CreateProjectRequest) bool {
		return req.Name == "Project 1" && req.Slug == "project-1"
	})).Return(project, nil)

	listener := NewTeamCreationListener(mockTeamSvc, mockProjectSvc, logger)
	event := NewUserCreatedEvent("user-123", "test@example.com", "Test User", time.Now())

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	mockTeamSvc.AssertCalled(t, "CreateDefaultTeam", mock.Anything, "user-123")
	mockProjectSvc.AssertCalled(t, "CreateProject", mock.Anything, mock.Anything, mock.Anything)
}

func TestTeamCreationListener_Handle_TeamCreationFails(t *testing.T) {
	mockTeamSvc := &MockTeamCreatorService{}
	mockProjectSvc := &MockProjectCreatorService{}
	logger := slog.New(slog.DiscardHandler)

	mockTeamSvc.On("CreateDefaultTeam", mock.Anything, "user-123").
		Return(nil, errors.New("database error"))

	listener := NewTeamCreationListener(mockTeamSvc, mockProjectSvc, logger)
	event := NewUserCreatedEvent("user-123", "test@example.com", "Test User", time.Now())

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err) // Non-blocking
	mockTeamSvc.AssertCalled(t, "CreateDefaultTeam", mock.Anything, "user-123")
	mockProjectSvc.AssertNotCalled(t, "CreateProject", mock.Anything, mock.Anything, mock.Anything)
}

func TestTeamCreationListener_Handle_ProjectCreationFails(t *testing.T) {
	mockTeamSvc := &MockTeamCreatorService{}
	mockProjectSvc := &MockProjectCreatorService{}
	logger := slog.New(slog.DiscardHandler)

	team := &models.Team{ID: "team-456", OwnerID: "user-123", Name: "Private Workspace"}
	mockTeamSvc.On("CreateDefaultTeam", mock.Anything, "user-123").Return(team, nil)
	mockProjectSvc.On("CreateProject", "user-123", "team-456", mock.Anything).
		Return(nil, errors.New("project creation failed"))

	listener := NewTeamCreationListener(mockTeamSvc, mockProjectSvc, logger)
	event := NewUserCreatedEvent("user-123", "test@example.com", "Test User", time.Now())

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err) // Non-blocking
	mockTeamSvc.AssertCalled(t, "CreateDefaultTeam", mock.Anything, "user-123")
	mockProjectSvc.AssertCalled(t, "CreateProject", mock.Anything, mock.Anything, mock.Anything)
}

func TestTeamCreationListener_Handle_WrongEventType(t *testing.T) {
	mockTeamSvc := &MockTeamCreatorService{}
	mockProjectSvc := &MockProjectCreatorService{}
	logger := slog.New(slog.DiscardHandler)

	listener := NewTeamCreationListener(mockTeamSvc, mockProjectSvc, logger)
	event := NewPromptCreatedEvent(PromptCreatedPayload{
		PromptID:    "prompt-123",
		UserID:      "user-123",
		Email:       "",
		ProjectName: "",
		Slug:        "",
		Title:       "",
		Description: "",
		Body:        "",
		CreatedAt:   time.Now(),
	})

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	mockTeamSvc.AssertNotCalled(t, "CreateDefaultTeam", mock.Anything, mock.Anything)
	mockProjectSvc.AssertNotCalled(t, "CreateProject", mock.Anything, mock.Anything, mock.Anything)
}

func TestTeamCreationListener_Handle_InvalidPayload(t *testing.T) {
	mockTeamSvc := &MockTeamCreatorService{}
	mockProjectSvc := &MockProjectCreatorService{}
	logger := slog.New(slog.DiscardHandler)

	listener := NewTeamCreationListener(mockTeamSvc, mockProjectSvc, logger)
	event := &BaseEvent{
		EventType:      EventTypeUserCreated,
		EventPayload:   "invalid-payload",
		EventTimestamp: time.Now(),
	}

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	mockTeamSvc.AssertNotCalled(t, "CreateDefaultTeam", mock.Anything, mock.Anything)
	mockProjectSvc.AssertNotCalled(t, "CreateProject", mock.Anything, mock.Anything, mock.Anything)
}

func TestTeamCreationListener_Handle_NilProjectService(t *testing.T) {
	mockTeamSvc := &MockTeamCreatorService{}
	logger := slog.New(slog.DiscardHandler)

	team := &models.Team{
		ID:      "team-456",
		OwnerID: "user-123",
		Name:    "Private Workspace",
	}
	mockTeamSvc.On("CreateDefaultTeam", mock.Anything, "user-123").Return(team, nil)

	// Create listener without project service
	listener := NewTeamCreationListener(mockTeamSvc, nil, logger)

	event := NewUserCreatedEvent("user-123", "test@example.com", "Test User", time.Now())

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	mockTeamSvc.AssertExpectations(t)
	// Project creation should be skipped gracefully
}

func TestTeamCreationListener_DefaultProjectAttributes(t *testing.T) {
	// Test that the default project has the correct attributes
	mockTeamSvc := &MockTeamCreatorService{}
	mockProjectSvc := &MockProjectCreatorService{}
	logger := slog.New(slog.DiscardHandler)

	team := &models.Team{
		ID:      "team-456",
		OwnerID: "user-123",
		Name:    "Private Workspace",
	}
	mockTeamSvc.On("CreateDefaultTeam", mock.Anything, "user-123").Return(team, nil)

	// Capture the project request to verify attributes
	var capturedReq *models.CreateProjectRequest
	mockProjectSvc.On("CreateProject", "user-123", "team-456", mock.Anything).
		Run(func(args mock.Arguments) {
			capturedReq = args.Get(2).(*models.CreateProjectRequest)
		}).
		Return(&models.Project{
			ID:     "project-789",
			Name:   "Project 1",
			Slug:   "project-1",
			UserID: "user-123",
			TeamID: "team-456",
		}, nil)

	listener := NewTeamCreationListener(mockTeamSvc, mockProjectSvc, logger)
	event := NewUserCreatedEvent("user-123", "test@example.com", "Test User", time.Now())

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)

	// Verify the default project attributes
	require.NotNil(t, capturedReq)
	assert.Equal(t, "Project 1", capturedReq.Name)
	assert.Equal(t, "project-1", capturedReq.Slug)
	assert.Equal(t, "Your first project - rename or customize as needed", capturedReq.Description)
	assert.Empty(t, capturedReq.GitURL)
	assert.Empty(t, capturedReq.Homepage)

	mockTeamSvc.AssertExpectations(t)
	mockProjectSvc.AssertExpectations(t)
}

func TestTeamCreationListener_ProjectAssociatedWithCorrectTeam(t *testing.T) {
	// Test that the project is created with the correct team_id
	mockTeamSvc := &MockTeamCreatorService{}
	mockProjectSvc := &MockProjectCreatorService{}
	logger := slog.New(slog.DiscardHandler)

	expectedTeamID := "private-workspace-team-id-abc123"
	team := &models.Team{
		ID:      expectedTeamID,
		OwnerID: "user-123",
		Name:    "Private Workspace",
	}
	mockTeamSvc.On("CreateDefaultTeam", mock.Anything, "user-123").Return(team, nil)

	// Verify project is created with the correct team_id
	mockProjectSvc.On("CreateProject", "user-123", expectedTeamID, mock.Anything).
		Return(&models.Project{
			ID:     "project-789",
			TeamID: expectedTeamID,
		}, nil)

	listener := NewTeamCreationListener(mockTeamSvc, mockProjectSvc, logger)
	event := NewUserCreatedEvent("user-123", "test@example.com", "Test User", time.Now())

	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
	mockTeamSvc.AssertExpectations(t)
	mockProjectSvc.AssertExpectations(t)
}

func TestTeamCreationListener_NonBlockingBehavior(t *testing.T) {
	// Test that errors don't propagate (non-blocking behavior)
	tests := []struct {
		name       string
		setupMocks func(*MockTeamCreatorService, *MockProjectCreatorService)
	}{
		{
			name: "team creation error is non-blocking",
			setupMocks: func(teamSvc *MockTeamCreatorService, projectSvc *MockProjectCreatorService) {
				teamSvc.On("CreateDefaultTeam", mock.Anything, mock.Anything).
					Return(nil, errors.New("database connection failed"))
			},
		},
		{
			name: "project creation error is non-blocking",
			setupMocks: func(teamSvc *MockTeamCreatorService, projectSvc *MockProjectCreatorService) {
				teamSvc.On("CreateDefaultTeam", mock.Anything, mock.Anything).
					Return(&models.Team{ID: "team-123"}, nil)
				projectSvc.On("CreateProject", mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("project slug already exists"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTeamSvc := &MockTeamCreatorService{}
			mockProjectSvc := &MockProjectCreatorService{}
			logger := slog.New(slog.DiscardHandler)

			tt.setupMocks(mockTeamSvc, mockProjectSvc)

			listener := NewTeamCreationListener(mockTeamSvc, mockProjectSvc, logger)
			event := NewUserCreatedEvent("user-123", "test@example.com", "Test User", time.Now())

			err := listener.Handle(context.Background(), event)

			// Must always return nil to avoid retry storms
			require.NoError(t, err)
		})
	}
}

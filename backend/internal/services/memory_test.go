package services

import (
	"fmt"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/pkg/events"
	event_mocks "github.com/vibexp/vibexp/pkg/events/mocks"
)

func createTestMemoryService(repo repositories.MemoryRepository) *MemoryService {
	return NewMemoryService(repo, nil, allowAllAuthz{}, nil, func() *slog.Logger { l, _ := logtest.New(); return l }(), nil, nil)
}

const testServiceProjectID = "550e8400-e29b-41d4-a716-446655440002"

func createTestMemory() *models.Memory {
	now := time.Now()
	metadata := map[string]interface{}{
		"category": "work",
		"priority": "high",
		"tags":     []string{"important", "project"},
	}
	return &models.Memory{
		ID:        "memory-123",
		UserID:    "user-123",
		ProjectID: testServiceProjectID,
		Text:      "This is a test memory for unit testing",
		Metadata:  metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func createTestCreateMemoryRequest() *models.CreateMemoryRequest {
	metadata := map[string]interface{}{
		"category": "work",
		"priority": "high",
	}
	return &models.CreateMemoryRequest{
		ProjectID: testServiceProjectID,
		Text:      "This is a new memory",
		Metadata:  metadata,
	}
}

func createTestUpdateMemoryRequest() *models.UpdateMemoryRequest {
	text := "Updated memory text"
	metadata := map[string]interface{}{
		"category": "personal",
		"priority": "medium",
	}
	return &models.UpdateMemoryRequest{
		Text:     &text,
		Metadata: metadata,
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestMemoryService_CreateMemory(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		request    *models.CreateMemoryRequest
		setupMock  func(*mocks.MockMemoryRepository)
		expectErr  bool
		errMessage string
	}{
		{
			name:    "successful creation",
			userID:  "user-123",
			request: createTestCreateMemoryRequest(),
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				mockRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(memory *models.Memory) bool {
					return memory.UserID == "user-123" &&
						memory.ProjectID == testServiceProjectID &&
						memory.Text == "This is a new memory" &&
						memory.Metadata != nil &&
						!memory.CreatedAt.IsZero() &&
						!memory.UpdatedAt.IsZero()
				})).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:    "creation with nil metadata",
			userID:  "user-123",
			request: &models.CreateMemoryRequest{ProjectID: testServiceProjectID, Text: "Simple memory"},
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				mockRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(memory *models.Memory) bool {
					return memory.UserID == "user-123" &&
						memory.ProjectID == testServiceProjectID &&
						memory.Text == "Simple memory" &&
						memory.Metadata != nil // Should be initialized as empty map
				})).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:    "repository error",
			userID:  "user-123",
			request: createTestCreateMemoryRequest(),
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				mockRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(fmt.Errorf("database error")).Once()
			},
			expectErr:  true,
			errMessage: "database error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockMemoryRepository(t)
			tt.setupMock(mockRepo)

			service := createTestMemoryService(mockRepo)
			result, err := service.CreateMemory(tt.userID, "team-123", tt.request)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.userID, result.UserID)
				assert.Equal(t, tt.request.ProjectID, result.ProjectID)
				assert.Equal(t, tt.request.Text, result.Text)
				assert.NotZero(t, result.CreatedAt)
				assert.NotZero(t, result.UpdatedAt)
			}
		})
	}
}

func TestMemoryService_GetMemory(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		memoryID   string
		setupMock  func(*mocks.MockMemoryRepository)
		expected   *models.Memory
		expectErr  bool
		errMessage string
	}{
		{
			name:     "successful get",
			userID:   "user-123",
			memoryID: "memory-123",
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				memory := createTestMemory()
				mockRepo.EXPECT().GetByID(mock.Anything, "user-123", mock.Anything, "memory-123").Return(memory, nil).Once()
			},
			expected:  createTestMemory(),
			expectErr: false,
		},
		{
			name:     "memory not found",
			userID:   "user-123",
			memoryID: "nonexistent",
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				mockRepo.EXPECT().
					GetByID(mock.Anything, "user-123", mock.Anything, "nonexistent").
					Return(nil, fmt.Errorf("memory not found")).Once()
			},
			expected:   nil,
			expectErr:  true,
			errMessage: "memory not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockMemoryRepository(t)
			tt.setupMock(mockRepo)

			service := createTestMemoryService(mockRepo)
			result, err := service.GetMemory(tt.userID, "team-123", tt.memoryID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected.ID, result.ID)
				assert.Equal(t, tt.expected.Text, result.Text)
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestMemoryService_ListMemories(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		filters     MemoryFilters
		setupMock   func(*mocks.MockMemoryRepository)
		expectErr   bool
		errMessage  string
		expectCount int
	}{
		{
			name:   "successful list with default pagination",
			userID: "user-123",
			filters: MemoryFilters{
				UserID: "user-123",
			},
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				memories := []models.Memory{*createTestMemory()}
				mockRepo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(filters repositories.MemoryFilters) bool {
					return filters.Page == 1 && filters.Limit == 50
				})).Return(memories, 1, nil).Once()
			},
			expectErr:   false,
			expectCount: 1,
		},
		{
			name:   "list with custom pagination",
			userID: "user-123",
			filters: MemoryFilters{
				UserID: "user-123",
				Page:   2,
				Limit:  25,
			},
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				memories := []models.Memory{}
				mockRepo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(filters repositories.MemoryFilters) bool {
					return filters.Page == 2 && filters.Limit == 25
				})).Return(memories, 0, nil).Once()
			},
			expectErr:   false,
			expectCount: 0,
		},
		{
			name:   "list with search filter",
			userID: "user-123",
			filters: MemoryFilters{
				UserID: "user-123",
				Search: "test",
				Page:   1,
				Limit:  50,
			},
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				memories := []models.Memory{*createTestMemory()}
				mockRepo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(filters repositories.MemoryFilters) bool {
					return filters.Search == "test"
				})).Return(memories, 1, nil).Once()
			},
			expectErr:   false,
			expectCount: 1,
		},
		{
			name:   "repository error",
			userID: "user-123",
			filters: MemoryFilters{
				UserID: "user-123",
			},
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				mockRepo.EXPECT().List(mock.Anything, "user-123", mock.Anything).Return(nil, 0, fmt.Errorf("database error")).Once()
			},
			expectErr:  true,
			errMessage: "database error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockMemoryRepository(t)
			tt.setupMock(mockRepo)

			service := createTestMemoryService(mockRepo)
			result, err := service.ListMemories(tt.userID, tt.filters)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectCount, len(result.Memories))
				assert.Equal(t, tt.expectCount, result.TotalCount)
			}
		})
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestMemoryService_UpdateMemory(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		memoryID   string
		request    *models.UpdateMemoryRequest
		setupMock  func(*mocks.MockMemoryRepository)
		expectErr  bool
		errMessage string
	}{
		{
			name:     "successful update",
			userID:   "user-123",
			memoryID: "memory-123",
			request:  createTestUpdateMemoryRequest(),
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				existingMemory := createTestMemory()
				mockRepo.EXPECT().GetByID(mock.Anything, "user-123", mock.Anything, "memory-123").Return(existingMemory, nil).Once()
				mockRepo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(memory *models.Memory) bool {
					return memory.Text == "Updated memory text" &&
						memory.Metadata["category"] == "personal"
				})).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:     "memory not found",
			userID:   "user-123",
			memoryID: "nonexistent",
			request:  createTestUpdateMemoryRequest(),
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				mockRepo.EXPECT().
					GetByID(mock.Anything, "user-123", mock.Anything, "nonexistent").
					Return(nil, fmt.Errorf("memory not found")).Once()
			},
			expectErr:  true,
			errMessage: "memory not found",
		},
		{
			name:     "update repository error",
			userID:   "user-123",
			memoryID: "memory-123",
			request:  createTestUpdateMemoryRequest(),
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				existingMemory := createTestMemory()
				mockRepo.EXPECT().GetByID(mock.Anything, "user-123", mock.Anything, "memory-123").Return(existingMemory, nil).Once()
				mockRepo.EXPECT().Update(mock.Anything, mock.Anything).Return(fmt.Errorf("update failed")).Once()
			},
			expectErr:  true,
			errMessage: "update failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockMemoryRepository(t)
			tt.setupMock(mockRepo)

			service := createTestMemoryService(mockRepo)
			result, err := service.UpdateMemory(tt.userID, "team-123", tt.memoryID, tt.request)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.request.Text != nil {
					assert.Equal(t, *tt.request.Text, result.Text)
				}
			}
		})
	}
}

func TestMemoryService_DeleteMemory(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		memoryID   string
		setupMock  func(*mocks.MockMemoryRepository)
		expectErr  bool
		errMessage string
	}{
		{
			name:     "successful delete",
			userID:   "user-123",
			memoryID: "memory-123",
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				// Delete now fetches first to learn the memory's owner: members
				// delete only their own, Admin+ delete anyone's.
				mockRepo.EXPECT().GetByID(mock.Anything, "user-123", mock.Anything, "memory-123").
					Return(&models.Memory{ID: "memory-123", UserID: "user-123"}, nil).Once()
				mockRepo.EXPECT().Delete(mock.Anything, "user-123", mock.Anything, "memory-123").Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:     "memory not found",
			userID:   "user-123",
			memoryID: "nonexistent",
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				// The owner-fetch is now what surfaces a missing memory.
				mockRepo.EXPECT().GetByID(
					mock.Anything, "user-123", mock.Anything, "nonexistent",
				).Return(nil, fmt.Errorf("memory not found")).Once()
			},
			expectErr:  true,
			errMessage: "memory not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockMemoryRepository(t)
			tt.setupMock(mockRepo)

			service := createTestMemoryService(mockRepo)
			err := service.DeleteMemory(tt.userID, "team-123", tt.memoryID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
			} else {
				assert.NoError(t, err)
			}
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestMemoryService_SearchMemoriesByMetadata(t *testing.T) {
	tests := []struct {
		name          string
		userID        string
		metadataKey   string
		metadataValue string
		filters       MemoryFilters
		setupMock     func(*mocks.MockMemoryRepository)
		expectErr     bool
		errMessage    string
		expectCount   int
	}{
		{
			name:          "successful search",
			userID:        "user-123",
			metadataKey:   "category",
			metadataValue: "work",
			filters: MemoryFilters{
				UserID: "user-123",
				Page:   1,
				Limit:  50,
			},
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				memories := []models.Memory{*createTestMemory()}
				mockRepo.EXPECT().
					SearchByMetadata(mock.Anything, "user-123", "category", "work", mock.Anything).
					Return(memories, 1, nil).Once()
			},
			expectErr:   false,
			expectCount: 1,
		},
		{
			name:          "search with additional text filter",
			userID:        "user-123",
			metadataKey:   "priority",
			metadataValue: "high",
			filters: MemoryFilters{
				UserID: "user-123",
				Search: "test",
				Page:   1,
				Limit:  50,
			},
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				memories := []models.Memory{}
				mockRepo.EXPECT().
					SearchByMetadata(
						mock.Anything, "user-123", "priority", "high",
						mock.MatchedBy(func(filters repositories.MemoryFilters) bool {
							return filters.Search == "test"
						})).
					Return(memories, 0, nil).Once()
			},
			expectErr:   false,
			expectCount: 0,
		},
		{
			name:          "repository error",
			userID:        "user-123",
			metadataKey:   "category",
			metadataValue: "work",
			filters: MemoryFilters{
				UserID: "user-123",
			},
			setupMock: func(mockRepo *mocks.MockMemoryRepository) {
				mockRepo.EXPECT().
					SearchByMetadata(mock.Anything, "user-123", "category", "work", mock.Anything).
					Return(nil, 0, fmt.Errorf("database error")).Once()
			},
			expectErr:  true,
			errMessage: "database error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockMemoryRepository(t)
			tt.setupMock(mockRepo)

			service := createTestMemoryService(mockRepo)
			result, err := service.SearchMemoriesByMetadata(tt.userID, tt.metadataKey, tt.metadataValue, tt.filters)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectCount, len(result.Memories))
				assert.Equal(t, tt.expectCount, result.TotalCount)
			}
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestMemoryService_PublishesMemoryEvents(t *testing.T) {
	tests := []struct {
		name             string
		setupMocks       func(*mocks.MockMemoryRepository, *event_mocks.MockEventPublisher)
		executeAction    func(*MemoryService) error
		expectEventCalls int
		eventType        string
	}{
		{
			name: "publishes memory.created event when creating memory",
			setupMocks: func(mockRepo *mocks.MockMemoryRepository, mockEventManager *event_mocks.MockEventPublisher) {
				mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Memory")).
					Return(nil).Run(func(args mock.Arguments) {
					memory := args.Get(1).(*models.Memory)
					memory.ID = "memory-new-123"
				})

				// Expect event to be published exactly once
				mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
					return event.Type() == events.EventTypeMemoryCreated
				})).Return(nil).Once()
			},
			executeAction: func(service *MemoryService) error {
				req := &models.CreateMemoryRequest{
					ProjectID: testServiceProjectID,
					Text:      "Test Memory",
				}
				_, err := service.CreateMemory("user-123", "team-123", req)
				return err
			},
			expectEventCalls: 1,
			eventType:        events.EventTypeMemoryCreated,
		},
		{
			name: "publishes memory.updated event when updating memory",
			setupMocks: func(mockRepo *mocks.MockMemoryRepository, mockEventManager *event_mocks.MockEventPublisher) {
				existingMemory := &models.Memory{
					ID:        "memory-123",
					UserID:    "user-123",
					ProjectID: testServiceProjectID,
					Text:      "Original Text",
					Metadata:  map[string]interface{}{},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				mockRepo.On("GetByID", mock.Anything, "user-123", mock.Anything, "memory-123").
					Return(existingMemory, nil)

				mockRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.Memory")).
					Return(nil)

				// Expect event to be published exactly once
				mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
					return event.Type() == events.EventTypeMemoryUpdated
				})).Return(nil).Once()
			},
			executeAction: func(service *MemoryService) error {
				text := "Updated Text"
				req := &models.UpdateMemoryRequest{
					Text: &text,
				}
				_, err := service.UpdateMemory("user-123", "team-123", "memory-123", req)
				return err
			},
			expectEventCalls: 1,
			eventType:        events.EventTypeMemoryUpdated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mocks.MockMemoryRepository{}
			mockEventManager := &event_mocks.MockEventPublisher{}

			service := NewMemoryService(
				mockRepo, nil, allowAllAuthz{}, mockEventManager,
				func() *slog.Logger { l, _ := logtest.New(); return l }(), nil, nil)

			tt.setupMocks(mockRepo, mockEventManager)

			err := tt.executeAction(service)
			assert.NoError(t, err)

			mockRepo.AssertExpectations(t)
			mockEventManager.AssertExpectations(t)

			// Verify the event was published the expected number of times
			if tt.expectEventCalls > 0 {
				mockEventManager.AssertNumberOfCalls(t, "Publish", tt.expectEventCalls)
			} else {
				mockEventManager.AssertNotCalled(t, "Publish")
			}
		})
	}
}

// TestMemoryService_UpdateMemory_PreservesTeamID tests that team_id and project_id are preserved during update
func TestMemoryService_UpdateMemory_PreservesTeamID(t *testing.T) {
	mockRepo := mocks.NewMockMemoryRepository(t)
	service := createTestMemoryService(mockRepo)

	// Create existing memory with team_id and project_id
	existingMemory := createTestMemory()
	existingMemory.TeamID = "team-789"

	mockRepo.On("GetByID", mock.AnythingOfType("context.backgroundCtx"), "user-123", mock.Anything, "memory-123").
		Return(existingMemory, nil)

	// Verify that Update is called with team_id and project_id preserved
	mockRepo.On("Update", mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(memory *models.Memory) bool {
			return memory.ID == "memory-123" &&
				memory.TeamID == "team-789" && // TeamID must be preserved
				memory.ProjectID == testServiceProjectID && // ProjectID must be preserved
				memory.Text == "Updated memory text"
		})).Return(nil)

	text := "Updated memory text"
	request := &models.UpdateMemoryRequest{
		Text: &text,
	}

	memory, err := service.UpdateMemory("user-123", "team-123", "memory-123", request)

	assert.NoError(t, err)
	assert.NotNil(t, memory)
	assert.Equal(t, "team-789", memory.TeamID, "TeamID should be preserved during update")
	assert.Equal(t, testServiceProjectID, memory.ProjectID, "ProjectID should be preserved during update")
	mockRepo.AssertExpectations(t)
}

// TestMemoryService_CreateMemory_StatusDefaulting verifies that create defaults
// the status to active when none is supplied, and threads an explicit status
// through to the persisted memory.
func TestMemoryService_CreateMemory_StatusDefaulting(t *testing.T) {
	draft := models.MemoryStatusDraft

	tests := []struct {
		name         string
		reqStatus    *string
		expectStatus string
	}{
		{name: "no status defaults to active", reqStatus: nil, expectStatus: models.MemoryStatusActive},
		{name: "empty status defaults to active", reqStatus: ptrTo(""), expectStatus: models.MemoryStatusActive},
		{name: "explicit status is preserved", reqStatus: &draft, expectStatus: models.MemoryStatusDraft},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockMemoryRepository(t)
			mockRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(memory *models.Memory) bool {
				return memory.Status == tt.expectStatus
			})).Return(nil)

			svc := createTestMemoryService(mockRepo)
			req := &models.CreateMemoryRequest{
				ProjectID: testServiceProjectID,
				Text:      "a new memory",
				Status:    tt.reqStatus,
			}

			memory, err := svc.CreateMemory("user-123", "team-123", req)

			assert.NoError(t, err)
			assert.NotNil(t, memory)
			assert.Equal(t, tt.expectStatus, memory.Status)
			mockRepo.AssertExpectations(t)
		})
	}
}

// TestMemoryService_UpdateMemory_ChangesStatus verifies that an update carrying a
// status transitions the persisted memory, while an update without a status
// leaves the loaded status untouched.
func TestMemoryService_UpdateMemory_ChangesStatus(t *testing.T) {
	archived := models.MemoryStatusArchived

	tests := []struct {
		name         string
		reqStatus    *string
		loadedStatus string
		expectStatus string
	}{
		{name: "status change persists", reqStatus: &archived, loadedStatus: models.MemoryStatusActive, expectStatus: models.MemoryStatusArchived},
		{name: "absent status keeps existing", reqStatus: nil, loadedStatus: models.MemoryStatusDraft, expectStatus: models.MemoryStatusDraft},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockMemoryRepository(t)
			existing := &models.Memory{
				ID: "memory-123", UserID: "user-123", TeamID: "team-123",
				ProjectID: testServiceProjectID, Text: "original", Status: tt.loadedStatus,
				Metadata: map[string]interface{}{},
			}
			mockRepo.EXPECT().GetByID(mock.Anything, "user-123", "team-123", "memory-123").
				Return(existing, nil)
			mockRepo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(memory *models.Memory) bool {
				return memory.Status == tt.expectStatus
			})).Return(nil)

			svc := createTestMemoryService(mockRepo)
			newText := "still here"
			req := &models.UpdateMemoryRequest{Text: &newText, Status: tt.reqStatus}

			memory, err := svc.UpdateMemory("user-123", "team-123", "memory-123", req)

			assert.NoError(t, err)
			assert.NotNil(t, memory)
			assert.Equal(t, tt.expectStatus, memory.Status)
			mockRepo.AssertExpectations(t)
		})
	}
}

// TestMemoryService_ListMemories_ThreadsStatusFilter verifies the status filter is
// passed through to the repository filters unchanged.
func TestMemoryService_ListMemories_ThreadsStatusFilter(t *testing.T) {
	draft := models.MemoryStatusDraft
	mockRepo := mocks.NewMockMemoryRepository(t)
	mockRepo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.MemoryFilters) bool {
		return f.Status != nil && *f.Status == models.MemoryStatusDraft
	})).Return([]models.Memory{}, 0, nil)

	svc := createTestMemoryService(mockRepo)
	_, err := svc.ListMemories("user-123", MemoryFilters{TeamID: "team-123", Status: &draft, Page: 1, Limit: 10})

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func ptrTo[T any](v T) *T { return &v }

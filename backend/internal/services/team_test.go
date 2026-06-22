package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func createTestTeamService(
	teamRepo *mocks.MockTeamRepository,
	teamMemberRepo *mocks.MockTeamMemberRepository,
	userRepo *mocks.MockUserRepository,
) *TeamService {
	logger, _ := logtest.New()
	return NewTeamService(teamRepo, teamMemberRepo, userRepo, logger)
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamService_CreateDefaultTeam(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		setupMock  func(*mocks.MockTeamRepository, *mocks.MockTeamMemberRepository, *mocks.MockUserRepository)
		expectErr  bool
		errMessage string
	}{
		{
			name:   "successful creation",
			userID: "user-123",
			setupMock: func(
				mockTeamRepo *mocks.MockTeamRepository,
				mockTeamMemberRepo *mocks.MockTeamMemberRepository,
				mockUserRepo *mocks.MockUserRepository,
			) {
				mockTeamRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(team *models.Team) bool {
					return team.OwnerID == "user-123" &&
						team.Name == "Private Workspace" &&
						team.Slug == "private-workspace" &&
						team.Description == "Your personal workspace for individual projects and resources" &&
						!team.CreatedAt.IsZero() &&
						!team.UpdatedAt.IsZero()
				})).Run(func(_ context.Context, team *models.Team) {
					team.ID = "team-456"
				}).Return(nil).Once()

				mockTeamMemberRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(member *models.TeamMember) bool {
					return member.TeamID == "team-456" &&
						member.UserID == "user-123" &&
						member.Role == models.TeamMemberRoleOwner &&
						!member.CreatedAt.IsZero() &&
						!member.UpdatedAt.IsZero()
				})).Run(func(_ context.Context, member *models.TeamMember) {
					member.ID = "member-789"
				}).Return(nil).Once()

				mockUserRepo.EXPECT().UpdateDefaultTeamID(
					mock.Anything, "user-123", "team-456",
				).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:   "team creation fails",
			userID: "user-123",
			setupMock: func(
				mockTeamRepo *mocks.MockTeamRepository,
				mockTeamMemberRepo *mocks.MockTeamMemberRepository,
				mockUserRepo *mocks.MockUserRepository,
			) {
				mockTeamRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(
					fmt.Errorf("database error"),
				).Once()
			},
			expectErr:  true,
			errMessage: "failed to create default team",
		},
		{
			name:   "team member creation fails",
			userID: "user-123",
			setupMock: func(
				mockTeamRepo *mocks.MockTeamRepository,
				mockTeamMemberRepo *mocks.MockTeamMemberRepository,
				mockUserRepo *mocks.MockUserRepository,
			) {
				mockTeamRepo.EXPECT().Create(mock.Anything, mock.Anything).Run(func(_ context.Context, team *models.Team) {
					team.ID = "team-456"
				}).Return(nil).Once()

				mockTeamMemberRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(
					fmt.Errorf("member creation error"),
				).Once()
			},
			expectErr:  true,
			errMessage: "failed to create team member",
		},
		{
			name:   "team created but user update fails - still returns team",
			userID: "user-123",
			setupMock: func(
				mockTeamRepo *mocks.MockTeamRepository,
				mockTeamMemberRepo *mocks.MockTeamMemberRepository,
				mockUserRepo *mocks.MockUserRepository,
			) {
				mockTeamRepo.EXPECT().Create(mock.Anything, mock.Anything).Run(func(_ context.Context, team *models.Team) {
					team.ID = "team-789"
				}).Return(nil).Once()

				mockTeamMemberRepo.EXPECT().Create(mock.Anything, mock.Anything).
					Run(func(_ context.Context, member *models.TeamMember) {
						member.ID = "member-abc"
					}).Return(nil).Once()

				mockUserRepo.EXPECT().UpdateDefaultTeamID(
					mock.Anything, "user-123", "team-789",
				).Return(fmt.Errorf("user update failed")).Once()
			},
			expectErr: false, // Team creation succeeded, user update failure is logged but not returned
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTeamRepo := mocks.NewMockTeamRepository(t)
			mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
			mockUserRepo := mocks.NewMockUserRepository(t)
			tt.setupMock(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)

			service := createTestTeamService(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)
			result, err := service.CreateDefaultTeam(context.Background(), tt.userID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.userID, result.OwnerID)
				assert.Equal(t, "Private Workspace", result.Name)
				assert.Equal(t, "private-workspace", result.Slug)
				assert.NotZero(t, result.CreatedAt)
				assert.NotZero(t, result.UpdatedAt)
			}
		})
	}
}

func TestTeamService_GetTeamByOwnerID(t *testing.T) {
	tests := []struct {
		name       string
		ownerID    string
		setupMock  func(*mocks.MockTeamRepository)
		expectErr  bool
		errMessage string
	}{
		{
			name:    "successful get",
			ownerID: "user-123",
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository) {
				mockTeamRepo.EXPECT().GetByOwnerID(mock.Anything, "user-123").Return(&models.Team{
					ID:          "team-456",
					OwnerID:     "user-123",
					Name:        "Private Workspace",
					Slug:        "private-workspace",
					Description: "Your personal workspace for individual projects and resources",
				}, nil).Once()
			},
			expectErr: false,
		},
		{
			name:    "team not found",
			ownerID: "user-nonexistent",
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository) {
				mockTeamRepo.EXPECT().GetByOwnerID(mock.Anything, "user-nonexistent").Return(
					nil, fmt.Errorf("team not found"),
				).Once()
			},
			expectErr:  true,
			errMessage: "failed to get team by owner ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTeamRepo := mocks.NewMockTeamRepository(t)
			mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
			mockUserRepo := mocks.NewMockUserRepository(t)
			tt.setupMock(mockTeamRepo)

			service := createTestTeamService(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)
			result, err := service.GetTeamByOwnerID(context.Background(), tt.ownerID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.ownerID, result.OwnerID)
			}
		})
	}
}

func TestNewTeamService_NilLogger(t *testing.T) {
	mockTeamRepo := mocks.NewMockTeamRepository(t)
	mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)

	service := NewTeamService(mockTeamRepo, mockTeamMemberRepo, mockUserRepo, nil)

	assert.NotNil(t, service)
	assert.NotNil(t, service.logger)
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamService_CreateTeam(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		req        *models.CreateTeamRequest
		setupMock  func(*mocks.MockTeamRepository, *mocks.MockTeamMemberRepository, *mocks.MockUserRepository)
		expectErr  bool
		errMessage string
	}{
		{
			name:   "successful creation",
			userID: "user-123",
			req: &models.CreateTeamRequest{
				Name:        "Development Team",
				Description: "Team for developers",
			},
			setupMock: func(
				mockTeamRepo *mocks.MockTeamRepository,
				mockTeamMemberRepo *mocks.MockTeamMemberRepository,
				mockUserRepo *mocks.MockUserRepository,
			) {
				mockTeamRepo.EXPECT().GetByOwnerAndSlug(
					mock.Anything, "user-123", "development-team",
				).Return(nil, fmt.Errorf("team not found")).Once()

				mockTeamRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(team *models.Team) bool {
					return team.OwnerID == "user-123" &&
						team.Name == "Development Team" &&
						team.Slug == "development-team" &&
						team.Description == "Team for developers"
				})).Run(func(_ context.Context, team *models.Team) {
					team.ID = "team-456"
				}).Return(nil).Once()

				mockTeamMemberRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(member *models.TeamMember) bool {
					return member.TeamID == "team-456" &&
						member.UserID == "user-123" &&
						member.Role == models.TeamMemberRoleOwner
				})).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:   "slug collision - generates unique slug",
			userID: "user-123",
			req: &models.CreateTeamRequest{
				Name: "Test Team",
			},
			setupMock: func(
				mockTeamRepo *mocks.MockTeamRepository,
				mockTeamMemberRepo *mocks.MockTeamMemberRepository,
				mockUserRepo *mocks.MockUserRepository,
			) {
				// First slug exists
				mockTeamRepo.EXPECT().GetByOwnerAndSlug(
					mock.Anything, "user-123", "test-team",
				).Return(&models.Team{ID: "existing-team"}, nil).Once()

				// Checking for unique slugs
				mockTeamRepo.EXPECT().GetByOwnerAndSlug(
					mock.Anything, "user-123", "test-team-1",
				).Return(nil, fmt.Errorf("team not found")).Once()

				mockTeamRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(team *models.Team) bool {
					return team.Slug == "test-team-1"
				})).Run(func(_ context.Context, team *models.Team) {
					team.ID = "team-new"
				}).Return(nil).Once()

				mockTeamMemberRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:   "team creation fails",
			userID: "user-123",
			req: &models.CreateTeamRequest{
				Name: "Test Team",
			},
			setupMock: func(
				mockTeamRepo *mocks.MockTeamRepository,
				mockTeamMemberRepo *mocks.MockTeamMemberRepository,
				mockUserRepo *mocks.MockUserRepository,
			) {
				mockTeamRepo.EXPECT().GetByOwnerAndSlug(
					mock.Anything, "user-123", "test-team",
				).Return(nil, fmt.Errorf("team not found")).Once()

				mockTeamRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(
					fmt.Errorf("database error"),
				).Once()
			},
			expectErr:  true,
			errMessage: "failed to create team",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTeamRepo := mocks.NewMockTeamRepository(t)
			mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
			mockUserRepo := mocks.NewMockUserRepository(t)
			tt.setupMock(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)

			service := createTestTeamService(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)
			result, err := service.CreateTeam(context.Background(), tt.userID, tt.req)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.userID, result.OwnerID)
				assert.Equal(t, tt.req.Name, result.Name)
			}
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamService_GetTeam(t *testing.T) {
	tests := []struct {
		name                string
		userID              string
		teamID              string
		setupMock           func(*mocks.MockTeamRepository)
		setupTeamMemberMock func(*mocks.MockTeamMemberRepository)
		expectErr           bool
		errMessage          string
		expectedRole        string
	}{
		{
			name:   "successful get - owner access",
			userID: "user-123",
			teamID: "team-456",
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository) {
				mockTeamRepo.EXPECT().GetByID(mock.Anything, "team-456").Return(&models.Team{
					ID:          "team-456",
					OwnerID:     "user-123",
					Name:        "Private Workspace",
					Slug:        "private-workspace",
					Description: "Test team",
				}, nil).Once()
			},
			setupTeamMemberMock: func(mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				mockTeamMemberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-456", "user-123").Return(&models.TeamMember{
					TeamID: "team-456",
					UserID: "user-123",
					Role:   models.TeamMemberRoleOwner,
				}, nil).Once()
			},
			expectErr:    false,
			expectedRole: "owner",
		},
		{
			name:   "team not found",
			userID: "user-123",
			teamID: "team-nonexistent",
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository) {
				mockTeamRepo.EXPECT().GetByID(mock.Anything, "team-nonexistent").Return(
					nil, fmt.Errorf("team not found"),
				).Once()
			},
			setupTeamMemberMock: func(mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				// No mock needed as GetByID fails first
			},
			expectErr:  true,
			errMessage: "team not found",
		},
		{
			name:   "unauthorized - not a member",
			userID: "user-123",
			teamID: "team-456",
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository) {
				mockTeamRepo.EXPECT().GetByID(mock.Anything, "team-456").Return(&models.Team{
					ID:      "team-456",
					OwnerID: "user-different",
					Name:    "Someone Else's Team",
				}, nil).Once()
			},
			setupTeamMemberMock: func(mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				// User is not a member of this team
				mockTeamMemberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-456", "user-123").Return(
					nil, fmt.Errorf("member not found"),
				).Once()
			},
			expectErr:  true,
			errMessage: "team not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTeamRepo := mocks.NewMockTeamRepository(t)
			mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
			mockUserRepo := mocks.NewMockUserRepository(t)
			tt.setupMock(mockTeamRepo)
			if tt.setupTeamMemberMock != nil {
				tt.setupTeamMemberMock(mockTeamMemberRepo)
			}

			service := createTestTeamService(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)
			result, err := service.GetTeam(context.Background(), tt.userID, tt.teamID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.teamID, result.ID)
				assert.Equal(t, tt.expectedRole, result.Role)
			}
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamService_UpdateTeam(t *testing.T) {
	newName := "Updated Team Name"
	newDesc := "Updated description"

	tests := []struct {
		name                string
		userID              string
		teamID              string
		req                 *models.UpdateTeamRequest
		setupMock           func(*mocks.MockTeamRepository)
		setupTeamMemberMock func(*mocks.MockTeamMemberRepository)
		expectErr           bool
		errMessage          string
	}{
		{
			name:   "successful update - name only",
			userID: "user-123",
			teamID: "team-456",
			req: &models.UpdateTeamRequest{
				Name: &newName,
			},
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository) {
				mockTeamRepo.EXPECT().GetByID(mock.Anything, "team-456").Return(&models.Team{
					ID:          "team-456",
					OwnerID:     "user-123",
					Name:        "Old Name",
					Slug:        "old-name",
					Description: "Old desc",
				}, nil).Once()

				mockTeamRepo.EXPECT().GetByOwnerAndSlug(
					mock.Anything, "user-123", "updated-team-name",
				).Return(nil, fmt.Errorf("team not found")).Once()

				mockTeamRepo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(team *models.Team) bool {
					return team.Name == "Updated Team Name" && team.Slug == "updated-team-name"
				})).Return(nil).Once()
			},
			setupTeamMemberMock: func(mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				mockTeamMemberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-456", "user-123").Return(&models.TeamMember{
					TeamID: "team-456",
					UserID: "user-123",
					Role:   models.TeamMemberRoleOwner,
				}, nil).Once()
			},
			expectErr: false,
		},
		{
			name:   "successful update - description only",
			userID: "user-123",
			teamID: "team-456",
			req: &models.UpdateTeamRequest{
				Description: &newDesc,
			},
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository) {
				mockTeamRepo.EXPECT().GetByID(mock.Anything, "team-456").Return(&models.Team{
					ID:          "team-456",
					OwnerID:     "user-123",
					Name:        "Existing Name",
					Slug:        "existing-name",
					Description: "Old desc",
				}, nil).Once()

				mockTeamRepo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(team *models.Team) bool {
					return team.Description == "Updated description"
				})).Return(nil).Once()
			},
			setupTeamMemberMock: func(mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				mockTeamMemberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-456", "user-123").Return(&models.TeamMember{
					TeamID: "team-456",
					UserID: "user-123",
					Role:   models.TeamMemberRoleOwner,
				}, nil).Once()
			},
			expectErr: false,
		},
		{
			name:   "team not found",
			userID: "user-123",
			teamID: "team-nonexistent",
			req: &models.UpdateTeamRequest{
				Name: &newName,
			},
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository) {
				mockTeamRepo.EXPECT().GetByID(mock.Anything, "team-nonexistent").Return(
					nil, fmt.Errorf("team not found"),
				).Once()
			},
			setupTeamMemberMock: func(mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				// No mock needed as GetByID fails first
			},
			expectErr:  true,
			errMessage: "team not found",
		},
		{
			name:   "unauthorized update",
			userID: "user-123",
			teamID: "team-456",
			req: &models.UpdateTeamRequest{
				Name: &newName,
			},
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository) {
				mockTeamRepo.EXPECT().GetByID(mock.Anything, "team-456").Return(&models.Team{
					ID:      "team-456",
					OwnerID: "user-different",
				}, nil).Once()
			},
			setupTeamMemberMock: func(mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				// No mock needed as ownership check fails when user is not owner
			},
			expectErr:  true,
			errMessage: "only team owners can perform this action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTeamRepo := mocks.NewMockTeamRepository(t)
			mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
			mockUserRepo := mocks.NewMockUserRepository(t)
			tt.setupMock(mockTeamRepo)
			if tt.setupTeamMemberMock != nil {
				tt.setupTeamMemberMock(mockTeamMemberRepo)
			}

			service := createTestTeamService(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)
			result, err := service.UpdateTeam(context.Background(), tt.userID, tt.teamID, tt.req)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamService_DeleteTeam(t *testing.T) {
	defaultTeamID := "default-team-123"

	tests := []struct {
		name       string
		userID     string
		teamID     string
		setupMock  func(*mocks.MockTeamRepository, *mocks.MockTeamMemberRepository, *mocks.MockUserRepository)
		expectErr  bool
		errMessage string
	}{
		// NOTE: This test is deprecated - use TestTeamService_DeleteTeam_Success instead
		// Keeping for backwards compatibility but will fail due to new subscription checks
		{
			name:   "successful delete",
			userID: "user-123",
			teamID: "team-456",
			setupMock: func(
				mockTeamRepo *mocks.MockTeamRepository,
				mockTeamMemberRepo *mocks.MockTeamMemberRepository,
				mockUserRepo *mocks.MockUserRepository,
			) {
				mockTeamRepo.EXPECT().GetByID(mock.Anything, "team-456").Return(&models.Team{
					ID:         "team-456",
					OwnerID:    "user-123",
					IsPersonal: false,
				}, nil).Once()

				// Mock for GetTeam's role population
				mockTeamMemberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-456", "user-123").Return(&models.TeamMember{
					TeamID: "team-456",
					UserID: "user-123",
					Role:   models.TeamMemberRoleOwner,
				}, nil).Once()

				mockUserRepo.EXPECT().GetByID(mock.Anything, "user-123").Return(&models.User{
					ID:            "user-123",
					DefaultTeamID: &defaultTeamID, // Different team is default
				}, nil).Once()

				mockTeamMemberRepo.EXPECT().GetByTeamID(mock.Anything, "team-456").Return(
					[]models.TeamMember{{TeamID: "team-456", UserID: "user-123"}}, nil,
				).Once()

				mockTeamMemberRepo.EXPECT().Delete(mock.Anything, "team-456", "user-123").Return(nil).Once()

				mockTeamRepo.EXPECT().Delete(mock.Anything, "user-123", "team-456").Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:   "cannot delete default team",
			userID: "user-123",
			teamID: "team-456",
			setupMock: func(
				mockTeamRepo *mocks.MockTeamRepository,
				mockTeamMemberRepo *mocks.MockTeamMemberRepository,
				mockUserRepo *mocks.MockUserRepository,
			) {
				teamID := "team-456"
				mockTeamRepo.EXPECT().GetByID(mock.Anything, "team-456").Return(&models.Team{
					ID:      "team-456",
					OwnerID: "user-123",
				}, nil).Once()

				// Mock for GetTeam's role population
				mockTeamMemberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-456", "user-123").Return(&models.TeamMember{
					TeamID: "team-456",
					UserID: "user-123",
					Role:   models.TeamMemberRoleOwner,
				}, nil).Once()

				mockUserRepo.EXPECT().GetByID(mock.Anything, "user-123").Return(&models.User{
					ID:            "user-123",
					DefaultTeamID: &teamID, // This team is default
				}, nil).Once()
			},
			expectErr:  true,
			errMessage: "cannot delete default team",
		},
		{
			name:   "team not found",
			userID: "user-123",
			teamID: "team-nonexistent",
			setupMock: func(
				mockTeamRepo *mocks.MockTeamRepository,
				mockTeamMemberRepo *mocks.MockTeamMemberRepository,
				mockUserRepo *mocks.MockUserRepository,
			) {
				mockTeamRepo.EXPECT().GetByID(mock.Anything, "team-nonexistent").Return(
					nil, fmt.Errorf("team not found"),
				).Once()
			},
			expectErr:  true,
			errMessage: "team not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTeamRepo := mocks.NewMockTeamRepository(t)
			mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
			mockUserRepo := mocks.NewMockUserRepository(t)
			tt.setupMock(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)

			logger, _ := logtest.New()
			service := NewTeamService(mockTeamRepo, mockTeamMemberRepo, mockUserRepo, logger)
			err := service.DeleteTeam(context.Background(), tt.userID, tt.teamID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestTeamService_ListTeams(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		page       int
		pageSize   int
		setupMock  func(*mocks.MockTeamRepository, *mocks.MockTeamMemberRepository)
		expectErr  bool
		errMessage string
	}{
		{
			name:     "successful list",
			userID:   "user-123",
			page:     1,
			pageSize: 20,
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository, mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				mockTeamRepo.EXPECT().ListByUserID(mock.Anything, "user-123", 20, 0).Return(
					[]models.Team{
						{ID: "team-1", OwnerID: "user-123", Name: "Team 1"},
						{ID: "team-2", OwnerID: "user-123", Name: "Team 2"},
					}, 2, nil,
				).Once()

				// Mock GetByTeamAndUser for team-1
				mockTeamMemberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-1", "user-123").Return(
					&models.TeamMember{
						ID:     "member-1",
						TeamID: "team-1",
						UserID: "user-123",
						Role:   models.TeamMemberRoleOwner,
					}, nil,
				).Once()

				// Mock GetByTeamID for team-1
				mockTeamMemberRepo.EXPECT().GetByTeamID(mock.Anything, "team-1").Return(
					[]models.TeamMember{
						{ID: "member-1", TeamID: "team-1", UserID: "user-123", Role: models.TeamMemberRoleOwner},
					}, nil,
				).Once()

				// Mock GetByTeamAndUser for team-2
				mockTeamMemberRepo.EXPECT().GetByTeamAndUser(mock.Anything, "team-2", "user-123").Return(
					&models.TeamMember{
						ID:     "member-2",
						TeamID: "team-2",
						UserID: "user-123",
						Role:   models.TeamMemberRoleOwner,
					}, nil,
				).Once()

				// Mock GetByTeamID for team-2
				mockTeamMemberRepo.EXPECT().GetByTeamID(mock.Anything, "team-2").Return(
					[]models.TeamMember{
						{ID: "member-2", TeamID: "team-2", UserID: "user-123", Role: models.TeamMemberRoleOwner},
						{ID: "member-3", TeamID: "team-2", UserID: "user-456", Role: models.TeamMemberRoleMember},
					}, nil,
				).Once()
			},
			expectErr: false,
		},
		{
			name:     "pagination - page 2",
			userID:   "user-123",
			page:     2,
			pageSize: 10,
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository, mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				mockTeamRepo.EXPECT().ListByUserID(mock.Anything, "user-123", 10, 10).Return(
					[]models.Team{}, 15, nil,
				).Once()
			},
			expectErr: false,
		},
		{
			name:     "invalid page defaults to 1",
			userID:   "user-123",
			page:     0,
			pageSize: 20,
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository, mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				mockTeamRepo.EXPECT().ListByUserID(mock.Anything, "user-123", 20, 0).Return(
					[]models.Team{}, 0, nil,
				).Once()
			},
			expectErr: false,
		},
		{
			name:     "database error",
			userID:   "user-123",
			page:     1,
			pageSize: 20,
			setupMock: func(mockTeamRepo *mocks.MockTeamRepository, mockTeamMemberRepo *mocks.MockTeamMemberRepository) {
				mockTeamRepo.EXPECT().ListByUserID(mock.Anything, "user-123", 20, 0).Return(
					nil, 0, fmt.Errorf("database error"),
				).Once()
			},
			expectErr:  true,
			errMessage: "failed to list teams",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTeamRepo := mocks.NewMockTeamRepository(t)
			mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
			mockUserRepo := mocks.NewMockUserRepository(t)
			tt.setupMock(mockTeamRepo, mockTeamMemberRepo)

			service := createTestTeamService(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)
			result, err := service.ListTeams(context.Background(), tt.userID, tt.page, tt.pageSize)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "simple name", input: "Private Workspace", expected: "private-workspace"},
		{name: "with numbers", input: "Team 123", expected: "team-123"},
		{name: "special characters", input: "Team @#$% Test!", expected: "team-test"},
		{name: "multiple spaces", input: "My   Team", expected: "my-team"},
		{name: "leading/trailing spaces", input: "  Private Workspace  ", expected: "private-workspace"},
		{name: "empty string", input: "", expected: "team"},
		{name: "only special chars", input: "!@#$%", expected: "team"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSlug(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTeamService_CreateDefaultTeam_IsPersonalFlag tests that default teams are marked as personal
func TestTeamService_CreateDefaultTeam_IsPersonalFlag(t *testing.T) {
	mockTeamRepo := &mocks.MockTeamRepository{}
	mockTeamMemberRepo := &mocks.MockTeamMemberRepository{}
	mockUserRepo := &mocks.MockUserRepository{}

	service := createTestTeamService(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)

	userID := "user-123"

	// Mock: Create team - verify IsPersonal is true
	mockTeamRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(team *models.Team) bool {
		return team.OwnerID == userID &&
			team.IsPersonal == true // Verify personal flag is set
	})).Run(func(_ context.Context, team *models.Team) {
		team.ID = "team-456"
	}).Return(nil).Once()

	// Mock: Create team member
	mockTeamMemberRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(member *models.TeamMember) bool {
		return member.UserID == userID && member.Role == models.TeamMemberRoleOwner
	})).Return(nil).Once()

	// Mock: Update user's default team ID
	mockUserRepo.EXPECT().UpdateDefaultTeamID(mock.Anything, userID, "team-456").Return(nil).Once()

	// Execute
	team, err := service.CreateDefaultTeam(context.Background(), userID)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, team)
	assert.Equal(t, "team-456", team.ID)
	assert.True(t, team.IsPersonal, "Default team should be marked as personal workspace")

	mockTeamRepo.AssertExpectations(t)
	mockTeamMemberRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
}

// TestTeamService_CreateTeam_IsPersonalFlag tests that manually created teams are NOT personal
func TestTeamService_CreateTeam_IsPersonalFlag(t *testing.T) {
	mockTeamRepo := &mocks.MockTeamRepository{}
	mockTeamMemberRepo := &mocks.MockTeamMemberRepository{}
	mockUserRepo := &mocks.MockUserRepository{}

	service := createTestTeamService(mockTeamRepo, mockTeamMemberRepo, mockUserRepo)

	userID := "user-123"
	req := &models.CreateTeamRequest{
		Name:        "My Collaborative Team",
		Description: "Team for collaboration",
	}

	// Mock: Check if slug exists (doesn't exist)
	mockTeamRepo.EXPECT().GetByOwnerAndSlug(mock.Anything, userID, "my-collaborative-team").
		Return(nil, fmt.Errorf("not found")).Once()

	// Mock: Create team - verify IsPersonal is false
	mockTeamRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(team *models.Team) bool {
		return team.OwnerID == userID &&
			team.Name == "My Collaborative Team" &&
			team.IsPersonal == false // Verify personal flag is NOT set
	})).Run(func(_ context.Context, team *models.Team) {
		team.ID = "team-789"
	}).Return(nil).Once()

	// Mock: Create team member
	mockTeamMemberRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(member *models.TeamMember) bool {
		return member.UserID == userID && member.Role == models.TeamMemberRoleOwner
	})).Return(nil).Once()

	// Execute
	team, err := service.CreateTeam(context.Background(), userID, req)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, team)
	assert.Equal(t, "team-789", team.ID)
	assert.False(t, team.IsPersonal, "Manually created team should NOT be marked as personal workspace")

	mockTeamRepo.AssertExpectations(t)
	mockTeamMemberRepo.AssertExpectations(t)
}

// Tests for DeleteTeam

func TestTeamService_DeleteTeam_WithMultipleMembers(t *testing.T) {
	mockTeamRepo := mocks.NewMockTeamRepository(t)
	mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	logger, _ := logtest.New()

	service := NewTeamService(
		mockTeamRepo, mockTeamMemberRepo, mockUserRepo, logger,
	)

	ctx := context.Background()
	userID := "user-123"
	teamID := "team-456"

	team := &models.Team{
		ID:         teamID,
		OwnerID:    userID,
		Name:       "Test Team",
		IsPersonal: false,
	}

	members := []models.TeamMember{
		{TeamID: teamID, UserID: userID, Role: models.TeamMemberRoleOwner},
		{TeamID: teamID, UserID: "user-789", Role: models.TeamMemberRoleMember},
		{TeamID: teamID, UserID: "user-101", Role: models.TeamMemberRoleMember},
	}

	// Mock: GetTeam
	mockTeamRepo.EXPECT().GetByID(ctx, teamID).Return(team, nil)
	mockTeamMemberRepo.EXPECT().GetByTeamAndUser(ctx, teamID, userID).Return(&members[0], nil)

	// Mock: GetByID for user
	mockUserRepo.EXPECT().GetByID(ctx, userID).Return(&models.User{
		ID: userID,
	}, nil)

	// Mock: GetByTeamID returns multiple members
	mockTeamMemberRepo.EXPECT().GetByTeamID(ctx, teamID).Return(members, nil)

	// Execute
	err := service.DeleteTeam(ctx, userID, teamID)

	// Assert
	assert.Error(t, err)
	membersErr, ok := err.(*TeamHasMembersError)
	assert.True(t, ok, "Expected TeamHasMembersError")
	assert.Equal(t, teamID, membersErr.TeamID)
	assert.Equal(t, 3, membersErr.MemberCount)
}

func TestTeamService_DeleteTeam_PersonalWorkspace(t *testing.T) {
	mockTeamRepo := mocks.NewMockTeamRepository(t)
	mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	logger, _ := logtest.New()

	service := NewTeamService(
		mockTeamRepo, mockTeamMemberRepo, mockUserRepo, logger,
	)

	ctx := context.Background()
	userID := "user-123"
	teamID := "team-456"

	team := &models.Team{
		ID:         teamID,
		OwnerID:    userID,
		Name:       "Private Workspace",
		IsPersonal: true,
	}

	// Mock: GetTeam
	mockTeamRepo.EXPECT().GetByID(ctx, teamID).Return(team, nil)
	mockTeamMemberRepo.EXPECT().GetByTeamAndUser(ctx, teamID, userID).Return(&models.TeamMember{
		TeamID: teamID,
		UserID: userID,
		Role:   models.TeamMemberRoleOwner,
	}, nil)

	// Execute
	err := service.DeleteTeam(ctx, userID, teamID)

	// Assert
	assert.Error(t, err)
	_, ok := err.(*CannotDeletePersonalWorkspaceError)
	assert.True(t, ok, "Expected CannotDeletePersonalWorkspaceError")
}

func TestTeamService_DeleteTeam_Success(t *testing.T) {
	mockTeamRepo := mocks.NewMockTeamRepository(t)
	mockTeamMemberRepo := mocks.NewMockTeamMemberRepository(t)
	mockUserRepo := mocks.NewMockUserRepository(t)
	logger, _ := logtest.New()

	service := NewTeamService(
		mockTeamRepo, mockTeamMemberRepo, mockUserRepo, logger,
	)

	ctx := context.Background()
	userID := "user-123"
	teamID := "team-456"

	team := &models.Team{
		ID:         teamID,
		OwnerID:    userID,
		Name:       "Test Team",
		IsPersonal: false,
	}

	members := []models.TeamMember{
		{TeamID: teamID, UserID: userID, Role: models.TeamMemberRoleOwner},
	}

	// Mock: GetTeam
	mockTeamRepo.EXPECT().GetByID(ctx, teamID).Return(team, nil)
	mockTeamMemberRepo.EXPECT().GetByTeamAndUser(ctx, teamID, userID).Return(&members[0], nil)

	// Mock: GetByID for user
	mockUserRepo.EXPECT().GetByID(ctx, userID).Return(&models.User{
		ID: userID,
	}, nil)

	// Mock: GetByTeamID returns only owner
	mockTeamMemberRepo.EXPECT().GetByTeamID(ctx, teamID).Return(members, nil)

	// Mock: Delete member
	mockTeamMemberRepo.EXPECT().Delete(ctx, teamID, userID).Return(nil)

	// Mock: Delete team
	mockTeamRepo.EXPECT().Delete(ctx, userID, teamID).Return(nil)

	// Execute
	err := service.DeleteTeam(ctx, userID, teamID)

	// Assert
	assert.NoError(t, err)
}

package services

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// MockEmailService is a mock implementation of EmailServiceInterface for testing
type MockEmailService struct {
	mock.Mock
}

func (m *MockEmailService) SendTeamInvitation(invitation *models.TeamInvitation, teamName, inviterName string) error {
	args := m.Called(invitation, teamName, inviterName)
	return args.Error(0)
}

func (m *MockEmailService) SendSupportRequest(userName, userEmail string, req *models.SupportRequest) error {
	args := m.Called(userName, userEmail, req)
	return args.Error(0)
}

func (m *MockEmailService) SendNotificationEmail(to, subject, htmlBody string) error {
	args := m.Called(to, subject, htmlBody)
	return args.Error(0)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestTeamInvitationService_InviteMembers_SingleDuplicate(t *testing.T) {
	// Setup mocks
	mockInvitationRepo := &mocks.MockTeamInvitationRepository{}
	mockTeamRepo := &mocks.MockTeamRepository{}
	mockTeamMemberRepo := &mocks.MockTeamMemberRepository{}
	mockUserRepo := &mocks.MockUserRepository{}
	mockEmailService := &MockEmailService{}

	service := NewTeamInvitationService(
		mockInvitationRepo,
		mockTeamRepo,
		mockTeamMemberRepo,
		mockUserRepo,
		mockEmailService,
		NewAuthorizationService(mockTeamMemberRepo, nil),
		&config.Config{},
		nil,
	)

	ctx := context.Background()
	userID := "user-123"
	teamID := "team-456"
	emails := []string{"duplicate@example.com"}
	role := models.TeamMemberRoleMember

	// Mock: User has permission to invite
	mockTeamMemberRepo.On("GetByTeamAndUser", ctx, teamID, userID).
		Return(&models.TeamMember{
			TeamID: teamID,
			UserID: userID,
			Role:   models.TeamMemberRoleOwner,
		}, nil)

	// Mock: Get team details
	mockTeamRepo.On("GetByID", ctx, teamID).
		Return(&models.Team{
			ID:         teamID,
			Name:       "Test Team",
			IsPersonal: false,
		}, nil)

	// Mock: Resolve inviter display name (called once per InviteMembers)
	mockUserRepo.On("GetByID", ctx, userID).
		Return(&models.User{
			ID:    userID,
			Name:  "Inviter Name",
			Email: "inviter@example.com",
		}, nil)

	// Mock: User exists
	mockUserRepo.On("GetByEmail", ctx, "duplicate@example.com").
		Return(&models.User{
			ID:    "user-789",
			Email: "duplicate@example.com",
		}, nil)

	// Mock: User is already a member
	mockTeamMemberRepo.On("GetByTeamAndUser", ctx, teamID, "user-789").
		Return(&models.TeamMember{
			TeamID: teamID,
			UserID: "user-789",
			Role:   models.TeamMemberRoleMember,
		}, nil)

	// Execute
	invitations, err := service.InviteMembers(ctx, userID, teamID, emails, role)

	// Assert
	assert.Error(t, err)
	assert.Empty(t, invitations)

	// Check that it's a DuplicateMembersError
	var duplicateErr *DuplicateMembersError
	assert.True(t, stderrors.As(err, &duplicateErr))
	assert.Len(t, duplicateErr.DuplicateEmails, 1)
	assert.Contains(t, duplicateErr.DuplicateEmails, "duplicate@example.com")
	assert.Contains(t, err.Error(), "duplicate@example.com")

	mockInvitationRepo.AssertExpectations(t)
	mockTeamRepo.AssertExpectations(t)
	mockTeamMemberRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
	mockEmailService.AssertExpectations(t)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestTeamInvitationService_InviteMembers_MultipleDuplicates(t *testing.T) {
	// Setup mocks
	mockInvitationRepo := &mocks.MockTeamInvitationRepository{}
	mockTeamRepo := &mocks.MockTeamRepository{}
	mockTeamMemberRepo := &mocks.MockTeamMemberRepository{}
	mockUserRepo := &mocks.MockUserRepository{}
	mockEmailService := &MockEmailService{}

	service := NewTeamInvitationService(
		mockInvitationRepo,
		mockTeamRepo,
		mockTeamMemberRepo,
		mockUserRepo,
		mockEmailService,
		NewAuthorizationService(mockTeamMemberRepo, nil),
		&config.Config{},
		nil,
	)

	ctx := context.Background()
	userID := "user-123"
	teamID := "team-456"
	emails := []string{"dup1@example.com", "dup2@example.com", "new@example.com"}
	role := models.TeamMemberRoleMember

	// Mock: User has permission to invite
	mockTeamMemberRepo.On("GetByTeamAndUser", ctx, teamID, userID).
		Return(&models.TeamMember{
			TeamID: teamID,
			UserID: userID,
			Role:   models.TeamMemberRoleAdmin,
		}, nil)

	// Mock: Get team details
	mockTeamRepo.On("GetByID", ctx, teamID).
		Return(&models.Team{
			ID:         teamID,
			Name:       "Test Team",
			IsPersonal: false,
		}, nil)

	// Mock: Resolve inviter display name (called once per InviteMembers)
	mockUserRepo.On("GetByID", ctx, userID).
		Return(&models.User{
			ID:    userID,
			Name:  "Inviter Name",
			Email: "inviter@example.com",
		}, nil)

	// Mock: First user exists and is a member
	mockUserRepo.On("GetByEmail", ctx, "dup1@example.com").
		Return(&models.User{
			ID:    "user-001",
			Email: "dup1@example.com",
		}, nil)
	mockTeamMemberRepo.On("GetByTeamAndUser", ctx, teamID, "user-001").
		Return(&models.TeamMember{
			TeamID: teamID,
			UserID: "user-001",
			Role:   models.TeamMemberRoleMember,
		}, nil)

	// Mock: Second user exists and is a member
	mockUserRepo.On("GetByEmail", ctx, "dup2@example.com").
		Return(&models.User{
			ID:    "user-002",
			Email: "dup2@example.com",
		}, nil)
	mockTeamMemberRepo.On("GetByTeamAndUser", ctx, teamID, "user-002").
		Return(&models.TeamMember{
			TeamID: teamID,
			UserID: "user-002",
			Role:   models.TeamMemberRoleMember,
		}, nil)

	// Mock: Third user doesn't exist (new user)
	mockUserRepo.On("GetByEmail", ctx, "new@example.com").
		Return((*models.User)(nil), repositories.ErrUserNotFound)

	// Mock: No pending invitation for new user
	mockInvitationRepo.On("GetPendingByEmail", ctx, "new@example.com").
		Return([]models.TeamInvitation{}, nil)

	// Mock: Create invitation for new user
	mockInvitationRepo.On("Create", ctx, mock.MatchedBy(func(inv *models.TeamInvitation) bool {
		return inv.InviteeEmail == "new@example.com"
	})).Return(nil)

	// Mock: Send email for new user — third arg must be the resolved name, not userID
	mockEmailService.On("SendTeamInvitation", mock.MatchedBy(func(inv *models.TeamInvitation) bool {
		return inv.InviteeEmail == "new@example.com"
	}), "Test Team", "Inviter Name").Return(nil)

	// Execute
	invitations, err := service.InviteMembers(ctx, userID, teamID, emails, role)

	// Assert
	assert.Error(t, err)
	assert.Len(t, invitations, 1) // Only new user invitation created
	assert.Equal(t, "new@example.com", invitations[0].InviteeEmail)

	// Check duplicate error
	var duplicateErr *DuplicateMembersError
	assert.True(t, stderrors.As(err, &duplicateErr))
	assert.Len(t, duplicateErr.DuplicateEmails, 2)
	assert.Contains(t, duplicateErr.DuplicateEmails, "dup1@example.com")
	assert.Contains(t, duplicateErr.DuplicateEmails, "dup2@example.com")

	mockInvitationRepo.AssertExpectations(t)
	mockTeamRepo.AssertExpectations(t)
	mockTeamMemberRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
	mockEmailService.AssertExpectations(t)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestTeamInvitationService_InviteMembers_NoDuplicatesSuccess(t *testing.T) {
	// Setup mocks
	mockInvitationRepo := &mocks.MockTeamInvitationRepository{}
	mockTeamRepo := &mocks.MockTeamRepository{}
	mockTeamMemberRepo := &mocks.MockTeamMemberRepository{}
	mockUserRepo := &mocks.MockUserRepository{}
	mockEmailService := &MockEmailService{}

	service := NewTeamInvitationService(
		mockInvitationRepo,
		mockTeamRepo,
		mockTeamMemberRepo,
		mockUserRepo,
		mockEmailService,
		NewAuthorizationService(mockTeamMemberRepo, nil),
		&config.Config{},
		nil,
	)

	ctx := context.Background()
	userID := "user-123"
	teamID := "team-456"
	emails := []string{"new1@example.com", "new2@example.com"}
	role := models.TeamMemberRoleMember

	// Mock: User has permission to invite
	mockTeamMemberRepo.On("GetByTeamAndUser", ctx, teamID, userID).
		Return(&models.TeamMember{
			TeamID: teamID,
			UserID: userID,
			Role:   models.TeamMemberRoleOwner,
		}, nil)

	// Mock: Get team details
	mockTeamRepo.On("GetByID", ctx, teamID).
		Return(&models.Team{
			ID:         teamID,
			Name:       "Test Team",
			IsPersonal: false,
		}, nil)

	// Mock: Resolve inviter display name (called once per InviteMembers)
	mockUserRepo.On("GetByID", ctx, userID).
		Return(&models.User{
			ID:    userID,
			Name:  "Inviter Name",
			Email: "inviter@example.com",
		}, nil)

	// Mock: Both users don't exist
	mockUserRepo.On("GetByEmail", ctx, "new1@example.com").
		Return((*models.User)(nil), repositories.ErrUserNotFound)
	mockUserRepo.On("GetByEmail", ctx, "new2@example.com").
		Return((*models.User)(nil), repositories.ErrUserNotFound)

	// Mock: No pending invitations
	mockInvitationRepo.On("GetPendingByEmail", ctx, "new1@example.com").
		Return([]models.TeamInvitation{}, nil)
	mockInvitationRepo.On("GetPendingByEmail", ctx, "new2@example.com").
		Return([]models.TeamInvitation{}, nil)

	// Mock: Create invitations
	mockInvitationRepo.On("Create", ctx, mock.MatchedBy(func(inv *models.TeamInvitation) bool {
		return inv.InviteeEmail == "new1@example.com" || inv.InviteeEmail == "new2@example.com"
	})).Return(nil)

	// Mock: Send emails — third arg must be the resolved name, not userID
	mockEmailService.On("SendTeamInvitation", mock.Anything, "Test Team", "Inviter Name").Return(nil)

	// Execute
	invitations, err := service.InviteMembers(ctx, userID, teamID, emails, role)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, invitations, 2)

	mockInvitationRepo.AssertExpectations(t)
	mockTeamRepo.AssertExpectations(t)
	mockTeamMemberRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
	mockEmailService.AssertExpectations(t)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestTeamInvitationService_InviteMembers_PendingInvitationExists(t *testing.T) {
	// Setup mocks
	mockInvitationRepo := &mocks.MockTeamInvitationRepository{}
	mockTeamRepo := &mocks.MockTeamRepository{}
	mockTeamMemberRepo := &mocks.MockTeamMemberRepository{}
	mockUserRepo := &mocks.MockUserRepository{}
	mockEmailService := &MockEmailService{}

	service := NewTeamInvitationService(
		mockInvitationRepo,
		mockTeamRepo,
		mockTeamMemberRepo,
		mockUserRepo,
		mockEmailService,
		NewAuthorizationService(mockTeamMemberRepo, nil),
		&config.Config{},
		nil,
	)

	ctx := context.Background()
	userID := "user-123"
	teamID := "team-456"
	emails := []string{"pending@example.com"}
	role := models.TeamMemberRoleMember

	// Mock: User has permission to invite
	mockTeamMemberRepo.On("GetByTeamAndUser", ctx, teamID, userID).
		Return(&models.TeamMember{
			TeamID: teamID,
			UserID: userID,
			Role:   models.TeamMemberRoleOwner,
		}, nil)

	// Mock: Get team details
	mockTeamRepo.On("GetByID", ctx, teamID).
		Return(&models.Team{
			ID:         teamID,
			Name:       "Test Team",
			IsPersonal: false,
		}, nil)

	// Mock: Resolve inviter display name (called once per InviteMembers)
	mockUserRepo.On("GetByID", ctx, userID).
		Return(&models.User{
			ID:    userID,
			Name:  "Inviter Name",
			Email: "inviter@example.com",
		}, nil)

	// Mock: User doesn't exist
	mockUserRepo.On("GetByEmail", ctx, "pending@example.com").
		Return((*models.User)(nil), repositories.ErrUserNotFound)

	// Mock: Pending invitation already exists for this team
	mockInvitationRepo.On("GetPendingByEmail", ctx, "pending@example.com").
		Return([]models.TeamInvitation{
			{
				ID:           "inv-123",
				TeamID:       teamID,
				InviteeEmail: "pending@example.com",
				Status:       models.InvitationStatusPending,
				ExpiresAt:    time.Now().Add(24 * time.Hour),
			},
		}, nil)

	// Execute
	invitations, err := service.InviteMembers(ctx, userID, teamID, emails, role)

	// Assert - should skip but not return error
	assert.NoError(t, err)
	assert.Empty(t, invitations) // No new invitations created

	mockInvitationRepo.AssertExpectations(t)
	mockTeamRepo.AssertExpectations(t)
	mockTeamMemberRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
	mockEmailService.AssertExpectations(t)
}

func TestTeamInvitationService_InviteMembers_NoPermission(t *testing.T) {
	// Setup mocks
	mockInvitationRepo := &mocks.MockTeamInvitationRepository{}
	mockTeamRepo := &mocks.MockTeamRepository{}
	mockTeamMemberRepo := &mocks.MockTeamMemberRepository{}
	mockUserRepo := &mocks.MockUserRepository{}
	mockEmailService := &MockEmailService{}

	service := NewTeamInvitationService(
		mockInvitationRepo,
		mockTeamRepo,
		mockTeamMemberRepo,
		mockUserRepo,
		mockEmailService,
		NewAuthorizationService(mockTeamMemberRepo, nil),
		&config.Config{},
		nil,
	)

	ctx := context.Background()
	userID := "user-123"
	teamID := "team-456"
	emails := []string{"test@example.com"}
	role := models.TeamMemberRoleMember

	// Mock: User is a regular member (no permission)
	mockTeamMemberRepo.On("GetByTeamAndUser", ctx, teamID, userID).
		Return(&models.TeamMember{
			TeamID: teamID,
			UserID: userID,
			Role:   models.TeamMemberRoleMember, // Not owner or admin
		}, nil)

	// Execute
	invitations, err := service.InviteMembers(ctx, userID, teamID, emails, role)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, invitations)
	assert.Contains(t, err.Error(), "permission")

	mockTeamMemberRepo.AssertExpectations(t)
}

func TestDuplicateMembersError_ErrorMessage_Single(t *testing.T) {
	err := NewDuplicateMembersError([]string{"test@example.com"})
	assert.Equal(t, "User test@example.com is already in the team", err.Error())
}

func TestDuplicateMembersError_ErrorMessage_Multiple(t *testing.T) {
	err := NewDuplicateMembersError([]string{"test1@example.com", "test2@example.com"})
	assert.Contains(t, err.Error(), "Users already in team")
	assert.Contains(t, err.Error(), "test1@example.com")
	assert.Contains(t, err.Error(), "test2@example.com")
}

// TestTeamInvitationService_InviteMembers_PersonalWorkspace tests that invitations are blocked for personal workspaces
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestTeamInvitationService_InviteMembers_PersonalWorkspace(t *testing.T) {
	// Setup mocks
	mockInvitationRepo := &mocks.MockTeamInvitationRepository{}
	mockTeamRepo := &mocks.MockTeamRepository{}
	mockTeamMemberRepo := &mocks.MockTeamMemberRepository{}
	mockUserRepo := &mocks.MockUserRepository{}
	mockEmailService := &MockEmailService{}

	service := NewTeamInvitationService(
		mockInvitationRepo,
		mockTeamRepo,
		mockTeamMemberRepo,
		mockUserRepo,
		mockEmailService,
		NewAuthorizationService(mockTeamMemberRepo, nil),
		&config.Config{},
		nil,
	)

	ctx := context.Background()
	userID := "user-123"
	teamID := "team-456"
	emails := []string{"newuser@example.com"}
	role := models.TeamMemberRoleMember

	// Mock: User has permission to invite (owner of personal workspace)
	mockTeamMemberRepo.On("GetByTeamAndUser", ctx, teamID, userID).
		Return(&models.TeamMember{
			TeamID: teamID,
			UserID: userID,
			Role:   models.TeamMemberRoleOwner,
		}, nil)

	// Mock: Team is a personal workspace
	mockTeamRepo.On("GetByID", ctx, teamID).
		Return(&models.Team{
			ID:         teamID,
			Name:       "Private Workspace",
			IsPersonal: true, // This is the key field being tested
		}, nil)

	// Execute
	invitations, err := service.InviteMembers(ctx, userID, teamID, emails, role)

	// Assert
	assert.Error(t, err)
	assert.Empty(t, invitations)

	// Check that it's a PersonalWorkspaceError
	var personalWorkspaceErr *PersonalWorkspaceError
	assert.True(t, stderrors.As(err, &personalWorkspaceErr))
	assert.Equal(t, teamID, personalWorkspaceErr.TeamID)
	assert.Contains(t, err.Error(), "cannot invite members to personal workspace")
	assert.Contains(t, err.Error(), "upgrade to a team plan")

	// Verify expectations
	mockTeamMemberRepo.AssertExpectations(t)
	mockTeamRepo.AssertExpectations(t)
	// No invitation should be created for personal workspaces
	mockInvitationRepo.AssertNotCalled(t, "Create")
	mockEmailService.AssertNotCalled(t, "SendTeamInvitation")
}

// TestTeamInvitationService_InviteMembers_NonPersonalWorkspace tests that invitations work for non-personal workspaces
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestTeamInvitationService_InviteMembers_NonPersonalWorkspace(t *testing.T) {
	// Setup mocks
	mockInvitationRepo := &mocks.MockTeamInvitationRepository{}
	mockTeamRepo := &mocks.MockTeamRepository{}
	mockTeamMemberRepo := &mocks.MockTeamMemberRepository{}
	mockUserRepo := &mocks.MockUserRepository{}
	mockEmailService := &MockEmailService{}

	service := NewTeamInvitationService(
		mockInvitationRepo,
		mockTeamRepo,
		mockTeamMemberRepo,
		mockUserRepo,
		mockEmailService,
		NewAuthorizationService(mockTeamMemberRepo, nil),
		&config.Config{},
		nil,
	)

	ctx := context.Background()
	userID := "user-123"
	teamID := "team-456"
	emails := []string{"newuser@example.com"}
	role := models.TeamMemberRoleMember

	// Mock: User has permission to invite
	mockTeamMemberRepo.On("GetByTeamAndUser", ctx, teamID, userID).
		Return(&models.TeamMember{
			TeamID: teamID,
			UserID: userID,
			Role:   models.TeamMemberRoleOwner,
		}, nil)

	// Mock: Team is NOT a personal workspace (manually created team)
	mockTeamRepo.On("GetByID", ctx, teamID).
		Return(&models.Team{
			ID:         teamID,
			Name:       "My Team",
			IsPersonal: false, // Non-personal team allows invitations
		}, nil)

	// Mock: Resolve inviter display name (called once per InviteMembers)
	mockUserRepo.On("GetByID", ctx, userID).
		Return(&models.User{
			ID:    userID,
			Name:  "Inviter Name",
			Email: "inviter@example.com",
		}, nil)

	// Mock: User doesn't exist yet (new invitation)
	mockUserRepo.On("GetByEmail", ctx, "newuser@example.com").
		Return((*models.User)(nil), repositories.ErrUserNotFound)

	// Mock: No pending invitations
	mockInvitationRepo.On("GetPendingByEmail", ctx, "newuser@example.com").
		Return([]models.TeamInvitation{}, nil)

	// Mock: Create invitation successfully
	mockInvitationRepo.On("Create", ctx, mock.MatchedBy(func(inv *models.TeamInvitation) bool {
		return inv.InviteeEmail == "newuser@example.com" && inv.TeamID == teamID
	})).Return(nil)

	// Mock: Send email successfully — third arg must be the resolved name, not userID
	mockEmailService.On("SendTeamInvitation", mock.MatchedBy(func(inv *models.TeamInvitation) bool {
		return inv.InviteeEmail == "newuser@example.com"
	}), "My Team", "Inviter Name").Return(nil)

	// Execute
	invitations, err := service.InviteMembers(ctx, userID, teamID, emails, role)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, invitations, 1)
	assert.Equal(t, "newuser@example.com", invitations[0].InviteeEmail)
	assert.Equal(t, teamID, invitations[0].TeamID)

	// Verify all expectations
	mockTeamMemberRepo.AssertExpectations(t)
	mockTeamRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
	mockInvitationRepo.AssertExpectations(t)
	mockEmailService.AssertExpectations(t)
}

// TestPersonalWorkspaceError_ErrorMessage tests the error message format
func TestPersonalWorkspaceError_ErrorMessage(t *testing.T) {
	err := NewPersonalWorkspaceError("team-123")
	assert.Equal(t, "team-123", err.TeamID)
	expectedMessage := "cannot invite members to personal workspace - " +
		"upgrade to a team plan to enable collaboration"
	assert.Equal(t, expectedMessage, err.Error())
}

// inviterNameTestMocks bundles the mocks used to drive InviteMembers through
// the happy path so each new test below can focus on the inviter-name behavior.
type inviterNameTestMocks struct {
	invitationRepo *mocks.MockTeamInvitationRepository
	teamRepo       *mocks.MockTeamRepository
	teamMemberRepo *mocks.MockTeamMemberRepository
	userRepo       *mocks.MockUserRepository
	emailService   *MockEmailService
}

// newInviterNameTestMocks builds a fresh set of repository/email mocks and
// the service under test wired to them.
func newInviterNameTestMocks() (*TeamInvitationService, *inviterNameTestMocks) {
	m := &inviterNameTestMocks{
		invitationRepo: &mocks.MockTeamInvitationRepository{},
		teamRepo:       &mocks.MockTeamRepository{},
		teamMemberRepo: &mocks.MockTeamMemberRepository{},
		userRepo:       &mocks.MockUserRepository{},
		emailService:   &MockEmailService{},
	}
	service := NewTeamInvitationService(
		m.invitationRepo, m.teamRepo, m.teamMemberRepo,
		m.userRepo, m.emailService, NewAuthorizationService(m.teamMemberRepo, nil), &config.Config{}, nil,
	)
	return service, m
}

// wireInviteMembersHappyPath sets up the repo expectations needed to drive
// InviteMembers through the permission check and into the per-email loop for a
// single new invitee.
func wireInviteMembersHappyPath(ctx context.Context, m *inviterNameTestMocks, userID, teamID, inviteeEmail string) {
	m.teamMemberRepo.On("GetByTeamAndUser", ctx, teamID, userID).
		Return(&models.TeamMember{TeamID: teamID, UserID: userID, Role: models.TeamMemberRoleOwner}, nil)
	m.teamRepo.On("GetByID", ctx, teamID).
		Return(&models.Team{ID: teamID, Name: "Test Team", IsPersonal: false}, nil)
	m.userRepo.On("GetByEmail", ctx, inviteeEmail).
		Return((*models.User)(nil), repositories.ErrUserNotFound)
	m.invitationRepo.On("GetPendingByEmail", ctx, inviteeEmail).Return([]models.TeamInvitation{}, nil)
	m.invitationRepo.On("Create", ctx, mock.MatchedBy(func(inv *models.TeamInvitation) bool {
		return inv.InviteeEmail == inviteeEmail && inv.TeamID == teamID
	})).Return(nil)
}

// setupInviteMembersHappyPath is the convenience shorthand used by every
// inviter-name test below.
func setupInviteMembersHappyPath(
	ctx context.Context, userID, teamID, inviteeEmail string,
) (*TeamInvitationService, *inviterNameTestMocks) {
	service, m := newInviterNameTestMocks()
	wireInviteMembersHappyPath(ctx, m, userID, teamID, inviteeEmail)
	return service, m
}

// captureInviterNameArg installs a SendTeamInvitation expectation that
// captures the inviter-name argument passed to the email service. The returned
// pointer is populated when the mock is called, letting the test assert on the
// exact value that flowed through.
func captureInviterNameArg(emailService *MockEmailService, inviteeEmail string) *string {
	captured := new(string)
	emailService.On(
		"SendTeamInvitation",
		mock.MatchedBy(func(inv *models.TeamInvitation) bool {
			return inv.InviteeEmail == inviteeEmail
		}),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
	).Run(func(args mock.Arguments) {
		*captured = args.String(2)
	}).Return(nil)
	return captured
}

// TestTeamInvitationService_InviteMembers_ResolvesInviterName verifies that
// the email service receives the inviter's display name, not their UUID.
func TestTeamInvitationService_InviteMembers_ResolvesInviterName(t *testing.T) {
	ctx := context.Background()
	const (
		userID  = "user-123"
		teamID  = "team-456"
		invitee = "newuser@example.com"
	)

	service, m := setupInviteMembersHappyPath(ctx, userID, teamID, invitee)

	m.userRepo.On("GetByID", ctx, userID).
		Return(&models.User{
			ID:    userID,
			Name:  "Jane Doe",
			Email: "jane@example.com",
		}, nil)

	captured := captureInviterNameArg(m.emailService, invitee)

	invitations, err := service.InviteMembers(
		ctx, userID, teamID, []string{invitee}, models.TeamMemberRoleMember,
	)

	assert.NoError(t, err)
	assert.Len(t, invitations, 1)
	assert.Equal(t, "Jane Doe", *captured, "inviter name should be the user's Name")
	assert.NotEqual(t, userID, *captured, "inviter name must never be the raw user ID")

	m.userRepo.AssertExpectations(t)
	m.emailService.AssertExpectations(t)
}

// TestTeamInvitationService_InviteMembers_InviterNameFallsBackToEmailLocalPart
// verifies the fallback to the email local-part when the user has no Name.
func TestTeamInvitationService_InviteMembers_InviterNameFallsBackToEmailLocalPart(t *testing.T) {
	ctx := context.Background()
	const (
		userID  = "user-123"
		teamID  = "team-456"
		invitee = "newuser@example.com"
	)

	service, m := setupInviteMembersHappyPath(ctx, userID, teamID, invitee)

	m.userRepo.On("GetByID", ctx, userID).
		Return(&models.User{
			ID:    userID,
			Name:  "   ", // whitespace-only — must be treated as empty
			Email: "alice@example.com",
		}, nil)

	captured := captureInviterNameArg(m.emailService, invitee)

	invitations, err := service.InviteMembers(
		ctx, userID, teamID, []string{invitee}, models.TeamMemberRoleMember,
	)

	assert.NoError(t, err)
	assert.Len(t, invitations, 1)
	assert.Equal(t, "alice", *captured, "inviter name should fall back to email local-part")
}

// TestTeamInvitationService_InviteMembers_InviterNameFallsBackToConstant
// verifies the final fallback when the user lookup itself fails.
func TestTeamInvitationService_InviteMembers_InviterNameFallsBackToConstant(t *testing.T) {
	ctx := context.Background()
	const (
		userID  = "user-123"
		teamID  = "team-456"
		invitee = "newuser@example.com"
	)

	service, m := setupInviteMembersHappyPath(ctx, userID, teamID, invitee)

	m.userRepo.On("GetByID", ctx, userID).
		Return((*models.User)(nil), stderrors.New("database unavailable"))

	captured := captureInviterNameArg(m.emailService, invitee)

	invitations, err := service.InviteMembers(
		ctx, userID, teamID, []string{invitee}, models.TeamMemberRoleMember,
	)

	assert.NoError(t, err, "user lookup failure must not block the invitation")
	assert.Len(t, invitations, 1)
	assert.Equal(t, "A teammate", *captured, "inviter name should fall back to the constant")
	assert.NotEqual(t, userID, *captured, "inviter name must never be the raw user ID")
}

// TestTeamInvitationService_InviteMembers_ResolvesInviterNameOncePerBatch
// guards the per-batch resolution invariant: even when several invitees are
// sent at once, the user repository is hit exactly once. A regression that
// moves the call inside the per-email loop would fail this test.
//
//nolint:funlen // Mock setup for a 3-invitee batch is verbose by nature.
func TestTeamInvitationService_InviteMembers_ResolvesInviterNameOncePerBatch(t *testing.T) {
	ctx := context.Background()
	const (
		userID  = "user-uuid-abc"
		teamID  = "team-456"
		email1  = "first@example.com"
		email2  = "second@example.com"
		email3  = "third@example.com"
		theName = "Jane Doe"
	)
	emails := []string{email1, email2, email3}

	service, m := newInviterNameTestMocks()

	// Permission and team checks (one call each, regardless of batch size).
	m.teamMemberRepo.On("GetByTeamAndUser", ctx, teamID, userID).
		Return(&models.TeamMember{TeamID: teamID, UserID: userID, Role: models.TeamMemberRoleOwner}, nil)
	m.teamRepo.On("GetByID", ctx, teamID).
		Return(&models.Team{ID: teamID, Name: "Test Team", IsPersonal: false}, nil)

	// Per-email setup: every invitee is a brand-new address.
	for _, email := range emails {
		m.userRepo.On("GetByEmail", ctx, email).
			Return((*models.User)(nil), repositories.ErrUserNotFound)
		m.invitationRepo.On("GetPendingByEmail", ctx, email).Return([]models.TeamInvitation{}, nil)
		m.invitationRepo.On("Create", ctx, mock.MatchedBy(func(inv *models.TeamInvitation) bool {
			return inv.InviteeEmail == email && inv.TeamID == teamID
		})).Return(nil)
	}

	// Inviter resolution: must happen exactly ONCE for the whole batch.
	m.userRepo.On("GetByID", ctx, userID).
		Return(&models.User{ID: userID, Name: theName, Email: "jane@example.com"}, nil).Once()

	// Email send: every invitee receives the same resolved name.
	m.emailService.On(
		"SendTeamInvitation",
		mock.AnythingOfType("*models.TeamInvitation"),
		"Test Team",
		theName,
	).Return(nil).Times(len(emails))

	invitations, err := service.InviteMembers(ctx, userID, teamID, emails, models.TeamMemberRoleMember)

	assert.NoError(t, err)
	assert.Len(t, invitations, len(emails))
	m.userRepo.AssertNumberOfCalls(t, "GetByID", 1)
	m.userRepo.AssertExpectations(t)
	m.emailService.AssertExpectations(t)
}

// resolveInviterCase describes one scenario for the resolveInviterDisplayName
// helper test below.
type resolveInviterCase struct {
	name     string
	user     *models.User
	userErr  error
	expected string
}

// resolveInviterCases enumerates the full fallback chain for the helper.
func resolveInviterCases(userID string) []resolveInviterCase {
	return []resolveInviterCase{
		{"uses Name when set",
			&models.User{ID: userID, Name: "Jane Doe", Email: "jane@example.com"}, nil, "Jane Doe"},
		{"trims whitespace from Name",
			&models.User{ID: userID, Name: "  Bob  ", Email: "bob@example.com"}, nil, "Bob"},
		{"falls back to email local-part when Name is empty",
			&models.User{ID: userID, Name: "", Email: "alice@example.com"}, nil, "alice"},
		{"falls back to email local-part when Name is whitespace",
			&models.User{ID: userID, Name: "   ", Email: "carol@example.com"}, nil, "carol"},
		{"falls back to constant when both Name and Email are empty",
			&models.User{ID: userID, Name: "", Email: ""}, nil, "A teammate"},
		{"falls back to constant when Email has no @",
			&models.User{ID: userID, Name: "", Email: "no-at-sign"}, nil, "A teammate"},
		{"falls back to constant when lookup fails",
			nil, stderrors.New("database unavailable"), "A teammate"},
		{"falls back to constant when lookup returns nil user without error",
			nil, nil, "A teammate"},
		{"trims whitespace around the email local-part",
			&models.User{ID: userID, Name: "", Email: "  dave@example.com  "}, nil, "dave"},
		{"falls back to constant when email local-part is whitespace-only",
			&models.User{ID: userID, Name: "", Email: "   @example.com"}, nil, "A teammate"},
	}
}

// TestTeamInvitationService_resolveInviterDisplayName tests the helper directly
// across the full fallback chain.
func TestTeamInvitationService_resolveInviterDisplayName(t *testing.T) {
	ctx := context.Background()
	const userID = "user-uuid-abc"

	for _, tc := range resolveInviterCases(userID) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			service, m := newInviterNameTestMocks()
			m.userRepo.On("GetByID", ctx, userID).Return(tc.user, tc.userErr)

			got := service.resolveInviterDisplayName(ctx, userID)

			assert.Equal(t, tc.expected, got)
			assert.NotEqual(t, userID, got, "must never return the raw user ID")
			m.userRepo.AssertExpectations(t)
		})
	}
}

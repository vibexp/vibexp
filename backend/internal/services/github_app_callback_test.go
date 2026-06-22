package services

import (
	"context"
	"errors"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// newCallbackTestService creates a GitHubAppService with the given mock deps for callback tests.
func newCallbackTestService(
	installationRepo *MockGitHubInstallationRepository,
	githubClient *MockGitHubAppClient,
	eventManager *MockEventPublisher,
) GitHubAppServiceInterface {
	projectRepo := new(MockProjectRepository)
	blueprintRepo := new(MockBlueprintRepository)
	encryptionSvc := new(MockEncryptionService)
	logger := slog.New(slog.DiscardHandler)

	return NewGitHubAppService(
		installationRepo,
		projectRepo,
		blueprintRepo,
		githubClient,
		encryptionSvc,
		eventManager,
		logger,
	)
}

// sampleInstallationInfo returns a reusable GitHubInstallationInfo for tests.
func sampleInstallationInfo() *external.GitHubInstallationInfo {
	return &external.GitHubInstallationInfo{
		AccountLogin: "myorg",
		AccountType:  "Organization",
		TargetType:   "Organization",
		Permissions:  map[string]string{"contents": "read"},
		Events:       []string{"push"},
		SuspendedAt:  nil,
	}
}

// TestHandleInstallationCallback_NewInstallation verifies that a brand-new installation
// (no existing record for the installationID) is created successfully and reconnected=false.
func TestHandleInstallationCallback_NewInstallation(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	const installationID = int64(12345)
	const teamID = "team-aaa"
	const userID = "user-bbb"

	githubClient.On("GetInstallation", mock.Anything, installationID).
		Return(sampleInstallationInfo(), nil)

	// No existing record by installationID
	installationRepo.On("GetByInstallationID", mock.Anything, installationID).
		Return(nil, repositories.ErrGitHubInstallationNotFound)

	// No existing record by teamID
	installationRepo.On("GetByTeamID", mock.Anything, teamID).
		Return(nil, repositories.ErrGitHubInstallationNotFound)

	// Create should succeed
	installationRepo.On("Create", mock.Anything, mock.MatchedBy(func(inst *models.GitHubInstallation) bool {
		return inst.TeamID == teamID && inst.InstallationID == installationID
	})).Return(nil)

	eventManager.On("Publish", mock.Anything, mock.Anything).Return(nil)

	svc := newCallbackTestService(installationRepo, githubClient, eventManager)

	reconnected, err := svc.HandleInstallationCallback(context.Background(), userID, teamID, installationID)

	assert.NoError(t, err)
	assert.False(t, reconnected, "new installation should have reconnected=false")

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
	eventManager.AssertExpectations(t)
}

// TestHandleInstallationCallback_SameTeamReconnect verifies that when the same team reconnects
// (the installationID already points to the same teamID), reconnected=true is returned.
func TestHandleInstallationCallback_SameTeamReconnect(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	const installationID = int64(12345)
	const teamID = "team-aaa"
	const userID = "user-bbb"

	githubClient.On("GetInstallation", mock.Anything, installationID).
		Return(sampleInstallationInfo(), nil)

	// Existing record found for installationID — same team
	existingByInstallID := &models.GitHubInstallation{
		ID:             "install-record-1",
		TeamID:         teamID, // same team
		InstallationID: installationID,
		AccountLogin:   "myorg",
	}
	installationRepo.On("GetByInstallationID", mock.Anything, installationID).
		Return(existingByInstallID, nil)

	// Existing record found for teamID (for upsert path)
	existingByTeamID := &models.GitHubInstallation{
		ID:             "install-record-1",
		TeamID:         teamID,
		InstallationID: installationID,
	}
	installationRepo.On("GetByTeamID", mock.Anything, teamID).
		Return(existingByTeamID, nil)

	// Update should be called (not create)
	installationRepo.On("Update", mock.Anything, mock.MatchedBy(func(inst *models.GitHubInstallation) bool {
		return inst.ID == "install-record-1" && inst.TeamID == teamID
	})).Return(nil)

	eventManager.On("Publish", mock.Anything, mock.Anything).Return(nil)

	svc := newCallbackTestService(installationRepo, githubClient, eventManager)

	reconnected, err := svc.HandleInstallationCallback(context.Background(), userID, teamID, installationID)

	assert.NoError(t, err)
	assert.True(t, reconnected, "same-team reconnect should have reconnected=true")

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
	eventManager.AssertExpectations(t)
}

// TestHandleInstallationCallback_CrossTeamConflict verifies that when a different team
// already has the installationID, ErrInstallationAlreadyConnected is returned.
func TestHandleInstallationCallback_CrossTeamConflict(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	const installationID = int64(12345)
	const teamID = "team-aaa"      // requesting team
	const otherTeamID = "team-bbb" // team that already owns the installation
	const userID = "user-ccc"

	githubClient.On("GetInstallation", mock.Anything, installationID).
		Return(sampleInstallationInfo(), nil)

	// Existing record for installationID belongs to a different team
	existingByInstallID := &models.GitHubInstallation{
		ID:             "install-record-other",
		TeamID:         otherTeamID, // different team!
		InstallationID: installationID,
		AccountLogin:   "myorg",
	}
	installationRepo.On("GetByInstallationID", mock.Anything, installationID).
		Return(existingByInstallID, nil)

	svc := newCallbackTestService(installationRepo, githubClient, eventManager)

	reconnected, err := svc.HandleInstallationCallback(context.Background(), userID, teamID, installationID)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInstallationAlreadyConnected),
		"expected ErrInstallationAlreadyConnected but got: %v", err)
	assert.False(t, reconnected)

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
	// eventManager should not be called on conflict
	eventManager.AssertNotCalled(t, "Publish")
}

// TestHandleInstallationCallback_GetInstallationError verifies that GitHub API errors
// propagate as expected.
func TestHandleInstallationCallback_GetInstallationError(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	const installationID = int64(12345)
	const teamID = "team-aaa"
	const userID = "user-bbb"

	githubClient.On("GetInstallation", mock.Anything, installationID).
		Return(nil, errors.New("github api error"))

	svc := newCallbackTestService(installationRepo, githubClient, eventManager)

	reconnected, err := svc.HandleInstallationCallback(context.Background(), userID, teamID, installationID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get installation info")
	assert.False(t, reconnected)

	installationRepo.AssertNotCalled(t, "GetByInstallationID")
	installationRepo.AssertNotCalled(t, "Create")
	installationRepo.AssertNotCalled(t, "Update")
}

// TestHandleInstallationCallback_ExistingByTeamIDOnly verifies that when there is no record
// by installationID but there IS a record by teamID, the installation is updated and
// reconnected=true is returned.
func TestHandleInstallationCallback_ExistingByTeamIDOnly(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	const installationID = int64(99999)
	const teamID = "team-zzz"
	const userID = "user-yyy"

	githubClient.On("GetInstallation", mock.Anything, installationID).
		Return(sampleInstallationInfo(), nil)

	// No record by installationID (first time this installationID is used)
	installationRepo.On("GetByInstallationID", mock.Anything, installationID).
		Return(nil, repositories.ErrGitHubInstallationNotFound)

	// But team already has a different installation (e.g. from a previous connection)
	existingByTeamID := &models.GitHubInstallation{
		ID:             "install-record-old",
		TeamID:         teamID,
		InstallationID: 11111, // old installation ID
	}
	installationRepo.On("GetByTeamID", mock.Anything, teamID).
		Return(existingByTeamID, nil)

	// Should update (not create)
	installationRepo.On("Update", mock.Anything, mock.MatchedBy(func(inst *models.GitHubInstallation) bool {
		return inst.ID == "install-record-old" && inst.InstallationID == installationID
	})).Return(nil)

	eventManager.On("Publish", mock.Anything, mock.Anything).Return(nil)

	svc := newCallbackTestService(installationRepo, githubClient, eventManager)

	reconnected, err := svc.HandleInstallationCallback(context.Background(), userID, teamID, installationID)

	assert.NoError(t, err)
	assert.True(t, reconnected, "team had an existing installation record so reconnected should be true")

	installationRepo.AssertExpectations(t)
}

// TestErrInstallationAlreadyConnectedIsSentinel verifies the sentinel error is usable with errors.Is.
func TestErrInstallationAlreadyConnectedIsSentinel(t *testing.T) {
	wrapped := errors.Join(errors.New("outer"), ErrInstallationAlreadyConnected)
	assert.True(t, errors.Is(wrapped, ErrInstallationAlreadyConnected))
}

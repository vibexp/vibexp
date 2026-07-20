package services_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicemocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// TestMemoryService_UpdateSnapshotsOldContent verifies the memory update hook:
// when the text changes, the prior text is handed to the content-version core via
// SnapshotIfChanged as a human-authored edit with no change summary (no user-facing
// change-summary field exists, mirroring artifacts/blueprints); when it does not change,
// no snapshot is taken. The memory's Text field is the versioned content.
//
//nolint:funlen // Table-driven test with comprehensive snapshot-request assertions
func TestMemoryService_UpdateSnapshotsOldContent(t *testing.T) {
	const (
		userID   = "user-1"
		teamID   = "team-1"
		memoryID = "mem-1"
	)

	tests := []struct {
		name          string
		oldContent    string
		newContent    string
		expectSnaphot bool
	}{
		{name: "text changed snapshots old text", oldContent: "v1", newContent: "v2", expectSnaphot: true},
		{name: "text unchanged does not snapshot", oldContent: "v1", newContent: "v1", expectSnaphot: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := repomocks.NewMockMemoryRepository(t)
			cvs := servicemocks.NewMockContentVersionServiceInterface(t)
			logger, _ := logtest.New()

			existing := &models.Memory{
				ID: memoryID, TeamID: teamID, UserID: userID, Text: tt.oldContent,
			}
			repo.EXPECT().
				GetByID(mock.Anything, userID, teamID, memoryID).
				Return(existing, nil).
				Once()
			repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

			if tt.expectSnaphot {
				cvs.EXPECT().
					SnapshotIfChanged(
						mock.Anything,
						mock.MatchedBy(func(req services.SnapshotRequest) bool {
							return req.ResourceType == "memory" &&
								req.ResourceID == memoryID &&
								req.TeamID == teamID &&
								req.UserID == userID &&
								req.OldContent == tt.oldContent &&
								req.NewContent == tt.newContent &&
								req.ActorType == models.ActorTypeHuman &&
								req.ChangeSummary == nil
						}),
					).
					Return(nil).
					Once()
			}

			svc := services.NewMemoryService(repo, nil, permissiveAuthz(t), nil, logger, cvs, nil, nil)

			text := tt.newContent
			_, err := svc.UpdateMemory(userID, teamID, memoryID, &models.UpdateMemoryRequest{Text: &text})
			require.NoError(t, err)
		})
	}
}

// TestMemoryService_RestoreSnapshotsAsSystem verifies that restoring a version snapshots the
// pre-restore text as a system-authored version with a "Restored Version N" summary, keeping
// restore non-destructive.
func TestMemoryService_RestoreSnapshotsAsSystem(t *testing.T) {
	const (
		userID   = "user-1"
		teamID   = "team-1"
		memoryID = "mem-1"
	)

	repo := repomocks.NewMockMemoryRepository(t)
	cvs := servicemocks.NewMockContentVersionServiceInterface(t)
	logger, _ := logtest.New()

	existing := &models.Memory{
		ID: memoryID, TeamID: teamID, UserID: userID, Text: "live-content",
	}
	repo.EXPECT().
		GetByID(mock.Anything, userID, teamID, memoryID).
		Return(existing, nil).
		Once()
	cvs.EXPECT().
		Restore(mock.Anything, teamID, "memory", memoryID, 2).
		Return("v2-content", nil).
		Once()
	cvs.EXPECT().
		SnapshotIfChanged(
			mock.Anything,
			mock.MatchedBy(func(req services.SnapshotRequest) bool {
				return req.OldContent == "live-content" && // pre-restore text snapshotted (non-destructive)
					req.NewContent == "v2-content" &&
					req.ActorType == models.ActorTypeSystem &&
					req.ChangeSummary != nil && *req.ChangeSummary == "Restored Version 2"
			}),
		).
		Return(nil).
		Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	svc := services.NewMemoryService(repo, nil, permissiveAuthz(t), nil, logger, cvs, nil, nil)
	restored, err := svc.RestoreMemoryVersion(userID, teamID, memoryID, 2)
	require.NoError(t, err)
	require.Equal(t, "v2-content", restored.Text)
}

// TestMemoryService_ListAndGetVersionsScopeByTeam verifies the list/get read paths load the
// memory through the team-scoped lookup and query the content-version core scoped by the
// resolved memory's TeamID.
func TestMemoryService_ListAndGetVersionsScopeByTeam(t *testing.T) {
	const (
		userID   = "user-1"
		teamID   = "team-1"
		memoryID = "mem-1"
	)

	t.Run("list", func(t *testing.T) {
		repo := repomocks.NewMockMemoryRepository(t)
		cvs := servicemocks.NewMockContentVersionServiceInterface(t)
		logger, _ := logtest.New()

		existing := &models.Memory{ID: memoryID, TeamID: teamID, UserID: userID, Text: "x"}
		repo.EXPECT().GetByID(mock.Anything, userID, teamID, memoryID).Return(existing, nil).Once()
		want := []*models.ContentVersion{{VersionNumber: 1}}
		cvs.EXPECT().ListVersions(mock.Anything, teamID, "memory", memoryID).Return(want, nil).Once()

		svc := services.NewMemoryService(repo, nil, permissiveAuthz(t), nil, logger, cvs, nil, nil)
		got, err := svc.ListMemoryVersions(userID, teamID, memoryID)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("get", func(t *testing.T) {
		repo := repomocks.NewMockMemoryRepository(t)
		cvs := servicemocks.NewMockContentVersionServiceInterface(t)
		logger, _ := logtest.New()

		existing := &models.Memory{ID: memoryID, TeamID: teamID, UserID: userID, Text: "x"}
		repo.EXPECT().GetByID(mock.Anything, userID, teamID, memoryID).Return(existing, nil).Once()
		want := &models.ContentVersion{VersionNumber: 3}
		cvs.EXPECT().GetVersion(mock.Anything, teamID, "memory", memoryID, 3).Return(want, nil).Once()

		svc := services.NewMemoryService(repo, nil, permissiveAuthz(t), nil, logger, cvs, nil, nil)
		got, err := svc.GetMemoryVersion(userID, teamID, memoryID, 3)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}

// TestRestoreMemoryVersion_IsGated pins that version-restore is authorized.
//
// Restore reaches applyAndPersistMemoryUpdate DIRECTLY, bypassing UpdateMemory —
// so gating the entry point instead of the shared helper would leave this write
// open. Artifact and blueprint restore have the same shape, which is why all
// three gate the helper (#237).
func TestRestoreMemoryVersion_IsGated(t *testing.T) {
	const (
		userID   = "user-caller"
		teamID   = "team-1"
		memoryID = "memory-1"
	)

	repo := repomocks.NewMockMemoryRepository(t)
	repo.EXPECT().GetByID(mock.Anything, userID, teamID, memoryID).
		Return(&models.Memory{ID: memoryID, UserID: "user-other", TeamID: teamID}, nil).Once()

	cvs := servicemocks.NewMockContentVersionServiceInterface(t)
	cvs.EXPECT().Restore(mock.Anything, teamID, "memory", memoryID, 2).
		Return("restored text", nil).Once()

	// A caller the matrix denies.
	authzMock := servicemocks.NewMockAuthorizationServiceInterface(t)
	authzMock.EXPECT().Can(mock.Anything, userID, teamID, mock.Anything).
		Return(services.ErrPermissionDenied).Once()

	logger, _ := logtest.New()
	svc := services.NewMemoryService(repo, nil, authzMock, nil, logger, cvs, nil, nil)

	_, err := svc.RestoreMemoryVersion(userID, teamID, memoryID, 2)

	require.ErrorIs(t, err, services.ErrPermissionDenied)
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

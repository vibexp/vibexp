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

// TestBlueprintService_UpdateSnapshotsOldContent verifies the blueprint update hook:
// when the content changes, the prior content is handed to the content-version core via
// SnapshotIfChanged as a human-authored edit with no change summary (no user-facing
// change-summary field exists, mirroring artifacts); when it does not change, no snapshot
// is taken.
//
//nolint:funlen // Table-driven test with comprehensive snapshot-request assertions
func TestBlueprintService_UpdateSnapshotsOldContent(t *testing.T) {
	const (
		userID    = "user-1"
		teamID    = "team-1"
		projectID = "proj-1"
		slug      = "doc"
	)

	tests := []struct {
		name          string
		oldContent    string
		newContent    string
		expectSnaphot bool
	}{
		{name: "content changed snapshots old content", oldContent: "v1", newContent: "v2", expectSnaphot: true},
		{name: "content unchanged does not snapshot", oldContent: "v1", newContent: "v1", expectSnaphot: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := repomocks.NewMockBlueprintRepository(t)
			cvs := servicemocks.NewMockContentVersionServiceInterface(t)
			logger, _ := logtest.New()

			existing := &models.Blueprint{
				ID: "bp-1", ProjectID: projectID, Slug: slug, TeamID: teamID,
				UserID: userID, Content: tt.oldContent,
			}
			repo.EXPECT().
				GetByProjectIDAndSlugCrossTeam(mock.Anything, userID, projectID, slug).
				Return(existing, nil).
				Once()
			repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

			if tt.expectSnaphot {
				cvs.EXPECT().
					SnapshotIfChanged(
						mock.Anything,
						mock.MatchedBy(func(req services.SnapshotRequest) bool {
							return req.ResourceType == "blueprint" &&
								req.ResourceID == "bp-1" &&
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

			svc := services.NewBlueprintService(services.BlueprintServiceDeps{
				Repo:              repo,
				TeamService:       nil,
				Authz:             permissiveAuthz(t),
				EventManager:      nil,
				ResourceUsageSvc:  nil,
				Logger:            logger,
				ContentVersionSvc: cvs,
				CommentRepo:       nil,
			})

			content := tt.newContent
			_, err := svc.UpdateBlueprintByProjectIDAndSlug(
				userID, projectID, slug, &models.UpdateBlueprintRequest{Content: &content},
			)
			require.NoError(t, err)
		})
	}
}

// TestBlueprintService_RestoreSnapshotsAsSystem verifies that restoring a version snapshots
// the pre-restore content as a system-authored version with a "Restored Version N" summary,
// keeping restore non-destructive.
func TestBlueprintService_RestoreSnapshotsAsSystem(t *testing.T) {
	const (
		userID    = "user-1"
		teamID    = "team-1"
		projectID = "proj-1"
		slug      = "doc"
	)

	repo := repomocks.NewMockBlueprintRepository(t)
	cvs := servicemocks.NewMockContentVersionServiceInterface(t)
	logger, _ := logtest.New()

	existing := &models.Blueprint{
		ID: "bp-1", ProjectID: projectID, Slug: slug, TeamID: teamID,
		UserID: userID, Content: "live-content",
	}
	repo.EXPECT().
		GetByProjectIDAndSlug(mock.Anything, userID, teamID, projectID, slug).
		Return(existing, nil).
		Once()
	cvs.EXPECT().
		Restore(mock.Anything, teamID, "blueprint", "bp-1", 2).
		Return("v2-content", nil).
		Once()
	cvs.EXPECT().
		SnapshotIfChanged(
			mock.Anything,
			mock.MatchedBy(func(req services.SnapshotRequest) bool {
				return req.OldContent == "live-content" && // pre-restore content snapshotted (non-destructive)
					req.NewContent == "v2-content" &&
					req.ActorType == models.ActorTypeSystem &&
					req.ChangeSummary != nil && *req.ChangeSummary == "Restored Version 2"
			}),
		).
		Return(nil).
		Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	svc := services.NewBlueprintService(services.BlueprintServiceDeps{
		Repo:              repo,
		TeamService:       nil,
		Authz:             permissiveAuthz(t),
		EventManager:      nil,
		ResourceUsageSvc:  nil,
		Logger:            logger,
		ContentVersionSvc: cvs,
		CommentRepo:       nil,
	})
	_, err := svc.RestoreBlueprintVersionInTeam(userID, teamID, projectID, slug, 2)
	require.NoError(t, err)
}

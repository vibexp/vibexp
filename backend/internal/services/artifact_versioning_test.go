package services_test

import (
	"testing"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicemocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// TestArtifactService_UpdateSnapshotsOldContent verifies the artifact update hook:
// when the content changes, the prior content is handed to the content-version core
// via SnapshotIfChanged; when it does not change, no snapshot is taken.
func TestArtifactService_UpdateSnapshotsOldContent(t *testing.T) {
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
			repo := repomocks.NewMockArtifactRepository(t)
			cvs := servicemocks.NewMockContentVersionServiceInterface(t)
			logger, _ := test.NewNullLogger()

			existing := &models.Artifact{
				ID: "art-1", ProjectID: projectID, Slug: slug, TeamID: teamID,
				UserID: userID, Content: tt.oldContent,
			}
			repo.EXPECT().
				GetByProjectIDAndSlug(mock.Anything, userID, teamID, projectID, slug).
				Return(existing, nil).
				Once()
			repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

			if tt.expectSnaphot {
				cvs.EXPECT().
					SnapshotIfChanged(
						mock.Anything,
						mock.MatchedBy(func(req services.SnapshotRequest) bool {
							return req.ResourceType == "artifact" &&
								req.ResourceID == "art-1" &&
								req.TeamID == teamID &&
								req.UserID == userID &&
								req.OldContent == tt.oldContent &&
								req.NewContent == tt.newContent &&
								req.ActorType == models.ActorTypeHuman
						}),
					).
					Return(nil).
					Once()
			}

			svc := services.NewArtifactService(repo, nil, nil, nil, logger, cvs)

			content := tt.newContent
			_, err := svc.UpdateArtifactByProjectIDAndSlugInTeam(
				userID, teamID, projectID, slug, &models.UpdateArtifactRequest{Content: &content},
			)
			require.NoError(t, err)
		})
	}
}

// TestArtifactService_RestoreSnapshotsAsSystem verifies that restoring a version snapshots
// the pre-restore content as a system-authored version with a "Restored Version N" summary,
// keeping restore non-destructive.
func TestArtifactService_RestoreSnapshotsAsSystem(t *testing.T) {
	const (
		userID    = "user-1"
		teamID    = "team-1"
		projectID = "proj-1"
		slug      = "doc"
	)

	repo := repomocks.NewMockArtifactRepository(t)
	cvs := servicemocks.NewMockContentVersionServiceInterface(t)
	logger, _ := test.NewNullLogger()

	existing := &models.Artifact{
		ID: "art-1", ProjectID: projectID, Slug: slug, TeamID: teamID,
		UserID: userID, Content: "live-content",
	}
	repo.EXPECT().
		GetByProjectIDAndSlug(mock.Anything, userID, teamID, projectID, slug).
		Return(existing, nil).
		Once()
	cvs.EXPECT().
		Restore(mock.Anything, teamID, "artifact", "art-1", 2).
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

	svc := services.NewArtifactService(repo, nil, nil, nil, logger, cvs)
	_, err := svc.RestoreArtifactVersionInTeam(userID, teamID, projectID, slug, 2)
	require.NoError(t, err)
}

package services_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicemocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// TestPromptService_UpdateSnapshotsOldBody verifies the prompt update hook: when the raw Body
// template changes, the prior body is handed to the content-version core via SnapshotIfChanged
// as a human-authored edit with no change summary (no user-facing change-summary field exists,
// mirroring artifacts/blueprints/memory); when it does not change, no snapshot is taken. The
// prompt's Body field (the raw template, not rendered output) is the versioned content.
//
//nolint:funlen // Table-driven test with comprehensive snapshot-request assertions
func TestPromptService_UpdateSnapshotsOldBody(t *testing.T) {
	const (
		userID   = "user-1"
		teamID   = "team-1"
		promptID = "prompt-1"
	)

	tests := []struct {
		name         string
		oldBody      string
		newBody      string
		expectSnapot bool
	}{
		// The raw template with placeholders/@refs is versioned verbatim.
		{
			name:    "body changed snapshots old template",
			oldBody: "Hello {{name}}", newBody: "Hi {{name}} @intro",
			expectSnapot: true,
		},
		{
			name:    "body unchanged does not snapshot",
			oldBody: "Hello {{name}}", newBody: "Hello {{name}}",
			expectSnapot: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := repomocks.NewMockPromptRepository(t)
			refRepo := repomocks.NewMockPromptReferenceRepository(t)
			cvs := servicemocks.NewMockContentVersionServiceInterface(t)
			logger, _ := logtest.New()

			existing := &models.Prompt{
				ID: promptID, TeamID: teamID, UserID: userID, Slug: "p", Body: tt.oldBody,
			}
			repo.EXPECT().
				GetByID(mock.Anything, userID, teamID, promptID).
				Return(existing, nil).
				Once()
			repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
			// Body is provided, so references are re-extracted (delete-then-recreate). The
			// test bodies contain no resolvable @refs, so only the delete is exercised.
			refRepo.EXPECT().DeleteByPromptID(mock.Anything, promptID).Return(nil).Once()
			if tt.newBody == "Hi {{name}} @intro" {
				// The @intro reference is looked up; treat it as not found so no batch insert.
				repo.EXPECT().
					GetBySlugCrossTeam(mock.Anything, userID, "intro").
					Return(nil, repositories.ErrPromptNotFound).
					Once()
			}

			if tt.expectSnapot {
				cvs.EXPECT().
					SnapshotIfChanged(
						mock.Anything,
						mock.MatchedBy(func(req services.SnapshotRequest) bool {
							return req.ResourceType == "prompt" &&
								req.ResourceID == promptID &&
								req.TeamID == teamID &&
								req.UserID == userID &&
								req.OldContent == tt.oldBody &&
								req.NewContent == tt.newBody &&
								req.ActorType == models.ActorTypeHuman &&
								req.ChangeSummary == nil
						}),
					).
					Return(nil).
					Once()
			}

			svc := services.NewPromptService(repo, refRepo, nil, nil, nil, permissiveAuthz(t), nil, logger, cvs, nil)

			body := tt.newBody
			_, err := svc.UpdatePrompt(userID, teamID, promptID, &models.UpdatePromptRequest{Body: &body})
			require.NoError(t, err)
		})
	}
}

// TestPromptService_RestoreSnapshotsAsSystem verifies that restoring a version snapshots the
// pre-restore body as a system-authored version with a "Restored Version N" summary, keeping
// restore non-destructive and re-applying the raw template verbatim.
func TestPromptService_RestoreSnapshotsAsSystem(t *testing.T) {
	const (
		userID   = "user-1"
		teamID   = "team-1"
		promptID = "prompt-1"
		slug     = "p"
	)

	repo := repomocks.NewMockPromptRepository(t)
	refRepo := repomocks.NewMockPromptReferenceRepository(t)
	cvs := servicemocks.NewMockContentVersionServiceInterface(t)
	logger, _ := logtest.New()

	existing := &models.Prompt{
		ID: promptID, TeamID: teamID, UserID: userID, Slug: slug, Body: "live {{x}}",
	}
	// Resolve the prompt by slug, then re-apply through the update path (re-loads by ID).
	repo.EXPECT().GetBySlug(mock.Anything, userID, teamID, slug).Return(existing, nil).Once()
	repo.EXPECT().GetByID(mock.Anything, userID, teamID, promptID).Return(existing, nil).Once()
	cvs.EXPECT().
		Restore(mock.Anything, teamID, "prompt", promptID, 2).
		Return("v2 {{x}}", nil).
		Once()
	cvs.EXPECT().
		SnapshotIfChanged(
			mock.Anything,
			mock.MatchedBy(func(req services.SnapshotRequest) bool {
				return req.OldContent == "live {{x}}" && // pre-restore body snapshotted (non-destructive)
					req.NewContent == "v2 {{x}}" &&
					req.ActorType == models.ActorTypeSystem &&
					req.ChangeSummary != nil && *req.ChangeSummary == "Restored Version 2"
			}),
		).
		Return(nil).
		Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
	refRepo.EXPECT().DeleteByPromptID(mock.Anything, promptID).Return(nil).Once()

	svc := services.NewPromptService(repo, refRepo, nil, nil, nil, permissiveAuthz(t), nil, logger, cvs, nil)
	restored, err := svc.RestorePromptVersion(userID, teamID, slug, 2)
	require.NoError(t, err)
	require.Equal(t, "v2 {{x}}", restored.Body)
}

// TestPromptService_ListAndGetVersionsScopeByTeam verifies the list/get read paths load the
// prompt through the team-scoped slug lookup and query the content-version core scoped by the
// resolved prompt's TeamID.
func TestPromptService_ListAndGetVersionsScopeByTeam(t *testing.T) {
	const (
		userID = "user-1"
		teamID = "team-1"
		slug   = "p"
		id     = "prompt-1"
	)

	t.Run("list", func(t *testing.T) {
		repo := repomocks.NewMockPromptRepository(t)
		cvs := servicemocks.NewMockContentVersionServiceInterface(t)
		logger, _ := logtest.New()

		existing := &models.Prompt{ID: id, TeamID: teamID, UserID: userID, Slug: slug, Body: "x"}
		repo.EXPECT().GetBySlug(mock.Anything, userID, teamID, slug).Return(existing, nil).Once()
		want := []*models.ContentVersion{{VersionNumber: 1}}
		cvs.EXPECT().ListVersions(mock.Anything, teamID, "prompt", id).Return(want, nil).Once()

		svc := services.NewPromptService(repo, nil, nil, nil, nil, permissiveAuthz(t), nil, logger, cvs, nil)
		got, err := svc.ListPromptVersions(userID, teamID, slug)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("get", func(t *testing.T) {
		repo := repomocks.NewMockPromptRepository(t)
		cvs := servicemocks.NewMockContentVersionServiceInterface(t)
		logger, _ := logtest.New()

		existing := &models.Prompt{ID: id, TeamID: teamID, UserID: userID, Slug: slug, Body: "x"}
		repo.EXPECT().GetBySlug(mock.Anything, userID, teamID, slug).Return(existing, nil).Once()
		want := &models.ContentVersion{VersionNumber: 3}
		cvs.EXPECT().GetVersion(mock.Anything, teamID, "prompt", id, 3).Return(want, nil).Once()

		svc := services.NewPromptService(repo, nil, nil, nil, nil, permissiveAuthz(t), nil, logger, cvs, nil)
		got, err := svc.GetPromptVersion(userID, teamID, slug, 3)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}

// permissiveAuthz allows every permission. These tests are in the external test
// package, so the in-package allowAllAuthz double is not reachable; they cover
// versioning mechanics rather than authorization, which the *_rbac_test.go files
// assert.
func permissiveAuthz(t *testing.T) services.AuthorizationServiceInterface {
	t.Helper()
	m := servicemocks.NewMockAuthorizationServiceInterface(t)
	m.EXPECT().Can(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	m.EXPECT().CanActOnResource(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(nil).Maybe()
	return m
}

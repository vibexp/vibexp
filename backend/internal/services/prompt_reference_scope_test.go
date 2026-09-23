package services

// #1100: prompt @references resolve within the team that owns the prompt being
// rendered or saved — never among the reader's (or author's) own prompts in any
// team. The mockery repo is strict, so a lookup against the wrong team, or one
// keyed on a user instead of a team, fails the test as an unexpected call.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	refScopeTeam   = "team-t"
	refScopeAuthor = "user-a"
	refScopeReader = "user-b"
)

func newRefScopeService(
	repo repositories.PromptRepository, refRepo repositories.PromptReferenceRepository,
	projectRepo repositories.ProjectRepository,
) *PromptService {
	logger := func() *slog.Logger { l, _ := logtest.New(); return l }()
	return NewPromptService(PromptServiceDeps{
		Repo:        repo,
		RefRepo:     refRepo,
		ProjectRepo: projectRepo,
		Authz:       allowAllAuthz{},
		Logger:      logger,
	})
}

func TestRenderPrompt_TeammateReadsAuthorsReferenceFromTheTeam(t *testing.T) {
	repo := mocks.NewMockPromptRepository(t)
	svc := createTestPromptService(repo, nil)

	// A authored "review" in team T; B (a member of T) renders it.
	repo.EXPECT().GetBySlug(mock.Anything, refScopeReader, refScopeTeam, "review").
		Return(&models.Prompt{
			ID: "review-id", Slug: "review", Body: "Review per @style-guide",
			UserID: refScopeAuthor, TeamID: refScopeTeam,
		}, nil).Once()
	// The reference resolves in T, whoever authored it — never among B's prompts.
	repo.EXPECT().GetBySlugInTeam(mock.Anything, refScopeTeam, "style-guide").
		Return(&models.Prompt{
			ID: "sg-id", Slug: "style-guide", Body: "TEAM STYLE",
			UserID: refScopeAuthor, TeamID: refScopeTeam,
		}, nil).Once()

	resp, err := svc.RenderPrompt(refScopeReader, refScopeTeam, "review", map[string]string{})

	require.NoError(t, err)
	assert.Equal(t, "Review per TEAM STYLE", resp.RenderedBody)
	assert.Equal(t, []string{"style-guide"}, resp.ReferencesUsed)
	assert.Empty(t, resp.Warnings)
}

func TestRenderPrompt_NestedReferencesStayInTheRootTeam(t *testing.T) {
	repo := mocks.NewMockPromptRepository(t)
	svc := createTestPromptService(repo, nil)

	// The user owns slug "x" in both T and another team; only T's may be used,
	// at every depth of the reference tree.
	repo.EXPECT().GetBySlug(mock.Anything, refScopeAuthor, refScopeTeam, "root").
		Return(&models.Prompt{ID: "root-id", Body: "R @x", UserID: refScopeAuthor, TeamID: refScopeTeam}, nil).Once()
	repo.EXPECT().GetBySlugInTeam(mock.Anything, refScopeTeam, "x").
		Return(&models.Prompt{ID: "x-t", Slug: "x", Body: "X-of-T @y", TeamID: refScopeTeam}, nil).Once()
	repo.EXPECT().GetBySlugInTeam(mock.Anything, refScopeTeam, "y").
		Return(&models.Prompt{ID: "y-t", Slug: "y", Body: "Y-of-T", TeamID: refScopeTeam}, nil).Once()

	resp, err := svc.RenderPrompt(refScopeAuthor, refScopeTeam, "root", map[string]string{})

	require.NoError(t, err)
	assert.Equal(t, "R X-of-T Y-of-T", resp.RenderedBody)
	assert.Equal(t, []string{"x", "y"}, resp.ReferencesUsed)
}

func TestGetPromptPlaceholders_FollowsTeammateReferenceInTeam(t *testing.T) {
	repo := mocks.NewMockPromptRepository(t)
	svc := createTestPromptService(repo, nil)

	repo.EXPECT().GetBySlug(mock.Anything, refScopeReader, refScopeTeam, "review").
		Return(&models.Prompt{
			ID: "review-id", Body: "{{topic}} @style-guide", UserID: refScopeAuthor, TeamID: refScopeTeam,
		}, nil).Once()
	repo.EXPECT().GetBySlugInTeam(mock.Anything, refScopeTeam, "style-guide").
		Return(&models.Prompt{ID: "sg-id", Body: "tone: {{tone}}", TeamID: refScopeTeam}, nil).Once()

	keys, err := svc.GetPromptPlaceholders(refScopeReader, refScopeTeam, "review")

	require.NoError(t, err)
	assert.Equal(t, []string{"topic", "tone"}, keys)
}

func TestRenderPromptBody_ResolvesWithinGivenTeam(t *testing.T) {
	repo := mocks.NewMockPromptRepository(t)
	svc := createTestPromptService(repo, nil)

	repo.EXPECT().GetBySlugInTeam(mock.Anything, refScopeTeam, "footer").
		Return(&models.Prompt{ID: "f-id", Body: "FOOTER", TeamID: refScopeTeam}, nil).Once()

	out, err := svc.RenderPromptBody(refScopeTeam, "Hi {{name}} @footer")

	require.NoError(t, err)
	assert.Equal(t, "Hi {{name}} FOOTER", out)
}

func TestUpdatePromptReferences_StoresTeamEdgesOnly(t *testing.T) {
	repo := mocks.NewMockPromptRepository(t)
	refRepo := mocks.NewMockPromptReferenceRepository(t)
	svc := newRefScopeService(repo, refRepo, nil)

	refRepo.EXPECT().DeleteByPromptID(mock.Anything, "saved-id").Return(nil).Once()
	// A teammate's prompt in the same team: stored as a dependency edge.
	repo.EXPECT().GetBySlugInTeam(mock.Anything, refScopeTeam, "teammates").
		Return(&models.Prompt{ID: "teammate-prompt-id", UserID: refScopeReader, TeamID: refScopeTeam}, nil).Once()
	// A slug that exists only in another team is not found in T: not stored.
	repo.EXPECT().GetBySlugInTeam(mock.Anything, refScopeTeam, "elsewhere").
		Return(nil, repositories.ErrPromptNotFound).Once()
	// A self-reference is not a dependency.
	repo.EXPECT().GetBySlugInTeam(mock.Anything, refScopeTeam, "saved").
		Return(&models.Prompt{ID: "saved-id", TeamID: refScopeTeam}, nil).Once()
	refRepo.EXPECT().CreateBatch(mock.Anything, mock.MatchedBy(func(refs []models.PromptReference) bool {
		return len(refs) == 1 &&
			refs[0].PromptID == "saved-id" &&
			refs[0].ReferencedPromptID == "teammate-prompt-id"
	})).Return(nil).Once()

	err := svc.updatePromptReferences(context.Background(), refScopeTeam, "saved-id",
		"@teammates @elsewhere @saved user@@example.com")

	require.NoError(t, err)
}

func TestCreatePrompt_ResolvesReferencesInTheCreatedPromptsTeam(t *testing.T) {
	repo := mocks.NewMockPromptRepository(t)
	refRepo := mocks.NewMockPromptReferenceRepository(t)
	projectRepo := mocks.NewMockProjectRepository(t)
	projectRepo.EXPECT().GetByID(mock.Anything, refScopeAuthor, "project-1").
		Return(&models.Project{ID: "project-1", UserID: refScopeAuthor}, nil).Once()
	svc := newRefScopeService(repo, refRepo, projectRepo)

	repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*models.Prompt")).
		Run(func(_ context.Context, p *models.Prompt) { p.ID = "new-id" }).
		Return(nil).Once()
	refRepo.EXPECT().DeleteByPromptID(mock.Anything, "new-id").Return(nil).Once()
	repo.EXPECT().GetBySlugInTeam(mock.Anything, refScopeTeam, "style-guide").
		Return(&models.Prompt{ID: "sg-id", UserID: refScopeReader, TeamID: refScopeTeam}, nil).Once()
	refRepo.EXPECT().CreateBatch(mock.Anything, mock.MatchedBy(func(refs []models.PromptReference) bool {
		return len(refs) == 1 && refs[0].ReferencedPromptID == "sg-id"
	})).Return(nil).Once()

	_, err := svc.CreatePrompt(refScopeAuthor, refScopeTeam, &models.CreatePromptRequest{
		Name: "Review", Slug: "review", Body: "Use @style-guide", ProjectID: "project-1",
	})

	require.NoError(t, err)
}

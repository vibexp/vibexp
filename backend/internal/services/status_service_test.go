package services

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The service layer is the ONLY place an out-of-subset status is rejected for
// an MCP client: the six MCP write tools call these methods directly, never the
// REST handlers, and the `validate:"oneof=..."` tags on the request models are
// inert. So each of these tests fails the moment its validateStatus call is
// removed from the service, which the handler-level tests cannot see (#912).
//
// Every mock is created with NO expectations on purpose: reaching the
// repository at all is the failure being guarded against.

func statusTestLogger() *slog.Logger {
	l, _ := logtest.New()
	return l
}

func TestPromptService_RejectsOutOfSubsetStatus(t *testing.T) {
	const (
		userID = "user-1"
		teamID = "team-1"
	)

	t.Run("create", func(t *testing.T) {
		svc := NewPromptService(PromptServiceDeps{
			Repo:   mocks.NewMockPromptRepository(t),
			Authz:  allowAllAuthz{},
			Logger: statusTestLogger(),
		})

		_, err := svc.CreatePrompt(userID, teamID, &models.CreatePromptRequest{
			Name: "n", Slug: "s", Body: "b", ProjectID: testServiceProjectID,
			Status: models.ArtifactStatusActive, // valid for artifacts, not for prompts
		})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})

	t.Run("update", func(t *testing.T) {
		repo := mocks.NewMockPromptRepository(t)
		repo.EXPECT().GetByID(mock.Anything, userID, teamID, "prompt-1").
			Return(&models.Prompt{ID: "prompt-1", UserID: userID, TeamID: teamID}, nil).Once()

		svc := NewPromptService(PromptServiceDeps{
			Repo:   repo,
			Authz:  allowAllAuthz{},
			Logger: statusTestLogger(),
		})

		status := models.BlueprintStatusExpired
		_, err := svc.UpdatePrompt(userID, teamID, "prompt-1", &models.UpdatePromptRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

func TestArtifactService_RejectsOutOfSubsetStatus(t *testing.T) {
	const (
		userID    = "user-1"
		teamID    = "team-1"
		projectID = testServiceProjectID
		slug      = "s"
	)

	t.Run("create", func(t *testing.T) {
		svc := NewArtifactService(ArtifactServiceDeps{
			Repo:   mocks.NewMockArtifactRepository(t),
			Authz:  allowAllAuthz{},
			Logger: statusTestLogger(),
		})

		_, err := svc.CreateArtifact(userID, teamID, &models.CreateArtifactRequest{
			ProjectID: projectID, Slug: slug, Title: "t", Content: "c",
			Status: models.PromptStatusPublished, // valid for prompts, not for artifacts
		})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})

	t.Run("update", func(t *testing.T) {
		repo := mocks.NewMockArtifactRepository(t)
		repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, userID, teamID, projectID, slug).
			Return(&models.Artifact{ID: "artifact-1", UserID: userID, TeamID: teamID}, nil).Once()

		svc := NewArtifactService(ArtifactServiceDeps{
			Repo:   repo,
			Authz:  allowAllAuthz{},
			Logger: statusTestLogger(),
		})

		status := models.BlueprintStatusExpired
		_, err := svc.UpdateArtifactByProjectIDAndSlugInTeam(userID, teamID, projectID, slug,
			&models.UpdateArtifactRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

func TestBlueprintService_RejectsOutOfSubsetStatus(t *testing.T) {
	const (
		userID    = "user-1"
		teamID    = "team-1"
		projectID = testServiceProjectID
		slug      = "s"
	)

	t.Run("create", func(t *testing.T) {
		svc := NewBlueprintService(BlueprintServiceDeps{
			Repo:   mocks.NewMockBlueprintRepository(t),
			Authz:  allowAllAuthz{},
			Logger: statusTestLogger(),
		})

		_, err := svc.CreateBlueprint(userID, teamID, &models.CreateBlueprintRequest{
			ProjectID: projectID, Slug: slug, Title: "t", Content: "c",
			Status: models.ArtifactStatusArchived, // valid for artifacts, not for blueprints
		})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})

	t.Run("update", func(t *testing.T) {
		repo := mocks.NewMockBlueprintRepository(t)
		repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, userID, teamID, projectID, slug).
			Return(&models.Blueprint{ID: "blueprint-1", UserID: userID, TeamID: teamID}, nil).Once()

		svc := NewBlueprintService(BlueprintServiceDeps{
			Repo:   repo,
			Authz:  allowAllAuthz{},
			Logger: statusTestLogger(),
		})

		status := models.MemoryStatusDraft
		_, err := svc.UpdateBlueprintByProjectIDAndSlugInTeam(userID, teamID, projectID, slug,
			&models.UpdateBlueprintRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

func TestMemoryService_RejectsOutOfSubsetStatus(t *testing.T) {
	const (
		userID   = "user-1"
		teamID   = "team-1"
		memoryID = "memory-1"
	)

	t.Run("create", func(t *testing.T) {
		svc := createTestMemoryService(mocks.NewMockMemoryRepository(t))

		status := models.BlueprintStatusExpired // valid for blueprints, not for memories
		_, err := svc.CreateMemory(userID, teamID, &models.CreateMemoryRequest{
			ProjectID: testServiceProjectID, Text: "t", Status: &status,
		})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})

	t.Run("update", func(t *testing.T) {
		repo := mocks.NewMockMemoryRepository(t)
		repo.EXPECT().GetByID(mock.Anything, userID, teamID, memoryID).
			Return(&models.Memory{ID: memoryID, UserID: userID, TeamID: teamID}, nil).Once()

		svc := createTestMemoryService(repo)

		status := models.PromptStatusPublished
		_, err := svc.UpdateMemory(userID, teamID, memoryID,
			&models.UpdateMemoryRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

// An EMPTY status is the other half of the contract, and the one with no
// handler standing in front of it: `status` is a REQUIRED response field
// constrained to an enum, so persisting "" puts a value on the wire that no
// generated client has a union member for. validateStatus deliberately accepts
// "" (it is the wire form of "not supplied"), which makes each update path
// solely responsible for not writing it -- so each one is pinned here (#912).
func TestServicesTreatEmptyStatusAsUnchanged(t *testing.T) {
	const (
		userID    = "user-1"
		teamID    = "team-1"
		projectID = testServiceProjectID
		slug      = "s"
	)
	empty := ""

	t.Run("prompt", func(t *testing.T) {
		repo := mocks.NewMockPromptRepository(t)
		repo.EXPECT().GetByID(mock.Anything, userID, teamID, "prompt-1").
			Return(&models.Prompt{ID: "prompt-1", UserID: userID, TeamID: teamID,
				Status: models.PromptStatusPublished}, nil).Once()
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
			return p.Status == models.PromptStatusPublished
		})).Return(nil).Once()

		svc := NewPromptService(PromptServiceDeps{
			Repo: repo, Authz: allowAllAuthz{}, Logger: statusTestLogger(),
		})

		_, err := svc.UpdatePrompt(userID, teamID, "prompt-1",
			&models.UpdatePromptRequest{Status: &empty})
		require.NoError(t, err)
	})

	t.Run("artifact", func(t *testing.T) {
		artifact := &models.Artifact{ID: "artifact-1", UserID: userID, TeamID: teamID,
			Status: models.ArtifactStatusArchived}
		applyArtifactUpdates(artifact, &models.UpdateArtifactRequest{Status: &empty})
		assert.Equal(t, models.ArtifactStatusArchived, artifact.Status)
	})

	t.Run("blueprint", func(t *testing.T) {
		blueprint := &models.Blueprint{ID: "blueprint-1", UserID: userID, TeamID: teamID,
			Status: models.BlueprintStatusExpired}
		applyBlueprintUpdates(blueprint, &models.UpdateBlueprintRequest{Status: &empty})
		assert.Equal(t, models.BlueprintStatusExpired, blueprint.Status)
	})

	t.Run("memory", func(t *testing.T) {
		memory := &models.Memory{ID: "memory-1", UserID: userID, TeamID: teamID,
			Status: models.MemoryStatusArchived}
		applyMemoryUpdates(memory, &models.UpdateMemoryRequest{Status: &empty})
		assert.Equal(t, models.MemoryStatusArchived, memory.Status)
	})
}

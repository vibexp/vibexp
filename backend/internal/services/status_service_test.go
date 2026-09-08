package services

import (
	"log/slog"
	"testing"

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
	svc := NewArtifactService(ArtifactServiceDeps{
		Repo:   mocks.NewMockArtifactRepository(t),
		Authz:  allowAllAuthz{},
		Logger: statusTestLogger(),
	})

	_, err := svc.CreateArtifact("user-1", "team-1", &models.CreateArtifactRequest{
		ProjectID: testServiceProjectID, Slug: "s", Title: "t", Content: "c",
		Status: models.PromptStatusPublished, // valid for prompts, not for artifacts
	})

	require.ErrorIs(t, err, ErrInvalidStatus)
}

func TestBlueprintService_RejectsOutOfSubsetStatus(t *testing.T) {
	svc := NewBlueprintService(BlueprintServiceDeps{
		Repo:   mocks.NewMockBlueprintRepository(t),
		Authz:  allowAllAuthz{},
		Logger: statusTestLogger(),
	})

	_, err := svc.CreateBlueprint("user-1", "team-1", &models.CreateBlueprintRequest{
		ProjectID: testServiceProjectID, Slug: "s", Title: "t", Content: "c",
		Status: models.ArtifactStatusArchived, // valid for artifacts, not for blueprints
	})

	require.ErrorIs(t, err, ErrInvalidStatus)
}

func TestMemoryService_RejectsOutOfSubsetStatus(t *testing.T) {
	svc := createTestMemoryService(mocks.NewMockMemoryRepository(t))

	status := models.BlueprintStatusExpired // valid for blueprints, not for memories
	_, err := svc.CreateMemory("user-1", "team-1", &models.CreateMemoryRequest{
		ProjectID: testServiceProjectID, Text: "t", Status: &status,
	})

	require.ErrorIs(t, err, ErrInvalidStatus)
}

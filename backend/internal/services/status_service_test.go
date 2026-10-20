package services

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

// The service layer is the ONLY place a status is checked: the REST handlers
// carry no copy of the check any more (#1266) and the six MCP write tools call
// these methods directly. So each of these tests fails the moment its
// s.statuses.Validate call is removed from the service, which nothing else in
// the suite can see (#912).
//
// The "rejects" tests build each service with NO status catalog, so only the
// system statuses are allowed, and with no write expectation on the repository:
// reaching Create or Update is the failure being guarded against. The custom
// status tests further down give the service a catalog.

func statusTestLogger() *slog.Logger {
	l, _ := logtest.New()
	return l
}

func TestPromptService_RejectsStatusOutsideCatalog(t *testing.T) {
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

		_, err := svc.CreatePrompt(context.Background(), userID, teamID, &models.CreatePromptRequest{
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
		_, err := svc.UpdatePrompt(context.Background(), userID, teamID, "prompt-1", &models.UpdatePromptRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

func TestArtifactService_RejectsStatusOutsideCatalog(t *testing.T) {
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

		_, err := svc.CreateArtifact(context.Background(), userID, teamID, &models.CreateArtifactRequest{
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
		_, err := svc.UpdateArtifactByProjectIDAndSlugInTeam(context.Background(), userID, teamID, projectID, slug,
			&models.UpdateArtifactRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

func TestBlueprintService_RejectsStatusOutsideCatalog(t *testing.T) {
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

		_, err := svc.CreateBlueprint(context.Background(), userID, teamID, &models.CreateBlueprintRequest{
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
		_, err := svc.UpdateBlueprintByProjectIDAndSlugInTeam(context.Background(), userID, teamID, projectID, slug,
			&models.UpdateBlueprintRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

func TestMemoryService_RejectsStatusOutsideCatalog(t *testing.T) {
	const (
		userID   = "user-1"
		teamID   = "team-1"
		memoryID = "memory-1"
	)

	t.Run("create", func(t *testing.T) {
		svc := createTestMemoryService(mocks.NewMockMemoryRepository(t))

		status := models.BlueprintStatusExpired // valid for blueprints, not for memories
		_, err := svc.CreateMemory(context.Background(), userID, teamID, &models.CreateMemoryRequest{
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
		_, err := svc.UpdateMemory(context.Background(), userID, teamID, memoryID,
			&models.UpdateMemoryRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

// An EMPTY status is the other half of the contract, and the one with no
// handler standing in front of it: `status` is a REQUIRED response field
// a client renders as a lifecycle state, so persisting "" puts a value on the
// wire that is neither a system status nor anything a team could have defined. The status validator deliberately
// accepts "" (it is the wire form of "not supplied"), which makes each update
// path solely responsible for not writing it -- so each one is pinned here
// (#912).
func TestServicesTreatEmptyStatusAsUnchanged(t *testing.T) {
	const (
		userID = "user-1"
		teamID = "team-1"
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

		_, err := svc.UpdatePrompt(context.Background(), userID, teamID, "prompt-1",
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

// The custom-status half of the contract (#1266): a status the resource's team
// defined for that resource type is persisted exactly as sent, on create and on
// update, while another team's custom status -- or the same team's status for
// a different resource type -- is rejected before anything is written.
//
// statusTestTeam's custom statuses come from newFakeStatusCatalog.

func TestPromptService_CustomStatus(t *testing.T) {
	const userID = "user-123" // the owner createTestProjectRepo knows
	newSvc := func(repo *mocks.MockPromptRepository) *PromptService {
		svc := createTestPromptService(repo, createTestProjectRepoInTeam(t, statusTestTeam))
		svc.statuses = statusValidator{catalog: newFakeStatusCatalog()}
		return svc
	}

	t.Run("create accepts the team's custom status", func(t *testing.T) {
		repo := mocks.NewMockPromptRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
			return p.Status == "prompt-review" && p.TeamID == statusTestTeam
		})).Return(nil).Once()

		prompt, err := newSvc(repo).CreatePrompt(context.Background(), userID, statusTestTeam, &models.CreatePromptRequest{
			Name: "n", Slug: "s", Body: "b", ProjectID: "project-123", Status: "prompt-review",
		})

		require.NoError(t, err)
		assert.Equal(t, "prompt-review", prompt.Status)
	})

	t.Run("update accepts the team's custom status", func(t *testing.T) {
		repo := mocks.NewMockPromptRepository(t)
		repo.EXPECT().GetByID(mock.Anything, userID, statusTestTeam, "prompt-1").
			Return(&models.Prompt{ID: "prompt-1", UserID: userID, TeamID: statusTestTeam,
				Status: models.PromptStatusDraft}, nil).Once()
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(p *models.Prompt) bool {
			return p.Status == "prompt-review"
		})).Return(nil).Once()

		status := "prompt-review"
		_, err := newSvc(repo).UpdatePrompt(context.Background(), userID, statusTestTeam, "prompt-1",
			&models.UpdatePromptRequest{Status: &status})

		require.NoError(t, err)
	})

	t.Run("create rejects another resource type's custom status", func(t *testing.T) {
		_, err := newSvc(mocks.NewMockPromptRepository(t)).CreatePrompt(context.Background(), userID, statusTestTeam,
			&models.CreatePromptRequest{
				Name: "n", Slug: "s", Body: "b", ProjectID: "project-123", Status: "in-review",
			})

		require.ErrorIs(t, err, ErrInvalidStatus)
		assert.Contains(t, err.Error(), "draft, published, prompt-review")
	})

	// The catalog consulted is the PROMPT's team's, not the one the caller
	// named: statusTestOtherTeam has no custom prompt status at all.
	t.Run("update validates against the prompt's own team", func(t *testing.T) {
		repo := mocks.NewMockPromptRepository(t)
		repo.EXPECT().GetByID(mock.Anything, userID, statusTestTeam, "prompt-1").
			Return(&models.Prompt{ID: "prompt-1", UserID: userID, TeamID: statusTestOtherTeam}, nil).Once()

		status := "prompt-review"
		_, err := newSvc(repo).UpdatePrompt(context.Background(), userID, statusTestTeam, "prompt-1",
			&models.UpdatePromptRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

func TestArtifactService_CustomStatus(t *testing.T) {
	const (
		userID    = "user-1"
		projectID = testServiceProjectID
		slug      = "s"
	)
	newSvc := func(repo *mocks.MockArtifactRepository) *ArtifactService {
		return NewArtifactService(ArtifactServiceDeps{
			Repo: repo, Authz: allowAllAuthz{},
			Logger: statusTestLogger(), StatusCatalog: newFakeStatusCatalog(),
		})
	}

	t.Run("create accepts the team's custom status", func(t *testing.T) {
		repo := mocks.NewMockArtifactRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(a *models.Artifact) bool {
			return a.Status == "in-review" && a.TeamID == statusTestTeam
		})).Return(nil).Once()

		artifact, err := newSvc(repo).CreateArtifact(context.Background(), userID, statusTestTeam, &models.CreateArtifactRequest{
			ProjectID: projectID, Slug: slug, Title: "t", Content: "c", Status: "in-review",
		})

		require.NoError(t, err)
		assert.Equal(t, "in-review", artifact.Status)
	})

	t.Run("update accepts the team's custom status", func(t *testing.T) {
		repo := mocks.NewMockArtifactRepository(t)
		repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, userID, statusTestTeam, projectID, slug).
			Return(&models.Artifact{ID: "artifact-1", UserID: userID, TeamID: statusTestTeam,
				Status: models.ArtifactStatusActive}, nil).Once()
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(a *models.Artifact) bool {
			return a.Status == "approved"
		})).Return(nil).Once()

		status := "approved"
		_, err := newSvc(repo).UpdateArtifactByProjectIDAndSlugInTeam(context.Background(), userID, statusTestTeam, projectID, slug,
			&models.UpdateArtifactRequest{Status: &status})

		require.NoError(t, err)
	})

	t.Run("create rejects another team's custom status", func(t *testing.T) {
		_, err := newSvc(mocks.NewMockArtifactRepository(t)).CreateArtifact(context.Background(), userID, statusTestTeam,
			&models.CreateArtifactRequest{
				ProjectID: projectID, Slug: slug, Title: "t", Content: "c", Status: "other-team-only",
			})

		require.ErrorIs(t, err, ErrInvalidStatus)
		assert.Contains(t, err.Error(), "active, draft, archived, in-review, approved")
		assert.NotContains(t, err.Error(), "other-team-only")
	})

	t.Run("update validates against the artifact's own team", func(t *testing.T) {
		repo := mocks.NewMockArtifactRepository(t)
		repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, userID, statusTestTeam, projectID, slug).
			Return(&models.Artifact{ID: "artifact-1", UserID: userID, TeamID: statusTestOtherTeam}, nil).Once()

		status := "in-review" // statusTestTeam's, but the artifact is statusTestOtherTeam's
		_, err := newSvc(repo).UpdateArtifactByProjectIDAndSlugInTeam(context.Background(), userID, statusTestTeam, projectID, slug,
			&models.UpdateArtifactRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

func TestBlueprintService_CustomStatus(t *testing.T) {
	const (
		userID    = "user-1"
		projectID = testServiceProjectID
		slug      = "s"
	)
	newSvc := func(repo *mocks.MockBlueprintRepository) *BlueprintService {
		return NewBlueprintService(BlueprintServiceDeps{
			Repo: repo, Authz: allowAllAuthz{},
			Logger: statusTestLogger(), StatusCatalog: newFakeStatusCatalog(),
		})
	}

	t.Run("create accepts the team's custom status", func(t *testing.T) {
		repo := mocks.NewMockBlueprintRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(b *models.Blueprint) bool {
			return b.Status == "blueprint-review" && b.TeamID == statusTestTeam
		})).Return(nil).Once()

		blueprint, err := newSvc(repo).CreateBlueprint(context.Background(), userID, statusTestTeam, &models.CreateBlueprintRequest{
			ProjectID: projectID, Slug: slug, Title: "t", Content: "c", Status: "blueprint-review",
		})

		require.NoError(t, err)
		assert.Equal(t, "blueprint-review", blueprint.Status)
	})

	t.Run("update accepts the team's custom status", func(t *testing.T) {
		repo := mocks.NewMockBlueprintRepository(t)
		repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, userID, statusTestTeam, projectID, slug).
			Return(&models.Blueprint{ID: "blueprint-1", UserID: userID, TeamID: statusTestTeam,
				Type: "general", Status: models.BlueprintStatusActive}, nil).Once()
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(b *models.Blueprint) bool {
			return b.Status == "blueprint-review"
		})).Return(nil).Once()

		status := "blueprint-review"
		_, err := newSvc(repo).UpdateBlueprintByProjectIDAndSlugInTeam(context.Background(), userID, statusTestTeam, projectID, slug,
			&models.UpdateBlueprintRequest{Status: &status})

		require.NoError(t, err)
	})

	t.Run("create rejects another resource type's custom status", func(t *testing.T) {
		_, err := newSvc(mocks.NewMockBlueprintRepository(t)).CreateBlueprint(context.Background(), userID, statusTestTeam,
			&models.CreateBlueprintRequest{
				ProjectID: projectID, Slug: slug, Title: "t", Content: "c", Status: "in-review",
			})

		require.ErrorIs(t, err, ErrInvalidStatus)
		assert.Contains(t, err.Error(), "active, expired, blueprint-review")
	})

	t.Run("update validates against the blueprint's own team", func(t *testing.T) {
		repo := mocks.NewMockBlueprintRepository(t)
		repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, userID, statusTestTeam, projectID, slug).
			Return(&models.Blueprint{ID: "blueprint-1", UserID: userID, TeamID: statusTestOtherTeam}, nil).Once()

		status := "blueprint-review"
		_, err := newSvc(repo).UpdateBlueprintByProjectIDAndSlugInTeam(context.Background(), userID, statusTestTeam, projectID, slug,
			&models.UpdateBlueprintRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

func TestMemoryService_CustomStatus(t *testing.T) {
	const (
		userID   = "user-1"
		memoryID = "memory-1"
	)
	newSvc := func(repo *mocks.MockMemoryRepository) *MemoryService {
		return NewMemoryService(MemoryServiceDeps{
			Repo: repo, Authz: allowAllAuthz{},
			Logger: statusTestLogger(), StatusCatalog: newFakeStatusCatalog(),
		})
	}

	t.Run("create accepts the team's custom status", func(t *testing.T) {
		repo := mocks.NewMockMemoryRepository(t)
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(m *models.Memory) bool {
			return m.Status == "memory-review" && m.TeamID == statusTestTeam
		})).Return(nil).Once()

		status := "memory-review"
		memory, err := newSvc(repo).CreateMemory(context.Background(), userID, statusTestTeam, &models.CreateMemoryRequest{
			ProjectID: testServiceProjectID, Text: "t", Status: &status,
		})

		require.NoError(t, err)
		assert.Equal(t, "memory-review", memory.Status)
	})

	t.Run("update accepts the team's custom status", func(t *testing.T) {
		repo := mocks.NewMockMemoryRepository(t)
		repo.EXPECT().GetByID(mock.Anything, userID, statusTestTeam, memoryID).
			Return(&models.Memory{ID: memoryID, UserID: userID, TeamID: statusTestTeam,
				Status: models.MemoryStatusActive}, nil).Once()
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(m *models.Memory) bool {
			return m.Status == "memory-review"
		})).Return(nil).Once()

		status := "memory-review"
		_, err := newSvc(repo).UpdateMemory(context.Background(), userID, statusTestTeam, memoryID,
			&models.UpdateMemoryRequest{Status: &status})

		require.NoError(t, err)
	})

	t.Run("create rejects another resource type's custom status", func(t *testing.T) {
		status := "in-review"
		_, err := newSvc(mocks.NewMockMemoryRepository(t)).CreateMemory(context.Background(), userID, statusTestTeam,
			&models.CreateMemoryRequest{ProjectID: testServiceProjectID, Text: "t", Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
		assert.Contains(t, err.Error(), "active, draft, archived, memory-review")
	})

	t.Run("update validates against the memory's own team", func(t *testing.T) {
		repo := mocks.NewMockMemoryRepository(t)
		repo.EXPECT().GetByID(mock.Anything, userID, statusTestTeam, memoryID).
			Return(&models.Memory{ID: memoryID, UserID: userID, TeamID: statusTestOtherTeam}, nil).Once()

		status := "memory-review"
		_, err := newSvc(repo).UpdateMemory(context.Background(), userID, statusTestTeam, memoryID,
			&models.UpdateMemoryRequest{Status: &status})

		require.ErrorIs(t, err, ErrInvalidStatus)
	})
}

// A list filter is validated by the same catalog as a write (#1266): an
// unknown status is an error the handlers answer with 400, never an empty
// page, and the repository is not reached. A custom status of the team is
// passed through to the repository as the filter.
func TestListFiltersValidateStatusAgainstTheCatalog(t *testing.T) {
	const userID = "user-1"
	unknown, custom := "retired", "memory-review"

	t.Run("prompts", func(t *testing.T) {
		repo := mocks.NewMockPromptRepository(t)
		svc := NewPromptService(PromptServiceDeps{
			Repo: repo, Authz: allowAllAuthz{},
			Logger: statusTestLogger(), StatusCatalog: newFakeStatusCatalog(),
		})

		_, err := svc.ListPrompts(userID, PromptFilters{TeamID: statusTestTeam, Status: "retired"})
		require.ErrorIs(t, err, ErrInvalidStatus)
		assert.Contains(t, err.Error(), "draft, published, prompt-review")

		repo.EXPECT().List(mock.Anything, userID, mock.MatchedBy(func(f repositories.PromptFilters) bool {
			return f.Status == "prompt-review"
		})).Return([]models.Prompt{}, 0, nil).Once()
		_, err = svc.ListPrompts(userID, PromptFilters{TeamID: statusTestTeam, Status: "prompt-review"})
		require.NoError(t, err)
	})

	t.Run("artifacts", func(t *testing.T) {
		repo := mocks.NewMockArtifactRepository(t)
		svc := NewArtifactService(ArtifactServiceDeps{
			Repo: repo, Authz: allowAllAuthz{},
			Logger: statusTestLogger(), StatusCatalog: newFakeStatusCatalog(),
		})

		_, err := svc.ListArtifacts(userID, ArtifactFilters{TeamID: statusTestTeam, Status: "retired", Limit: 20})
		require.ErrorIs(t, err, ErrInvalidStatus)
		// ListArtifactsByProject is the MCP list tool's entry point.
		_, err = svc.ListArtifactsByProject(userID, testServiceProjectID,
			ArtifactFilters{TeamID: statusTestTeam, Status: "other-team-only", Limit: 20})
		require.ErrorIs(t, err, ErrInvalidStatus)

		repo.EXPECT().List(mock.Anything, userID, mock.MatchedBy(func(f repositories.ArtifactFilters) bool {
			return f.Status != nil && *f.Status == "in-review"
		})).Return([]models.Artifact{}, 0, nil).Once()
		_, err = svc.ListArtifacts(userID, ArtifactFilters{TeamID: statusTestTeam, Status: "in-review", Limit: 20})
		require.NoError(t, err)
	})

	t.Run("blueprints", func(t *testing.T) {
		repo := mocks.NewMockBlueprintRepository(t)
		svc := NewBlueprintService(BlueprintServiceDeps{
			Repo: repo, Authz: allowAllAuthz{},
			Logger: statusTestLogger(), StatusCatalog: newFakeStatusCatalog(),
		})

		_, err := svc.ListBlueprints(userID, BlueprintFilters{TeamID: statusTestTeam, Status: "retired", Limit: 20})
		require.ErrorIs(t, err, ErrInvalidStatus)
		_, err = svc.ListBlueprintsByProject(userID, testServiceProjectID,
			BlueprintFilters{TeamID: statusTestTeam, Status: "in-review", Limit: 20})
		require.ErrorIs(t, err, ErrInvalidStatus)

		repo.EXPECT().List(mock.Anything, userID, mock.MatchedBy(func(f repositories.BlueprintFilters) bool {
			return f.Status != nil && *f.Status == "blueprint-review"
		})).Return([]models.Blueprint{}, 0, nil).Once()
		_, err = svc.ListBlueprints(userID,
			BlueprintFilters{TeamID: statusTestTeam, Status: "blueprint-review", Limit: 20})
		require.NoError(t, err)
	})

	t.Run("memories", func(t *testing.T) {
		repo := mocks.NewMockMemoryRepository(t)
		svc := NewMemoryService(MemoryServiceDeps{
			Repo: repo, Authz: allowAllAuthz{},
			Logger: statusTestLogger(), StatusCatalog: newFakeStatusCatalog(),
		})

		_, err := svc.ListMemories(userID, MemoryFilters{TeamID: statusTestTeam, Status: &unknown})
		require.ErrorIs(t, err, ErrInvalidStatus)

		repo.EXPECT().List(mock.Anything, userID, mock.MatchedBy(func(f repositories.MemoryFilters) bool {
			return f.Status != nil && *f.Status == "memory-review"
		})).Return([]models.Memory{}, 0, nil).Once()
		_, err = svc.ListMemories(userID, MemoryFilters{TeamID: statusTestTeam, Status: &custom})
		require.NoError(t, err)
	})
}

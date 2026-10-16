package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The post-write confirmation (#1332). Validate reads the catalog before the
// write and holds nothing, so a custom status can be deleted between the two;
// ConfirmAfterWrite is what notices, after the write, and moves the resource
// to the default of its type the way the delete would have.
//
// The interleavings themselves are an integration test against Postgres
// (TestIntegrationTeamStatusRepository_ConfirmSlug*). These tests pin what the
// services do with the answer.

// statusGoneCatalog is newFakeStatusCatalog with slug deleted "after Validate":
// still allowed before the write, no longer confirmed after it.
func statusGoneCatalog(resourceType, slug string) *fakeStatusCatalog {
	catalog := newFakeStatusCatalog()
	catalog.gone = map[string]bool{statusTestTeam + "/" + resourceType + "/" + slug: true}
	return catalog
}

func TestStatusValidator_ConfirmAfterWrite(t *testing.T) {
	ctx := context.Background()
	const resourceID = "resource-1"

	t.Run("a system status costs no confirmation", func(t *testing.T) {
		for _, resourceType := range models.StatusResourceTypes {
			for _, status := range models.SystemStatusesFor(resourceType) {
				catalog := newFakeStatusCatalog()
				stored := status

				require.NoError(t, statusValidator{catalog: catalog}.ConfirmAfterWrite(
					ctx, statusTestLogger(), statusTestTeam, resourceType, resourceID, &stored))

				assert.Equal(t, status, stored)
				assert.Zero(t, catalog.confirms, "%s %s", resourceType, status)
				assert.Empty(t, catalog.reassigned)
			}
		}
	})

	t.Run("a custom status the team still has is left alone", func(t *testing.T) {
		catalog := newFakeStatusCatalog()
		stored := "in-review"

		require.NoError(t, statusValidator{catalog: catalog}.ConfirmAfterWrite(
			ctx, statusTestLogger(), statusTestTeam, models.StatusResourceTypeArtifact, resourceID, &stored))

		assert.Equal(t, "in-review", stored)
		assert.Equal(t, 1, catalog.confirms)
		assert.Empty(t, catalog.reassigned)
	})

	custom := map[string]string{
		models.StatusResourceTypePrompt:    "prompt-review",
		models.StatusResourceTypeArtifact:  "in-review",
		models.StatusResourceTypeBlueprint: "blueprint-review",
		models.StatusResourceTypeMemory:    "memory-review",
	}
	require.Len(t, custom, len(models.StatusResourceTypes))
	for resourceType, slug := range custom {
		t.Run("a deleted "+resourceType+" status becomes the type's default", func(t *testing.T) {
			catalog := statusGoneCatalog(resourceType, slug)
			stored := slug

			require.NoError(t, statusValidator{catalog: catalog}.ConfirmAfterWrite(
				ctx, statusTestLogger(), statusTestTeam, resourceType, resourceID, &stored))

			assert.Equal(t, models.DefaultStatusFor(resourceType), stored)
			assert.Equal(t, []string{resourceType + "/" + resourceID + "/" + slug}, catalog.reassigned,
				"the stored row is moved, not only the response")
		})
	}

	// Either failure can leave the resource on a deleted status, so neither is
	// swallowed, and the caller's status is not rewritten to something that
	// was never stored.
	boom := errors.New("connection reset")
	t.Run("a failed confirmation is an error", func(t *testing.T) {
		catalog := newFakeStatusCatalog()
		catalog.confirmErr = boom
		stored := "in-review"

		err := statusValidator{catalog: catalog}.ConfirmAfterWrite(
			ctx, statusTestLogger(), statusTestTeam, models.StatusResourceTypeArtifact, resourceID, &stored)

		require.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, ErrInvalidStatus, "a 400 would blame the caller for a server fault")
		assert.Contains(t, err.Error(), resourceID)
		assert.Equal(t, "in-review", stored)
	})

	t.Run("a failed repair is an error", func(t *testing.T) {
		catalog := statusGoneCatalog(models.StatusResourceTypeArtifact, "in-review")
		catalog.reassignErr = boom
		stored := "in-review"

		err := statusValidator{catalog: catalog}.ConfirmAfterWrite(
			ctx, statusTestLogger(), statusTestTeam, models.StatusResourceTypeArtifact, resourceID, &stored)

		require.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), resourceID)
		assert.Equal(t, "in-review", stored)
	})

	// The read waits for a concurrent delete's transaction, and every service
	// write path arrives here on context.Background(): without a deadline of
	// its own a stuck delete would hold each waiting write and its connection.
	t.Run("the confirmation and the repair run on a deadline", func(t *testing.T) {
		catalog := statusGoneCatalog(models.StatusResourceTypeArtifact, "in-review")
		stored := "in-review"

		require.NoError(t, statusValidator{catalog: catalog}.ConfirmAfterWrite(
			context.Background(), statusTestLogger(),
			statusTestTeam, models.StatusResourceTypeArtifact, resourceID, &stored))

		assert.Equal(t, 1, catalog.confirms)
		assert.Len(t, catalog.reassigned, 1)
		assert.Zero(t, catalog.unbounded, "a call reached the catalog without a deadline")
	})

	// Such a validator only ever admitted system statuses, so there is nothing
	// it could confirm, and nothing to dereference.
	t.Run("a validator without a catalog confirms nothing", func(t *testing.T) {
		stored := "in-review"

		require.NoError(t, statusValidator{}.ConfirmAfterWrite(
			ctx, statusTestLogger(), statusTestTeam, models.StatusResourceTypeArtifact, resourceID, &stored))

		assert.Equal(t, "in-review", stored)
	})
}

// statusConfirmCase drives one resource type's create and update through its
// service with the given catalog. Each returns the status the service answered
// with, plus a field other than the status as the write stored it, so a test
// can tell the write was kept.
type statusConfirmCase struct {
	resourceType string
	// custom is statusTestTeam's custom status for the type.
	custom string
	// create writes a new resource carrying status.
	create func(t *testing.T, catalog *fakeStatusCatalog, status string) (gotStatus, gotText string, err error)
	// update edits the text of a resource stored on storedStatus, and sets
	// status too unless it is nil.
	update func(
		t *testing.T, catalog *fakeStatusCatalog, storedStatus string, status *string,
	) (gotStatus, gotText string, err error)
}

const (
	statusConfirmUser = "user-123" // the owner createTestProjectRepo knows
	statusConfirmID   = "resource-1"
	statusConfirmText = "edited text"
)

func statusConfirmCases() []statusConfirmCase {
	return []statusConfirmCase{
		promptStatusConfirmCase(), artifactStatusConfirmCase(),
		blueprintStatusConfirmCase(), memoryStatusConfirmCase(),
	}
}

func promptStatusConfirmCase() statusConfirmCase {
	newSvc := func(t *testing.T, repo *mocks.MockPromptRepository, catalog *fakeStatusCatalog) *PromptService {
		svc := createTestPromptService(repo, createTestProjectRepo(t))
		svc.statuses = statusValidator{catalog: catalog}
		return svc
	}
	return statusConfirmCase{
		resourceType: models.StatusResourceTypePrompt,
		custom:       "prompt-review",
		create: func(t *testing.T, catalog *fakeStatusCatalog, status string) (string, string, error) {
			repo := mocks.NewMockPromptRepository(t)
			repo.EXPECT().Create(mock.Anything, mock.Anything).
				Run(func(_ context.Context, p *models.Prompt) { p.ID = statusConfirmID }).Return(nil).Once()
			prompt, err := newSvc(t, repo, catalog).CreatePrompt(context.Background(), statusConfirmUser, statusTestTeam,
				&models.CreatePromptRequest{
					Name: "n", Slug: "s", Body: statusConfirmText, ProjectID: "project-123", Status: status,
				})
			if err != nil {
				return "", "", err
			}
			return prompt.Status, prompt.Body, nil
		},
		update: func(t *testing.T, catalog *fakeStatusCatalog, stored string, status *string) (string, string, error) {
			repo := mocks.NewMockPromptRepository(t)
			repo.EXPECT().GetByID(mock.Anything, statusConfirmUser, statusTestTeam, statusConfirmID).
				Return(&models.Prompt{ID: statusConfirmID, UserID: statusConfirmUser, TeamID: statusTestTeam,
					Status: stored}, nil).Once()
			repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
			body := statusConfirmText
			prompt, err := newSvc(t, repo, catalog).UpdatePrompt(context.Background(), statusConfirmUser, statusTestTeam, statusConfirmID,
				&models.UpdatePromptRequest{Body: &body, Status: status})
			if err != nil {
				return "", "", err
			}
			return prompt.Status, prompt.Body, nil
		},
	}
}

func artifactStatusConfirmCase() statusConfirmCase {
	const slug = "s"
	newSvc := func(repo *mocks.MockArtifactRepository, catalog *fakeStatusCatalog) *ArtifactService {
		return NewArtifactService(ArtifactServiceDeps{
			Repo: repo, Authz: allowAllAuthz{}, Logger: statusTestLogger(), StatusCatalog: catalog,
		})
	}
	return statusConfirmCase{
		resourceType: models.StatusResourceTypeArtifact,
		custom:       "in-review",
		create: func(t *testing.T, catalog *fakeStatusCatalog, status string) (string, string, error) {
			repo := mocks.NewMockArtifactRepository(t)
			repo.EXPECT().Create(mock.Anything, mock.Anything).
				Run(func(_ context.Context, a *models.Artifact) { a.ID = statusConfirmID }).Return(nil).Once()
			artifact, err := newSvc(repo, catalog).CreateArtifact(context.Background(), statusConfirmUser, statusTestTeam,
				&models.CreateArtifactRequest{
					ProjectID: testServiceProjectID, Slug: slug, Title: "t", Content: statusConfirmText, Status: status,
				})
			if err != nil {
				return "", "", err
			}
			return artifact.Status, artifact.Content, nil
		},
		update: func(t *testing.T, catalog *fakeStatusCatalog, stored string, status *string) (string, string, error) {
			repo := mocks.NewMockArtifactRepository(t)
			repo.EXPECT().
				GetByProjectIDAndSlug(mock.Anything, statusConfirmUser, statusTestTeam, testServiceProjectID, slug).
				Return(&models.Artifact{ID: statusConfirmID, UserID: statusConfirmUser, TeamID: statusTestTeam,
					Status: stored}, nil).Once()
			repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
			content := statusConfirmText
			artifact, err := newSvc(repo, catalog).UpdateArtifactByProjectIDAndSlugInTeam(
				context.Background(),
				statusConfirmUser, statusTestTeam, testServiceProjectID, slug,
				&models.UpdateArtifactRequest{Content: &content, Status: status})
			if err != nil {
				return "", "", err
			}
			return artifact.Status, artifact.Content, nil
		},
	}
}

func blueprintStatusConfirmCase() statusConfirmCase {
	const slug = "s"
	newSvc := func(repo *mocks.MockBlueprintRepository, catalog *fakeStatusCatalog) *BlueprintService {
		return NewBlueprintService(BlueprintServiceDeps{
			Repo: repo, Authz: allowAllAuthz{}, Logger: statusTestLogger(), StatusCatalog: catalog,
		})
	}
	return statusConfirmCase{
		resourceType: models.StatusResourceTypeBlueprint,
		custom:       "blueprint-review",
		create: func(t *testing.T, catalog *fakeStatusCatalog, status string) (string, string, error) {
			repo := mocks.NewMockBlueprintRepository(t)
			repo.EXPECT().Create(mock.Anything, mock.Anything).
				Run(func(_ context.Context, b *models.Blueprint) { b.ID = statusConfirmID }).Return(nil).Once()
			blueprint, err := newSvc(repo, catalog).CreateBlueprint(context.Background(), statusConfirmUser, statusTestTeam,
				&models.CreateBlueprintRequest{
					ProjectID: testServiceProjectID, Slug: slug, Title: "t", Content: statusConfirmText, Status: status,
				})
			if err != nil {
				return "", "", err
			}
			return blueprint.Status, blueprint.Content, nil
		},
		update: func(t *testing.T, catalog *fakeStatusCatalog, stored string, status *string) (string, string, error) {
			repo := mocks.NewMockBlueprintRepository(t)
			repo.EXPECT().
				GetByProjectIDAndSlug(mock.Anything, statusConfirmUser, statusTestTeam, testServiceProjectID, slug).
				Return(&models.Blueprint{ID: statusConfirmID, UserID: statusConfirmUser, TeamID: statusTestTeam,
					Type: "general", Status: stored}, nil).Once()
			repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
			content := statusConfirmText
			blueprint, err := newSvc(repo, catalog).UpdateBlueprintByProjectIDAndSlugInTeam(
				context.Background(),
				statusConfirmUser, statusTestTeam, testServiceProjectID, slug,
				&models.UpdateBlueprintRequest{Content: &content, Status: status})
			if err != nil {
				return "", "", err
			}
			return blueprint.Status, blueprint.Content, nil
		},
	}
}

func memoryStatusConfirmCase() statusConfirmCase {
	newSvc := func(repo *mocks.MockMemoryRepository, catalog *fakeStatusCatalog) *MemoryService {
		return NewMemoryService(MemoryServiceDeps{
			Repo: repo, Authz: allowAllAuthz{}, Logger: statusTestLogger(), StatusCatalog: catalog,
		})
	}
	return statusConfirmCase{
		resourceType: models.StatusResourceTypeMemory,
		custom:       "memory-review",
		create: func(t *testing.T, catalog *fakeStatusCatalog, status string) (string, string, error) {
			repo := mocks.NewMockMemoryRepository(t)
			repo.EXPECT().Create(mock.Anything, mock.Anything).
				Run(func(_ context.Context, m *models.Memory) { m.ID = statusConfirmID }).Return(nil).Once()
			memory, err := newSvc(repo, catalog).CreateMemory(context.Background(), statusConfirmUser, statusTestTeam,
				&models.CreateMemoryRequest{ProjectID: testServiceProjectID, Text: statusConfirmText, Status: &status})
			if err != nil {
				return "", "", err
			}
			return memory.Status, memory.Text, nil
		},
		update: func(t *testing.T, catalog *fakeStatusCatalog, stored string, status *string) (string, string, error) {
			repo := mocks.NewMockMemoryRepository(t)
			repo.EXPECT().GetByID(mock.Anything, statusConfirmUser, statusTestTeam, statusConfirmID).
				Return(&models.Memory{ID: statusConfirmID, UserID: statusConfirmUser, TeamID: statusTestTeam,
					Status: stored}, nil).Once()
			repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
			text := statusConfirmText
			memory, err := newSvc(repo, catalog).UpdateMemory(statusConfirmUser, statusTestTeam, statusConfirmID,
				&models.UpdateMemoryRequest{Text: &text, Status: status})
			if err != nil {
				return "", "", err
			}
			return memory.Status, memory.Text, nil
		},
	}
}

// The "status gone" branch of every create and update: the write is kept, the
// stored row is moved to the type's default, and the response reports that
// default rather than the status the caller sent.
func TestServicesMoveAResourceOffAStatusDeletedDuringItsWrite(t *testing.T) {
	for _, tc := range statusConfirmCases() {
		fallback := models.DefaultStatusFor(tc.resourceType)
		repaired := []string{tc.resourceType + "/" + statusConfirmID + "/" + tc.custom}

		t.Run(tc.resourceType+" create", func(t *testing.T) {
			catalog := statusGoneCatalog(tc.resourceType, tc.custom)

			status, text, err := tc.create(t, catalog, tc.custom)

			require.NoError(t, err)
			assert.Equal(t, fallback, status)
			assert.Equal(t, statusConfirmText, text, "the write's other fields are kept")
			assert.Equal(t, repaired, catalog.reassigned)
		})

		t.Run(tc.resourceType+" update that sets the status", func(t *testing.T) {
			catalog := statusGoneCatalog(tc.resourceType, tc.custom)

			status, text, err := tc.update(t, catalog, fallback, &tc.custom)

			require.NoError(t, err)
			assert.Equal(t, fallback, status)
			assert.Equal(t, statusConfirmText, text, "the write's other fields are kept")
			assert.Equal(t, repaired, catalog.reassigned)
		})

		// The update rewrites the whole row, status included, so it stores the
		// slug it loaded even though the request never mentioned it.
		t.Run(tc.resourceType+" update that leaves a custom status alone", func(t *testing.T) {
			catalog := statusGoneCatalog(tc.resourceType, tc.custom)

			status, text, err := tc.update(t, catalog, tc.custom, nil)

			require.NoError(t, err)
			assert.Equal(t, fallback, status)
			assert.Equal(t, statusConfirmText, text)
			assert.Equal(t, repaired, catalog.reassigned)
		})
	}
}

// A custom status the team still has survives the confirmation, at the cost of
// exactly one locking read per write.
func TestServicesKeepACustomStatusTheTeamStillHas(t *testing.T) {
	for _, tc := range statusConfirmCases() {
		t.Run(tc.resourceType+" create", func(t *testing.T) {
			catalog := newFakeStatusCatalog()

			status, _, err := tc.create(t, catalog, tc.custom)

			require.NoError(t, err)
			assert.Equal(t, tc.custom, status)
			assert.Equal(t, 1, catalog.confirms)
			assert.Empty(t, catalog.reassigned)
		})

		t.Run(tc.resourceType+" update", func(t *testing.T) {
			catalog := newFakeStatusCatalog()

			status, _, err := tc.update(t, catalog, models.DefaultStatusFor(tc.resourceType), &tc.custom)

			require.NoError(t, err)
			assert.Equal(t, tc.custom, status)
			assert.Equal(t, 1, catalog.confirms)
			assert.Empty(t, catalog.reassigned)
		})
	}
}

// The confirmation is a locking read, and the common write must not pay for
// it: a resource on a system status is never confirmed, whether the request
// set that status or did not mention one.
func TestServicesDoNotConfirmASystemStatus(t *testing.T) {
	for _, tc := range statusConfirmCases() {
		system := models.SystemStatusesFor(tc.resourceType)
		other := system[len(system)-1]

		t.Run(tc.resourceType+" create with a system status", func(t *testing.T) {
			catalog := newFakeStatusCatalog()

			status, _, err := tc.create(t, catalog, other)

			require.NoError(t, err)
			assert.Equal(t, other, status)
			assert.Zero(t, catalog.confirms)
		})

		t.Run(tc.resourceType+" update that sets a system status", func(t *testing.T) {
			catalog := newFakeStatusCatalog()

			// Moving OFF a custom status: what is stored afterwards is a
			// system status, so there is nothing left to confirm.
			status, _, err := tc.update(t, catalog, tc.custom, &other)

			require.NoError(t, err)
			assert.Equal(t, other, status)
			assert.Zero(t, catalog.confirms)
		})

		t.Run(tc.resourceType+" update that does not touch the status", func(t *testing.T) {
			catalog := newFakeStatusCatalog()

			status, text, err := tc.update(t, catalog, other, nil)

			require.NoError(t, err)
			assert.Equal(t, other, status)
			assert.Equal(t, statusConfirmText, text)
			assert.Zero(t, catalog.confirms)
		})
	}
}

// A confirmation that cannot be made fails the request: the resource may be
// sitting on a deleted status, and answering 200 would report it as fine.
func TestServicesFailAWriteWhoseStatusCannotBeConfirmed(t *testing.T) {
	boom := errors.New("connection reset")

	for _, tc := range statusConfirmCases() {
		t.Run(tc.resourceType+" create", func(t *testing.T) {
			catalog := newFakeStatusCatalog()
			catalog.confirmErr = boom

			_, _, err := tc.create(t, catalog, tc.custom)

			require.ErrorIs(t, err, boom)
		})

		t.Run(tc.resourceType+" update whose repair fails", func(t *testing.T) {
			catalog := statusGoneCatalog(tc.resourceType, tc.custom)
			catalog.reassignErr = boom

			_, _, err := tc.update(t, catalog, models.DefaultStatusFor(tc.resourceType), &tc.custom)

			require.ErrorIs(t, err, boom)
		})
	}
}

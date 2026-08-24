package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/freshness"
)

// fakeClearer records the reversal calls the resource services make.
//
// Hand-written rather than generated: internal/services/mocks imports
// internal/services, so an internal test importing it back would be an import
// cycle — the same reason allowAllAuthz is hand-written in this package.
type fakeClearer struct {
	calls []reversalCall
	err   error
}

type reversalCall struct {
	teamID       string
	resourceType string
	resourceID   string
	reason       string
	medium       string
}

func (f *fakeClearer) ClearIfStale(
	_ context.Context, teamID, resourceType, resourceID, reason, medium string,
) error {
	f.calls = append(f.calls, reversalCall{teamID, resourceType, resourceID, reason, medium})
	return f.err
}

// oneReversal is the call every edit path must make for its own resource.
//
// The medium is empty by design (#770): an edit moves `updated_at`, which every
// rule watches whatever mediums it names, so an edit-triggered clear can never
// be undone by the next evaluation run and must not be scoped to a medium.
func oneReversal(resourceType, resourceID string) []reversalCall {
	return []reversalCall{{
		teamID:       reversalTeamID,
		resourceType: resourceType,
		resourceID:   resourceID,
		reason:       models.FreshnessReasonEdited,
		medium:       models.FreshnessMediumNone,
	}}
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// An edit must attempt the reversal with the `edited` reason — the audit tab
// distinguishes a read from an edit, and that distinction only exists here.
func TestClearFreshnessAfterEdit_CallsTheClearer(t *testing.T) {
	clearer := &fakeClearer{}

	clearFreshnessAfterEdit(context.Background(), clearer, discardLogger(),
		reversalTeamID, "prompt", "res-1")

	assert.Equal(t, oneReversal("prompt", "res-1"), clearer.calls)
}

// The edit is already persisted and is what the caller asked for, so a
// freshness failure must be swallowed rather than turned into an error the
// user cannot act on. The next scheduled run reconciles the row anyway.
func TestClearFreshnessAfterEdit_SwallowsFailures(t *testing.T) {
	clearer := &fakeClearer{err: errors.New("boom")}

	assert.NotPanics(t, func() {
		clearFreshnessAfterEdit(context.Background(), clearer, discardLogger(), "team-1", "prompt", "res-1")
	})
	assert.Len(t, clearer.calls, 1, "the reversal was attempted before it failed")
}

// Every pre-existing resource-service test constructs its service without a
// clearer, so the nil guards are what keep them green — and a deployment that
// wired none must behave exactly as it did before #733.
func TestClearFreshnessAfterEdit_ToleratesNilCollaborators(t *testing.T) {
	assert.NotPanics(t, func() {
		clearFreshnessAfterEdit(context.Background(), nil, discardLogger(), "team-1", "prompt", "res-1")
	})

	assert.NotPanics(t, func() {
		clearFreshnessAfterEdit(context.Background(), &fakeClearer{err: errors.New("boom")}, nil,
			"team-1", "prompt", "res-1")
	})
}

const (
	reversalTeamID = "team-reversal"
	reversalUserID = "user-123"
)

// expectReversal asserts the service reached the shared helper with its own
// team id, its own resource-type literal and the id of the row it just wrote.
// A transposed or hardcoded argument at one of the four call sites would clear
// the wrong resource — or nothing — and nothing else in the suite would notice.
func assertReversed(t *testing.T, clearer *fakeClearer, resourceType, resourceID string) {
	t.Helper()
	assert.Equal(t, oneReversal(resourceType, resourceID), clearer.calls)
}

func TestPromptService_UpdateClearsFreshness(t *testing.T) {
	clearer := &fakeClearer{}

	existing := createTestPrompt()
	existing.TeamID = reversalTeamID
	repo := mocks.NewMockPromptRepository(t)
	repo.EXPECT().GetByID(mock.Anything, reversalUserID, mock.Anything, "prompt-123").
		Return(existing, nil).Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	refRepo := mocks.NewMockPromptReferenceRepository(t)
	refRepo.EXPECT().DeleteByPromptID(mock.Anything, mock.Anything).Return(nil).Maybe()
	refRepo.EXPECT().CreateBatch(mock.Anything, mock.Anything).Return(nil).Maybe()

	svc := NewPromptService(PromptServiceDeps{
		Repo: repo, RefRepo: refRepo, Authz: allowAllAuthz{},
		Logger: discardLogger(), FreshnessClearer: clearer,
	})

	body := "updated body"
	_, err := svc.UpdatePrompt(reversalUserID, reversalTeamID, "prompt-123",
		&models.UpdatePromptRequest{Body: &body})
	require.NoError(t, err)
	assertReversed(t, clearer, "prompt", "prompt-123")
}

// A request that changes nothing returns early WITHOUT writing, so it is not an
// edit and must not clear anything.
func TestPromptService_NoOpUpdateDoesNotClearFreshness(t *testing.T) {
	existing := createTestPrompt()
	existing.TeamID = reversalTeamID
	repo := mocks.NewMockPromptRepository(t)
	repo.EXPECT().GetByID(mock.Anything, reversalUserID, mock.Anything, "prompt-123").
		Return(existing, nil).Once()

	clearer := &fakeClearer{}
	svc := NewPromptService(PromptServiceDeps{
		Repo: repo, Authz: allowAllAuthz{}, Logger: discardLogger(), FreshnessClearer: clearer,
	})

	_, err := svc.UpdatePrompt(reversalUserID, reversalTeamID, "prompt-123", &models.UpdatePromptRequest{})
	require.NoError(t, err)
	assert.Empty(t, clearer.calls, "a request that writes nothing is not an edit")
}

func TestArtifactService_UpdateClearsFreshness(t *testing.T) {
	clearer := &fakeClearer{}

	existing := &models.Artifact{
		ID: "artifact-123", TeamID: reversalTeamID, ProjectID: "project-123",
		Slug: "test-artifact", UserID: reversalUserID, Title: "Test", Content: "body", Status: "active",
	}
	repo := mocks.NewMockArtifactRepository(t)
	repo.EXPECT().
		GetByProjectIDAndSlug(mock.Anything, reversalUserID, reversalTeamID, "project-123", "test-artifact").
		Return(existing, nil).Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	svc := NewArtifactService(ArtifactServiceDeps{
		Repo: repo, Authz: allowAllAuthz{}, Logger: discardLogger(), FreshnessClearer: clearer,
	})

	title := "Updated"
	_, err := svc.UpdateArtifactByProjectIDAndSlugInTeam(
		reversalUserID, reversalTeamID, "project-123", "test-artifact",
		&models.UpdateArtifactRequest{Title: &title})
	require.NoError(t, err)
	assertReversed(t, clearer, "artifact", "artifact-123")
}

func TestBlueprintService_UpdateClearsFreshness(t *testing.T) {
	clearer := &fakeClearer{}

	existing := &models.Blueprint{
		ID: "blueprint-123", TeamID: reversalTeamID, ProjectID: "project-123",
		Slug: "test-blueprint", UserID: reversalUserID, Title: "Test",
		Content: "body", Type: "general", Status: "active",
	}
	repo := mocks.NewMockBlueprintRepository(t)
	repo.EXPECT().
		GetByProjectIDAndSlug(mock.Anything, reversalUserID, reversalTeamID, "project-123", "test-blueprint").
		Return(existing, nil).Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	svc := NewBlueprintService(BlueprintServiceDeps{
		Repo: repo, Authz: allowAllAuthz{}, Logger: discardLogger(), FreshnessClearer: clearer,
	})

	title := "Updated"
	_, err := svc.UpdateBlueprintByProjectIDAndSlugInTeam(
		reversalUserID, reversalTeamID, "project-123", "test-blueprint",
		&models.UpdateBlueprintRequest{Title: &title})
	require.NoError(t, err)
	assertReversed(t, clearer, "blueprint", "blueprint-123")
}

func TestMemoryService_UpdateClearsFreshness(t *testing.T) {
	clearer := &fakeClearer{}

	existing := &models.Memory{
		ID: "memory-123", TeamID: reversalTeamID, ProjectID: "project-123",
		UserID: reversalUserID, Text: "original",
	}
	repo := mocks.NewMockMemoryRepository(t)
	repo.EXPECT().GetByID(mock.Anything, reversalUserID, mock.Anything, "memory-123").
		Return(existing, nil).Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	svc := NewMemoryService(repo, nil, allowAllAuthz{}, nil, discardLogger(), nil, nil, nil, clearer, nil)

	text := "updated"
	_, err := svc.UpdateMemory(reversalUserID, reversalTeamID, "memory-123",
		&models.UpdateMemoryRequest{Text: &text})
	require.NoError(t, err)
	assertReversed(t, clearer, "memory", "memory-123")
}

// The types the rule validator accepts and the types reversal can clear must be
// the same set. This assertion lives here because it is the only place both
// lists are visible: freshnessRuleResourceTypes is unexported in this package,
// and freshness.EvaluableResourceTypes is exported from the other.
//
// It matters because the two gates fail differently. A rule type the candidate
// repository cannot evaluate aborts the run with
// ErrUnsupportedFreshnessResource — loud. A rule type reversal does not know
// simply never clears — silent, and indistinguishable from reversibility being
// switched off.
func TestFreshnessRuleTypes_AreAllReversible(t *testing.T) {
	assert.ElementsMatch(t, freshnessRuleResourceTypes, freshness.EvaluableResourceTypes)
}

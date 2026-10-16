package services_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicemocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// Lock enforcement (#1289): a locked prompt, blueprint or artifact cannot be
// deleted by any role, and an update may not change a frozen field. These
// tests drive the real services over a REAL AuthorizationService, so "owners
// and admins too" is asserted against the shipped matrix.
//
// The repository mocks are strict: a rejected write that reached Update or
// Delete, or took a content-version snapshot, fails the test by itself.

const (
	lockTeamID    = "team-lock"
	lockCaller    = "user-caller"
	lockOther     = "user-other"
	lockProjectID = "project-1"
	lockSlug      = "doc"
	lockLockerID  = "user-locker"
	lockLocker    = "Ada Lovelace"
	lockComment   = "Used by the nightly release agent"
)

var lockAllRoles = []models.TeamMemberRole{
	models.TeamMemberRoleMember, models.TeamMemberRoleAdmin, models.TeamMemberRoleOwner,
}

func lockPtr[T any](v T) *T { return &v }

func heldLock() *models.ResourceLock {
	return &models.ResourceLock{LockedBy: lockLockerID, LockedByName: lockLocker, Comment: lockComment}
}

// lockAuthz is the real authorization service for a caller holding role in
// lockTeamID; an empty role is a non-member.
func lockAuthz(t *testing.T, role models.TeamMemberRole) services.AuthorizationServiceInterface {
	t.Helper()
	logger, _ := logtest.New()
	members := repomocks.NewMockTeamMemberRepository(t)
	lookup := members.EXPECT().GetByTeamAndUser(mock.Anything, lockTeamID, lockCaller)
	if role == "" {
		lookup.Return(nil, repositories.ErrTeamMemberNotFound).Maybe()
	} else {
		lookup.Return(&models.TeamMember{TeamID: lockTeamID, UserID: lockCaller, Role: role}, nil).Maybe()
	}
	return services.NewAuthorizationService(members, logger)
}

// requireLockRefusal asserts err is the lock refusal for resourceType naming
// field (empty for a delete), the locker and the lock comment.
func requireLockRefusal(t *testing.T, err error, resourceType, field string) {
	t.Helper()
	var locked *services.ResourceLockedError
	require.ErrorAs(t, err, &locked)
	assert.Equal(t, services.ResourceLockedError{
		ResourceType: resourceType, Field: field,
		LockedBy: lockLockerID, LockedByName: lockLocker, Comment: lockComment,
	}, *locked)
	assert.Contains(t, err.Error(), lockComment, "the MCP tools print this message as is")
}

// ---- prompts ----

func lockablePrompt(lock *models.ResourceLock) *models.Prompt {
	return &models.Prompt{
		ID: "prompt-1", TeamID: lockTeamID, UserID: lockCaller, ProjectID: lockProjectID,
		Name: "Doc", Slug: lockSlug, Description: "d", Body: "body", Status: "published",
		MCPExpose: true, Labels: []string{"alpha", "beta"}, Version: 7, Lock: lock,
	}
}

type promptLockHarness struct {
	svc  *services.PromptService
	repo *repomocks.MockPromptRepository
	refs *repomocks.MockPromptReferenceRepository
	cvs  *servicemocks.MockContentVersionServiceInterface
}

func newPromptLockHarness(t *testing.T, role models.TeamMemberRole, existing *models.Prompt) *promptLockHarness {
	t.Helper()
	logger, _ := logtest.New()
	h := &promptLockHarness{
		repo: repomocks.NewMockPromptRepository(t),
		refs: repomocks.NewMockPromptReferenceRepository(t),
		cvs:  servicemocks.NewMockContentVersionServiceInterface(t),
	}
	h.repo.EXPECT().GetByID(mock.Anything, lockCaller, lockTeamID, existing.ID).Return(existing, nil).Maybe()
	h.repo.EXPECT().GetBySlug(mock.Anything, lockCaller, lockTeamID, existing.Slug).Return(existing, nil).Maybe()

	projects := repomocks.NewMockProjectRepository(t)
	projects.EXPECT().GetByID(mock.Anything, lockCaller, mock.Anything).
		Return(&models.Project{ID: lockProjectID, UserID: lockCaller}, nil).Maybe()

	h.svc = services.NewPromptService(services.PromptServiceDeps{
		Repo: h.repo, RefRepo: h.refs, ProjectRepo: projects,
		Authz: lockAuthz(t, role), Logger: logger, ContentVersionSvc: h.cvs,
	})
	return h
}

// expectWrite lets one update through: the row write, the reference rebuild a
// body change triggers and the snapshot a body change takes.
func (h *promptLockHarness) expectWrite() {
	h.repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
	h.refs.EXPECT().DeleteByPromptID(mock.Anything, mock.Anything).Return(nil).Maybe()
	h.cvs.EXPECT().SnapshotIfChanged(mock.Anything, mock.Anything).Return(nil).Maybe()
}

var promptFrozenChanges = []struct {
	field string
	req   models.UpdatePromptRequest
}{
	{"slug", models.UpdatePromptRequest{Slug: lockPtr("renamed")}},
	{"status", models.UpdatePromptRequest{Status: lockPtr("draft")}},
	{"project_id", models.UpdatePromptRequest{ProjectID: lockPtr("project-2")}},
	{"mcp_expose", models.UpdatePromptRequest{MCPExpose: lockPtr(false)}},
	{"labels", models.UpdatePromptRequest{Labels: []string{"alpha"}}},
}

func TestPromptService_Update_LockedRejectsFrozenFieldChange(t *testing.T) {
	for _, tc := range promptFrozenChanges {
		t.Run(tc.field, func(t *testing.T) {
			existing := lockablePrompt(heldLock())
			before := *existing
			h := newPromptLockHarness(t, models.TeamMemberRoleOwner, existing)
			// An editable field rides along: the whole update is refused.
			req := tc.req
			req.Body = lockPtr("new body")

			_, err := h.svc.UpdatePrompt(context.Background(), lockCaller, lockTeamID, existing.ID, &req)

			requireLockRefusal(t, err, "prompt", tc.field)
			assert.Equal(t, before, *existing, "the loaded prompt is untouched")
		})
	}
}

func TestPromptService_Update_UnlockedAcceptsFrozenFieldChange(t *testing.T) {
	for _, tc := range promptFrozenChanges {
		t.Run(tc.field, func(t *testing.T) {
			existing := lockablePrompt(nil)
			h := newPromptLockHarness(t, models.TeamMemberRoleMember, existing)
			h.expectWrite()
			req := tc.req

			_, err := h.svc.UpdatePrompt(context.Background(), lockCaller, lockTeamID, existing.ID, &req)

			require.NoError(t, err)
		})
	}
}

func TestPromptService_Update_LockedAcceptsEditableFieldsAndUnchangedFrozenValues(t *testing.T) {
	tests := []struct {
		name string
		req  models.UpdatePromptRequest
	}{
		{"body only", models.UpdatePromptRequest{Body: lockPtr("new body")}},
		{"name and description only", models.UpdatePromptRequest{Name: lockPtr("New"), Description: lockPtr("new")}},
		{
			name: "full body repeating the frozen values, labels in another order",
			req: models.UpdatePromptRequest{
				Name: lockPtr("New"), Description: lockPtr("new"), Body: lockPtr("new body"),
				Slug: lockPtr(lockSlug), Status: lockPtr("published"), ProjectID: lockPtr(lockProjectID),
				MCPExpose: lockPtr(true), Labels: []string{"beta", "alpha"},
			},
		},
		{"empty status is not a change", models.UpdatePromptRequest{Status: lockPtr(""), Body: lockPtr("new body")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing := lockablePrompt(heldLock())
			h := newPromptLockHarness(t, models.TeamMemberRoleMember, existing)
			h.expectWrite()
			req := tt.req

			updated, err := h.svc.UpdatePrompt(context.Background(), lockCaller, lockTeamID, existing.ID, &req)

			require.NoError(t, err)
			assert.Equal(t, lockSlug, updated.Slug)
			assert.Equal(t, "published", updated.Status)
			assert.Equal(t, heldLock(), updated.Lock, "the answer still shows the lock")
		})
	}
}

func TestPromptService_Update_LockedNonMemberGetsTheDenialNotTheLock(t *testing.T) {
	existing := lockablePrompt(heldLock())
	h := newPromptLockHarness(t, "", existing)

	_, err := h.svc.UpdatePrompt(context.Background(), lockCaller, lockTeamID, existing.ID,
		&models.UpdatePromptRequest{Slug: lockPtr("renamed")})

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
}

func TestPromptService_Restore_LockedStaysAllowed(t *testing.T) {
	existing := lockablePrompt(heldLock())
	h := newPromptLockHarness(t, models.TeamMemberRoleMember, existing)
	h.cvs.EXPECT().Restore(mock.Anything, lockTeamID, "prompt", existing.ID, 2).Return("v2 body", nil).Once()
	h.expectWrite()

	restored, err := h.svc.RestorePromptVersion(context.Background(), lockCaller, lockTeamID, lockSlug, 2, nil)

	require.NoError(t, err)
	assert.Equal(t, "v2 body", restored.Body)
	assert.Equal(t, heldLock(), restored.Lock, "the answer still shows the lock")
}

func TestPromptService_Delete_LockedRejectedForEveryRole(t *testing.T) {
	for _, role := range lockAllRoles {
		t.Run(string(role), func(t *testing.T) {
			existing := lockablePrompt(heldLock())
			h := newPromptLockHarness(t, role, existing)

			requireLockRefusal(t, h.svc.DeletePrompt(lockCaller, lockTeamID, existing.ID), "prompt", "")
			requireLockRefusal(t, h.svc.DeletePromptBySlug(lockCaller, lockTeamID, lockSlug), "prompt", "")
		})
	}
}

func TestPromptService_Delete_LockedMemberDeletingAnothersGetsTheDenial(t *testing.T) {
	existing := lockablePrompt(heldLock())
	existing.UserID = lockOther
	h := newPromptLockHarness(t, models.TeamMemberRoleMember, existing)

	assert.ErrorIs(t, h.svc.DeletePrompt(lockCaller, lockTeamID, existing.ID), services.ErrPermissionDenied)
}

func TestPromptService_Delete_WorksAgainAfterUnlock(t *testing.T) {
	existing := lockablePrompt(nil)
	h := newPromptLockHarness(t, models.TeamMemberRoleMember, existing)
	h.refs.EXPECT().HasDependents(mock.Anything, existing.ID).Return(false, nil).Once()
	h.repo.EXPECT().Delete(mock.Anything, lockCaller, lockTeamID, existing.ID).Return(nil).Once()

	require.NoError(t, h.svc.DeletePrompt(lockCaller, lockTeamID, existing.ID))
}

// ---- blueprints ----

func lockableBlueprint(lock *models.ResourceLock) *models.Blueprint {
	return &models.Blueprint{
		ID: "bp-1", TeamID: lockTeamID, UserID: lockCaller, ProjectID: lockProjectID,
		Slug: lockSlug, Title: "Doc", Description: "d", Content: "body", Status: "active",
		Type: "claude-code", Subtype: lockPtr("others"),
		Path: ".claude/doc.md", PathDerived: true, RawContent: "body",
		Labels:   models.LabelList{"alpha", "beta"},
		Metadata: map[string]interface{}{"model": "x", "retries": 3},
		Version:  7, Lock: lock,
	}
}

type blueprintLockHarness struct {
	svc  *services.BlueprintService
	repo *repomocks.MockBlueprintRepository
	cvs  *servicemocks.MockContentVersionServiceInterface
}

func newBlueprintLockHarness(
	t *testing.T, role models.TeamMemberRole, existing *models.Blueprint,
) *blueprintLockHarness {
	t.Helper()
	logger, _ := logtest.New()
	h := &blueprintLockHarness{
		repo: repomocks.NewMockBlueprintRepository(t),
		cvs:  servicemocks.NewMockContentVersionServiceInterface(t),
	}
	h.repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, lockCaller, lockTeamID, lockProjectID, existing.Slug).
		Return(existing, nil).Maybe()
	h.svc = services.NewBlueprintService(services.BlueprintServiceDeps{
		Repo: h.repo, Authz: lockAuthz(t, role), Logger: logger, ContentVersionSvc: h.cvs,
	})
	return h
}

func (h *blueprintLockHarness) expectWrite() {
	h.repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
	h.cvs.EXPECT().SnapshotIfChanged(mock.Anything, mock.Anything).Return(nil).Maybe()
}

func (h *blueprintLockHarness) update(req *models.UpdateBlueprintRequest) (*models.Blueprint, error) {
	return h.svc.UpdateBlueprintByProjectIDAndSlugInTeam(context.Background(), lockCaller, lockTeamID, lockProjectID, lockSlug, req)
}

var blueprintFrozenChanges = []struct {
	field string
	req   models.UpdateBlueprintRequest
}{
	{"slug", models.UpdateBlueprintRequest{Slug: lockPtr("renamed")}},
	{"status", models.UpdateBlueprintRequest{Status: lockPtr("expired")}},
	{"type", models.UpdateBlueprintRequest{Type: lockPtr("general")}},
	{"subtype", models.UpdateBlueprintRequest{Subtype: lockPtr("commands")}},
	{"project_id", models.UpdateBlueprintRequest{ProjectID: lockPtr("project-2")}},
	{"labels", models.UpdateBlueprintRequest{Labels: []string{"alpha", "gamma"}}},
	{"metadata", models.UpdateBlueprintRequest{Metadata: map[string]interface{}{"model": "y"}}},
	{"path", models.UpdateBlueprintRequest{Path: lockPtr("docs/elsewhere.md")}},
}

func TestBlueprintService_Update_LockedRejectsFrozenFieldChange(t *testing.T) {
	for _, tc := range blueprintFrozenChanges {
		t.Run(tc.field, func(t *testing.T) {
			existing := lockableBlueprint(heldLock())
			before := *existing
			h := newBlueprintLockHarness(t, models.TeamMemberRoleOwner, existing)
			req := tc.req
			req.Content = lockPtr("new body")

			_, err := h.update(&req)

			requireLockRefusal(t, err, "blueprint", tc.field)
			assert.Equal(t, before, *existing, "the loaded blueprint is untouched")
		})
	}
}

func TestBlueprintService_Update_UnlockedAcceptsFrozenFieldChange(t *testing.T) {
	for _, tc := range blueprintFrozenChanges {
		t.Run(tc.field, func(t *testing.T) {
			h := newBlueprintLockHarness(t, models.TeamMemberRoleMember, lockableBlueprint(nil))
			h.expectWrite()
			req := tc.req

			_, err := h.update(&req)

			require.NoError(t, err)
		})
	}
}

func TestBlueprintService_Update_LockedAcceptsEditableFieldsAndUnchangedFrozenValues(t *testing.T) {
	tests := []struct {
		name string
		req  models.UpdateBlueprintRequest
	}{
		{"content only", models.UpdateBlueprintRequest{Content: lockPtr("new body")}},
		{"title and description only", models.UpdateBlueprintRequest{Title: lockPtr("New"), Description: lockPtr("new")}},
		{
			// What a form that always sends the whole object produces: the
			// numbers arrive as float64 and the labels in the form's own order.
			name: "full body repeating the frozen values",
			req: models.UpdateBlueprintRequest{
				Title: lockPtr("New"), Description: lockPtr("new"), Content: lockPtr("new body"),
				Slug: lockPtr(lockSlug), Status: lockPtr("active"), Type: lockPtr("claude-code"),
				Subtype: lockPtr("others"), ProjectID: lockPtr(lockProjectID),
				Labels:   []string{" beta ", "alpha", "beta"},
				Metadata: map[string]interface{}{"retries": float64(3), "model": "x"},
				Path:     lockPtr(".claude/doc.md"),
			},
		},
		{"empty status and empty path are not changes", models.UpdateBlueprintRequest{
			Status: lockPtr(""), Path: lockPtr(""), Content: lockPtr("new body"),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newBlueprintLockHarness(t, models.TeamMemberRoleMember, lockableBlueprint(heldLock()))
			h.expectWrite()
			req := tt.req

			updated, err := h.update(&req)

			require.NoError(t, err)
			assert.Equal(t, lockSlug, updated.Slug)
			assert.Equal(t, ".claude/doc.md", updated.Path)
			assert.Equal(t, heldLock(), updated.Lock, "the answer still shows the lock")
		})
	}
}

// A content edit on a skill re-derives the title from the path and regenerates
// raw_content and its hash. None of that was asked for, so a lock allows it.
func TestBlueprintService_Update_LockedAllowsChangesDerivedFromAContentEdit(t *testing.T) {
	existing := lockableBlueprint(heldLock())
	existing.Subtype = lockPtr("skills")
	existing.Path = ".claude/skills/doc/SKILL.md"
	existing.Title = "stale title"
	existing.ContentSHA = "stale"
	h := newBlueprintLockHarness(t, models.TeamMemberRoleMember, existing)
	h.expectWrite()

	updated, err := h.update(&models.UpdateBlueprintRequest{Content: lockPtr("new body")})

	require.NoError(t, err)
	assert.Equal(t, "doc", updated.Title, "the skill name follows its directory")
	assert.Contains(t, updated.RawContent, "new body")
	assert.NotEqual(t, "stale", updated.ContentSHA)
}

func TestBlueprintService_Update_LockedNonMemberGetsTheDenialNotTheLock(t *testing.T) {
	h := newBlueprintLockHarness(t, "", lockableBlueprint(heldLock()))

	_, err := h.update(&models.UpdateBlueprintRequest{Slug: lockPtr("renamed")})

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
}

func TestBlueprintService_Restore_LockedStaysAllowed(t *testing.T) {
	existing := lockableBlueprint(heldLock())
	h := newBlueprintLockHarness(t, models.TeamMemberRoleMember, existing)
	h.cvs.EXPECT().GetVersion(mock.Anything, lockTeamID, "blueprint", existing.ID, 2).
		Return(&models.ContentVersion{Content: "v2 body", RawContent: "v2 raw"}, nil).Once()
	h.expectWrite()

	restored, err := h.svc.RestoreBlueprintVersionInTeam(context.Background(), lockCaller, lockTeamID, lockProjectID, lockSlug, 2, nil)

	require.NoError(t, err)
	assert.Equal(t, "v2 body", restored.Content)
	assert.Equal(t, "v2 raw", restored.RawContent)
}

func TestBlueprintService_Delete_LockedRejectedForEveryRole(t *testing.T) {
	for _, role := range lockAllRoles {
		t.Run(string(role), func(t *testing.T) {
			h := newBlueprintLockHarness(t, role, lockableBlueprint(heldLock()))

			err := h.svc.DeleteBlueprintByProjectIDAndSlug(context.Background(), lockCaller, lockTeamID, lockProjectID, lockSlug)

			requireLockRefusal(t, err, "blueprint", "")
		})
	}
}

func TestBlueprintService_Delete_LockedMemberDeletingAnothersGetsTheDenial(t *testing.T) {
	existing := lockableBlueprint(heldLock())
	existing.UserID = lockOther
	h := newBlueprintLockHarness(t, models.TeamMemberRoleMember, existing)

	err := h.svc.DeleteBlueprintByProjectIDAndSlug(context.Background(), lockCaller, lockTeamID, lockProjectID, lockSlug)

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
}

func TestBlueprintService_Delete_WorksAgainAfterUnlock(t *testing.T) {
	existing := lockableBlueprint(nil)
	h := newBlueprintLockHarness(t, models.TeamMemberRoleMember, existing)
	h.repo.EXPECT().Delete(mock.Anything, lockCaller, lockTeamID, existing.ID).Return(nil).Once()

	require.NoError(t, h.svc.DeleteBlueprintByProjectIDAndSlug(context.Background(), lockCaller, lockTeamID, lockProjectID, lockSlug))
}

// ---- artifacts ----

func lockableArtifact(lock *models.ResourceLock) *models.Artifact {
	return &models.Artifact{
		ID: "art-1", TeamID: lockTeamID, UserID: lockCaller, ProjectID: lockProjectID,
		Slug: lockSlug, Title: "Doc", Description: "d", Content: "body", Status: "active", Type: "document",
		Labels:   models.LabelList{"alpha", "beta"},
		Metadata: map[string]interface{}{"model": "x", "retries": 3},
		Version:  7, Lock: lock,
	}
}

type artifactLockHarness struct {
	svc  *services.ArtifactService
	repo *repomocks.MockArtifactRepository
	cvs  *servicemocks.MockContentVersionServiceInterface
}

func newArtifactLockHarness(t *testing.T, role models.TeamMemberRole, existing *models.Artifact) *artifactLockHarness {
	t.Helper()
	logger, _ := logtest.New()
	h := &artifactLockHarness{
		repo: repomocks.NewMockArtifactRepository(t),
		cvs:  servicemocks.NewMockContentVersionServiceInterface(t),
	}
	h.repo.EXPECT().GetByProjectIDAndSlug(mock.Anything, lockCaller, lockTeamID, lockProjectID, existing.Slug).
		Return(existing, nil).Maybe()
	h.repo.EXPECT().GetByProjectIDAndSlugCrossTeam(mock.Anything, lockCaller, lockProjectID, existing.Slug).
		Return(existing, nil).Maybe()
	h.svc = services.NewArtifactService(services.ArtifactServiceDeps{
		Repo: h.repo, Authz: lockAuthz(t, role), Logger: logger, ContentVersionSvc: h.cvs,
	})
	return h
}

func (h *artifactLockHarness) expectWrite() {
	h.repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
	h.cvs.EXPECT().SnapshotIfChanged(mock.Anything, mock.Anything).Return(nil).Maybe()
}

func (h *artifactLockHarness) update(req *models.UpdateArtifactRequest) (*models.Artifact, error) {
	return h.svc.UpdateArtifactByProjectIDAndSlugInTeam(context.Background(), lockCaller, lockTeamID, lockProjectID, lockSlug, req)
}

var artifactFrozenChanges = []struct {
	field string
	req   models.UpdateArtifactRequest
}{
	{"slug", models.UpdateArtifactRequest{Slug: lockPtr("renamed")}},
	{"status", models.UpdateArtifactRequest{Status: lockPtr("archived")}},
	{"type", models.UpdateArtifactRequest{Type: lockPtr("report")}},
	{"project_id", models.UpdateArtifactRequest{ProjectID: lockPtr("project-2")}},
	{"labels", models.UpdateArtifactRequest{Labels: []string{}}},
	{"metadata", models.UpdateArtifactRequest{Metadata: map[string]interface{}{"model": "x", "retries": 4}}},
}

func TestArtifactService_Update_LockedRejectsFrozenFieldChange(t *testing.T) {
	for _, tc := range artifactFrozenChanges {
		t.Run(tc.field, func(t *testing.T) {
			existing := lockableArtifact(heldLock())
			before := *existing
			h := newArtifactLockHarness(t, models.TeamMemberRoleOwner, existing)
			req := tc.req
			req.Content = lockPtr("new body")

			_, err := h.update(&req)

			requireLockRefusal(t, err, "artifact", tc.field)
			assert.Equal(t, before, *existing, "the loaded artifact is untouched")
		})
	}
}

// The cross-team entry point funnels into the same helper, so it is guarded
// by the same check.
func TestArtifactService_Update_LockedRejectsFrozenFieldChangeAcrossTeams(t *testing.T) {
	h := newArtifactLockHarness(t, models.TeamMemberRoleAdmin, lockableArtifact(heldLock()))

	_, err := h.svc.UpdateArtifactByProjectIDAndSlug(context.Background(), lockCaller, lockProjectID, lockSlug,
		&models.UpdateArtifactRequest{Slug: lockPtr("renamed")})

	requireLockRefusal(t, err, "artifact", "slug")
}

func TestArtifactService_Update_UnlockedAcceptsFrozenFieldChange(t *testing.T) {
	for _, tc := range artifactFrozenChanges {
		t.Run(tc.field, func(t *testing.T) {
			h := newArtifactLockHarness(t, models.TeamMemberRoleMember, lockableArtifact(nil))
			h.expectWrite()
			req := tc.req

			_, err := h.update(&req)

			require.NoError(t, err)
		})
	}
}

func TestArtifactService_Update_LockedAcceptsEditableFieldsAndUnchangedFrozenValues(t *testing.T) {
	tests := []struct {
		name string
		req  models.UpdateArtifactRequest
	}{
		{"content only", models.UpdateArtifactRequest{Content: lockPtr("new body")}},
		{"title and description only", models.UpdateArtifactRequest{Title: lockPtr("New"), Description: lockPtr("new")}},
		{
			name: "full body repeating the frozen values",
			req: models.UpdateArtifactRequest{
				Title: lockPtr("New"), Description: lockPtr("new"), Content: lockPtr("new body"),
				ChangeSummary: lockPtr("tidy"),
				Slug:          lockPtr(lockSlug), Status: lockPtr("active"), Type: lockPtr("document"),
				ProjectID: lockPtr(lockProjectID),
				Labels:    []string{"beta", "alpha"},
				Metadata:  map[string]interface{}{"retries": float64(3), "model": "x"},
			},
		},
		{"empty status is not a change", models.UpdateArtifactRequest{Status: lockPtr(""), Content: lockPtr("new body")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newArtifactLockHarness(t, models.TeamMemberRoleMember, lockableArtifact(heldLock()))
			h.expectWrite()
			req := tt.req

			updated, err := h.update(&req)

			require.NoError(t, err)
			assert.Equal(t, lockSlug, updated.Slug)
			assert.Equal(t, "active", updated.Status)
			assert.Equal(t, heldLock(), updated.Lock, "the answer still shows the lock")
		})
	}
}

func TestArtifactService_Update_LockedNonMemberGetsTheDenialNotTheLock(t *testing.T) {
	h := newArtifactLockHarness(t, "", lockableArtifact(heldLock()))

	_, err := h.update(&models.UpdateArtifactRequest{Slug: lockPtr("renamed")})

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
}

func TestArtifactService_Restore_LockedStaysAllowed(t *testing.T) {
	existing := lockableArtifact(heldLock())
	h := newArtifactLockHarness(t, models.TeamMemberRoleMember, existing)
	h.cvs.EXPECT().Restore(mock.Anything, lockTeamID, "artifact", existing.ID, 2).Return("v2 body", nil).Once()
	h.expectWrite()

	restored, err := h.svc.RestoreArtifactVersionInTeam(context.Background(), lockCaller, lockTeamID, lockProjectID, lockSlug, 2, nil)

	require.NoError(t, err)
	assert.Equal(t, "v2 body", restored.Content)
}

func TestArtifactService_Delete_LockedRejectedForEveryRole(t *testing.T) {
	for _, role := range lockAllRoles {
		t.Run(string(role), func(t *testing.T) {
			h := newArtifactLockHarness(t, role, lockableArtifact(heldLock()))

			err := h.svc.DeleteArtifactByProjectIDAndSlug(context.Background(), lockCaller, lockTeamID, lockProjectID, lockSlug)

			requireLockRefusal(t, err, "artifact", "")
		})
	}
}

func TestArtifactService_Delete_LockedMemberDeletingAnothersGetsTheDenial(t *testing.T) {
	existing := lockableArtifact(heldLock())
	existing.UserID = lockOther
	h := newArtifactLockHarness(t, models.TeamMemberRoleMember, existing)

	err := h.svc.DeleteArtifactByProjectIDAndSlug(context.Background(), lockCaller, lockTeamID, lockProjectID, lockSlug)

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
}

func TestArtifactService_Delete_WorksAgainAfterUnlock(t *testing.T) {
	existing := lockableArtifact(nil)
	h := newArtifactLockHarness(t, models.TeamMemberRoleMember, existing)
	h.repo.EXPECT().Delete(mock.Anything, lockCaller, lockTeamID, existing.ID).Return(nil).Once()

	require.NoError(t, h.svc.DeleteArtifactByProjectIDAndSlug(context.Background(), lockCaller, lockTeamID, lockProjectID, lockSlug))
}

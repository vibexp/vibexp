package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Custom types and statuses in the instance audit log (#1520). Statuses keep
// the per-team history's action names.

func TestCustomTypeAudit_CreateAndDelete(t *testing.T) {
	svc, repo := newTypeService(t)
	events := &auditEvents{}
	svc.recorder = events
	repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "runbook").
		Return(nil, repositories.ErrTypeNotFound)
	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, ty *models.Type) { ty.ID = "type-1" }).Return(nil)

	_, err := svc.CreateCustom(context.Background(), CreateTypeParams{
		TeamID: testTypeTeamID, UserID: testTypeUserID, ResourceType: "artifacts", Slug: "runbook", Name: "Runbook",
	})

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testTypeTeamID, teamSettingsSurfaceCustomTypes,
		teamSettingsActionCreate)
	assert.Equal(t, "runbook", metadata["type_slug"])
	assert.Equal(t, map[string]any{"new": "Runbook"}, metadata["name"])
	assert.Equal(t, testTypeUserID, *events.only(t).Actor.UserID)

	svc, repo = newTypeService(t)
	events = &auditEvents{}
	svc.recorder = events
	repo.EXPECT().List(mock.Anything, testTypeTeamID, "artifacts").Return([]models.Type{
		{ID: "general", Slug: "general", Name: "General", ResourceType: "artifacts"},
		{ID: "type-1", Slug: "runbook", Name: "Runbook", ResourceType: "artifacts"},
	}, nil)
	repo.EXPECT().DeleteCustom(mock.Anything, testTypeTeamID, "type-1", "general").Return(nil)

	require.NoError(t, svc.Delete(context.Background(), testTypeTeamID, "type-1"))

	metadata = requireTeamSettingsEvent(t, events, testTypeTeamID, teamSettingsSurfaceCustomTypes,
		teamSettingsActionDelete)
	assert.Equal(t, "runbook", metadata["type_slug"])
	assert.Equal(t, map[string]any{"old": "Runbook"}, metadata["name"])
	assert.Nil(t, events.only(t).Actor, "Delete has no user id: the recorder reads the actor from the request")
}

func TestCustomTypeAudit_ADeleteOfAnUnlistedTypeStillRecordsItsID(t *testing.T) {
	svc, repo := newTypeService(t)
	events := &auditEvents{}
	svc.recorder = events
	repo.EXPECT().List(mock.Anything, testTypeTeamID, "artifacts").Return(nil, errSettingsStore)
	repo.EXPECT().DeleteCustom(mock.Anything, testTypeTeamID, "type-9", "general").Return(nil)

	require.NoError(t, svc.Delete(context.Background(), testTypeTeamID, "type-9"))

	metadata := requireTeamSettingsEvent(t, events, testTypeTeamID, teamSettingsSurfaceCustomTypes,
		teamSettingsActionDelete)
	assert.Equal(t, "type-9", metadata["type_id"])
	assert.NotContains(t, metadata, "type_slug")
}

func TestCustomTypeAudit_NothingOnFailure(t *testing.T) {
	svc, repo := newTypeService(t)
	events := &auditEvents{}
	svc.recorder = events
	repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "runbook").
		Return(nil, repositories.ErrTypeNotFound)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(repositories.ErrTypeAlreadyExists)
	repo.EXPECT().List(mock.Anything, testTypeTeamID, "artifacts").Return(nil, nil)
	repo.EXPECT().DeleteCustom(mock.Anything, testTypeTeamID, "type-1", "general").Return(repositories.ErrTypeNotFound)

	_, err := svc.CreateCustom(context.Background(), CreateTypeParams{
		TeamID: testTypeTeamID, UserID: testTypeUserID, ResourceType: "artifacts", Slug: "runbook", Name: "Runbook",
	})
	require.Error(t, err)
	require.Error(t, svc.Delete(context.Background(), testTypeTeamID, "type-1"))

	assert.Empty(t, events.got)
}

func TestCustomTypeAudit_CopyRecordsAddedAndSkippedSlugs(t *testing.T) {
	svc, repo := newTypeService(t)
	events := &auditEvents{}
	svc.recorder = events
	repo.EXPECT().List(mock.Anything, "team-src", "artifacts").Return([]models.Type{
		{ID: "s1", Slug: "runbook", Name: "Runbook", ResourceType: "artifacts"},
		{ID: "s2", Slug: "adr", Name: "ADR", ResourceType: "artifacts"},
	}, nil)
	repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "runbook").
		Return(nil, repositories.ErrTypeNotFound)
	repo.EXPECT().GetBySlug(mock.Anything, testTypeTeamID, "artifacts", "adr").
		Return(&models.Type{Slug: "adr"}, nil)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

	_, err := svc.CopyFromTeam(context.Background(), CopyTypesParams{
		TeamID: testTypeTeamID, SourceTeamID: "team-src", UserID: testTypeUserID,
	})

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testTypeTeamID, teamSettingsSurfaceCustomTypes,
		teamSettingsActionCopy)
	assert.Equal(t, "team-src", metadata["source_team_id"])
	assert.Equal(t, "runbook", metadata["added_slugs"])
	assert.Equal(t, "adr", metadata["skipped_slugs"])
}

func TestCustomStatusAudit_CreateRenameDelete(t *testing.T) {
	h := newStatusManagementHarness(t).as(models.TeamMemberRoleAdmin)
	events := &auditEvents{}
	h.svc.recorder = events
	h.repo.EXPECT().Count(mock.Anything, testStatusTeamID, models.StatusResourceTypeBlueprint).Return(0, nil)
	h.expectCreate(func(*models.TeamStatus) bool { return true })

	_, err := h.svc.Create(context.Background(), CreateStatusParams{
		TeamID: testStatusTeamID, UserID: testStatusUserID, ResourceType: models.StatusResourceTypeBlueprint,
		Slug: "in-review", Name: "In review",
	})

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testStatusTeamID, teamSettingsSurfaceCustomStatuses,
		statusAuditActionCreated)
	assert.Equal(t, testStatusRowID, metadata["status_id"])
	assert.Equal(t, map[string]any{"new": "In review"}, metadata["name"])

	h = newStatusManagementHarness(t).as(models.TeamMemberRoleAdmin)
	events = &auditEvents{}
	h.svc.recorder = events
	stored := customStatus(testStatusRowID, models.StatusResourceTypeBlueprint, "in-review", "In review")
	h.expectRename(stored, "Under review", "Being reviewed")
	h.repo.EXPECT().UsageCounts(mock.Anything, testStatusTeamID, models.StatusResourceTypeBlueprint).
		Return(map[string]int{}, nil)

	_, err = h.svc.Rename(context.Background(), RenameStatusParams{
		TeamID: testStatusTeamID, UserID: testStatusUserID, ID: testStatusRowID, Name: "Under review",
	})

	require.NoError(t, err)
	metadata = requireTeamSettingsEvent(t, events, testStatusTeamID, teamSettingsSurfaceCustomStatuses,
		statusAuditActionRenamed)
	assert.Equal(t, []string{"name"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": "Being reviewed", "new": "Under review"}, metadata["name"])

	h = newStatusManagementHarness(t).as(models.TeamMemberRoleAdmin)
	events = &auditEvents{}
	h.svc.recorder = events
	h.repo.EXPECT().GetByID(mock.Anything, testStatusTeamID, testStatusRowID).Return(&stored, nil)
	wantDefault := models.DefaultStatusFor(models.StatusResourceTypeBlueprint)
	h.expectDelete(stored, wantDefault, 3)

	_, err = h.svc.Delete(context.Background(), DeleteStatusParams{
		TeamID: testStatusTeamID, UserID: testStatusUserID, ID: testStatusRowID,
	})

	require.NoError(t, err)
	metadata = requireTeamSettingsEvent(t, events, testStatusTeamID, teamSettingsSurfaceCustomStatuses,
		statusAuditActionDeleted)
	assert.Equal(t, 3, metadata["reassigned_count"])
	assert.Equal(t, wantDefault, metadata["reassigned_to"])
	assert.Equal(t, map[string]any{"old": "In review"}, metadata["name"])
}

func TestCustomStatusAudit_NothingOnFailureOrDenial(t *testing.T) {
	h := newStatusManagementHarness(t).as(models.TeamMemberRoleAdmin)
	events := &auditEvents{}
	h.svc.recorder = events
	h.repo.EXPECT().UpdateName(mock.Anything, testStatusTeamID, testStatusRowID, "Under review", mock.Anything).
		Return(nil, errStatusBoom)
	_, err := h.svc.Rename(context.Background(), RenameStatusParams{
		TeamID: testStatusTeamID, UserID: testStatusUserID, ID: testStatusRowID, Name: "Under review",
	})
	require.Error(t, err)

	denied := newStatusManagementHarness(t).as(models.TeamMemberRoleMember)
	denied.svc.recorder = events
	_, err = denied.svc.Rename(context.Background(), RenameStatusParams{
		TeamID: testStatusTeamID, UserID: testStatusUserID, ID: testStatusRowID, Name: "Under review",
	})
	require.Error(t, err)

	assert.Empty(t, events.got)
}

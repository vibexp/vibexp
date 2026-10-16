package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Team events in the audit log (#1515).

const (
	teamAuditActor  = "actor-1"
	teamAuditTeamID = "team-1"
	teamAuditTarget = "member-1"
	teamAuditOwner  = "owner-1"
)

// ownerAuthz authorizes every check as the team's owner.
type ownerAuthz struct{ allowAllAuthz }

func (ownerAuthz) Authorize(context.Context, string, string, authz.Permission) (models.TeamMemberRole, error) {
	return models.TeamMemberRoleOwner, nil
}

type teamAuditFixture struct {
	svc     *TeamService
	teams   *mocks.MockTeamRepository
	members *mocks.MockTeamMemberRepository
	users   *mocks.MockUserRepository
	events  *auditEvents
	team    *models.Team
}

func newTeamAuditFixture(t *testing.T, rec audit.Recorder) *teamAuditFixture {
	t.Helper()
	f := &teamAuditFixture{
		teams:   mocks.NewMockTeamRepository(t),
		members: mocks.NewMockTeamMemberRepository(t),
		users:   mocks.NewMockUserRepository(t),
		events:  &auditEvents{},
		team:    &models.Team{ID: teamAuditTeamID, Name: "Platform", Slug: "platform", OwnerID: teamAuditOwner},
	}
	if rec == nil {
		rec = f.events
	}
	logger, _ := logtest.New()
	f.svc = NewTeamService(f.teams, f.members, f.users, ownerAuthz{}, logger, nil, WithTeamAuditRecorder(rec))
	f.teams.EXPECT().GetByID(mock.Anything, teamAuditTeamID).
		RunAndReturn(func(context.Context, string) (*models.Team, error) { c := *f.team; return &c, nil }).Maybe()
	return f
}

func TestTeamAudit_Created(t *testing.T) {
	f := newTeamAuditFixture(t, nil)
	f.teams.EXPECT().GetByOwnerAndSlug(mock.Anything, teamAuditActor, "platform").Return(nil, errors.New("none"))
	f.teams.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, team *models.Team) { team.ID = "team-new" }).Return(nil)
	f.members.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

	_, err := f.svc.CreateTeam(context.Background(), teamAuditActor, &models.CreateTeamRequest{Name: "Platform"})

	require.NoError(t, err)
	e := f.events.only(t)
	assert.Equal(t, models.AuditOpTeamCreated, e.Operation)
	assert.Equal(t, "team-new", e.TeamID)
	assert.Equal(t, "Platform", e.TeamName)
	assert.Equal(t, teamAuditActor, *e.Actor.UserID)
}

func TestTeamAudit_UpdatedRecordsOldAndNew(t *testing.T) {
	f := newTeamAuditFixture(t, nil)
	f.teams.EXPECT().GetByOwnerAndSlug(mock.Anything, teamAuditOwner, "core").Return(nil, errors.New("none"))
	f.teams.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)
	name := "Core"

	_, err := f.svc.UpdateTeam(context.Background(), teamAuditActor, teamAuditTeamID, &models.UpdateTeamRequest{Name: &name})

	require.NoError(t, err)
	e := f.events.only(t)
	assert.Equal(t, models.AuditOpTeamUpdated, e.Operation)
	assert.Equal(t, map[string]any{
		"name": map[string]any{"old": "Platform", "new": "Core"},
		"slug": map[string]any{"old": "platform", "new": "core"},
	}, e.Metadata["changes"])
}

func TestTeamAudit_Deleted(t *testing.T) {
	f := newTeamAuditFixture(t, nil)
	f.users.EXPECT().GetByID(mock.Anything, teamAuditActor).Return(&models.User{ID: teamAuditActor}, nil)
	f.members.EXPECT().GetByTeamID(mock.Anything, teamAuditTeamID).Return([]models.TeamMember{{}}, nil)
	f.members.EXPECT().Delete(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	f.teams.EXPECT().Delete(mock.Anything, teamAuditOwner, teamAuditTeamID).Return(nil)

	require.NoError(t, f.svc.DeleteTeam(context.Background(), teamAuditActor, teamAuditTeamID))

	e := f.events.only(t)
	assert.Equal(t, models.AuditOpTeamDeleted, e.Operation)
	assert.Equal(t, "Platform", e.ResourceName, "the name is snapshotted before the cascade")
}

func TestTeamAudit_RoleChangedRecordsOldAndNewRole(t *testing.T) {
	f := newTeamAuditFixture(t, nil)
	f.members.EXPECT().GetByTeamAndUser(mock.Anything, teamAuditTeamID, teamAuditTarget).
		Return(&models.TeamMember{UserID: teamAuditTarget, Role: models.TeamMemberRoleMember}, nil).Once()
	f.members.EXPECT().UpdateRole(mock.Anything, teamAuditTeamID, teamAuditTarget, models.TeamMemberRoleAdmin).Return(nil)
	f.members.EXPECT().GetByTeamAndUser(mock.Anything, teamAuditTeamID, teamAuditTarget).
		Return(&models.TeamMember{UserID: teamAuditTarget, Role: models.TeamMemberRoleAdmin}, nil).Once()
	f.users.EXPECT().GetByID(mock.Anything, teamAuditTarget).
		Return(&models.User{ID: teamAuditTarget, Email: "grace@example.com"}, nil)

	_, err := f.svc.UpdateMemberRole(context.Background(), teamAuditActor, teamAuditTeamID, teamAuditTarget,
		models.TeamMemberRoleAdmin)

	require.NoError(t, err)
	e := f.events.only(t)
	assert.Equal(t, models.AuditOpTeamMemberRoleChanged, e.Operation)
	assert.Equal(t, models.AuditResourceTypeTeamMember, e.ResourceType)
	assert.Equal(t, map[string]any{
		"target_user_id": teamAuditTarget, "target_email": "grace@example.com",
		"old_role": "member", "new_role": "admin",
	}, e.Metadata)
}

func TestTeamAudit_OwnershipTransferred(t *testing.T) {
	f := newTeamAuditFixture(t, nil)
	f.members.EXPECT().GetByTeamAndUser(mock.Anything, teamAuditTeamID, teamAuditTarget).
		Return(&models.TeamMember{UserID: teamAuditTarget}, nil)
	f.teams.EXPECT().TransferOwnership(mock.Anything, teamAuditTeamID, teamAuditOwner, teamAuditTarget).Return(nil)

	_, err := f.svc.TransferOwnership(context.Background(), teamAuditActor, teamAuditTeamID, teamAuditTarget)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"from_user_id": teamAuditOwner, "to_user_id": teamAuditTarget},
		f.events.only(t).Metadata)
}

func TestTeamAudit_MemberRemovedNamesWhoItWas(t *testing.T) {
	f := newTeamAuditFixture(t, nil)
	f.members.EXPECT().GetByTeamAndUser(mock.Anything, teamAuditTeamID, teamAuditTarget).
		Return(&models.TeamMember{UserID: teamAuditTarget, Role: models.TeamMemberRoleAdmin}, nil)
	f.users.EXPECT().GetByID(mock.Anything, teamAuditTarget).
		Return(&models.User{ID: teamAuditTarget, Email: "grace@example.com"}, nil)
	f.members.EXPECT().Delete(mock.Anything, teamAuditTeamID, teamAuditTarget).Return(nil)

	require.NoError(t, f.svc.RemoveTeamMember(context.Background(), teamAuditActor, teamAuditTeamID, teamAuditTarget))

	e := f.events.only(t)
	assert.Equal(t, models.AuditOpTeamMemberRemoved, e.Operation)
	assert.Equal(t, "grace@example.com", e.ResourceName)
	assert.Equal(t, "admin", e.Metadata["role"])
}

func TestTeamAudit_FailedChangeRecordsNothing(t *testing.T) {
	f := newTeamAuditFixture(t, nil)
	f.members.EXPECT().GetByTeamAndUser(mock.Anything, teamAuditTeamID, teamAuditTarget).
		Return(&models.TeamMember{UserID: teamAuditTarget}, nil)
	f.users.EXPECT().GetByID(mock.Anything, teamAuditTarget).Return(&models.User{}, nil)
	f.members.EXPECT().Delete(mock.Anything, teamAuditTeamID, teamAuditTarget).Return(errors.New("db down"))

	require.Error(t, f.svc.RemoveTeamMember(context.Background(), teamAuditActor, teamAuditTeamID, teamAuditTarget))
	assert.Empty(t, f.events.got)
}

func TestTeamAudit_NoRecorderSkipsThePreReads(t *testing.T) {
	teams := mocks.NewMockTeamRepository(t)
	members := mocks.NewMockTeamMemberRepository(t)
	logger, _ := logtest.New()
	svc := NewTeamService(teams, members, nil, ownerAuthz{}, logger, nil)
	teams.EXPECT().GetByID(mock.Anything, teamAuditTeamID).
		Return(&models.Team{ID: teamAuditTeamID, OwnerID: teamAuditOwner}, nil)
	members.EXPECT().Delete(mock.Anything, teamAuditTeamID, teamAuditTarget).Return(nil)

	// The strict mocks fail on any read the recorder-less service should skip.
	require.NoError(t, svc.RemoveTeamMember(context.Background(), teamAuditActor, teamAuditTeamID, teamAuditTarget))
}

func TestTeamAudit_PanickingRecorderNeverChangesTheResult(t *testing.T) {
	f := newTeamAuditFixture(t, panickingAuditRecorder{})
	f.members.EXPECT().GetByTeamAndUser(mock.Anything, teamAuditTeamID, teamAuditTarget).
		Return(&models.TeamMember{UserID: teamAuditTarget}, nil)
	f.teams.EXPECT().TransferOwnership(mock.Anything, teamAuditTeamID, teamAuditOwner, teamAuditTarget).Return(nil)

	team, err := f.svc.TransferOwnership(context.Background(), teamAuditActor, teamAuditTeamID, teamAuditTarget)

	require.NoError(t, err)
	assert.Equal(t, teamAuditTarget, team.OwnerID)
}

package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Invitation events in the audit log (#1515).

type invitationAuditFixture struct {
	svc         *TeamInvitationService
	invitations *mocks.MockTeamInvitationRepository
	users       *mocks.MockUserRepository
	members     *mocks.MockTeamMemberRepository
	email       *MockEmailService
	events      *auditEvents
	invitation  *models.TeamInvitation
}

func newInvitationAuditFixture(t *testing.T) *invitationAuditFixture {
	t.Helper()
	f := &invitationAuditFixture{
		invitations: mocks.NewMockTeamInvitationRepository(t),
		users:       mocks.NewMockUserRepository(t),
		members:     mocks.NewMockTeamMemberRepository(t),
		email:       &MockEmailService{},
		events:      &auditEvents{},
		invitation: &models.TeamInvitation{
			ID: "inv-1", TeamID: teamAuditTeamID, InviteeEmail: "grace@example.com", Role: models.TeamMemberRoleAdmin,
			Token: "SECRET-INVITE-TOKEN", Status: models.InvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	f.svc = NewTeamInvitationService(TeamInvitationServiceDeps{
		InvitationRepo: f.invitations, TeamMemberRepo: f.members, UserRepo: f.users,
		EmailService: f.email, Authz: allowAllAuthz{}, Cfg: &config.Config{}, Audit: f.events,
	})
	return f
}

// assertInvitationEvent checks op on the fixture invitation, and that the
// token never reaches the event.
func (f *invitationAuditFixture) assertInvitationEvent(t *testing.T, op models.AuditOperation, actor string) {
	t.Helper()
	e := f.events.only(t)
	assert.Equal(t, op, e.Operation)
	assert.Equal(t, teamAuditTeamID, e.TeamID)
	assert.Equal(t, models.AuditResourceTypeInvitation, e.ResourceType)
	assert.Equal(t, "inv-1", e.ResourceID)
	assert.Equal(t, map[string]any{"invitee_email": "grace@example.com", "role": "admin"}, e.Metadata)
	require.NotNil(t, e.Actor)
	assert.Equal(t, actor, *e.Actor.UserID)
	doc, err := json.Marshal(e)
	require.NoError(t, err)
	assert.NotContains(t, string(doc), "SECRET-INVITE-TOKEN")
}

func TestInvitationAudit_Sent(t *testing.T) {
	f := newInvitationAuditFixture(t)
	f.invitations.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, inv *models.TeamInvitation) { inv.ID = "inv-1" }).Return(nil)
	f.email.On("SendTeamInvitation", mock.Anything, teamAuditTeamID, mock.Anything, "Platform", "Ada").Return(nil)

	_, err := f.svc.createAndSendInvitation(context.Background(), invitationRequest{
		team: &models.Team{ID: teamAuditTeamID, Name: "Platform"}, inviterID: teamAuditActor, inviterName: "Ada",
		email: "grace@example.com", role: models.TeamMemberRoleAdmin, now: time.Now(), expiresAt: time.Now().Add(time.Hour),
	})

	require.NoError(t, err)
	e := f.events.only(t)
	assert.Equal(t, models.AuditOpTeamInviteSent, e.Operation)
	assert.Equal(t, "Platform", e.TeamName)
	assert.Equal(t, map[string]any{"invitee_email": "grace@example.com", "role": "admin"}, e.Metadata)
	doc, err := json.Marshal(e)
	require.NoError(t, err)
	assert.NotContains(t, string(doc), "token")
}

func TestInvitationAudit_Accepted(t *testing.T) {
	f := newInvitationAuditFixture(t)
	f.invitations.EXPECT().GetByToken(mock.Anything, "SECRET-INVITE-TOKEN").Return(f.invitation, nil)
	f.users.EXPECT().GetByID(mock.Anything, "grace").Return(&models.User{ID: "grace", Email: "grace@example.com"}, nil)
	f.members.EXPECT().GetByTeamAndUser(mock.Anything, teamAuditTeamID, "grace").Return(nil, assert.AnError)
	f.invitations.EXPECT().Accept(mock.Anything, "inv-1", mock.Anything).Return(nil)

	_, err := f.svc.AcceptInvitation(context.Background(), "SECRET-INVITE-TOKEN", "grace")

	require.NoError(t, err)
	f.assertInvitationEvent(t, models.AuditOpTeamInviteAccepted, "grace")
}

func TestInvitationAudit_Rejected(t *testing.T) {
	f := newInvitationAuditFixture(t)
	f.invitations.EXPECT().GetByToken(mock.Anything, "SECRET-INVITE-TOKEN").Return(f.invitation, nil)
	f.users.EXPECT().GetByID(mock.Anything, "grace").Return(&models.User{ID: "grace", Email: "grace@example.com"}, nil)
	f.invitations.EXPECT().UpdateStatus(mock.Anything, "inv-1", models.InvitationStatusRejected).Return(nil)

	require.NoError(t, f.svc.RejectInvitation(context.Background(), "SECRET-INVITE-TOKEN", "grace"))
	f.assertInvitationEvent(t, models.AuditOpTeamInviteRejected, "grace")
}

func TestInvitationAudit_Revoked(t *testing.T) {
	f := newInvitationAuditFixture(t)
	f.invitations.EXPECT().GetByID(mock.Anything, "inv-1").Return(f.invitation, nil)
	f.invitations.EXPECT().UpdateStatus(mock.Anything, "inv-1", models.InvitationStatusRevoked).Return(nil)

	require.NoError(t, f.svc.RevokeInvitation(context.Background(), teamAuditActor, "inv-1"))
	f.assertInvitationEvent(t, models.AuditOpTeamInviteRevoked, teamAuditActor)
}

func TestInvitationAudit_FailedRevokeRecordsNothing(t *testing.T) {
	f := newInvitationAuditFixture(t)
	f.invitations.EXPECT().GetByID(mock.Anything, "inv-1").Return(f.invitation, nil)
	f.invitations.EXPECT().UpdateStatus(mock.Anything, "inv-1", models.InvitationStatusRevoked).Return(assert.AnError)

	require.Error(t, f.svc.RevokeInvitation(context.Background(), teamAuditActor, "inv-1"))
	assert.Empty(t, f.events.got)
}

// Registering through an invitation accepts it: the new account is the actor.
func TestInvitationAudit_AcceptedByRegistration(t *testing.T) {
	f := newInvRegFixture(t, passwordLoginOn)
	events := &auditEvents{}
	f.svc.deps.Audit = events
	f.pending()
	f.noAccount()
	f.users.On("CreateWithCredential", mock.Anything, mock.Anything, invRegHash).
		Run(func(args mock.Arguments) { args.Get(1).(*models.User).ID = "user-new" }).Return(nil).Once()
	f.users.On("GetByID", mock.Anything, "user-new").Return(&models.User{ID: "user-new", Email: invRegEmail}, nil)
	f.invitations.On("Accept", mock.Anything, invRegInvID, mock.Anything).Return(nil).Once()

	result, err := f.svc.Register(context.Background(), invRegToken, invRegPhrase, "Grace")

	require.NoError(t, err)
	require.True(t, result.InvitationAccepted)
	e := events.only(t)
	assert.Equal(t, models.AuditOpTeamInviteAccepted, e.Operation)
	assert.Equal(t, invRegTeamID, e.TeamID)
	assert.Equal(t, "user-new", *e.Actor.UserID)
	doc, err := json.Marshal(e)
	require.NoError(t, err)
	assert.NotContains(t, string(doc), invRegToken)
}

func TestInvitationAudit_RegistrationThatCouldNotJoinRecordsNothing(t *testing.T) {
	f := newInvRegFixture(t, passwordLoginOn)
	events := &auditEvents{}
	f.svc.deps.Audit = events
	f.pending()
	f.noAccount()
	f.users.On("CreateWithCredential", mock.Anything, mock.Anything, invRegHash).
		Run(func(args mock.Arguments) { args.Get(1).(*models.User).ID = "user-new" }).Return(nil).Once()
	f.users.On("GetByID", mock.Anything, "user-new").Return(&models.User{ID: "user-new", Email: invRegEmail}, nil)
	f.invitations.On("Accept", mock.Anything, invRegInvID, mock.Anything).Return(assert.AnError).Once()

	result, err := f.svc.Register(context.Background(), invRegToken, invRegPhrase, "Grace")

	require.NoError(t, err)
	assert.False(t, result.InvitationAccepted)
	assert.Empty(t, events.got)
}

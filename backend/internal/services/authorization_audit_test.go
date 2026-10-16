package services

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Authorization denials in the audit log (#1515).

func newAuditedAuthz(t *testing.T) (*AuthorizationService, *mocks.MockTeamMemberRepository, *auditEvents) {
	t.Helper()
	repo := mocks.NewMockTeamMemberRepository(t)
	events := &auditEvents{}
	logger, _ := logtest.New()
	return NewAuthorizationService(repo, logger, WithAuthorizationAuditRecorder(events)), repo, events
}

func nonMember(repo *mocks.MockTeamMemberRepository) {
	repo.EXPECT().GetByTeamAndUser(mock.Anything, testAuthzTeamID, testAuthzUserID).
		Return(nil, repositories.ErrTeamMemberNotFound).Once()
}

func TestAuthorization_RoleDenialIsRecorded(t *testing.T) {
	for name, check := range map[string]func(*AuthorizationService) error{
		"Can": func(s *AuthorizationService) error {
			return s.Can(context.Background(), testAuthzUserID, testAuthzTeamID, authz.TeamDelete)
		},
		"Authorize": func(s *AuthorizationService) error {
			_, err := s.Authorize(context.Background(), testAuthzUserID, testAuthzTeamID, authz.TeamDelete)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc, repo, events := newAuditedAuthz(t)
			memberWithRole(repo, models.TeamMemberRoleMember)

			require.ErrorIs(t, check(svc), ErrPermissionDenied)

			e := events.only(t)
			assert.Equal(t, models.AuditOpAuthzDenied, e.Operation)
			assert.Equal(t, models.AuditOutcomeDenied, e.Outcome)
			assert.Equal(t, testAuthzTeamID, e.TeamID)
			require.NotNil(t, e.Actor)
			assert.Equal(t, testAuthzUserID, *e.Actor.UserID)
			assert.Equal(t, map[string]any{"permission": string(authz.TeamDelete), "role": "member"}, e.Metadata)
		})
	}
}

func TestAuthorization_NonMemberDenialIsRecorded(t *testing.T) {
	svc, repo, events := newAuditedAuthz(t)
	nonMember(repo)

	_, err := svc.Authorize(context.Background(), testAuthzUserID, testAuthzTeamID, authz.ResourceCreate)

	require.ErrorIs(t, err, ErrPermissionDenied)
	assert.Equal(t, auditRoleNonMember, events.only(t).Metadata["role"])
}

func TestAuthorization_DenialKeepsAuthorizeContract(t *testing.T) {
	svc, repo, _ := newAuditedAuthz(t)
	memberWithRole(repo, models.TeamMemberRoleMember)

	role, err := svc.Authorize(context.Background(), testAuthzUserID, testAuthzTeamID, authz.TeamDelete)

	require.Error(t, err)
	assert.Empty(t, role, "a denied Authorize still returns no role")
}

func TestAuthorization_SuccessAndDatabaseErrorRecordNothing(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		svc, repo, events := newAuditedAuthz(t)
		memberWithRole(repo, models.TeamMemberRoleOwner)

		require.NoError(t, svc.Can(context.Background(), testAuthzUserID, testAuthzTeamID, authz.TeamDelete))
		assert.Empty(t, events.got)
	})

	t.Run("database error is not a denial", func(t *testing.T) {
		svc, repo, events := newAuditedAuthz(t)
		repo.EXPECT().GetByTeamAndUser(mock.Anything, testAuthzTeamID, testAuthzUserID).
			Return(nil, stderrors.New("db down")).Once()

		err := svc.Can(context.Background(), testAuthzUserID, testAuthzTeamID, authz.TeamDelete)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrPermissionDenied)
		assert.Empty(t, events.got)
	})

	t.Run("IsMember never records", func(t *testing.T) {
		svc, repo, events := newAuditedAuthz(t)
		nonMember(repo)

		require.ErrorIs(t, svc.IsMember(context.Background(), testAuthzUserID, testAuthzTeamID), ErrPermissionDenied)
		assert.Empty(t, events.got)
	})
}

func TestAuthorization_CanActOnResourceRecordsTheOwner(t *testing.T) {
	svc, repo, events := newAuditedAuthz(t)
	memberWithRole(repo, models.TeamMemberRoleMember)

	err := svc.CanActOnResource(context.Background(), testAuthzUserID, testAuthzTeamID, "someone-else",
		authz.ResourceDeleteOwn, authz.ResourceDeleteAny)

	require.ErrorIs(t, err, ErrPermissionDenied)
	assert.Equal(t, map[string]any{
		"permission": string(authz.ResourceDeleteAny), "role": "member", "resource_owner_id": "someone-else",
	}, events.only(t).Metadata, "exactly one denial, for the one permission checked")
}

func TestAuthorization_CanActOnResourceOwnAllowedRecordsNothing(t *testing.T) {
	svc, repo, events := newAuditedAuthz(t)
	memberWithRole(repo, models.TeamMemberRoleMember)

	require.NoError(t, svc.CanActOnResource(context.Background(), testAuthzUserID, testAuthzTeamID, testAuthzUserID,
		authz.ResourceDeleteOwn, authz.ResourceDeleteAny))
	assert.Empty(t, events.got)
}

func TestAuthorization_AllowsNeverRecords(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(*mocks.MockTeamMemberRepository)
		want    bool
		wantErr bool
	}{
		{"allowed", func(r *mocks.MockTeamMemberRepository) { memberWithRole(r, models.TeamMemberRoleOwner) }, true, false},
		{"refused role", func(r *mocks.MockTeamMemberRepository) { memberWithRole(r, models.TeamMemberRoleMember) }, false, false},
		{"non-member", nonMember, false, false},
		{"database error", func(r *mocks.MockTeamMemberRepository) {
			r.EXPECT().GetByTeamAndUser(mock.Anything, testAuthzTeamID, testAuthzUserID).
				Return(nil, stderrors.New("db down")).Once()
		}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, events := newAuditedAuthz(t)
			tc.arrange(repo)

			ok, err := svc.Allows(context.Background(), testAuthzUserID, testAuthzTeamID, authz.TeamDelete)

			assert.Equal(t, tc.want, ok)
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Empty(t, events.got, "Allows is the non-recording probe")
		})
	}
}

type panickingAuditRecorder struct{}

func (panickingAuditRecorder) Record(context.Context, audit.Event) { panic("audit down") }

func TestAuthorization_PanickingRecorderNeverChangesTheDecision(t *testing.T) {
	repo := mocks.NewMockTeamMemberRepository(t)
	memberWithRole(repo, models.TeamMemberRoleMember)
	logger, _ := logtest.New()
	svc := NewAuthorizationService(repo, logger, WithAuthorizationAuditRecorder(panickingAuditRecorder{}))

	var err error
	assert.NotPanics(t, func() {
		err = svc.Can(context.Background(), testAuthzUserID, testAuthzTeamID, authz.TeamDelete)
	})
	assert.ErrorIs(t, err, ErrPermissionDenied, "the denial is still the answer")
}

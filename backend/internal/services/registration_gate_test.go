package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// stubAuthPolicy serves a fixed registration mode, or an error.
type stubAuthPolicy struct {
	mode string
	err  error
}

func (p stubAuthPolicy) Policy(context.Context) (models.InstanceAuthPolicy, error) {
	if p.err != nil {
		return models.InstanceAuthPolicy{}, p.err
	}
	return models.InstanceAuthPolicy{RegistrationMode: p.mode}, nil
}

func (stubAuthPolicy) PasswordResetChannel(context.Context) (string, error) {
	return PasswordResetByAdmin, nil
}

const gateRootAdmin = "root@corp.example"

// gateFixture is the policy-backed gate over a mock invitation repository, with
// gateRootAdmin as the only root instance admin.
type gateFixture struct {
	gate        RegistrationGate
	invitations *repo_mocks.MockTeamInvitationRepository
	logs        *logtest.Recorder
}

func newGateFixture(t *testing.T, policy stubAuthPolicy) *gateFixture {
	t.Helper()
	logger, recorder := logtest.New()
	invitations := repo_mocks.NewMockTeamInvitationRepository(t)
	return &gateFixture{
		gate: NewRegistrationGate(RegistrationGateDeps{
			Policy:      policy,
			Invitations: invitations,
			RootAdmins:  NewInstanceAdminService([]string{gateRootAdmin}, nil, nil, logger),
			Logger:      logger,
		}),
		invitations: invitations,
		logs:        recorder,
	}
}

// closedReason returns the `reason` of the sign-up refusal that was logged.
func (f *gateFixture) closedReason(t *testing.T) string {
	t.Helper()
	for _, entry := range f.logs.AllEntries() {
		if reason, ok := entry.Data["reason"].(string); ok {
			assert.Equal(t, slog.LevelInfo, entry.Level, "a refusal is a policy outcome, not an error")
			return reason
		}
	}
	t.Fatal("no refusal was logged")
	return ""
}

func TestRegistrationGate_OpenModeAdmitsEveryone(t *testing.T) {
	ctx := context.Background()
	for _, verified := range []bool{true, false} {
		f := newGateFixture(t, stubAuthPolicy{mode: models.RegistrationModeOpen})

		require.NoError(t, f.gate.AllowNewAccount(ctx, "anyone@example.com", verified))

		f.invitations.AssertNotCalled(t, "GetPendingByEmail", mock.Anything, mock.Anything)
	}
}

func TestRegistrationGate_InviteOnly(t *testing.T) {
	ctx := context.Background()
	pending := []models.TeamInvitation{{ID: "inv-1", InviteeEmail: "ada@example.com"}}

	t.Run("a verified address with a pending invitation is admitted", func(t *testing.T) {
		f := newGateFixture(t, stubAuthPolicy{mode: models.RegistrationModeInviteOnly})
		f.invitations.On("GetPendingByEmail", ctx, "Ada@Example.com").Return(pending, nil)

		require.NoError(t, f.gate.AllowNewAccount(ctx, "Ada@Example.com", true))
	})

	t.Run("a verified address with no pending invitation is refused", func(t *testing.T) {
		// The repository returns only pending, unexpired invitations, so this is
		// also what a revoked, declined, accepted or expired one looks like.
		f := newGateFixture(t, stubAuthPolicy{mode: models.RegistrationModeInviteOnly})
		f.invitations.On("GetPendingByEmail", ctx, "stranger@example.com").Return([]models.TeamInvitation{}, nil)

		err := f.gate.AllowNewAccount(ctx, "stranger@example.com", true)

		require.ErrorIs(t, err, ErrRegistrationClosed)
		assert.Equal(t, "not_invited", f.closedReason(t))
	})

	t.Run("an unverified address is refused even when it was invited", func(t *testing.T) {
		f := newGateFixture(t, stubAuthPolicy{mode: models.RegistrationModeInviteOnly})

		err := f.gate.AllowNewAccount(ctx, "ada@example.com", false)

		require.ErrorIs(t, err, ErrRegistrationClosed)
		assert.Equal(t, "unverified_email", f.closedReason(t))
		f.invitations.AssertNotCalled(t, "GetPendingByEmail", mock.Anything, mock.Anything)
	})

	t.Run("a verified root instance admin is always admitted", func(t *testing.T) {
		f := newGateFixture(t, stubAuthPolicy{mode: models.RegistrationModeInviteOnly})

		require.NoError(t, f.gate.AllowNewAccount(ctx, "ROOT@corp.example", true))

		f.invitations.AssertNotCalled(t, "GetPendingByEmail", mock.Anything, mock.Anything)
	})

	t.Run("an unverified claim of a root admin address is refused", func(t *testing.T) {
		// The exemption grants access by address, so an address the person
		// merely claimed must not satisfy it.
		f := newGateFixture(t, stubAuthPolicy{mode: models.RegistrationModeInviteOnly})

		require.ErrorIs(t, f.gate.AllowNewAccount(ctx, gateRootAdmin, false), ErrRegistrationClosed)
	})
}

// A mode that cannot be read, or an invitation lookup that fails, never opens
// registration and is not reported as "closed" either.
func TestRegistrationGate_FailsClosedOnErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("db down")

	t.Run("unreadable policy", func(t *testing.T) {
		f := newGateFixture(t, stubAuthPolicy{err: boom})

		err := f.gate.AllowNewAccount(ctx, "ada@example.com", true)

		require.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, ErrRegistrationClosed)
	})

	t.Run("invitation lookup failure", func(t *testing.T) {
		f := newGateFixture(t, stubAuthPolicy{mode: models.RegistrationModeInviteOnly})
		f.invitations.On("GetPendingByEmail", ctx, "ada@example.com").Return(nil, boom)

		err := f.gate.AllowNewAccount(ctx, "ada@example.com", true)

		require.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, ErrRegistrationClosed)
	})
}

// The AuthService default admits everyone, and a nil option keeps it.
func TestRegistrationGate_DefaultIsOpen(t *testing.T) {
	logger, _ := logtest.New()
	svc := NewAuthService(nil, nil, staticIDPResolver{}, nil, logger, newStaticAllowlistResolver(logger, nil),
		WithRegistrationGate(nil))

	require.NotNil(t, svc.registrationGate)
	require.NoError(t, svc.registrationGate.AllowNewAccount(context.Background(), "anyone@example.com", false))
}

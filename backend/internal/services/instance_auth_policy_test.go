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
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

func newAuthPolicyService(policies repositories.InstanceAuthPolicyRepository) *InstanceAuthSettingsService {
	return NewInstanceAuthSettingsService(InstanceAuthSettingsDeps{
		Policies: policies, Logger: slog.New(slog.DiscardHandler),
	})
}

func TestValidateInstanceAuthPolicy(t *testing.T) {
	for _, mode := range []string{models.RegistrationModeOpen, models.RegistrationModeInviteOnly} {
		assert.NoError(t, ValidateInstanceAuthPolicy(models.InstanceAuthPolicy{RegistrationMode: mode}), mode)
	}
	for _, mode := range []string{"", "closed", "OPEN", "invite-only", " open"} {
		t.Run("rejects "+mode, func(t *testing.T) {
			err := ValidateInstanceAuthPolicy(models.InstanceAuthPolicy{RegistrationMode: mode})

			require.ErrorIs(t, err, ErrInvalidInstanceAuthPolicy)
			var field *SettingsFieldError
			require.ErrorAs(t, err, &field, "the error names the field at fault")
		})
	}
}

func TestInstanceAuthSettingsService_GetPolicy(t *testing.T) {
	ctx := context.Background()

	t.Run("nothing stored returns the defaults", func(t *testing.T) {
		policies := repo_mocks.NewMockInstanceAuthPolicyRepository(t)
		policies.On("Get", ctx).Return(nil, repositories.ErrInstanceAuthPolicyNotFound)

		policy, err := newAuthPolicyService(policies).GetPolicy(ctx)

		require.NoError(t, err)
		assert.Equal(t, models.DefaultInstanceAuthPolicy(), *policy)
		assert.False(t, policy.IsStored())
	})

	t.Run("a stored policy is returned as stored", func(t *testing.T) {
		stored := &models.InstanceAuthPolicy{
			PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeInviteOnly, Version: 4,
		}
		policies := repo_mocks.NewMockInstanceAuthPolicyRepository(t)
		policies.On("Get", ctx).Return(stored, nil)

		policy, err := newAuthPolicyService(policies).GetPolicy(ctx)

		require.NoError(t, err)
		assert.Same(t, stored, policy)
	})

	t.Run("a read failure is not mistaken for nothing stored", func(t *testing.T) {
		boom := errors.New("connection refused")
		policies := repo_mocks.NewMockInstanceAuthPolicyRepository(t)
		policies.On("Get", ctx).Return(nil, boom)

		_, err := newAuthPolicyService(policies).GetPolicy(ctx)

		require.ErrorIs(t, err, boom)
	})
}

func TestInstanceAuthSettingsService_UpdatePolicy(t *testing.T) {
	ctx := context.Background()
	expected := int64(3)

	t.Run("stores the two settings and returns what was written", func(t *testing.T) {
		policies := repo_mocks.NewMockInstanceAuthPolicyRepository(t)
		policies.On("UpsertAudited", ctx,
			mock.MatchedBy(func(p *models.InstanceAuthPolicy) bool {
				// Only the settings travel; a version or author the caller put on
				// the request must not reach storage.
				return p.PasswordLoginEnabled && p.RegistrationMode == models.RegistrationModeInviteOnly &&
					p.Version == 0 && p.UpdatedBy == nil
			}),
			mock.MatchedBy(func(actor *string) bool { return actor != nil && *actor == "admin-1" }),
			&expected,
		).Run(func(args mock.Arguments) {
			args.Get(1).(*models.InstanceAuthPolicy).Version = 4
		}).Return(nil)
		someoneElse := "someone-else"

		policy, err := newAuthPolicyService(policies).UpdatePolicy(ctx, "admin-1", models.InstanceAuthPolicy{
			PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeInviteOnly,
			Version: 99, UpdatedBy: &someoneElse,
		}, &expected)

		require.NoError(t, err)
		assert.Equal(t, int64(4), policy.Version)
		assert.True(t, policy.PasswordLoginEnabled)
	})

	t.Run("an invalid mode is rejected before storage", func(t *testing.T) {
		policies := repo_mocks.NewMockInstanceAuthPolicyRepository(t)

		_, err := newAuthPolicyService(policies).UpdatePolicy(ctx, "admin-1",
			models.InstanceAuthPolicy{RegistrationMode: "closed"}, nil)

		require.ErrorIs(t, err, ErrInvalidInstanceAuthPolicy)
		policies.AssertNotCalled(t, "UpsertAudited")
	})

	t.Run("a version conflict stays recognisable", func(t *testing.T) {
		policies := repo_mocks.NewMockInstanceAuthPolicyRepository(t)
		policies.On("UpsertAudited", ctx, mock.Anything, mock.Anything, &expected).
			Return(repositories.ErrInstanceSettingsVersionConflict)

		_, err := newAuthPolicyService(policies).UpdatePolicy(ctx, "admin-1",
			models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeOpen}, &expected)

		require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)
	})

	t.Run("no acting user is stored as no author", func(t *testing.T) {
		policies := repo_mocks.NewMockInstanceAuthPolicyRepository(t)
		policies.On("UpsertAudited", ctx, mock.Anything, (*string)(nil), (*int64)(nil)).Return(nil)

		_, err := newAuthPolicyService(policies).UpdatePolicy(ctx, "",
			models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeOpen}, nil)

		require.NoError(t, err)
	})
}

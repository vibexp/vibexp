package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Every method that loads the team's config first must answer
// ErrGitHubAppNotConfigured when there is none — a caller distinguishing "no App
// yet" from a real failure is how the UI decides between offering setup and
// showing an error.
func TestGitHubAppConfigService_NotConfigured(t *testing.T) {
	ctx := context.Background()
	ptr := func(s string) *string { return &s }

	newSvc := func(t *testing.T) *GitHubAppConfigService {
		t.Helper()
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).
			Return(nil, repositories.ErrGitHubAppConfigNotFound)
		return newAppConfigService(t, repo)
	}

	t.Run("update", func(t *testing.T) {
		_, err := newSvc(t).UpdateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID,
			models.UpdateGitHubAppConfigRequest{AppSlug: ptr("x")})
		assert.ErrorIs(t, err, ErrGitHubAppNotConfigured)
	})

	t.Run("delete", func(t *testing.T) {
		err := newSvc(t).DeleteAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
		assert.ErrorIs(t, err, ErrGitHubAppNotConfigured)
	})

	t.Run("rotate webhook token", func(t *testing.T) {
		_, err := newSvc(t).RotateWebhookToken(ctx, testAppConfigTeamID, testAppConfigUserID)
		assert.ErrorIs(t, err, ErrGitHubAppNotConfigured)
	})

	t.Run("rotate webhook secret", func(t *testing.T) {
		_, err := newSvc(t).RotateWebhookSecret(ctx, testAppConfigTeamID, testAppConfigUserID)
		assert.ErrorIs(t, err, ErrGitHubAppNotConfigured)
	})
}

// A transport-level repository failure must NOT be laundered into
// "not configured" — that would tell the UI to offer setup for a team that
// already has an App, and a retry would then 409.
func TestGitHubAppConfigService_RepositoryFailuresAreNotNotConfigured(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("connection reset")

	t.Run("read failure", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(nil, boom)
		svc := newAppConfigService(t, repo)

		_, err := svc.GetAppConfig(ctx, testAppConfigTeamID)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrGitHubAppNotConfigured)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("delete failure", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(storedConfig(t, svc), nil)
		repo.EXPECT().Delete(ctx, testAppConfigTeamID, "cfg-1").Return(boom)

		err := svc.DeleteAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("create failure", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		repo.EXPECT().Create(ctx, mock.Anything).Return(boom)

		_, err := svc.CreateAppConfig(
			ctx, testAppConfigTeamID, testAppConfigUserID, validCreateRequest(t))
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("update failure", func(t *testing.T) {
		repo := mocks.NewMockGitHubAppConfigRepository(t)
		svc := newAppConfigService(t, repo)
		stored := storedConfig(t, svc)
		repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)
		repo.EXPECT().Update(ctx, stored).Return(boom)

		_, err := svc.UpdateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID,
			models.UpdateGitHubAppConfigRequest{})
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("rotation failures", func(t *testing.T) {
		for _, name := range []string{"token", "secret"} {
			t.Run(name, func(t *testing.T) {
				repo := mocks.NewMockGitHubAppConfigRepository(t)
				svc := newAppConfigService(t, repo)
				stored := storedConfig(t, svc)
				repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)
				repo.EXPECT().Update(ctx, stored).Return(boom)

				var err error
				if name == "token" {
					_, err = svc.RotateWebhookToken(ctx, testAppConfigTeamID, testAppConfigUserID)
				} else {
					_, err = svc.RotateWebhookSecret(ctx, testAppConfigTeamID, testAppConfigUserID)
				}
				require.Error(t, err)
				assert.ErrorIs(t, err, boom)
			})
		}
	})
}

// An update that swaps in an unusable key must be rejected before it is stored,
// for the same reason create parses: the alternative is a config that looks
// saved and fails at install time.
func TestGitHubAppConfigService_UpdateRejectsUnusableKey(t *testing.T) {
	ctx := context.Background()
	repo := mocks.NewMockGitHubAppConfigRepository(t)
	svc := newAppConfigService(t, repo)
	stored := storedConfig(t, svc)
	before := stored.PrivateKeyEncrypted
	repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)

	// A truncated paste is the realistic bad input, and truncating a real key
	// keeps the fixture out of the source (no PEM literal for the secret
	// scanners to trip on) while still being PEM-shaped enough to be lifelike.
	garbage := testRSAPEM(t)[:120]
	_, err := svc.UpdateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID,
		models.UpdateGitHubAppConfigRequest{PrivateKey: &garbage})

	require.Error(t, err)
	assert.Equal(t, before, stored.PrivateKeyEncrypted, "the stored key must survive a rejected update")
	// repo.Update is never EXPECTed: mockery fails if the bad key reached the DB.
}

// webhookURL degrades to empty rather than emitting a malformed URL, so a
// misconfigured instance shows "no URL yet" instead of one an admin would paste
// into GitHub and silently never receive deliveries on.
func TestGitHubAppConfigService_WebhookURLDegradesSafely(t *testing.T) {
	ctx := context.Background()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	repo := mocks.NewMockGitHubAppConfigRepository(t)
	svc := NewGitHubAppConfigService(repo, enc, permissiveProviderAuthz{}, nil, "")
	stored := storedConfig(t, svc)
	repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)

	got, err := svc.GetAppConfig(ctx, testAppConfigTeamID)
	require.NoError(t, err)
	assert.Empty(t, got.WebhookURL, "no base URL means no webhook URL, not a relative one")
}

// A trailing slash on the configured base URL must not produce a double slash
// in the webhook path — GitHub would accept the URL and route nowhere useful.
func TestGitHubAppConfigService_WebhookURLTrimsTrailingSlash(t *testing.T) {
	ctx := context.Background()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	repo := mocks.NewMockGitHubAppConfigRepository(t)
	svc := NewGitHubAppConfigService(repo, enc, permissiveProviderAuthz{}, nil, "https://vibexp.example/")
	stored := storedConfig(t, svc)
	repo.EXPECT().GetByTeamID(ctx, testAppConfigTeamID).Return(stored, nil)

	got, err := svc.GetAppConfig(ctx, testAppConfigTeamID)
	require.NoError(t, err)
	assert.Equal(t, "https://vibexp.example/api/v1/webhooks/github/tok-abc", got.WebhookURL)
}

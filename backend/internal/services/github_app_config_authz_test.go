package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// These rows hold a team's GitHub credentials, so every write — and the probe,
// which makes the server act on the team's behalf — is owner/admin only
// (authz.TeamUpdate). A plain member must be refused at the service, not merely
// hidden in the UI.
//
// The repository mocks below deliberately EXPECT nothing: mockery fails the test
// if a denied call still reaches the database, which is what proves the gate
// runs before any work rather than after.
func TestGitHubAppConfigService_MemberIsDenied(t *testing.T) {
	ctx := context.Background()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	newDenied := func(t *testing.T) *GitHubAppConfigService {
		t.Helper()
		return NewGitHubAppConfigService(
			mocks.NewMockGitHubAppConfigRepository(t), enc,
			denyingProviderAuthz{}, "https://vibexp.example",
		)
	}

	t.Run("create", func(t *testing.T) {
		_, err := newDenied(t).CreateAppConfig(
			ctx, testAppConfigTeamID, testAppConfigUserID, validCreateRequest(t))
		assert.ErrorIs(t, err, ErrPermissionDenied)
	})

	t.Run("update", func(t *testing.T) {
		_, err := newDenied(t).UpdateAppConfig(
			ctx, testAppConfigTeamID, testAppConfigUserID,
			models.UpdateGitHubAppConfigRequest{})
		assert.ErrorIs(t, err, ErrPermissionDenied)
	})

	t.Run("delete", func(t *testing.T) {
		err := newDenied(t).DeleteAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
		assert.ErrorIs(t, err, ErrPermissionDenied)
	})

	// Gated like a mutation even though it persists nothing: it makes the server
	// perform an authenticated outbound call using the team's credentials.
	t.Run("validate", func(t *testing.T) {
		_, err := newDenied(t).ValidateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
		assert.ErrorIs(t, err, ErrPermissionDenied)
	})

	t.Run("rotate webhook token", func(t *testing.T) {
		_, err := newDenied(t).RotateWebhookToken(ctx, testAppConfigTeamID, testAppConfigUserID)
		assert.ErrorIs(t, err, ErrPermissionDenied)
	})

	t.Run("rotate webhook secret", func(t *testing.T) {
		_, err := newDenied(t).RotateWebhookSecret(ctx, testAppConfigTeamID, testAppConfigUserID)
		assert.ErrorIs(t, err, ErrPermissionDenied)
	})
}

// TestGitHubAppConfigService_NilAuthzFailsClosed pins that a service constructed
// without an authorization service denies rather than allows. Wiring regresses
// silently, and the failure mode must be "nobody can" rather than "everybody
// can" — mirrors TestProviderService_NilAuthzFailsClosed.
func TestGitHubAppConfigService_NilAuthzFailsClosed(t *testing.T) {
	ctx := context.Background()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	svc := NewGitHubAppConfigService(
		mocks.NewMockGitHubAppConfigRepository(t), enc, nil, "https://vibexp.example",
	)

	_, createErr := svc.CreateAppConfig(
		ctx, testAppConfigTeamID, testAppConfigUserID, validCreateRequest(t))
	assert.True(t, errors.Is(createErr, ErrPermissionDenied), "got: %v", createErr)

	_, validateErr := svc.ValidateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
	assert.True(t, errors.Is(validateErr, ErrPermissionDenied), "got: %v", validateErr)

	deleteErr := svc.DeleteAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID)
	assert.True(t, errors.Is(deleteErr, ErrPermissionDenied), "got: %v", deleteErr)
}

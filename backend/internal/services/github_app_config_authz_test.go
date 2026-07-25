package services

import (
	"context"
	"errors"
	"testing"

	"github.com/vibexp/vibexp/internal/authz"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/mock"

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

// recordingAuthz captures which permission was demanded, so the CHOICE of
// permission is pinned rather than merely "some check happened".
type recordingAuthz struct{ got []authz.Permission }

func (r *recordingAuthz) Can(_ context.Context, _, _ string, p authz.Permission) error {
	r.got = append(r.got, p)
	return nil
}

func (r *recordingAuthz) CanActOnResource(
	_ context.Context, _, _, _ string, _, _ authz.Permission,
) error {
	return nil
}

func (r *recordingAuthz) Authorize(
	_ context.Context, _, _ string, _ authz.Permission,
) (models.TeamMemberRole, error) {
	panic("recordingAuthz: unexpected Authorize call")
}

// TestGitHubAppConfigService_UsesTeamSettingsUpdate pins the permission itself.
//
// A GitHub App registration is team-level CONFIGURATION, not team identity, so
// it belongs to team.settings.update rather than team.update. The two are
// deliberately separate (#489) so configuration and identity can diverge later
// without a breaking rename; today both grant owner+admin, which is exactly why
// a silent swap between them would pass every other test in this file.
func TestGitHubAppConfigService_UsesTeamSettingsUpdate(t *testing.T) {
	ctx := context.Background()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	rec := &recordingAuthz{}
	repo := mocks.NewMockGitHubAppConfigRepository(t)
	svc := NewGitHubAppConfigService(repo, enc, rec, "https://vibexp.example")
	repo.EXPECT().Create(ctx, mock.Anything).Return(nil)

	_, err = svc.CreateAppConfig(
		ctx, testAppConfigTeamID, testAppConfigUserID, validCreateRequest(t))
	require.NoError(t, err)

	require.Len(t, rec.got, 1)
	assert.Equal(t, authz.TeamSettingsUpdate, rec.got[0],
		"an App registration is team configuration, not team identity")
}

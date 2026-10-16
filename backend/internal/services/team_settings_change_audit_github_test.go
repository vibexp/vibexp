package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The GitHub App configuration and installation in the instance audit log
// (#1520). The private key, client secret, webhook secret and webhook token
// are named but never carried.

func newAppConfigAuditService(
	t *testing.T,
) (*GitHubAppConfigService, *mocks.MockGitHubAppConfigRepository, *auditEvents) {
	t.Helper()
	repo := mocks.NewMockGitHubAppConfigRepository(t)
	svc := newAppConfigService(t, repo)
	events := &auditEvents{}
	svc.recorder = events
	return svc, repo, events
}

// pemBody is the key material of a PEM block, without its fixed header.
func pemBody(pem string) string {
	return pem[len("-----BEGIN RSA PRIVATE KEY-----\n"):]
}

func TestGitHubAppConfigAudit_CreateNamesEverySecret(t *testing.T) {
	svc, repo, events := newAppConfigAuditService(t)
	req := validCreateRequest(t)
	req.ClientSecret = "Tz7Rq2Lm9Vx4Kw" // gitleaks:allow - test fixture, not a credential
	var persisted *models.GitHubAppConfig
	repo.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, c *models.GitHubAppConfig) error { persisted = c; c.ID = "cfg-1"; return nil })

	created, err := svc.CreateAppConfig(context.Background(), testAppConfigTeamID, testAppConfigUserID, req)

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testAppConfigTeamID, teamSettingsSurfaceGitHubApp,
		teamSettingsActionCreate)
	assert.Equal(t, []string{"app_id", "app_slug", "client_id", "private_key", "client_secret",
		"webhook_secret", "webhook_token"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"new": "my-app"}, metadata["app_slug"])
	assert.Equal(t, "cfg-1", metadata["app_config_id"])
	assertNoSecretLeak(t, events.only(t), req.ClientSecret, pemBody(req.PrivateKey), created.WebhookSecret,
		persisted.PrivateKeyEncrypted, persisted.ClientSecretEncrypted, persisted.WebhookSecretEncrypted,
		persisted.WebhookToken)
}

func TestGitHubAppConfigAudit_UpdateRotateAndDelete(t *testing.T) {
	svc, repo, events := newAppConfigAuditService(t)
	stored := storedConfig(t, svc)
	repo.EXPECT().GetByTeamID(mock.Anything, testAppConfigTeamID).Return(stored, nil)
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)
	slug := "renamed-app"
	secret := "Wn5Bc8Hj3Fy6Gd" // gitleaks:allow - test fixture, not a credential
	_, err := svc.UpdateAppConfig(context.Background(), testAppConfigTeamID, testAppConfigUserID,
		models.UpdateGitHubAppConfigRequest{AppSlug: &slug, ClientSecret: &secret})
	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testAppConfigTeamID, teamSettingsSurfaceGitHubApp,
		teamSettingsActionUpdate)
	assert.Equal(t, []string{"app_slug", "client_secret"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": "my-app", "new": "renamed-app"}, metadata["app_slug"])
	assertNoSecretLeak(t, events.only(t), secret, stored.ClientSecretEncrypted, stored.PrivateKeyEncrypted)

	svc, repo, events = newAppConfigAuditService(t)
	stored = storedConfig(t, svc)
	repo.EXPECT().GetByTeamID(mock.Anything, testAppConfigTeamID).Return(stored, nil)
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)
	rotated, err := svc.RotateWebhookSecret(context.Background(), testAppConfigTeamID, testAppConfigUserID)
	require.NoError(t, err)
	metadata = requireTeamSettingsEvent(t, events, testAppConfigTeamID, teamSettingsSurfaceGitHubApp,
		teamSettingsActionRotate)
	assert.Equal(t, []string{"webhook_secret"}, changedFields(t, metadata))
	assertNoSecretLeak(t, events.only(t), rotated.WebhookSecret, stored.WebhookSecretEncrypted)

	svc, repo, events = newAppConfigAuditService(t)
	stored = storedConfig(t, svc)
	repo.EXPECT().GetByTeamID(mock.Anything, testAppConfigTeamID).Return(stored, nil)
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)
	_, err = svc.RotateWebhookToken(context.Background(), testAppConfigTeamID, testAppConfigUserID)
	require.NoError(t, err)
	metadata = requireTeamSettingsEvent(t, events, testAppConfigTeamID, teamSettingsSurfaceGitHubApp,
		teamSettingsActionRotate)
	assert.Equal(t, []string{"webhook_token"}, changedFields(t, metadata))
	assertNoSecretLeak(t, events.only(t), stored.WebhookToken)

	svc, repo, events = newAppConfigAuditService(t)
	stored = storedConfig(t, svc)
	repo.EXPECT().GetByTeamID(mock.Anything, testAppConfigTeamID).Return(stored, nil)
	repo.EXPECT().Delete(mock.Anything, testAppConfigTeamID, stored.ID).Return(nil)
	require.NoError(t, svc.DeleteAppConfig(context.Background(), testAppConfigTeamID, testAppConfigUserID))
	metadata = requireTeamSettingsEvent(t, events, testAppConfigTeamID, teamSettingsSurfaceGitHubApp,
		teamSettingsActionDelete)
	assert.Equal(t, map[string]any{"old": "123456"}, metadata["app_id"])
	assertNoSecretLeak(t, events.only(t), stored.PrivateKeyEncrypted, stored.ClientSecretEncrypted,
		stored.WebhookSecretEncrypted, stored.WebhookToken)
}

func TestGitHubAppConfigAudit_NothingOnFailure(t *testing.T) {
	svc, repo, events := newAppConfigAuditService(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errSettingsStore)
	repo.EXPECT().GetByTeamID(mock.Anything, testAppConfigTeamID).Return(storedConfig(t, svc), nil)
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(errSettingsStore)
	repo.EXPECT().Delete(mock.Anything, testAppConfigTeamID, mock.Anything).Return(errSettingsStore)

	ctx := context.Background()
	_, err := svc.CreateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID, validCreateRequest(t))
	require.Error(t, err)
	slug := "x"
	_, err = svc.UpdateAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID,
		models.UpdateGitHubAppConfigRequest{AppSlug: &slug})
	require.Error(t, err)
	_, err = svc.RotateWebhookToken(ctx, testAppConfigTeamID, testAppConfigUserID)
	require.Error(t, err)
	require.Error(t, svc.DeleteAppConfig(ctx, testAppConfigTeamID, testAppConfigUserID))

	assert.Empty(t, events.got)
}

func TestGitHubAppAudit_ConnectRecordsTheInstallation(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)
	const installationID = int64(12345)
	githubClient.On("GetInstallation", mock.Anything, installationID).Return(sampleInstallationInfo(), nil)
	installationRepo.On("GetByInstallationID", mock.Anything, installationID).
		Return(nil, repositories.ErrGitHubInstallationNotFound)
	installationRepo.On("GetByTeamID", mock.Anything, settingsAuditTeam).
		Return(nil, repositories.ErrGitHubInstallationNotFound)
	installationRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	eventManager.On("Publish", mock.Anything, mock.Anything).Return(nil)
	svc := newCallbackTestService(installationRepo, githubClient, eventManager).(*GitHubAppService)
	events := &auditEvents{}
	svc.recorder = events

	_, err := svc.HandleInstallationCallback(
		context.Background(), settingsAuditActor, settingsAuditTeam, installationID, testInstallCode)

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceGitHubApp,
		teamSettingsActionConnect)
	assert.Equal(t, map[string]any{"new": "myorg"}, metadata["account_login"])
	assert.Equal(t, map[string]any{"new": installationID}, metadata["installation_id"])
	assert.Equal(t, false, metadata["reconnected"])
}

func TestGitHubAppAudit_DisconnectRecordsTheRemovedInstallation(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	eventManager := new(MockEventPublisher)
	installationRepo.On("GetByTeamID", mock.Anything, settingsAuditTeam).
		Return(&models.GitHubInstallation{InstallationID: 777, AccountLogin: "myorg"}, nil)
	installationRepo.On("Delete", mock.Anything, settingsAuditTeam).Return(nil)
	eventManager.On("Publish", mock.Anything, mock.Anything).Return(nil)
	svc := newCallbackTestServiceWithAuthz(installationRepo, new(MockGitHubAppClient), eventManager,
		allowAllAuthz{}).(*GitHubAppService)
	events := &auditEvents{}
	svc.recorder = events

	require.NoError(t, svc.DisconnectInstallation(context.Background(), settingsAuditActor, settingsAuditTeam))

	metadata := requireTeamSettingsEvent(t, events, settingsAuditTeam, teamSettingsSurfaceGitHubApp,
		teamSettingsActionDisconnect)
	assert.Equal(t, map[string]any{"old": int64(777)}, metadata["installation_id"])
}

func TestGitHubAppAudit_NothingOnFailureOrDenial(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	installationRepo.On("GetByTeamID", mock.Anything, settingsAuditTeam).
		Return(&models.GitHubInstallation{InstallationID: 777}, nil)
	installationRepo.On("Delete", mock.Anything, settingsAuditTeam).Return(errSettingsStore)
	svc := newCallbackTestServiceWithAuthz(installationRepo, new(MockGitHubAppClient), new(MockEventPublisher),
		allowAllAuthz{}).(*GitHubAppService)
	events := &auditEvents{}
	svc.recorder = events
	require.Error(t, svc.DisconnectInstallation(context.Background(), settingsAuditActor, settingsAuditTeam))

	denied := newCallbackTestServiceWithAuthz(new(MockGitHubInstallationRepository), new(MockGitHubAppClient),
		new(MockEventPublisher), denyAllAuthz{}).(*GitHubAppService)
	denied.recorder = events
	_, err := denied.HandleInstallationCallback(
		context.Background(), settingsAuditActor, settingsAuditTeam, 1, testInstallCode)
	require.ErrorIs(t, err, ErrPermissionDenied)

	assert.Empty(t, events.got)
}

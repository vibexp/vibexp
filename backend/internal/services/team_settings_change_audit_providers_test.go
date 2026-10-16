package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// The email, model and embedding providers in the instance audit log (#1520).
// Each holds a secret, which an event names but never carries.

// Distinctive, so a leak check cannot match an ordinary word of the event.
const (
	leakEmailSecret = "Zq9vKx7Lm2WpSmtpPw"  // gitleaks:allow - test fixture, not a credential
	leakModelKey    = "sk-Hv4Rt8Yb2Nc6Mx0Q" // gitleaks:allow - test fixture, not a credential
	leakEmbedKey    = "sk-Jd3Kf7Lg1Pq5Ws9E" // gitleaks:allow - test fixture, not a credential
)

func emailAuditRequest() models.UpsertTeamEmailProviderRequest {
	req := validSMTPRequest()
	secret := leakEmailSecret
	req.Secret = &secret
	return req
}

func TestTeamEmailProviderAudit_FirstUpsertNamesTheSecretWithoutItsValue(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t), permissiveProviderAuthz{})
	events := &auditEvents{}
	svc.recorder = events
	repo.EXPECT().GetByTeamID(mock.Anything, testProviderTeamID).Return(nil, repositories.ErrTeamEmailProviderNotFound)
	var stored *models.TeamEmailProvider
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).
		Run(func(_ context.Context, p *models.TeamEmailProvider) { stored = p }).Return(nil)

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, emailAuditRequest())

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceEmailProvider,
		teamSettingsActionUpdate)
	assert.Contains(t, changedFields(t, metadata), "secret")
	assert.NotContains(t, metadata, "secret")
	assert.Equal(t, map[string]any{"new": "localhost"}, metadata["host"])
	assert.Equal(t, map[string]any{"new": "team@example.com"}, metadata["from_address"])
	assertNoSecretLeak(t, events.only(t), leakEmailSecret, stored.SecretEncrypted)
}

func TestTeamEmailProviderAudit_UpdateKeepingTheSecretRecordsOnlyTheChange(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t), permissiveProviderAuthz{})
	events := &auditEvents{}
	svc.recorder = events
	existing := &models.TeamEmailProvider{
		TeamID: testProviderTeamID, ProviderType: EmailProviderTypeSMTP,
		Settings:        []byte(`{"host":"localhost","port":"587","username":"mailer@example.com"}`),
		SecretEncrypted: "stored-ciphertext-Ab12Cd34", FromAddress: "old@example.com",
	}
	repo.EXPECT().GetByTeamID(mock.Anything, testProviderTeamID).Return(existing, nil)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)
	req := emailAuditRequest()
	req.Secret = nil

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, req)

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceEmailProvider,
		teamSettingsActionUpdate)
	assert.Equal(t, []string{"from_address"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": "old@example.com", "new": "team@example.com"}, metadata["from_address"])
	assertNoSecretLeak(t, events.only(t), existing.SecretEncrypted)
}

func TestTeamEmailProviderAudit_DeleteRecordsTheRemovedProvider(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t), permissiveProviderAuthz{})
	events := &auditEvents{}
	svc.recorder = events
	repo.EXPECT().GetByTeamID(mock.Anything, testProviderTeamID).Return(&models.TeamEmailProvider{
		ProviderType: EmailProviderTypeSendGrid, FromAddress: "team@example.com",
		SecretEncrypted: "stored-ciphertext-Ef56Gh78",
	}, nil)
	repo.EXPECT().Delete(mock.Anything, testProviderTeamID).Return(nil)

	require.NoError(t, svc.Delete(context.Background(), testProviderUserID, testProviderTeamID))

	metadata := requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceEmailProvider,
		teamSettingsActionDelete)
	assert.Equal(t, map[string]any{"old": EmailProviderTypeSendGrid}, metadata["provider_type"])
	assertNoSecretLeak(t, events.only(t), "stored-ciphertext-Ef56Gh78")
}

func TestTeamEmailProviderAudit_NothingOnFailureOrDenial(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	svc := newTestTeamEmailProviderService(t, repo, repomocks.NewMockUserRepository(t), permissiveProviderAuthz{})
	events := &auditEvents{}
	svc.recorder = events
	repo.EXPECT().GetByTeamID(mock.Anything, testProviderTeamID).Return(nil, repositories.ErrTeamEmailProviderNotFound)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(errSettingsStore)
	repo.EXPECT().Delete(mock.Anything, testProviderTeamID).Return(errSettingsStore)

	_, err := svc.Upsert(context.Background(), testProviderUserID, testProviderTeamID, emailAuditRequest())
	require.Error(t, err)
	require.Error(t, svc.Delete(context.Background(), testProviderUserID, testProviderTeamID))

	denied := newTestTeamEmailProviderService(t, repomocks.NewMockTeamEmailProviderRepository(t),
		repomocks.NewMockUserRepository(t), denyingProviderAuthz{})
	denied.recorder = events
	require.ErrorIs(t, denied.Delete(context.Background(), testProviderUserID, testProviderTeamID), ErrPermissionDenied)

	assert.Empty(t, events.got)
}

func newModelAuditService(t *testing.T) (*ModelProviderService, *repomocks.MockModelProviderRepository, *auditEvents) {
	t.Helper()
	svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, &recordingAuditService{})
	events := &auditEvents{}
	svc.recorder = events
	return svc, repo, events
}

func TestModelProviderAudit_CreateUpdateDeleteNeverCarryTheKey(t *testing.T) {
	svc, repo, events := newModelAuditService(t)
	var created *models.ModelProvider
	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, p *models.ModelProvider) { p.ID = "mp-1"; created = p }).Return(nil)
	key := leakModelKey
	_, err := svc.CreateModelProvider(context.Background(), testProviderTeamID, testProviderUserID,
		models.CreateModelProviderRequest{Name: "Main", ProviderType: ProviderTypeOpenAICompatible,
			Model: "gpt-4o-mini", APIKey: &key})
	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceModelProvider,
		teamSettingsActionCreate)
	assert.Equal(t, "mp-1", metadata["provider_id"])
	assert.Contains(t, changedFields(t, metadata), "api_key")
	assert.NotContains(t, changedFields(t, metadata), "configuration", "the default empty configuration")
	assertNoSecretLeak(t, events.only(t), leakModelKey, *created.APIKeyEncrypted)

	svc, repo, events = newModelAuditService(t)
	stored := *created
	repo.EXPECT().GetByID(mock.Anything, testProviderTeamID, "mp-1").Return(&stored, nil)
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)
	newModel := "gpt-4o"
	rotated := leakModelKey + "-rotated"
	_, err = svc.UpdateModelProvider(context.Background(), testProviderTeamID, testProviderUserID, "mp-1",
		models.UpdateModelProviderRequest{Model: &newModel, APIKey: &rotated,
			Configuration: map[string]interface{}{"temperature": 0.2}})
	require.NoError(t, err)
	metadata = requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceModelProvider,
		teamSettingsActionUpdate)
	assert.Equal(t, []string{"model", "api_key", "configuration"}, changedFields(t, metadata))
	assert.Equal(t, map[string]any{"old": "gpt-4o-mini", "new": "gpt-4o"}, metadata["model"])
	assertNoSecretLeak(t, events.only(t), rotated, *stored.APIKeyEncrypted, *created.APIKeyEncrypted)

	svc, repo, events = newModelAuditService(t)
	repo.EXPECT().GetByID(mock.Anything, testProviderTeamID, "mp-1").Return(created, nil)
	repo.EXPECT().Count(mock.Anything, testProviderTeamID).Return(2, nil)
	repo.EXPECT().Delete(mock.Anything, testProviderTeamID, "mp-1").Return(nil)
	require.NoError(t, svc.DeleteModelProvider(context.Background(), testProviderTeamID, testProviderUserID, "mp-1"))
	metadata = requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceModelProvider,
		teamSettingsActionDelete)
	assert.Equal(t, map[string]any{"old": "Main"}, metadata["name"])
	assertNoSecretLeak(t, events.only(t), *created.APIKeyEncrypted)
}

func TestModelProviderAudit_NothingOnFailureOrDenial(t *testing.T) {
	svc, repo, events := newModelAuditService(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errSettingsStore)
	repo.EXPECT().GetByID(mock.Anything, testProviderTeamID, "mp-1").Return(&models.ModelProvider{ID: "mp-1"}, nil)
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(errSettingsStore)
	repo.EXPECT().Count(mock.Anything, testProviderTeamID).Return(1, nil)

	ctx := context.Background()
	_, err := svc.CreateModelProvider(ctx, testProviderTeamID, testProviderUserID,
		models.CreateModelProviderRequest{Name: "Main", ProviderType: ProviderTypeOpenAICompatible, Model: "m"})
	require.Error(t, err)
	_, err = svc.UpdateModelProvider(ctx, testProviderTeamID, testProviderUserID, "mp-1",
		models.UpdateModelProviderRequest{})
	require.Error(t, err)
	require.ErrorIs(t, svc.DeleteModelProvider(ctx, testProviderTeamID, testProviderUserID, "mp-1"),
		ErrLastModelProviderDelete)

	denied, _ := newCopyProviderService(t, denyingProviderAuthz{}, &recordingAuditService{})
	denied.recorder = events
	require.ErrorIs(t, denied.DeleteModelProvider(ctx, testProviderTeamID, testProviderUserID, "mp-1"),
		ErrPermissionDenied)

	assert.Empty(t, events.got)
}

func TestModelProviderAudit_CopyRecordsTheSourceAndNamesTheKey(t *testing.T) {
	svc, repo, events := newModelAuditService(t)
	source := sourceProviderRow()
	repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).Return(source, nil)
	repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(nil, nil)
	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, p *models.ModelProvider) { p.ID = "dst-provider-1" }).Return(nil)

	_, err := svc.CopyFromTeam(context.Background(), copyProviderParams())

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceModelProvider,
		teamSettingsActionCopy)
	assert.Equal(t, testCopyProviderSourceTeamID, metadata["source_team_id"])
	assert.Equal(t, testCopyProviderID, metadata["source_provider_id"])
	assert.Equal(t, "dst-provider-1", metadata["provider_id"])
	assert.Contains(t, changedFields(t, metadata), "api_key")
	assertNoSecretLeak(t, events.only(t), *source.APIKeyEncrypted)
}

func TestModelProviderAudit_AFailedCopyRecordsNothing(t *testing.T) {
	svc, repo := newCopyProviderService(t, permissiveProviderAuthz{}, &recordingAuditService{err: errSettingsStore})
	events := &auditEvents{}
	svc.recorder = events
	repo.EXPECT().GetByID(mock.Anything, testCopyProviderSourceTeamID, testCopyProviderID).
		Return(sourceProviderRow(), nil)
	repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(nil, nil)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

	_, err := svc.CopyFromTeam(context.Background(), copyProviderParams())

	require.Error(t, err, "the fail-closed team history still fails the copy")
	assert.Empty(t, events.got)
}

func newEmbeddingAuditService(
	t *testing.T,
) (*EmbeddingProviderService, *repomocks.MockEmbeddingProviderRepository, *auditEvents) {
	t.Helper()
	svc, repo, _ := newCopyEmbeddingService(t, permissiveProviderAuthz{}, &recordingAuditService{})
	events := &auditEvents{}
	svc.recorder = events
	return svc, repo, events
}

func TestEmbeddingProviderAudit_CreateUpdateDeleteNeverCarryTheKey(t *testing.T) {
	svc, repo, events := newEmbeddingAuditService(t)
	var created *models.EmbeddingProvider
	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, p *models.EmbeddingProvider) { p.ID = "ep-1"; created = p }).Return(nil)
	key := leakEmbedKey
	_, err := svc.CreateEmbeddingProvider(context.Background(), testProviderTeamID, testProviderUserID,
		models.CreateEmbeddingProviderRequest{Name: "Embed", ProviderType: ProviderTypeOpenAICompatible,
			Model: "text-embedding-3-small", APIKey: &key})
	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceEmbeddingProvider,
		teamSettingsActionCreate)
	assert.Contains(t, changedFields(t, metadata), "api_key")
	assert.Contains(t, metadata, "chunk_size")
	assertNoSecretLeak(t, events.only(t), leakEmbedKey, *created.APIKeyEncrypted)

	svc, repo, events = newEmbeddingAuditService(t)
	stored := *created
	repo.EXPECT().GetByID(mock.Anything, testProviderTeamID, "ep-1").Return(&stored, nil)
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)
	chunk := created.ChunkSize + 100
	_, err = svc.UpdateEmbeddingProvider(context.Background(), testProviderTeamID, testProviderUserID, "ep-1",
		models.UpdateEmbeddingProviderRequest{ChunkSize: &chunk})
	require.NoError(t, err)
	metadata = requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceEmbeddingProvider,
		teamSettingsActionUpdate)
	assert.Equal(t, []string{"chunk_size"}, changedFields(t, metadata))
	assertNoSecretLeak(t, events.only(t), *created.APIKeyEncrypted)

	svc, repo, events = newEmbeddingAuditService(t)
	repo.EXPECT().GetByID(mock.Anything, testProviderTeamID, "ep-1").Return(created, nil)
	repo.EXPECT().Count(mock.Anything, testProviderTeamID).Return(2, nil)
	repo.EXPECT().Delete(mock.Anything, testProviderTeamID, "ep-1").Return(nil)
	require.NoError(t, svc.DeleteEmbeddingProvider(context.Background(), testProviderTeamID, testProviderUserID, "ep-1"))
	requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceEmbeddingProvider,
		teamSettingsActionDelete)
	assertNoSecretLeak(t, events.only(t), *created.APIKeyEncrypted)
}

func TestEmbeddingProviderAudit_NothingOnFailure(t *testing.T) {
	svc, repo, events := newEmbeddingAuditService(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errSettingsStore)
	repo.EXPECT().GetByID(mock.Anything, testProviderTeamID, "ep-1").Return(&models.EmbeddingProvider{ID: "ep-1"}, nil)
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(errSettingsStore)
	repo.EXPECT().Count(mock.Anything, testProviderTeamID).Return(1, nil)

	ctx := context.Background()
	_, err := svc.CreateEmbeddingProvider(ctx, testProviderTeamID, testProviderUserID,
		models.CreateEmbeddingProviderRequest{Name: "E", ProviderType: ProviderTypeOpenAICompatible, Model: "m"})
	require.Error(t, err)
	_, err = svc.UpdateEmbeddingProvider(ctx, testProviderTeamID, testProviderUserID, "ep-1",
		models.UpdateEmbeddingProviderRequest{})
	require.Error(t, err)
	require.Error(t, svc.DeleteEmbeddingProvider(ctx, testProviderTeamID, testProviderUserID, "ep-1"))

	assert.Empty(t, events.got)
}

func TestEmbeddingProviderAudit_CopyRecordsTheSourceAndActivation(t *testing.T) {
	svc, repo, events := newEmbeddingAuditService(t)
	source := sourceEmbeddingProviderRow()
	repo.EXPECT().GetByID(mock.Anything, testEmbedCopySourceTeamID, testEmbedCopyProviderID).Return(source, nil)
	repo.EXPECT().ListNames(mock.Anything, testProviderTeamID).Return(nil, nil)
	repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).
		Return(destinationProviderRow("dst-default", "text-embedding-3-small", true), nil)
	expectCreateAssigningID(repo)

	_, err := svc.CopyFromTeam(context.Background(), copyEmbeddingParams())

	require.NoError(t, err)
	metadata := requireTeamSettingsEvent(t, events, testProviderTeamID, teamSettingsSurfaceEmbeddingProvider,
		teamSettingsActionCopy)
	assert.Equal(t, testEmbedCopySourceTeamID, metadata["source_team_id"])
	assert.Equal(t, false, metadata["becomes_active"])
	assert.Contains(t, changedFields(t, metadata), "api_key")
	assertNoSecretLeak(t, events.only(t), *source.APIKeyEncrypted)
}

package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// instanceLeakSentinel is the plaintext credential used across these tests; no
// serialized view or audit entry may ever contain it.
const instanceLeakSentinel = "instance-leak-sentinel-7f3a"

type instanceProviderFixture struct {
	repo     *repomocks.MockInstanceEmailProviderRepository
	audit    *repomocks.MockInstanceSettingsAuditRepository
	userRepo *repomocks.MockUserRepository
	enc      EncryptionServiceInterface
	svc      *InstanceEmailProviderService
}

func newInstanceProviderFixture(t *testing.T) *instanceProviderFixture {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	f := &instanceProviderFixture{
		repo:     repomocks.NewMockInstanceEmailProviderRepository(t),
		audit:    repomocks.NewMockInstanceSettingsAuditRepository(t),
		userRepo: repomocks.NewMockUserRepository(t),
		enc:      enc,
	}
	f.svc = NewInstanceEmailProviderService(f.repo, f.audit, f.userRepo, f.enc, slog.New(slog.DiscardHandler))
	return f
}

// storedRow is an SMTP row whose secret is encrypted with the test key.
func (f *instanceProviderFixture) storedRow(t *testing.T) *models.InstanceEmailProvider {
	t.Helper()
	ciphertext, err := f.enc.Encrypt(instanceLeakSentinel)
	require.NoError(t, err)
	return &models.InstanceEmailProvider{
		ProviderType:    EmailProviderTypeSMTP,
		Settings:        json.RawMessage(`{"host":"127.0.0.1","port":"1","username":"relay"}`),
		SecretEncrypted: &ciphertext,
		FromAddress:     "noreply@instance.test",
	}
}

func validInstanceSMTPRequest() models.UpsertInstanceEmailProviderRequest {
	secret := instanceLeakSentinel
	return models.UpsertInstanceEmailProviderRequest{
		UpsertTeamEmailProviderRequest: models.UpsertTeamEmailProviderRequest{
			ProviderType: EmailProviderTypeSMTP,
			Settings: models.TeamEmailProviderSettings{
				// Loopback is legitimate for the operator (epic decision 5).
				SMTP: &models.SMTPProviderSettings{Host: "127.0.0.1", Port: "1", Username: "relay"},
			},
			Secret:      &secret,
			FromAddress: "noreply@instance.test",
		},
	}
}

// captureAppend records the one audit entry a write appends.
func (f *instanceProviderFixture) captureAppend() *models.InstanceSettingsAuditEntry {
	captured := &models.InstanceSettingsAuditEntry{}
	f.audit.On("Append", mock.Anything, mock.AnythingOfType("*models.InstanceSettingsAuditEntry")).
		Run(func(args mock.Arguments) {
			*captured = *args.Get(1).(*models.InstanceSettingsAuditEntry)
		}).Return(nil).Once()
	return captured
}

func snapshotMap(t *testing.T, doc json.RawMessage) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(doc, &out))
	return out
}

// --- Get ---------------------------------------------------------------------

func TestInstanceEmailProvider_Get_Unconfigured(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)

	effective, err := f.svc.Get(context.Background())

	require.NoError(t, err, "an unconfigured instance is a state, not a failure")
	assert.False(t, effective.Configured)
	assert.Nil(t, effective.ProviderType)
}

func TestInstanceEmailProvider_Get_NeverReturnsTheSecret(t *testing.T) {
	f := newInstanceProviderFixture(t)
	row := f.storedRow(t)
	f.repo.On("Get", mock.Anything).Return(row, nil)

	effective, err := f.svc.Get(context.Background())

	require.NoError(t, err)
	assert.True(t, effective.Configured)
	assert.True(t, effective.HasCredential)
	require.NotNil(t, effective.Settings)
	require.NotNil(t, effective.Settings.SMTP, "settings are the per-type union, like the request")
	assert.Equal(t, "127.0.0.1", effective.Settings.SMTP.Host)

	encoded, err := json.Marshal(effective)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), instanceLeakSentinel)
	assert.NotContains(t, string(encoded), *row.SecretEncrypted)
}

func TestInstanceEmailProvider_Get_ReadFailureIsAnError(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, errors.New("db down"))

	_, err := f.svc.Get(context.Background())

	require.Error(t, err)
}

// --- Upsert ------------------------------------------------------------------

func TestInstanceEmailProvider_Upsert_CreateRequiresASecret(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)

	req := validInstanceSMTPRequest()
	req.Secret = nil

	_, err := f.svc.Upsert(context.Background(), testProviderUserID, req)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTeamEmailProviderValidation)
	assert.Contains(t, err.Error(), "secret")
	f.repo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	f.audit.AssertNotCalled(t, "Append", mock.Anything, mock.Anything)
}

func TestInstanceEmailProvider_Upsert_CreateEncryptsAndAuditsOnce(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)
	var stored *models.InstanceEmailProvider
	f.repo.On("Upsert", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		stored = args.Get(1).(*models.InstanceEmailProvider)
	}).Return(nil).Once()
	entry := f.captureAppend()

	contact := " support@instance.test "
	req := validInstanceSMTPRequest()
	req.ContactRecipientAddress = &contact

	effective, err := f.svc.Upsert(context.Background(), testProviderUserID, req)

	require.NoError(t, err)
	assert.True(t, effective.Configured)
	assert.True(t, effective.HasCredential)

	require.NotNil(t, stored)
	require.NotNil(t, stored.SecretEncrypted)
	assert.NotEqual(t, instanceLeakSentinel, *stored.SecretEncrypted, "the secret is stored encrypted")
	plaintext, err := f.enc.Decrypt(*stored.SecretEncrypted)
	require.NoError(t, err)
	assert.Equal(t, instanceLeakSentinel, plaintext)
	assert.Equal(t, "support@instance.test", *stored.ContactRecipientAddress)
	require.NotNil(t, stored.UpdatedBy)
	assert.Equal(t, testProviderUserID, *stored.UpdatedBy)

	assert.Equal(t, models.InstanceSettingEmailProvider, entry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionUpsert, entry.Action)
	require.NotNil(t, entry.ActorUserID)
	assert.Equal(t, testProviderUserID, *entry.ActorUserID)
	assert.Nil(t, entry.Before, "a create has no before snapshot")
	after := snapshotMap(t, entry.After)
	assert.Equal(t, models.InstanceSettingsAuditSecretChanged, after["secret"])
	assert.Equal(t, true, after["has_credential"])
	assert.Equal(t, "noreply@instance.test", after["from_address"])

	encoded, err := json.Marshal(entry)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), instanceLeakSentinel)
	assert.NotContains(t, string(encoded), *stored.SecretEncrypted)
}

func TestInstanceEmailProvider_Upsert_UpdateWithoutSecretKeepsTheCiphertext(t *testing.T) {
	f := newInstanceProviderFixture(t)
	existing := f.storedRow(t)
	f.repo.On("Get", mock.Anything).Return(existing, nil)
	var stored *models.InstanceEmailProvider
	f.repo.On("Upsert", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		stored = args.Get(1).(*models.InstanceEmailProvider)
	}).Return(nil).Once()
	entry := f.captureAppend()

	req := validInstanceSMTPRequest()
	req.Secret = nil
	req.FromAddress = "mail@instance.test"

	_, err := f.svc.Upsert(context.Background(), testProviderUserID, req)

	require.NoError(t, err)
	require.NotNil(t, stored.SecretEncrypted)
	assert.Equal(t, *existing.SecretEncrypted, *stored.SecretEncrypted)

	before := snapshotMap(t, entry.Before)
	after := snapshotMap(t, entry.After)
	assert.Equal(t, "noreply@instance.test", before["from_address"])
	assert.NotContains(t, before, "secret")
	assert.Equal(t, "mail@instance.test", after["from_address"])
	assert.Equal(t, models.InstanceSettingsAuditSecretUnchanged, after["secret"])
}

func TestInstanceEmailProvider_Upsert_ValidatesInstanceOnlyFields(t *testing.T) {
	tests := []struct {
		name      string
		contact   string
		privacy   string
		wantField string
	}{
		{name: "contact is not an address", contact: "not-an-address", wantField: "contact_recipient_address"},
		{name: "contact carries a display name", contact: "Ops <ops@instance.test>",
			wantField: "contact_recipient_address"},
		{name: "privacy url is relative", privacy: "/privacy", wantField: "privacy_policy_url"},
		{name: "privacy url is not http", privacy: "ftp://instance.test/privacy", wantField: "privacy_policy_url"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newInstanceProviderFixture(t)
			f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)

			req := validInstanceSMTPRequest()
			if tc.contact != "" {
				req.ContactRecipientAddress = &tc.contact
			}
			if tc.privacy != "" {
				req.PrivacyPolicyURL = &tc.privacy
			}

			_, err := f.svc.Upsert(context.Background(), testProviderUserID, req)

			var verr *TeamEmailProviderValidationError
			require.ErrorAs(t, err, &verr)
			require.Len(t, verr.Fields, 1)
			assert.Equal(t, tc.wantField, verr.Fields[0].Field)
		})
	}
}

// Shared-rule and instance-only field errors are reported together.
func TestInstanceEmailProvider_Upsert_ReportsAllFieldErrorsTogether(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)

	privacy := "not a url"
	req := validInstanceSMTPRequest()
	req.FromAddress = ""
	req.PrivacyPolicyURL = &privacy

	_, err := f.svc.Upsert(context.Background(), testProviderUserID, req)

	var verr *TeamEmailProviderValidationError
	require.ErrorAs(t, err, &verr)
	fields := make([]string, 0, len(verr.Fields))
	for _, field := range verr.Fields {
		fields = append(fields, field.Field)
	}
	assert.ElementsMatch(t, []string{"from_address", "privacy_policy_url"}, fields)
}

func TestInstanceEmailProvider_Upsert_AcceptsAValidPrivacyURL(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)
	f.repo.On("Upsert", mock.Anything, mock.Anything).Return(nil).Once()
	f.captureAppend()

	privacy := "https://instance.test/privacy"
	req := validInstanceSMTPRequest()
	req.PrivacyPolicyURL = &privacy

	effective, err := f.svc.Upsert(context.Background(), testProviderUserID, req)

	require.NoError(t, err)
	assert.Equal(t, privacy, *effective.PrivacyPolicyURL)
}

// The row is written before the audit entry; an append failure is surfaced,
// never swallowed.
func TestInstanceEmailProvider_Upsert_AuditFailureIsReturned(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)
	f.repo.On("Upsert", mock.Anything, mock.Anything).Return(nil).Once()
	f.audit.On("Append", mock.Anything, mock.Anything).Return(errors.New("audit down")).Once()

	_, err := f.svc.Upsert(context.Background(), testProviderUserID, validInstanceSMTPRequest())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to audit")
}

func TestInstanceEmailProvider_Upsert_NilEncryptionFailsClosed(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.svc.enc = nil
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)

	_, err := f.svc.Upsert(context.Background(), testProviderUserID, validInstanceSMTPRequest())

	require.ErrorIs(t, err, ErrEncryptionUnavailable)
	f.repo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
}

// Epic #1185 decision 5, pinned against the team path: the same loopback SMTP
// host and localhost Mailgun base URL the team service rejects are accepted for
// the instance, whose admin is the operator.
func TestInstanceEmailProvider_NoSSRFGuard_UnlikeTheTeamPath(t *testing.T) {
	mailgunSecret := instanceLeakSentinel
	mailgun := models.UpsertInstanceEmailProviderRequest{
		UpsertTeamEmailProviderRequest: models.UpsertTeamEmailProviderRequest{
			ProviderType: EmailProviderTypeMailgun,
			Settings: models.TeamEmailProviderSettings{
				Mailgun: &models.MailgunProviderSettings{
					Domain: "mg.instance.test", BaseURL: "http://localhost:8025/v3",
				},
			},
			Secret:      &mailgunSecret,
			FromAddress: "noreply@instance.test",
		},
	}

	for name, req := range map[string]models.UpsertInstanceEmailProviderRequest{
		"smtp 127.0.0.1":    validInstanceSMTPRequest(),
		"mailgun localhost": mailgun,
	} {
		t.Run(name, func(t *testing.T) {
			f := newInstanceProviderFixture(t)
			f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)
			f.repo.On("Upsert", mock.Anything, mock.Anything).Return(nil).Once()
			f.captureAppend()

			_, err := f.svc.Upsert(context.Background(), testProviderUserID, req)
			require.NoError(t, err, "the instance path has no SSRF guard")

			teamRepo := repomocks.NewMockTeamEmailProviderRepository(t)
			teamRepo.On("GetByTeamID", mock.Anything, testProviderTeamID).
				Return(nil, repositories.ErrTeamEmailProviderNotFound)
			team := newTestTeamEmailProviderService(t, teamRepo, repomocks.NewMockUserRepository(t),
				permissiveProviderAuthz{})
			team.guard = defaultSSRFGuard

			_, teamErr := team.Upsert(context.Background(), testProviderUserID, testProviderTeamID,
				req.UpsertTeamEmailProviderRequest)
			require.ErrorIs(t, teamErr, ErrTeamEmailProviderValidation, "the team path keeps the guard")
		})
	}
}

// --- Delete ------------------------------------------------------------------

func TestInstanceEmailProvider_Delete_AuditsOnce(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)
	f.repo.On("Delete", mock.Anything).Return(nil).Once()
	entry := f.captureAppend()

	require.NoError(t, f.svc.Delete(context.Background(), testProviderUserID))

	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entry.Action)
	assert.Equal(t, models.InstanceSettingEmailProvider, entry.Setting)
	assert.Nil(t, entry.After, "a delete has no after snapshot")
	before := snapshotMap(t, entry.Before)
	assert.Equal(t, true, before["has_credential"])
	encoded, err := json.Marshal(entry)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), instanceLeakSentinel)
}

func TestInstanceEmailProvider_Delete_NothingStoredIsANoOp(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)

	require.NoError(t, f.svc.Delete(context.Background(), testProviderUserID))
	f.repo.AssertNotCalled(t, "Delete", mock.Anything)
	f.audit.AssertNotCalled(t, "Append", mock.Anything, mock.Anything)
}

func TestInstanceEmailProvider_Delete_ConcurrentDeleteWritesNoEntry(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)
	f.repo.On("Delete", mock.Anything).Return(repositories.ErrInstanceEmailProviderNotFound).Once()

	require.NoError(t, f.svc.Delete(context.Background(), testProviderUserID))
	f.audit.AssertNotCalled(t, "Append", mock.Anything, mock.Anything)
}

func TestInstanceEmailProvider_Delete_RepositoryFailureIsReturned(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)
	f.repo.On("Delete", mock.Anything).Return(errors.New("db down")).Once()

	require.Error(t, f.svc.Delete(context.Background(), testProviderUserID))
	f.audit.AssertNotCalled(t, "Append", mock.Anything, mock.Anything)
}

// --- Test send ---------------------------------------------------------------

// actingAdminEmail is the acting admin's account email, the default recipient.
const actingAdminEmail = "admin@instance.test"

func (f *instanceProviderFixture) expectActingUser() {
	f.userRepo.On("GetByID", mock.Anything, testProviderUserID).
		Return(&models.User{ID: testProviderUserID, Email: actingAdminEmail}, nil)
}

// An empty request tests the STORED configuration, and the recipient defaults
// to the acting admin. Port 1 on loopback refuses the dial, so the failure comes
// back as a result value.
func TestInstanceEmailProvider_Test_StoredConfigAndDefaultRecipient(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)
	f.expectActingUser()

	result, err := f.svc.Test(context.Background(), testProviderUserID, models.TestInstanceEmailProviderRequest{})

	require.NoError(t, err, "a delivery failure is a result, not an error")
	assert.False(t, result.Success)
	assert.Equal(t, actingAdminEmail, result.Recipient)
	assert.Equal(t, models.TeamEmailProviderErrSendFailed, result.ErrorDetails)
}

func TestInstanceEmailProvider_Test_RecipientOverride(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)
	recipient := "ops@instance.test"

	result, err := f.svc.Test(context.Background(), testProviderUserID,
		models.TestInstanceEmailProviderRequest{Recipient: &recipient})

	require.NoError(t, err)
	assert.Equal(t, recipient, result.Recipient)
	f.userRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
}

func TestInstanceEmailProvider_Test_InvalidRecipientIsRejected(t *testing.T) {
	f := newInstanceProviderFixture(t)
	recipient := "a@instance.test, b@instance.test"

	_, err := f.svc.Test(context.Background(), testProviderUserID,
		models.TestInstanceEmailProviderRequest{Recipient: &recipient})

	var verr *TeamEmailProviderValidationError
	require.ErrorAs(t, err, &verr)
	assert.Equal(t, "recipient", verr.Fields[0].Field)
}

func TestInstanceEmailProvider_Test_NothingToTest(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)
	f.expectActingUser()

	_, err := f.svc.Test(context.Background(), testProviderUserID, models.TestInstanceEmailProviderRequest{})

	var verr *TeamEmailProviderValidationError
	require.ErrorAs(t, err, &verr)
	assert.Equal(t, "config", verr.Fields[0].Field)
}

// A submitted configuration is used instead of the stored one, with no SSRF
// guard on the loopback host.
func TestInstanceEmailProvider_Test_SubmittedConfig(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)
	f.expectActingUser()
	req := validInstanceSMTPRequest()

	result, err := f.svc.Test(context.Background(), testProviderUserID,
		models.TestInstanceEmailProviderRequest{Config: &req})

	require.NoError(t, err)
	assert.Equal(t, models.TeamEmailProviderErrSendFailed, result.ErrorDetails,
		"the send was attempted, so the loopback host was not rejected")
}

// A submitted config without a secret borrows the stored one only when the
// stored provider type matches.
func TestInstanceEmailProvider_Test_BorrowsTheStoredSecretOnlyForTheSameType(t *testing.T) {
	t.Run("same type", func(t *testing.T) {
		f := newInstanceProviderFixture(t)
		f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)
		f.expectActingUser()
		req := validInstanceSMTPRequest()
		req.Secret = nil

		result, err := f.svc.Test(context.Background(), testProviderUserID,
			models.TestInstanceEmailProviderRequest{Config: &req})

		require.NoError(t, err)
		assert.Equal(t, models.TeamEmailProviderErrSendFailed, result.ErrorDetails)
	})

	t.Run("different type", func(t *testing.T) {
		f := newInstanceProviderFixture(t)
		f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)
		f.expectActingUser()
		req := models.UpsertInstanceEmailProviderRequest{
			UpsertTeamEmailProviderRequest: models.UpsertTeamEmailProviderRequest{
				ProviderType: EmailProviderTypeSendGrid,
				FromAddress:  "noreply@instance.test",
			},
		}

		_, err := f.svc.Test(context.Background(), testProviderUserID,
			models.TestInstanceEmailProviderRequest{Config: &req})

		var verr *TeamEmailProviderValidationError
		require.ErrorAs(t, err, &verr)
		assert.Equal(t, "secret", verr.Fields[0].Field)
	})
}

// A stored row that builds the no-op stub would "send" by discarding the
// message, so it is reported as an invalid configuration, not a success.
func TestInstanceEmailProvider_Test_StubBuildIsConfigInvalid(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(&models.InstanceEmailProvider{
		ProviderType: EmailProviderTypeSMTP,
		Settings:     json.RawMessage(`{"host":"","port":""}`),
		FromAddress:  "noreply@instance.test",
	}, nil)
	f.expectActingUser()

	result, err := f.svc.Test(context.Background(), testProviderUserID, models.TestInstanceEmailProviderRequest{})

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, models.TeamEmailProviderErrConfigInvalid, result.ErrorDetails)
}

func TestInstanceEmailProvider_Test_FactoryFailureIsConfigInvalid(t *testing.T) {
	f := newInstanceProviderFixture(t)
	// Mailgun with no sending key fails in the factory.
	f.repo.On("Get", mock.Anything).Return(&models.InstanceEmailProvider{
		ProviderType: EmailProviderTypeMailgun,
		Settings:     json.RawMessage(`{"domain":"mg.instance.test"}`),
		FromAddress:  "noreply@instance.test",
	}, nil)
	f.expectActingUser()

	result, err := f.svc.Test(context.Background(), testProviderUserID, models.TestInstanceEmailProviderRequest{})

	require.NoError(t, err)
	assert.Equal(t, models.TeamEmailProviderErrConfigInvalid, result.ErrorDetails)
}

func TestInstanceEmailAuditSnapshot_NilRowIsNoDocument(t *testing.T) {
	doc, err := InstanceEmailAuditSnapshot(nil, models.InstanceSettingsAuditSecretChanged)
	require.NoError(t, err)
	assert.Nil(t, doc)
}

// A delivered test message reports success and names the mailbox to check.
func TestSendTestMessage_Success(t *testing.T) {
	provider := new(MockEmailProvider)
	provider.On("SendEmail", mock.Anything, mock.Anything).Return(nil).Once()

	result := sendTestMessage(context.Background(), provider,
		testSender{FromAddress: "noreply@instance.test"}, "ops@instance.test")

	assert.True(t, result.Success)
	assert.Equal(t, "ops@instance.test", result.Recipient)
	assert.Empty(t, result.ErrorDetails)
	provider.AssertExpectations(t)
}

// A stored row with malformed settings or an unsupported type is a build
// failure: a config_invalid result the admin can read, not a server error.
func TestInstanceEmailProvider_Test_UnmappableStoredRowIsConfigInvalid(t *testing.T) {
	for name, row := range map[string]*models.InstanceEmailProvider{
		"malformed settings": {
			ProviderType: EmailProviderTypeSMTP,
			Settings:     json.RawMessage(`{"host": 42}`),
			FromAddress:  "noreply@instance.test",
		},
		"unsupported type": {
			ProviderType: "ses",
			Settings:     json.RawMessage(`{}`),
			FromAddress:  "noreply@instance.test",
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newInstanceProviderFixture(t)
			f.repo.On("Get", mock.Anything).Return(row, nil)
			f.expectActingUser()

			result, err := f.svc.Test(context.Background(), testProviderUserID,
				models.TestInstanceEmailProviderRequest{})

			require.NoError(t, err)
			assert.False(t, result.Success)
			assert.Equal(t, models.TeamEmailProviderErrConfigInvalid, result.ErrorDetails)
		})
	}
}

// A stored secret that will not decrypt stays an error: it is a server-side
// fault, not something the admin's configuration can fix.
func TestInstanceEmailProvider_Test_StoredDecryptFailureIsAnError(t *testing.T) {
	f := newInstanceProviderFixture(t)
	row := f.storedRow(t)
	garbage := "not-valid-ciphertext"
	row.SecretEncrypted = &garbage
	f.repo.On("Get", mock.Anything).Return(row, nil)
	f.expectActingUser()

	_, err := f.svc.Test(context.Background(), testProviderUserID, models.TestInstanceEmailProviderRequest{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt")
}

// Changing the provider type must carry a new secret: the stored one was issued
// for the old provider and is never reused for another (the same rule a test
// send applies when borrowing it).
func TestInstanceEmailProvider_Upsert_TypeChangeWithoutSecretIsRejected(t *testing.T) {
	f := newInstanceProviderFixture(t)
	f.repo.On("Get", mock.Anything).Return(f.storedRow(t), nil)

	req := models.UpsertInstanceEmailProviderRequest{
		UpsertTeamEmailProviderRequest: models.UpsertTeamEmailProviderRequest{
			ProviderType: EmailProviderTypeSendGrid,
			FromAddress:  "noreply@instance.test",
		},
	}

	_, err := f.svc.Upsert(context.Background(), testProviderUserID, req)

	var verr *TeamEmailProviderValidationError
	require.ErrorAs(t, err, &verr)
	assert.Equal(t, "secret", verr.Fields[0].Field)
	f.repo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	f.audit.AssertNotCalled(t, "Append", mock.Anything, mock.Anything)
}

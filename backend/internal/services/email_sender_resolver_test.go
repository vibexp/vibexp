package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/external/implementations"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

func newTestResolver(
	t *testing.T,
	repo repositories.TeamEmailProviderRepository,
	enc EncryptionServiceInterface,
	instanceRepo repositories.InstanceEmailProviderRepository,
) EmailSenderResolver {
	t.Helper()
	if instanceRepo == nil {
		// No expectations: a team-branch test that reached the instance row
		// would fail on the unexpected call.
		instanceRepo = repomocks.NewMockInstanceEmailProviderRepository(t)
	}
	return NewEmailSenderResolver(repo, instanceRepo, enc, slog.New(slog.DiscardHandler))
}

// storedTeamProvider builds a row whose secret is encrypted with the test key, as
// the service would have written it.
func storedTeamProvider(t *testing.T, enc EncryptionServiceInterface) *models.TeamEmailProvider {
	t.Helper()
	ciphertext, err := enc.Encrypt("mailgun-sending-key")
	require.NoError(t, err)

	fromName := "Acme Team"
	replyTo := "reply@acme.test"
	settings, err := json.Marshal(models.MailgunProviderSettings{Domain: "mg.acme.test"})
	require.NoError(t, err)

	return &models.TeamEmailProvider{
		TeamID:          testProviderTeamID,
		ProviderType:    EmailProviderTypeMailgun,
		Settings:        settings,
		SecretEncrypted: ciphertext,
		FromAddress:     "hello@acme.test",
		FromName:        &fromName,
		ReplyTo:         &replyTo,
	}
}

// storedInstanceSMTPRow is an instance row for a credential-less SMTP relay on
// localhost, which builds a real provider without dialling anything.
func storedInstanceSMTPRow(fromAddress string) *models.InstanceEmailProvider {
	return &models.InstanceEmailProvider{
		ProviderType: EmailProviderTypeSMTP,
		Settings:     json.RawMessage(`{"host":"localhost","port":"1025"}`),
		FromAddress:  fromAddress,
	}
}

// An empty team ID means "no team context" — the instance row, without a team
// repository lookup (the team mock has no expectations).
func TestResolve_EmptyTeamIDUsesInstance(t *testing.T) {
	instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
	instanceRepo.On("Get", mock.Anything).Return(storedInstanceSMTPRow("instance@example.com"), nil)
	resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), nil, instanceRepo)

	resolved, err := resolver.Resolve(context.Background(), "")

	require.NoError(t, err)
	assert.Equal(t, EmailSenderSourceInstance, resolved.Source)
	_, isSMTP := resolved.Provider.(*implementations.SMTPEmailProvider)
	assert.True(t, isSMTP, "the provider is built from the instance row")
	assert.Equal(t, "instance@example.com", resolved.FromAddress)
	assert.Empty(t, resolved.TeamID)
}

// A team with no configured row inherits the instance provider. This is the
// fallback the whole epic hinges on.
func TestResolve_NoRowUsesInstance(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrTeamEmailProviderNotFound)
	instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
	instanceRepo.On("Get", mock.Anything).Return(storedInstanceSMTPRow("instance@example.com"), nil)

	resolver := newTestResolver(t, repo, nil, instanceRepo)

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.NoError(t, err)
	assert.Equal(t, EmailSenderSourceInstance, resolved.Source)
	assert.Equal(t, "instance@example.com", resolved.FromAddress)
}

// A configured team gets its own provider and its own sender identity.
func TestResolve_ConfiguredTeamUsesItsOwnProvider(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(storedTeamProvider(t, enc), nil)

	resolver := newTestResolver(t, repo, enc, nil)

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.NoError(t, err)
	assert.Equal(t, EmailSenderSourceTeam, resolved.Source)
	assert.Equal(t, testProviderTeamID, resolved.TeamID)
	assert.Equal(t, "hello@acme.test", resolved.FromAddress)
	assert.Equal(t, "Acme Team", resolved.FromName)
	assert.Equal(t, "reply@acme.test", resolved.ReplyTo)

	// It must be a real Mailgun provider built from the row, not the instance one.
	_, ok := resolved.Provider.(*implementations.MailgunEmailProvider)
	assert.True(t, ok, "expected the team's Mailgun provider")
}

// Epic decision 7: a configured row that cannot be used is an ERROR, never a
// silent fallback. Falling back would send the team's mail from the operator's
// address on the operator's credentials — the exact thing configuring a provider
// is meant to prevent.
func TestResolve_DecryptFailureIsAnErrorNotAFallback(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	broken := storedTeamProvider(t, enc)
	broken.SecretEncrypted = "not-valid-ciphertext"

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).Return(broken, nil)

	resolver := newTestResolver(t, repo, enc, nil)

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.Error(t, err)
	assert.Nil(t, resolved, "must not hand back the instance sender")
	assert.Contains(t, err.Error(), "decrypt")
}

// A row whose configuration cannot build a provider is likewise an error.
func TestResolve_FactoryFailureIsAnErrorNotAFallback(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	// Mailgun with an empty domain fails in NewMailgunEmailProvider.
	row := storedTeamProvider(t, enc)
	row.Settings = json.RawMessage(`{"domain": ""}`)

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).Return(row, nil)

	resolver := newTestResolver(t, repo, enc, nil)

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "failed to build the email provider")
}

// A repository failure must not be mistaken for "this team has no provider".
func TestResolve_RepositoryErrorIsNotAFallback(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, errors.New("connection reset"))

	resolver := newTestResolver(t, repo, nil, nil)

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.NotErrorIs(t, err, repositories.ErrTeamEmailProviderNotFound)
}

// The no-op stub is correct for an unconfigured INSTANCE, but for a team that has
// configured a provider it would accept and discard every message while reporting
// success. Validation keeps such a row out via the API, so this is the
// defence-in-depth path for a row written some other way.
func TestResolve_TeamRowThatWouldBuildTheStubIsAnError(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	row := storedTeamProvider(t, enc)
	row.ProviderType = EmailProviderTypeSMTP
	// Blank host is what makes NewEmailProvider hand back the stub.
	row.Settings = json.RawMessage(`{"host": "", "port": ""}`)

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).Return(row, nil)

	resolver := newTestResolver(t, repo, enc, nil)

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.Error(t, err)
	assert.Nil(t, resolved, "must not silently discard the team's mail")
	assert.Contains(t, err.Error(), "discard messages silently")
}

// A row with an unrecognised provider type is reported, not silently rerouted.
func TestResolve_UnsupportedRowTypeIsAnError(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	row := storedTeamProvider(t, enc)
	row.ProviderType = "ses"

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).Return(row, nil)

	resolver := newTestResolver(t, repo, enc, nil)

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "unsupported email provider type")
}

// A row stored while encryption was configured cannot be read once it is not:
// fail rather than send with a garbage credential.
func TestResolve_NilEncryptionWithStoredSecretIsAnError(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(storedTeamProvider(t, enc), nil)

	resolver := newTestResolver(t, repo, nil, nil)

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.ErrorIs(t, err, ErrEncryptionUnavailable)
}

func TestProviderSpecFromRow_MapsEachType(t *testing.T) {
	tests := []struct {
		name     string
		row      *models.TeamEmailProvider
		assertOn func(t *testing.T, spec implementations.ProviderSpec)
	}{
		{
			name: "smtp",
			row: &models.TeamEmailProvider{
				ProviderType: EmailProviderTypeSMTP,
				Settings: json.RawMessage(
					`{"host":"smtp.acme.test","port":"2525","username":"mailer"}`),
			},
			assertOn: func(t *testing.T, spec implementations.ProviderSpec) {
				assert.Equal(t, "smtp.acme.test", spec.SMTP.Host)
				assert.Equal(t, "2525", spec.SMTP.Port)
				assert.Equal(t, "mailer", spec.SMTP.Username)
				assert.Equal(t, "the-secret", spec.SMTP.Password)
			},
		},
		{
			name: "mailgun",
			row: &models.TeamEmailProvider{
				ProviderType: EmailProviderTypeMailgun,
				Settings: json.RawMessage(
					`{"domain":"mg.acme.test","base_url":"https://api.eu.mailgun.net/v3"}`),
			},
			assertOn: func(t *testing.T, spec implementations.ProviderSpec) {
				assert.Equal(t, "mg.acme.test", spec.Mailgun.Domain)
				assert.Equal(t, "https://api.eu.mailgun.net/v3", spec.Mailgun.BaseURL)
				assert.Equal(t, "the-secret", spec.Mailgun.SendingKey)
			},
		},
		{
			name: "postmark",
			row: &models.TeamEmailProvider{
				ProviderType: EmailProviderTypePostmark,
				Settings:     json.RawMessage(`{"message_stream":"broadcast"}`),
			},
			assertOn: func(t *testing.T, spec implementations.ProviderSpec) {
				assert.Equal(t, "the-secret", spec.Postmark.ServerToken)
				assert.Equal(t, "broadcast", spec.Postmark.MessageStream)
			},
		},
		{
			name: "sendgrid ignores settings entirely",
			row: &models.TeamEmailProvider{
				ProviderType: EmailProviderTypeSendGrid,
				Settings:     json.RawMessage(`{}`),
			},
			assertOn: func(t *testing.T, spec implementations.ProviderSpec) {
				assert.Equal(t, "the-secret", spec.SendGrid.APIKey)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := providerSpecFromRow(tc.row, "the-secret")
			require.NoError(t, err)
			tc.assertOn(t, spec)
		})
	}
}

func TestProviderSpecFromRow_MalformedSettingsIsAnError(t *testing.T) {
	row := &models.TeamEmailProvider{
		TeamID:       testProviderTeamID,
		ProviderType: EmailProviderTypeSMTP,
		Settings:     json.RawMessage(`{"host": 42}`),
	}

	_, err := providerSpecFromRow(row, "secret")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode the email provider settings")
}

// --- Instance branch (#1188) -------------------------------------------------

// The instance row is read per send, never cached: a change takes effect on the
// next message with no restart.
func TestResolve_InstanceConfigChangeAppliesOnTheNextSend(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	ciphertext, err := enc.Encrypt("mailgun-key")
	require.NoError(t, err)

	rowB := &models.InstanceEmailProvider{
		ProviderType:    EmailProviderTypeMailgun,
		Settings:        json.RawMessage(`{"domain":"mg.instance.test"}`),
		SecretEncrypted: &ciphertext,
		FromAddress:     "b@instance.test",
	}

	instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
	instanceRepo.On("Get", mock.Anything).Return(storedInstanceSMTPRow("a@instance.test"), nil).Once()
	instanceRepo.On("Get", mock.Anything).Return(rowB, nil).Once()
	resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), enc, instanceRepo)

	first, err := resolver.Resolve(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "a@instance.test", first.FromAddress)
	assert.Equal(t, "smtp", implementations.ProviderLabel(first.Provider))

	second, err := resolver.Resolve(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "b@instance.test", second.FromAddress)
	assert.Equal(t, "mailgun", implementations.ProviderLabel(second.Provider))
}

// An unconfigured instance is non-fatal (epic #1185 decision 8): the stub plus
// a warning, rate-limited so a busy instance does not flood the log.
func TestResolve_UnconfiguredInstanceUsesTheStubAndWarnsRateLimited(t *testing.T) {
	instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
	instanceRepo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)

	logger, recorder := logtest.New()
	resolver := NewEmailSenderResolver(
		repomocks.NewMockTeamEmailProviderRepository(t), instanceRepo, nil, logger).(*teamEmailSenderResolver)
	clock := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	resolver.now = func() time.Time { return clock }

	resolved, err := resolver.Resolve(context.Background(), "")

	require.NoError(t, err)
	assert.Equal(t, EmailSenderSourceInstance, resolved.Source)
	_, isStub := resolved.Provider.(*implementations.StubEmailProvider)
	assert.True(t, isStub)
	require.Len(t, recorder.AllEntries(), 1)
	assert.Equal(t, slog.LevelWarn, recorder.LastEntry().Level)

	clock = clock.Add(instanceUnconfiguredWarnInterval - time.Second)
	_, err = resolver.Resolve(context.Background(), "")
	require.NoError(t, err)
	assert.Len(t, recorder.AllEntries(), 1, "no second warning inside the window")

	clock = clock.Add(2 * time.Second)
	_, err = resolver.Resolve(context.Background(), "")
	require.NoError(t, err)
	assert.Len(t, recorder.AllEntries(), 2, "warns again once the window has passed")
}

// Only "no row" falls back to the stub. Any other failure on the instance
// branch is an error (epic #499 decision 7).
func TestResolve_InstanceFailuresAreErrorsNotFallbacks(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	garbage := "not-valid-ciphertext"

	tests := []struct {
		name    string
		row     *models.InstanceEmailProvider
		repoErr error
		wantErr string
	}{
		{name: "repository error", repoErr: errors.New("connection reset"),
			wantErr: "failed to resolve the instance email provider"},
		{name: "decrypt failure", row: func() *models.InstanceEmailProvider {
			row := storedInstanceSMTPRow("i@instance.test")
			row.SecretEncrypted = &garbage
			return row
		}(), wantErr: "decrypt"},
		{name: "row that builds the stub", row: &models.InstanceEmailProvider{
			ProviderType: EmailProviderTypeSMTP,
			Settings:     json.RawMessage(`{"host":"","port":""}`),
			FromAddress:  "i@instance.test",
		}, wantErr: "discard messages silently"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
			if tc.repoErr != nil {
				instanceRepo.On("Get", mock.Anything).Return(nil, tc.repoErr)
			} else {
				instanceRepo.On("Get", mock.Anything).Return(tc.row, nil)
			}
			resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), enc, instanceRepo)

			resolved, err := resolver.Resolve(context.Background(), "")

			require.Error(t, err)
			assert.Nil(t, resolved)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// Instance health is stamped on the instance row, on success and on failure.
func TestRecordSendOutcome_StampsInstanceHealth(t *testing.T) {
	sender := &ResolvedEmailSender{
		Provider: &implementations.SMTPEmailProvider{}, Source: EmailSenderSourceInstance,
	}

	t.Run("success", func(t *testing.T) {
		instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
		instanceRepo.On("RecordSuccess", mock.Anything, mock.AnythingOfType("time.Time")).Return(nil).Once()
		resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), nil, instanceRepo)

		require.NoError(t, resolver.RecordSendOutcome(context.Background(), sender, nil))
	})

	t.Run("failure", func(t *testing.T) {
		sendErr := errors.New("relay refused")
		instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
		instanceRepo.On("RecordError", mock.Anything, sendErr, mock.AnythingOfType("time.Time")).Return(nil).Once()
		resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), nil, instanceRepo)

		require.NoError(t, resolver.RecordSendOutcome(context.Background(), sender, sendErr))
	})

	t.Run("row deleted mid-send is a no-op", func(t *testing.T) {
		instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
		instanceRepo.On("RecordSuccess", mock.Anything, mock.Anything).
			Return(repositories.ErrInstanceEmailProviderNotFound)
		resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), nil, instanceRepo)

		require.NoError(t, resolver.RecordSendOutcome(context.Background(), sender, nil))
	})

	t.Run("a bookkeeping failure is reported", func(t *testing.T) {
		instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
		instanceRepo.On("RecordSuccess", mock.Anything, mock.Anything).Return(errors.New("db down"))
		resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), nil, instanceRepo)

		require.Error(t, resolver.RecordSendOutcome(context.Background(), sender, nil))
	})

	t.Run("the stub of an unconfigured instance has no row", func(t *testing.T) {
		// No expectations: any call fails the test.
		resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), nil, nil)
		stub := &ResolvedEmailSender{
			Provider: &implementations.StubEmailProvider{}, Source: EmailSenderSourceInstance,
		}

		require.NoError(t, resolver.RecordSendOutcome(context.Background(), stub, errors.New("x")))
	})
}

// A team send still stamps the team row and never the instance row.
func TestRecordSendOutcome_TeamSenderStampsTheTeamRow(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("RecordSendResult", mock.Anything, testProviderTeamID, nil, mock.AnythingOfType("time.Time")).
		Return(nil).Once()
	resolver := newTestResolver(t, repo, nil, nil)

	err := resolver.RecordSendOutcome(context.Background(), &ResolvedEmailSender{
		Provider: &implementations.SMTPEmailProvider{}, Source: EmailSenderSourceTeam, TeamID: testProviderTeamID,
	}, nil)

	require.NoError(t, err)
}

func TestInstanceIdentity(t *testing.T) {
	t.Run("from the row", func(t *testing.T) {
		contact := "support@instance.test"
		privacy := "https://instance.test/privacy"
		row := storedInstanceSMTPRow("noreply@instance.test")
		row.ContactRecipientAddress = &contact
		row.PrivacyPolicyURL = &privacy

		instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
		instanceRepo.On("Get", mock.Anything).Return(row, nil)
		resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), nil, instanceRepo)

		identity, err := resolver.InstanceIdentity(context.Background())

		require.NoError(t, err)
		assert.Equal(t, InstanceEmailIdentity{
			FromAddress: "noreply@instance.test", ContactRecipientAddress: contact, PrivacyPolicyURL: privacy,
		}, identity)
	})

	t.Run("zero when unconfigured", func(t *testing.T) {
		instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
		instanceRepo.On("Get", mock.Anything).Return(nil, repositories.ErrInstanceEmailProviderNotFound)
		resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), nil, instanceRepo)

		identity, err := resolver.InstanceIdentity(context.Background())

		require.NoError(t, err)
		assert.Equal(t, InstanceEmailIdentity{}, identity)
	})

	t.Run("a read failure is an error", func(t *testing.T) {
		instanceRepo := repomocks.NewMockInstanceEmailProviderRepository(t)
		instanceRepo.On("Get", mock.Anything).Return(nil, errors.New("db down"))
		resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), nil, instanceRepo)

		_, err := resolver.InstanceIdentity(context.Background())

		require.Error(t, err)
	})
}

// Nothing to stamp for a missing sender or an unknown source.
func TestRecordSendOutcome_NoOps(t *testing.T) {
	// No expectations on either repository: any call fails the test.
	resolver := newTestResolver(t, repomocks.NewMockTeamEmailProviderRepository(t), nil, nil)

	require.NoError(t, resolver.RecordSendOutcome(context.Background(), nil, nil))
	require.NoError(t, resolver.RecordSendOutcome(context.Background(),
		&ResolvedEmailSender{Source: EmailSenderSourceTeam}, nil), "a team sender without a team")
	require.NoError(t, resolver.RecordSendOutcome(context.Background(),
		&ResolvedEmailSender{Source: "unknown"}, nil))
}

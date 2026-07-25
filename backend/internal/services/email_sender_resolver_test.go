package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/darkrockmountain/gomail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/external/implementations"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// recordingProvider stands in for the instance provider so tests can assert
// identity by pointer rather than by type.
type recordingProvider struct{}

func (*recordingProvider) SendEmail(context.Context, *gomail.EmailMessage) error { return nil }

func instanceConfig() *config.Config {
	return &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
		Email: config.EmailConfig{
			FromAddress: "instance@example.com",
			SMTP:        config.SMTPConfig{Username: "smtp-user@example.com"},
		},
	}
}

func newTestResolver(
	t *testing.T,
	repo repositories.TeamEmailProviderRepository,
	enc EncryptionServiceInterface,
	instance external.EmailProvider,
	cfg *config.Config,
) EmailSenderResolver {
	t.Helper()
	return NewEmailSenderResolver(repo, enc, instance, cfg, slog.New(slog.DiscardHandler))
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

// An empty team ID means "no team context" — the instance provider, without a
// repository lookup (the mock has no expectations).
func TestResolve_EmptyTeamIDUsesInstance(t *testing.T) {
	instance := &recordingProvider{}
	resolver := newTestResolver(t,
		repomocks.NewMockTeamEmailProviderRepository(t), nil, instance, instanceConfig())

	resolved, err := resolver.Resolve(context.Background(), "")

	require.NoError(t, err)
	assert.Equal(t, EmailSenderSourceInstance, resolved.Source)
	assert.Same(t, instance, resolved.Provider)
	assert.Equal(t, "instance@example.com", resolved.FromAddress)
	assert.Empty(t, resolved.TeamID)
}

// A team with no configured row inherits the instance provider. This is the
// fallback the whole epic hinges on.
func TestResolve_NoRowUsesInstance(t *testing.T) {
	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(nil, repositories.ErrTeamEmailProviderNotFound)

	instance := &recordingProvider{}
	resolver := newTestResolver(t, repo, nil, instance, instanceConfig())

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.NoError(t, err)
	assert.Equal(t, EmailSenderSourceInstance, resolved.Source)
	assert.Same(t, instance, resolved.Provider)
}

// The instance from-address chain must stay byte-identical to
// EmailService.sendEmail: FromAddress, falling back to SMTP.Username.
func TestResolve_InstanceFromAddressChain(t *testing.T) {
	t.Run("prefers the configured from address", func(t *testing.T) {
		resolver := newTestResolver(t,
			repomocks.NewMockTeamEmailProviderRepository(t), nil, &recordingProvider{}, instanceConfig())

		resolved, err := resolver.Resolve(context.Background(), "")

		require.NoError(t, err)
		assert.Equal(t, "instance@example.com", resolved.FromAddress)
	})

	t.Run("falls back to the smtp username", func(t *testing.T) {
		cfg := instanceConfig()
		cfg.Email.FromAddress = ""

		resolver := newTestResolver(t,
			repomocks.NewMockTeamEmailProviderRepository(t), nil, &recordingProvider{}, cfg)

		resolved, err := resolver.Resolve(context.Background(), "")

		require.NoError(t, err)
		assert.Equal(t, "smtp-user@example.com", resolved.FromAddress)
	})
}

// A configured team gets its own provider and its own sender identity.
func TestResolve_ConfiguredTeamUsesItsOwnProvider(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).
		Return(storedTeamProvider(t, enc), nil)

	instance := &recordingProvider{}
	resolver := newTestResolver(t, repo, enc, instance, instanceConfig())

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.NoError(t, err)
	assert.Equal(t, EmailSenderSourceTeam, resolved.Source)
	assert.Equal(t, testProviderTeamID, resolved.TeamID)
	assert.Equal(t, "hello@acme.test", resolved.FromAddress)
	assert.Equal(t, "Acme Team", resolved.FromName)
	assert.Equal(t, "reply@acme.test", resolved.ReplyTo)

	// It must be a real Mailgun provider built from the row, not the instance one.
	assert.NotSame(t, instance, resolved.Provider)
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

	resolver := newTestResolver(t, repo, enc, &recordingProvider{}, instanceConfig())

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

	resolver := newTestResolver(t, repo, enc, &recordingProvider{}, instanceConfig())

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

	resolver := newTestResolver(t, repo, nil, &recordingProvider{}, instanceConfig())

	resolved, err := resolver.Resolve(context.Background(), testProviderTeamID)

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.NotErrorIs(t, err, repositories.ErrTeamEmailProviderNotFound)
}

// A row with an unrecognised provider type is reported, not silently rerouted.
func TestResolve_UnsupportedRowTypeIsAnError(t *testing.T) {
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)

	row := storedTeamProvider(t, enc)
	row.ProviderType = "ses"

	repo := repomocks.NewMockTeamEmailProviderRepository(t)
	repo.On("GetByTeamID", mock.Anything, testProviderTeamID).Return(row, nil)

	resolver := newTestResolver(t, repo, enc, &recordingProvider{}, instanceConfig())

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

	resolver := newTestResolver(t, repo, nil, &recordingProvider{}, instanceConfig())

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

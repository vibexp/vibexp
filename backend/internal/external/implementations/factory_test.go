package implementations

import (
	"context"
	"log/slog"
	"testing"

	"github.com/darkrockmountain/gomail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/logging/logtest"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// validSpec returns a spec with usable credentials for every provider, so a
// test can select one by setting Type and be sure the others are not the reason
// construction succeeded or failed.
func validSpec(providerType string) ProviderSpec {
	return ProviderSpec{
		Type: providerType,
		SMTP: SMTPSpec{
			Host:     "smtp.example.com",
			Port:     "587",
			Username: "test@example.com",
			Password: "password",
		},
		Mailgun: MailgunSpec{
			Domain:     "mg.example.com",
			SendingKey: "key-abc123",
		},
		Postmark: PostmarkSpec{
			ServerToken: "token-abc123",
		},
		SendGrid: SendGridSpec{
			APIKey: "test-sendgrid-key",
		},
	}
}

func TestNewEmailProvider_BuildsEachProviderType(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		assertType   func(t *testing.T, provider any)
	}{
		{
			name:         "smtp",
			providerType: "smtp",
			assertType: func(t *testing.T, provider any) {
				_, ok := provider.(*SMTPEmailProvider)
				assert.True(t, ok, "expected *SMTPEmailProvider")
			},
		},
		{
			name:         "mailgun",
			providerType: "mailgun",
			assertType: func(t *testing.T, provider any) {
				_, ok := provider.(*MailgunEmailProvider)
				assert.True(t, ok, "expected *MailgunEmailProvider")
			},
		},
		{
			name:         "postmark",
			providerType: "postmark",
			assertType: func(t *testing.T, provider any) {
				_, ok := provider.(*PostmarkEmailProvider)
				assert.True(t, ok, "expected *PostmarkEmailProvider")
			},
		},
		{
			name:         "sendgrid",
			providerType: "sendgrid",
			assertType: func(t *testing.T, provider any) {
				_, ok := provider.(*SendGridEmailProvider)
				assert.True(t, ok, "expected *SendGridEmailProvider")
			},
		},
		{
			name:         "empty type falls through to smtp",
			providerType: "",
			assertType: func(t *testing.T, provider any) {
				_, ok := provider.(*SMTPEmailProvider)
				assert.True(t, ok, "empty Type should build the SMTP provider")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewEmailProvider(validSpec(tc.providerType), discardLogger())

			require.NoError(t, err)
			require.NotNil(t, provider)
			tc.assertType(t, provider)
		})
	}
}

func TestNewEmailProvider_TypeMatchingIsCaseAndWhitespaceTolerant(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
	}{
		{"uppercase MAILGUN", "MAILGUN"},
		{"mixed case Mailgun", "Mailgun"},
		{"padded mailgun", "  mailgun  "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewEmailProvider(validSpec(tc.providerType), discardLogger())

			require.NoError(t, err)
			_, ok := provider.(*MailgunEmailProvider)
			assert.True(t, ok, "expected *MailgunEmailProvider for %q", tc.providerType)
		})
	}
}

func TestNewEmailProvider_StubWhenSMTPIncomplete(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		host         string
		port         string
	}{
		{"smtp with blank host and port", "smtp", "", ""},
		{"smtp with blank host only", "smtp", "", "587"},
		{"smtp with blank port only", "smtp", "smtp.example.com", ""},
		{"empty type with blank host and port", "", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec(tc.providerType)
			spec.SMTP.Host = tc.host
			spec.SMTP.Port = tc.port

			provider, err := NewEmailProvider(spec, discardLogger())

			require.NoError(t, err, "incomplete SMTP settings must not be an error")
			_, ok := provider.(*StubEmailProvider)
			assert.True(t, ok, "expected the no-op *StubEmailProvider")
		})
	}
}

func TestNewEmailProvider_StubFallbackIsLogged(t *testing.T) {
	logger, recorder := logtest.New()

	spec := validSpec("smtp")
	spec.SMTP.Host = ""
	spec.SMTP.Port = ""

	_, err := NewEmailProvider(spec, logger)

	require.NoError(t, err)
	entry := recorder.LastEntry()
	require.NotNil(t, entry, "the silent stub fallback must leave a trace")
	assert.Equal(t, slog.LevelDebug, entry.Level)
	assert.Contains(t, entry.Message, "no-op stub")
}

func TestNewEmailProvider_MissingSecretPerType(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		mutate       func(spec *ProviderSpec)
		wantContains string
	}{
		{
			name:         "mailgun without sending key",
			providerType: "mailgun",
			mutate:       func(s *ProviderSpec) { s.Mailgun.SendingKey = "" },
			wantContains: "MAILGUN_SENDING_KEY",
		},
		{
			name:         "mailgun without domain",
			providerType: "mailgun",
			mutate:       func(s *ProviderSpec) { s.Mailgun.Domain = "" },
			wantContains: "MAILGUN_DOMAIN",
		},
		{
			name:         "mailgun with a URL as domain",
			providerType: "mailgun",
			mutate:       func(s *ProviderSpec) { s.Mailgun.Domain = "https://api.mailgun.net/v3" },
			wantContains: "bare domain",
		},
		{
			name:         "postmark without server token",
			providerType: "postmark",
			mutate:       func(s *ProviderSpec) { s.Postmark.ServerToken = "" },
			wantContains: "POSTMARK_SERVER_TOKEN",
		},
		{
			name:         "sendgrid without api key",
			providerType: "sendgrid",
			mutate:       func(s *ProviderSpec) { s.SendGrid.APIKey = "" },
			wantContains: "SENDGRID_API_KEY",
		},
		{
			name:         "smtp with a non-numeric port",
			providerType: "smtp",
			mutate:       func(s *ProviderSpec) { s.SMTP.Port = "not-a-port" },
			wantContains: "invalid SMTP port",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := validSpec(tc.providerType)
			tc.mutate(&spec)

			provider, err := NewEmailProvider(spec, discardLogger())

			require.Error(t, err)
			assert.Nil(t, provider)
			assert.Contains(t, err.Error(), tc.wantContains)
			assert.Contains(t, err.Error(), "email provider factory",
				"construction failures stay attributable to the factory")
		})
	}
}

func TestNewEmailProvider_UnknownTypeIsAnError(t *testing.T) {
	provider, err := NewEmailProvider(validSpec("ses"), discardLogger())

	require.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "unknown email provider")
	// The value is reported as configured, not normalised, so the operator
	// recognises it.
	assert.Contains(t, err.Error(), `"ses"`)
}

func TestNewEmailProvider_UnknownTypeReportsRawValue(t *testing.T) {
	provider, err := NewEmailProvider(validSpec("  SES  "), discardLogger())

	require.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), `"  SES  "`)
}

func TestProviderLabel_NamesWhatTheFactoryBuilt(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		want         string
	}{
		{"smtp", "smtp", "smtp"},
		{"mailgun", "mailgun", "mailgun"},
		{"postmark", "postmark", "postmark"},
		{"sendgrid", "sendgrid", "sendgrid"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewEmailProvider(validSpec(tc.providerType), discardLogger())
			require.NoError(t, err)

			assert.Equal(t, tc.want, ProviderLabel(provider))
		})
	}
}

func TestProviderLabel_StubFallback(t *testing.T) {
	spec := validSpec("smtp")
	spec.SMTP.Host = ""
	spec.SMTP.Port = ""

	provider, err := NewEmailProvider(spec, discardLogger())
	require.NoError(t, err)

	// "stub" rather than "smtp" is the whole point: it tells an operator that
	// nothing will actually be delivered.
	assert.Equal(t, "stub", ProviderLabel(provider))
}

func TestProviderLabel_UnrecognisedProviderIsNotMislabelled(t *testing.T) {
	// A provider type the label switch has not been taught about must not be
	// reported as one of the known providers.
	assert.Equal(t, "unknown", ProviderLabel(&unlabelledProvider{}))
}

// unlabelledProvider stands in for a provider added to the factory but not yet
// to ProviderLabel.
type unlabelledProvider struct{}

func (unlabelledProvider) SendEmail(_ context.Context, _ *external.OutgoingMessage) error {
	return nil
}

func TestStubEmailProvider_SendEmailDiscards(t *testing.T) {
	stub := &StubEmailProvider{}

	assert.NoError(t, stub.SendEmail(context.Background(), &external.OutgoingMessage{
		Message: gomail.NewEmailMessage(
			"from@example.com", []string{"to@example.com"}, "subject", "body",
		),
		FromName: "Acme Team",
	}))
}

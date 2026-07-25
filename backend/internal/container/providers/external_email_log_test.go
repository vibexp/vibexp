package providers

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/logging/logtest"
)

// The startup log line is operator-visible: it is how a deployment confirms
// which email provider it came up with, and in particular whether it silently
// fell back to the no-op stub. These tests pin the message and the
// email_provider value for every branch, since the selection itself now lives
// in implementations.NewEmailProvider.
func TestProvideEmailProvider_LogsResolvedProvider(t *testing.T) {
	tests := []struct {
		name      string
		email     config.EmailConfig
		wantValue string
	}{
		{
			name: "smtp",
			email: config.EmailConfig{
				Provider: "smtp",
				SMTP:     config.SMTPConfig{Host: "smtp.example.com", Port: "587"},
			},
			wantValue: "smtp",
		},
		{
			name: "stub when smtp host and port are absent",
			email: config.EmailConfig{
				Provider: "smtp",
				SMTP:     config.SMTPConfig{Host: "", Port: ""},
			},
			wantValue: "stub",
		},
		{
			name: "mailgun",
			email: config.EmailConfig{
				Provider: "mailgun",
				Mailgun:  config.MailgunConfig{Domain: "mg.example.com", SendingKey: "key-abc123"},
			},
			wantValue: "mailgun",
		},
		{
			name: "postmark",
			email: config.EmailConfig{
				Provider: "postmark",
				Postmark: config.PostmarkConfig{ServerToken: "token-abc123"},
			},
			wantValue: "postmark",
		},
		{
			name: "sendgrid",
			email: config.EmailConfig{
				Provider: "sendgrid",
				SendGrid: config.SendGridConfig{APIKey: "test-sendgrid-key"},
			},
			wantValue: "sendgrid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, recorder := logtest.New()

			provider, err := ProvideEmailProvider(&config.Config{Email: tc.email}, logger)
			require.NoError(t, err)
			require.NotNil(t, provider)

			entry := recorder.LastEntry()
			require.NotNil(t, entry, "provider construction must log")
			assert.Equal(t, slog.LevelInfo, entry.Level)
			assert.Equal(t, msgEmailProviderInitialized, entry.Message)
			assert.Equal(t, tc.wantValue, entry.Data["email_provider"])
		})
	}
}

// The config → ProviderSpec mapping is easy to get wrong by omission: a field
// left unmapped fails no compile check and, for a credential, would surface only
// as a delivery failure in production. Assert every field is carried across.
func TestProvideEmailProvider_MapsEverySpecField(t *testing.T) {
	cfg := &config.Config{
		Email: config.EmailConfig{
			Provider: "mailgun",
			SMTP: config.SMTPConfig{
				Host:     "smtp.example.com",
				Port:     "587",
				Username: "user@example.com",
				Password: "smtp-password",
			},
			Mailgun: config.MailgunConfig{
				BaseURL:    "https://api.eu.mailgun.net/v3",
				Domain:     "mg.example.com",
				SendingKey: "key-abc123",
			},
			Postmark: config.PostmarkConfig{
				ServerToken:   "token-abc123",
				MessageStream: "broadcast",
			},
			SendGrid: config.SendGridConfig{
				APIKey: "test-sendgrid-key",
			},
		},
	}

	spec := emailProviderSpec(cfg)

	assert.Equal(t, cfg.Email.Provider, spec.Type)
	assert.Equal(t, cfg.Email.SMTP.Host, spec.SMTP.Host)
	assert.Equal(t, cfg.Email.SMTP.Port, spec.SMTP.Port)
	assert.Equal(t, cfg.Email.SMTP.Username, spec.SMTP.Username)
	assert.Equal(t, cfg.Email.SMTP.Password, spec.SMTP.Password)
	assert.Equal(t, cfg.Email.Mailgun.BaseURL, spec.Mailgun.BaseURL)
	assert.Equal(t, cfg.Email.Mailgun.Domain, spec.Mailgun.Domain)
	assert.Equal(t, cfg.Email.Mailgun.SendingKey, spec.Mailgun.SendingKey)
	assert.Equal(t, cfg.Email.Postmark.ServerToken, spec.Postmark.ServerToken)
	assert.Equal(t, cfg.Email.Postmark.MessageStream, spec.Postmark.MessageStream)
	assert.Equal(t, cfg.Email.SendGrid.APIKey, spec.SendGrid.APIKey)
}

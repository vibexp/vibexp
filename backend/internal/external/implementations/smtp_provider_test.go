package implementations

import (
	"context"
	"testing"

	"github.com/darkrockmountain/gomail"
	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/config"
)

func TestNewSMTPEmailProvider(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid SMTP configuration",
			config: &config.Config{
				Email: config.EmailConfig{
					SMTP: config.SMTPConfig{
						Host:     "smtp.example.com",
						Port:     "587",
						Username: "test@example.com",
						Password: "password123",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid SMTP port - non-numeric",
			config: &config.Config{
				Email: config.EmailConfig{
					SMTP: config.SMTPConfig{
						Host:     "smtp.example.com",
						Port:     "invalid",
						Username: "test@example.com",
						Password: "password123",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid SMTP port",
		},
		{
			name: "Empty SMTP port",
			config: &config.Config{
				Email: config.EmailConfig{
					SMTP: config.SMTPConfig{
						Host:     "smtp.example.com",
						Port:     "",
						Username: "test@example.com",
						Password: "password123",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid SMTP port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewSMTPEmailProvider(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)

				// Verify it's the correct type
				_, ok := provider.(*SMTPEmailProvider)
				assert.True(t, ok, "Provider should be of type *SMTPEmailProvider")
			}
		})
	}
}

func TestSMTPEmailProvider_SendEmail(t *testing.T) {
	// Note: This test validates the interface and error handling
	// Actual SMTP sending requires live SMTP server or mocking at network level

	cfg := &config.Config{
		Email: config.EmailConfig{
			SMTP: config.SMTPConfig{
				Host:     "smtp.example.com",
				Port:     "587",
				Username: "test@example.com",
				Password: "password123",
			},
		},
	}

	provider, err := NewSMTPEmailProvider(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, provider)

	// Create a test email message
	message := gomail.NewFullEmailMessage(
		"test@example.com",
		[]string{"recipient@example.com"},
		"Test Subject",
		nil, // cc
		nil, // bcc
		"",  // replyTo
		"Plain text body",
		"<p>HTML body</p>",
		nil, // attachments
	)

	ctx := context.Background()

	// This will fail because we can't connect to the SMTP server in tests
	// but it validates the interface and method signature
	err = provider.SendEmail(ctx, message)
	assert.Error(t, err, "Should error when connecting to non-existent SMTP server")
}

func TestSMTPEmailProvider_InterfaceCompliance(t *testing.T) {
	// Verify that SMTPEmailProvider implements the EmailProvider interface
	cfg := &config.Config{
		Email: config.EmailConfig{
			SMTP: config.SMTPConfig{
				Host:     "smtp.example.com",
				Port:     "587",
				Username: "test@example.com",
				Password: "password123",
			},
		},
	}

	provider, err := NewSMTPEmailProvider(cfg)
	assert.NoError(t, err)

	// Check that it has the SendEmail method with correct signature
	assert.NotNil(t, provider)

	// Create a test message
	message := gomail.NewFullEmailMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Subject",
		nil, nil, "", "text", "html", nil,
	)

	// Should accept context and message
	err = provider.SendEmail(context.Background(), message)
	// Error expected since SMTP server doesn't exist, but method should be callable
	assert.Error(t, err)
}

package implementations

import (
	"context"
	"errors"
	"testing"

	"github.com/darkrockmountain/gomail"
	"github.com/mailgun/mailgun-go/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
)

// fakeMailgunSender implements the mailgunSender interface without making
// network calls. It records the last call for assertions.
type fakeMailgunSender struct {
	sendErr     error
	lastCtx     context.Context
	lastMessage *mailgun.Message
}

func (f *fakeMailgunSender) NewMessage(from, subject, text string, to ...string) *mailgun.Message {
	return mailgun.NewMessage(from, subject, text, to...)
}

func (f *fakeMailgunSender) Send(ctx context.Context, m *mailgun.Message) (string, string, error) {
	f.lastCtx = ctx
	f.lastMessage = m
	return "queued", "msg-id-123", f.sendErr
}

func TestNewMailgunEmailProvider_EmptySendingKey(t *testing.T) {
	cfg := &config.Config{
		Email: config.EmailConfig{
			Mailgun: config.MailgunConfig{
				Domain:     "mg.example.com",
				SendingKey: "",
			},
		},
	}

	provider, err := NewMailgunEmailProvider(cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MAILGUN_SENDING_KEY")
	assert.Nil(t, provider)
}

func TestNewMailgunEmailProvider_EmptyDomain(t *testing.T) {
	cfg := &config.Config{
		Email: config.EmailConfig{
			Mailgun: config.MailgunConfig{
				Domain:     "",
				SendingKey: "key-abc123",
			},
		},
	}

	provider, err := NewMailgunEmailProvider(cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MAILGUN_DOMAIN")
	assert.Nil(t, provider)
}

func TestNewMailgunEmailProvider_DomainIsURL(t *testing.T) {
	tests := []struct {
		name   string
		domain string
	}{
		{"full URL with scheme", "https://api.mailgun.net/v3"},
		{"path suffix", "mg.example.com/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Email: config.EmailConfig{
					Mailgun: config.MailgunConfig{
						Domain:     tc.domain,
						SendingKey: "key-abc123",
					},
				},
			}
			provider, err := NewMailgunEmailProvider(cfg)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "bare domain")
			assert.Nil(t, provider)
		})
	}
}

func TestNewMailgunEmailProvider_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		Email: config.EmailConfig{
			Mailgun: config.MailgunConfig{
				Domain:     "mg.example.com",
				SendingKey: "key-abc123",
			},
		},
	}

	provider, err := NewMailgunEmailProvider(cfg)

	require.NoError(t, err)
	assert.NotNil(t, provider)

	_, ok := provider.(*MailgunEmailProvider)
	assert.True(t, ok, "Provider should be of type *MailgunEmailProvider")
}

func TestNewMailgunEmailProvider_WithBaseURL(t *testing.T) {
	// Setting MAILGUN_BASE_URL to the EU endpoint should not error during construction.
	cfg := &config.Config{
		Email: config.EmailConfig{
			Mailgun: config.MailgunConfig{
				Domain:     "mg.example.com",
				SendingKey: "key-abc123",
				BaseURL:    "https://api.eu.mailgun.net/v3",
			},
		},
	}

	provider, err := NewMailgunEmailProvider(cfg)

	require.NoError(t, err)
	assert.NotNil(t, provider)
}

func TestNormalizeMailgunBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"whitespace stays empty", "   ", ""},
		{"v3 suffix preserved", "https://api.eu.mailgun.net/v3", "https://api.eu.mailgun.net/v3"},
		{"v2 suffix preserved", "https://api.mailgun.net/v2", "https://api.mailgun.net/v2"},
		{"v4 suffix preserved", "https://api.mailgun.net/v4", "https://api.mailgun.net/v4"},
		{"trailing slash stripped", "https://api.eu.mailgun.net/v3/", "https://api.eu.mailgun.net/v3"},
		{"missing suffix gets /v3", "https://api.eu.mailgun.net", "https://api.eu.mailgun.net/v3"},
		{"missing suffix with trailing slash gets /v3", "https://api.eu.mailgun.net/", "https://api.eu.mailgun.net/v3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeMailgunBaseURL(tc.in))
		})
	}
}

func TestNewMailgunEmailProvider_NormalizesBaseURL(t *testing.T) {
	// The deployed prod misconfiguration was MAILGUN_BASE_URL=https://api.eu.mailgun.net
	// (no /v3 suffix), causing mailgun-go to fail at send time. The provider must
	// accept this value and normalize it rather than failing or passing it through.
	cfg := &config.Config{
		Email: config.EmailConfig{
			Mailgun: config.MailgunConfig{
				Domain:     "mg.example.com",
				SendingKey: "key-abc123",
				BaseURL:    "https://api.eu.mailgun.net",
			},
		},
	}

	provider, err := NewMailgunEmailProvider(cfg)

	require.NoError(t, err)
	assert.NotNil(t, provider)
}

func TestNewMailgunEmailProvider_InterfaceCompliance(t *testing.T) {
	var _ external.EmailProvider = (*MailgunEmailProvider)(nil)
}

func TestMailgunEmailProvider_SendEmail_Success(t *testing.T) {
	fake := &fakeMailgunSender{sendErr: nil}
	provider := &MailgunEmailProvider{sender: fake}

	message := gomail.NewFullEmailMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Test Subject", []string{"cc@example.com"}, []string{"bcc@example.com"}, "reply@example.com",
		"Plain text body", "<p>HTML body</p>", nil,
	)

	ctx := context.Background()
	err := provider.SendEmail(ctx, message)

	require.NoError(t, err)
	assert.Equal(t, ctx, fake.lastCtx, "ctx must be propagated to mailgun-go's Send")
	assert.NotNil(t, fake.lastMessage, "message must be passed to Send")
}

func TestMailgunEmailProvider_SendEmail_ContextPropagation(t *testing.T) {
	fake := &fakeMailgunSender{sendErr: nil}
	provider := &MailgunEmailProvider{sender: fake}
	type ctxKey string

	ctx := context.WithValue(context.Background(), ctxKey("trace"), "abc-123")
	message := gomail.NewFullEmailMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Test", nil, nil, "", "text", "<p>html</p>", nil,
	)

	err := provider.SendEmail(ctx, message)

	require.NoError(t, err)
	assert.Equal(t, "abc-123", fake.lastCtx.Value(ctxKey("trace")), "ctx values must be preserved end-to-end")
}

func TestMailgunEmailProvider_SendEmail_Error(t *testing.T) {
	sendErr := errors.New("mailgun api unavailable")
	provider := &MailgunEmailProvider{sender: &fakeMailgunSender{sendErr: sendErr}}

	message := gomail.NewFullEmailMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Test Subject", nil, nil, "",
		"text", "<p>html</p>", nil,
	)

	err := provider.SendEmail(context.Background(), message)

	require.Error(t, err)
	assert.ErrorIs(t, err, sendErr, "underlying error must be wrapped with %%w")
}

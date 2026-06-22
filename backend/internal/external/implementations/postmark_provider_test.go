package implementations

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/darkrockmountain/gomail"
	"github.com/mrz1836/postmark"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
)

// fakePostmarkSender implements the postmarkSender interface without making
// network calls. It records the last call for assertions.
type fakePostmarkSender struct {
	sendErr   error
	lastCtx   context.Context
	lastEmail postmark.Email
}

func (f *fakePostmarkSender) SendEmail(ctx context.Context, email postmark.Email) (postmark.EmailResponse, error) {
	f.lastCtx = ctx
	f.lastEmail = email
	return postmark.EmailResponse{MessageID: "msg-id-123"}, f.sendErr
}

func TestNewPostmarkEmailProvider_EmptyServerToken(t *testing.T) {
	cfg := &config.Config{
		PostmarkServerToken: "",
	}

	provider, err := NewPostmarkEmailProvider(cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "POSTMARK_SERVER_TOKEN")
	assert.Nil(t, provider)
}

func TestNewPostmarkEmailProvider_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		PostmarkServerToken: "token-abc123",
	}

	provider, err := NewPostmarkEmailProvider(cfg)

	require.NoError(t, err)
	assert.NotNil(t, provider)

	_, ok := provider.(*PostmarkEmailProvider)
	assert.True(t, ok, "Provider should be of type *PostmarkEmailProvider")
}

func TestNewPostmarkEmailProvider_DefaultsMessageStream(t *testing.T) {
	cfg := &config.Config{
		PostmarkServerToken:   "token-abc123",
		PostmarkMessageStream: "",
	}

	provider, err := NewPostmarkEmailProvider(cfg)

	require.NoError(t, err)
	pm, ok := provider.(*PostmarkEmailProvider)
	require.True(t, ok)
	assert.Equal(t, "outbound", pm.messageStream, "empty message stream must default to outbound")
}

func TestNewPostmarkEmailProvider_CustomMessageStream(t *testing.T) {
	cfg := &config.Config{
		PostmarkServerToken:   "token-abc123",
		PostmarkMessageStream: "broadcast",
	}

	provider, err := NewPostmarkEmailProvider(cfg)

	require.NoError(t, err)
	pm, ok := provider.(*PostmarkEmailProvider)
	require.True(t, ok)
	assert.Equal(t, "broadcast", pm.messageStream)
}

func TestNewPostmarkEmailProvider_InterfaceCompliance(t *testing.T) {
	var _ external.EmailProvider = (*PostmarkEmailProvider)(nil)
}

func TestPostmarkEmailProvider_SendEmail_MapsAllFields(t *testing.T) {
	fake := &fakePostmarkSender{}
	provider := &PostmarkEmailProvider{sender: fake, messageStream: "outbound"}

	attachment := gomail.NewAttachment("report.pdf", []byte("file content"))
	message := gomail.NewFullEmailMessage(
		"from@example.com",
		[]string{"to1@example.com", "to2@example.com"},
		"Test Subject",
		[]string{"cc@example.com"},
		[]string{"bcc@example.com"},
		"reply@example.com",
		"Plain text body",
		"<p>HTML body</p>",
		[]gomail.Attachment{*attachment},
	)

	err := provider.SendEmail(context.Background(), message)

	require.NoError(t, err)
	sent := fake.lastEmail
	assert.Equal(t, "from@example.com", sent.From)
	assert.Equal(t, "to1@example.com,to2@example.com", sent.To)
	assert.Equal(t, "cc@example.com", sent.Cc)
	assert.Equal(t, "bcc@example.com", sent.Bcc)
	assert.Equal(t, "Test Subject", sent.Subject)
	assert.Equal(t, "Plain text body", sent.TextBody)
	assert.Equal(t, "<p>HTML body</p>", sent.HTMLBody)
	assert.Equal(t, "reply@example.com", sent.ReplyTo)
	assert.Equal(t, "outbound", sent.MessageStream)

	require.Len(t, sent.Attachments, 1)
	att := sent.Attachments[0]
	assert.Equal(t, "report.pdf", att.Name)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("file content")), att.Content)
	assert.Equal(t, attachmentContentType("report.pdf"), att.ContentType)
	assert.NotEmpty(t, att.ContentType)
}

func TestPostmarkEmailProvider_SendEmail_OmitsEmptyOptionalFields(t *testing.T) {
	fake := &fakePostmarkSender{}
	provider := &PostmarkEmailProvider{sender: fake, messageStream: "outbound"}

	message := gomail.NewFullEmailMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Subject", nil, nil, "", "text", "<p>html</p>", nil,
	)

	err := provider.SendEmail(context.Background(), message)

	require.NoError(t, err)
	assert.Empty(t, fake.lastEmail.Cc)
	assert.Empty(t, fake.lastEmail.Bcc)
	assert.Empty(t, fake.lastEmail.ReplyTo)
	assert.Empty(t, fake.lastEmail.Attachments)
}

func TestPostmarkEmailProvider_SendEmail_ContextPropagation(t *testing.T) {
	fake := &fakePostmarkSender{}
	provider := &PostmarkEmailProvider{sender: fake, messageStream: "outbound"}
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

func TestPostmarkEmailProvider_SendEmail_Error(t *testing.T) {
	sendErr := errors.New("postmark api unavailable")
	provider := &PostmarkEmailProvider{
		sender:        &fakePostmarkSender{sendErr: sendErr},
		messageStream: "outbound",
	}

	message := gomail.NewFullEmailMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Test Subject", nil, nil, "", "text", "<p>html</p>", nil,
	)

	err := provider.SendEmail(context.Background(), message)

	require.Error(t, err)
	assert.ErrorIs(t, err, sendErr, "underlying error must be wrapped with %w")
}

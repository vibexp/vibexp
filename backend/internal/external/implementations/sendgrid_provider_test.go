package implementations

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/darkrockmountain/gomail"
	"github.com/sendgrid/rest"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
)

// fakeSendgridSender implements the sendgridSender interface without making
// network calls. It records the last call and returns a configurable response.
type fakeSendgridSender struct {
	resp        *rest.Response
	sendErr     error
	lastCtx     context.Context
	lastMessage *mail.SGMailV3
}

func (f *fakeSendgridSender) SendWithContext(
	ctx context.Context, email *mail.SGMailV3,
) (*rest.Response, error) {
	f.lastCtx = ctx
	f.lastMessage = email
	return f.resp, f.sendErr
}

func okResponse() *rest.Response {
	return &rest.Response{StatusCode: 202, Body: ""}
}

func TestNewSendGridEmailProvider_EmptyAPIKey(t *testing.T) {
	cfg := &config.Config{
		SendGridAPIKey: "",
	}

	provider, err := NewSendGridEmailProvider(cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SENDGRID_API_KEY")
	assert.Nil(t, provider)
}

func TestNewSendGridEmailProvider_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		SendGridAPIKey: "test-sendgrid-key",
	}

	provider, err := NewSendGridEmailProvider(cfg)

	require.NoError(t, err)
	assert.NotNil(t, provider)

	_, ok := provider.(*SendGridEmailProvider)
	assert.True(t, ok, "Provider should be of type *SendGridEmailProvider")
}

func TestNewSendGridEmailProvider_InterfaceCompliance(t *testing.T) {
	var _ external.EmailProvider = (*SendGridEmailProvider)(nil)
}

func TestSendGridEmailProvider_SendEmail_MapsAllFields(t *testing.T) {
	fake := &fakeSendgridSender{resp: okResponse()}
	provider := &SendGridEmailProvider{sender: fake}

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
	sent := fake.lastMessage
	require.NotNil(t, sent)

	require.NotNil(t, sent.From)
	assert.Equal(t, "from@example.com", sent.From.Address)
	assert.Equal(t, "Test Subject", sent.Subject)

	require.Len(t, sent.Personalizations, 1)
	p := sent.Personalizations[0]
	require.Len(t, p.To, 2)
	assert.Equal(t, "to1@example.com", p.To[0].Address)
	assert.Equal(t, "to2@example.com", p.To[1].Address)
	require.Len(t, p.CC, 1)
	assert.Equal(t, "cc@example.com", p.CC[0].Address)
	require.Len(t, p.BCC, 1)
	assert.Equal(t, "bcc@example.com", p.BCC[0].Address)

	// SendGrid requires plain text before HTML.
	require.Len(t, sent.Content, 2)
	assert.Equal(t, "text/plain", sent.Content[0].Type)
	assert.Equal(t, "Plain text body", sent.Content[0].Value)
	assert.Equal(t, "text/html", sent.Content[1].Type)
	assert.Equal(t, "<p>HTML body</p>", sent.Content[1].Value)

	require.NotNil(t, sent.ReplyTo)
	assert.Equal(t, "reply@example.com", sent.ReplyTo.Address)

	require.Len(t, sent.Attachments, 1)
	att := sent.Attachments[0]
	assert.Equal(t, "report.pdf", att.Filename)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("file content")), att.Content)
	assert.Equal(t, attachmentContentType("report.pdf"), att.Type)
	assert.NotEmpty(t, att.Type)
	assert.Equal(t, "attachment", att.Disposition)
}

func TestSendGridEmailProvider_SendEmail_OmitsEmptyOptionalParts(t *testing.T) {
	fake := &fakeSendgridSender{resp: okResponse()}
	provider := &SendGridEmailProvider{sender: fake}

	message := gomail.NewFullEmailMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Subject", nil, nil, "", "Plain only", "", nil,
	)

	err := provider.SendEmail(context.Background(), message)

	require.NoError(t, err)
	sent := fake.lastMessage
	require.Len(t, sent.Content, 1, "no HTML body means a single content part")
	assert.Equal(t, "text/plain", sent.Content[0].Type)
	assert.Nil(t, sent.ReplyTo)
	assert.Empty(t, sent.Attachments)
	assert.Empty(t, sent.Personalizations[0].CC)
	assert.Empty(t, sent.Personalizations[0].BCC)
}

func TestSendGridEmailProvider_SendEmail_ContextPropagation(t *testing.T) {
	fake := &fakeSendgridSender{resp: okResponse()}
	provider := &SendGridEmailProvider{sender: fake}
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

func TestSendGridEmailProvider_SendEmail_TransportError(t *testing.T) {
	sendErr := errors.New("sendgrid network unreachable")
	provider := &SendGridEmailProvider{
		sender: &fakeSendgridSender{sendErr: sendErr},
	}

	message := gomail.NewFullEmailMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Test Subject", nil, nil, "", "text", "<p>html</p>", nil,
	)

	err := provider.SendEmail(context.Background(), message)

	require.Error(t, err)
	assert.ErrorIs(t, err, sendErr, "underlying transport error must be wrapped with %w")
}

func TestSendGridEmailProvider_SendEmail_Non2xxStatus(t *testing.T) {
	provider := &SendGridEmailProvider{
		sender: &fakeSendgridSender{
			resp: &rest.Response{StatusCode: 401, Body: "unauthorized"},
		},
	}

	message := gomail.NewFullEmailMessage(
		"from@example.com",
		[]string{"to@example.com"},
		"Test Subject", nil, nil, "", "text", "<p>html</p>", nil,
	)

	err := provider.SendEmail(context.Background(), message)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
	assert.Contains(t, err.Error(), "unauthorized")
}

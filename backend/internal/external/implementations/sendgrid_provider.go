package implementations

import (
	"context"
	"fmt"
	"net/http"

	"github.com/darkrockmountain/gomail"
	"github.com/sendgrid/rest"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
)

// sendgridSender is the subset of sendgrid-go used by SendGridEmailProvider.
// Defined as an interface so tests can inject a fake without making network
// calls. *sendgrid.Client satisfies it.
type sendgridSender interface {
	SendWithContext(ctx context.Context, email *mail.SGMailV3) (*rest.Response, error)
}

// SendGridEmailProvider implements external.EmailProvider using the SendGrid v3
// Mail Send API. It uses SendWithContext so the caller's context.Context is
// propagated to the HTTP layer.
type SendGridEmailProvider struct {
	sender sendgridSender
}

// NewSendGridEmailProvider creates a new SendGrid email provider.
// Required config: SendGridAPIKey (an API key with "Mail Send" permission).
func NewSendGridEmailProvider(cfg *config.Config) (external.EmailProvider, error) {
	if cfg.Email.SendGrid.APIKey == "" {
		return nil, fmt.Errorf("sendgrid provider: SENDGRID_API_KEY is required")
	}

	return &SendGridEmailProvider{sender: sendgrid.NewSendClient(cfg.Email.SendGrid.APIKey)}, nil
}

// SendEmail sends an email via the SendGrid v3 Mail Send API. The caller's ctx
// controls cancellation and deadline propagation through to the HTTP request.
func (p *SendGridEmailProvider) SendEmail(ctx context.Context, message *gomail.EmailMessage) error {
	m := mail.NewV3Mail()
	m.SetFrom(mail.NewEmail("", message.GetFrom()))
	m.Subject = message.GetSubject()

	personalization := mail.NewPersonalization()
	for _, to := range message.GetTo() {
		personalization.AddTos(mail.NewEmail("", to))
	}
	for _, cc := range message.GetCC() {
		personalization.AddCCs(mail.NewEmail("", cc))
	}
	for _, bcc := range message.GetBCC() {
		personalization.AddBCCs(mail.NewEmail("", bcc))
	}
	m.AddPersonalizations(personalization)

	// SendGrid requires the plain-text part to precede the HTML part.
	if text := message.GetText(); text != "" {
		m.AddContent(mail.NewContent("text/plain", text))
	}
	if html := message.GetHTML(); html != "" {
		m.AddContent(mail.NewContent("text/html", html))
	}

	if replyTo := message.GetReplyTo(); replyTo != "" {
		m.SetReplyTo(mail.NewEmail("", replyTo))
	}

	for _, attachment := range message.GetAttachments() {
		a := mail.NewAttachment()
		a.SetContent(attachment.GetBase64StringContent())
		a.SetFilename(attachment.GetFilename())
		a.SetType(attachmentContentType(attachment.GetFilename()))
		a.SetDisposition("attachment")
		m.AddAttachment(a)
	}

	resp, err := p.sender.SendWithContext(ctx, m)
	if err != nil {
		return fmt.Errorf("sendgrid: failed to send email: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf(
			"sendgrid: failed to send email: unexpected status %d: %s",
			resp.StatusCode, resp.Body,
		)
	}

	return nil
}

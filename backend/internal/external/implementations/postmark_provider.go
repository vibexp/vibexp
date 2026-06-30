package implementations

import (
	"context"
	"fmt"
	"strings"

	"github.com/darkrockmountain/gomail"
	"github.com/mrz1836/postmark"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
)

// defaultPostmarkMessageStream is Postmark's default transactional stream,
// used when POSTMARK_MESSAGE_STREAM is not set.
const defaultPostmarkMessageStream = "outbound"

// postmarkSender is the subset of the Postmark client used by
// PostmarkEmailProvider. Defined as an interface so tests can inject a fake
// without making network calls.
type postmarkSender interface {
	SendEmail(ctx context.Context, email postmark.Email) (postmark.EmailResponse, error)
}

// PostmarkEmailProvider implements external.EmailProvider using the Postmark
// Email API. The caller's context.Context is propagated through to the HTTP
// request, and outbound mail is sent on the configured message stream.
type PostmarkEmailProvider struct {
	sender        postmarkSender
	messageStream string
}

// NewPostmarkEmailProvider creates a new Postmark email provider.
// Required config: PostmarkServerToken.
// Optional: PostmarkMessageStream — the Postmark message stream to send on
// (defaults to "outbound", the default transactional stream).
func NewPostmarkEmailProvider(cfg *config.Config) (external.EmailProvider, error) {
	if cfg.Email.Postmark.ServerToken == "" {
		return nil, fmt.Errorf("postmark provider: POSTMARK_SERVER_TOKEN is required")
	}

	stream := strings.TrimSpace(cfg.Email.Postmark.MessageStream)
	if stream == "" {
		stream = defaultPostmarkMessageStream
	}

	// Postmark separates the server token (used for sending) from the optional
	// account token (used for account-level APIs); only the server token is
	// needed to deliver email.
	client := postmark.NewClient(cfg.Email.Postmark.ServerToken, "")

	return &PostmarkEmailProvider{sender: client, messageStream: stream}, nil
}

// SendEmail sends an email via the Postmark API. The caller's ctx controls
// cancellation and deadline propagation through to the HTTP request.
func (p *PostmarkEmailProvider) SendEmail(ctx context.Context, message *gomail.EmailMessage) error {
	email := postmark.Email{
		From:          message.GetFrom(),
		To:            strings.Join(message.GetTo(), ","),
		Subject:       message.GetSubject(),
		TextBody:      message.GetText(),
		HTMLBody:      message.GetHTML(),
		ReplyTo:       message.GetReplyTo(),
		MessageStream: p.messageStream,
	}

	if cc := message.GetCC(); len(cc) > 0 {
		email.Cc = strings.Join(cc, ",")
	}

	if bcc := message.GetBCC(); len(bcc) > 0 {
		email.Bcc = strings.Join(bcc, ",")
	}

	for _, attachment := range message.GetAttachments() {
		email.Attachments = append(email.Attachments, postmark.Attachment{
			Name:        attachment.GetFilename(),
			Content:     attachment.GetBase64StringContent(),
			ContentType: attachmentContentType(attachment.GetFilename()),
		})
	}

	// SendEmail already inspects the response ErrorCode and returns an error
	// for API-level failures, so any non-nil error is terminal.
	if _, err := p.sender.SendEmail(ctx, email); err != nil {
		return fmt.Errorf("postmark: failed to send email: %w", err)
	}

	return nil
}

package implementations

import (
	"context"
	"fmt"
	"strings"

	"github.com/darkrockmountain/gomail"
	"github.com/mailgun/mailgun-go/v4"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
)

// mailgunSender is the subset of mailgun-go used by MailgunEmailProvider.
// Defined as an interface so tests can inject a fake without making network calls.
type mailgunSender interface {
	NewMessage(from, subject, text string, to ...string) *mailgun.Message
	Send(ctx context.Context, m *mailgun.Message) (string, string, error)
}

// MailgunEmailProvider implements external.EmailProvider using the Mailgun API.
// It calls mailgun-go directly (rather than via gomail's wrapper) so the caller's
// context.Context is propagated to the HTTP layer and the API base URL can be
// configured for EU vs US regions.
type MailgunEmailProvider struct {
	sender mailgunSender
}

// NewMailgunEmailProvider creates a new Mailgun email provider.
// Required config: MailgunSendingKey, MailgunDomain.
// Optional: MailgunBaseURL — when set, overrides the default US endpoint
// (e.g. "https://api.eu.mailgun.net/v3" for EU customers). The version suffix
// is normalized — if MailgunBaseURL is set without a /v2|/v3|/v4 suffix, /v3
// is appended automatically.
// MailgunDomain must be a bare domain (e.g. mg.example.com), not a URL.
func NewMailgunEmailProvider(cfg *config.Config) (external.EmailProvider, error) {
	if cfg.MailgunSendingKey == "" {
		return nil, fmt.Errorf("mailgun provider: MAILGUN_SENDING_KEY is required")
	}

	if cfg.MailgunDomain == "" {
		return nil, fmt.Errorf("mailgun provider: MAILGUN_DOMAIN is required")
	}

	if strings.Contains(cfg.MailgunDomain, "://") || strings.HasSuffix(cfg.MailgunDomain, "/") {
		return nil, fmt.Errorf(
			"mailgun provider: MAILGUN_DOMAIN must be a bare domain (e.g. mg.example.com), not a URL; got %q",
			cfg.MailgunDomain,
		)
	}

	mg := mailgun.NewMailgun(cfg.MailgunDomain, cfg.MailgunSendingKey)
	if base := normalizeMailgunBaseURL(cfg.MailgunBaseURL); base != "" {
		mg.SetAPIBase(base)
	}

	return &MailgunEmailProvider{sender: mg}, nil
}

// normalizeMailgunBaseURL ensures the configured base URL ends with a
// supported Mailgun API version suffix (/v2, /v3, /v4). If the suffix is
// missing, /v3 is appended (Mailgun's stable default at time of writing).
// An empty input returns empty so the mailgun-go library default is used.
//
// Why: mailgun-go validates the suffix only at send time, so a misconfigured
// MAILGUN_BASE_URL (e.g. "https://api.eu.mailgun.net" without "/v3") fails
// at runtime instead of at startup, silently breaking outbound email.
func normalizeMailgunBaseURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, "/v2") ||
		strings.HasSuffix(trimmed, "/v3") ||
		strings.HasSuffix(trimmed, "/v4") {
		return trimmed
	}
	return trimmed + "/v3"
}

// SendEmail sends an email via the Mailgun API. The caller's ctx controls
// cancellation and deadline propagation through to the HTTP request.
func (p *MailgunEmailProvider) SendEmail(ctx context.Context, message *gomail.EmailMessage) error {
	mgMessage := p.sender.NewMessage(
		message.GetFrom(),
		message.GetSubject(),
		message.GetText(),
		message.GetTo()...,
	)

	if html := message.GetHTML(); html != "" {
		mgMessage.SetHTML(html)
	}

	for _, cc := range message.GetCC() {
		mgMessage.AddCC(cc)
	}

	for _, bcc := range message.GetBCC() {
		mgMessage.AddBCC(bcc)
	}

	if replyTo := message.GetReplyTo(); replyTo != "" {
		mgMessage.SetReplyTo(replyTo)
	}

	for _, attachment := range message.GetAttachments() {
		mgMessage.AddBufferAttachment(attachment.GetFilename(), attachment.GetRawContent())
	}

	if _, _, err := p.sender.Send(ctx, mgMessage); err != nil {
		return fmt.Errorf("mailgun: failed to send email: %w", err)
	}

	return nil
}

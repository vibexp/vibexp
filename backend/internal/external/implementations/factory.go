package implementations

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vibexp/vibexp/internal/external"
)

// SMTPSpec holds the values needed to build an SMTP email provider. Port is a
// string (not an int) because it mirrors the configured value verbatim; parsing
// and its error belong to NewSMTPEmailProvider.
type SMTPSpec struct {
	Host     string
	Port     string
	Username string
	Password string
}

// MailgunSpec holds the values needed to build a Mailgun email provider.
// Domain and SendingKey are required; BaseURL is optional and overrides the
// default US endpoint (e.g. the EU endpoint).
type MailgunSpec struct {
	BaseURL    string
	Domain     string
	SendingKey string
}

// PostmarkSpec holds the values needed to build a Postmark email provider.
// ServerToken is required; MessageStream is optional and defaults to
// Postmark's "outbound" transactional stream.
type PostmarkSpec struct {
	ServerToken   string
	MessageStream string
}

// SendGridSpec holds the values needed to build a SendGrid email provider.
type SendGridSpec struct {
	APIKey string
}

// ProviderSpec fully describes which email provider to build and with which
// credentials, with no reference to *config.Config.
//
// Decoupling provider construction from the config type is the point: the same
// switch can then be driven by process-wide config at container build time (see
// providers.ProvideEmailProvider) or by per-team values decrypted from the
// database at send time. Only the sub-spec matching Type is read.
type ProviderSpec struct {
	// Type selects the provider: "smtp" (the default), "mailgun", "postmark"
	// or "sendgrid". Matching is case-insensitive and whitespace-tolerant, and
	// an empty value means "smtp".
	Type string

	SMTP     SMTPSpec
	Mailgun  MailgunSpec
	Postmark PostmarkSpec
	SendGrid SendGridSpec
}

// NewEmailProvider builds the email provider described by spec.
//
// When Type is "smtp" or empty and no SMTP host or port is given, a no-op
// StubEmailProvider is returned rather than an error, so a deployment can wire
// up (and a team can exist) without email credentials. An unrecognised Type is
// an error.
func NewEmailProvider(spec ProviderSpec, logger *slog.Logger) (external.EmailProvider, error) {
	switch strings.ToLower(strings.TrimSpace(spec.Type)) {
	case "mailgun":
		provider, err := NewMailgunEmailProvider(spec.Mailgun)
		if err != nil {
			return nil, fmt.Errorf("email provider factory: %w", err)
		}
		return provider, nil
	case "postmark":
		provider, err := NewPostmarkEmailProvider(spec.Postmark)
		if err != nil {
			return nil, fmt.Errorf("email provider factory: %w", err)
		}
		return provider, nil
	case "sendgrid":
		provider, err := NewSendGridEmailProvider(spec.SendGrid)
		if err != nil {
			return nil, fmt.Errorf("email provider factory: %w", err)
		}
		return provider, nil
	case "smtp", "":
		if spec.SMTP.Host == "" || spec.SMTP.Port == "" {
			// A silent no-op sender is easy to mistake for a delivery bug, so
			// leave a trace of why nothing was sent.
			logger.Debug("email provider factory: no SMTP host/port configured, using no-op stub")
			return &StubEmailProvider{}, nil
		}
		provider, err := NewSMTPEmailProvider(spec.SMTP)
		if err != nil {
			return nil, fmt.Errorf("email provider factory: %w", err)
		}
		return provider, nil
	default:
		// Report the value as given, not the normalised one, so the operator
		// sees what they configured.
		return nil, fmt.Errorf("email provider factory: unknown email provider %q", spec.Type)
	}
}

// ProviderLabel names the provider that was actually built, for logging.
//
// It reads the concrete type rather than re-deriving the choice from a spec, so
// it cannot disagree with NewEmailProvider about which provider is in use, and
// it reports "stub" for the credential-less fallback — which a spec alone does
// not distinguish from "smtp".
//
// Keep the cases below in step with NewEmailProvider's switch above: a provider
// added there but not here logs as "unknown".
func ProviderLabel(provider external.EmailProvider) string {
	switch provider.(type) {
	case *MailgunEmailProvider:
		return "mailgun"
	case *PostmarkEmailProvider:
		return "postmark"
	case *SendGridEmailProvider:
		return "sendgrid"
	case *SMTPEmailProvider:
		return "smtp"
	case *StubEmailProvider:
		return "stub"
	default:
		return "unknown"
	}
}

// StubEmailProvider is a no-op provider used when email is not configured.
// It accepts and discards every message.
type StubEmailProvider struct{}

// SendEmail discards the message and reports success.
func (s *StubEmailProvider) SendEmail(_ context.Context, _ *external.OutgoingMessage) error {
	return nil
}

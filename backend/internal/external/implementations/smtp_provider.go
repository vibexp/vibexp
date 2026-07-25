package implementations

import (
	"context"
	"fmt"
	"strconv"

	"github.com/darkrockmountain/gomail"
	gomailsmtp "github.com/darkrockmountain/gomail/providers/smtp"

	"github.com/vibexp/vibexp/internal/external"
)

// SMTPEmailProvider implements the EmailProvider interface using gomail's SMTP provider
type SMTPEmailProvider struct {
	sender interface {
		SendEmail(*gomail.EmailMessage) error
	}
}

// NewSMTPEmailProvider creates a new SMTP email provider using gomail library
func NewSMTPEmailProvider(spec SMTPSpec) (external.EmailProvider, error) {
	port, err := strconv.Atoi(spec.Port)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP port: %w", err)
	}

	sender, err := gomailsmtp.NewSmtpEmailSender(
		spec.Host,
		port,
		spec.Username,
		spec.Password,
		gomailsmtp.AUTH_PLAIN,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create SMTP sender: %w", err)
	}

	return &SMTPEmailProvider{sender: sender}, nil
}

// SendEmail sends an email using the gomail SMTP provider
func (p *SMTPEmailProvider) SendEmail(ctx context.Context, message *gomail.EmailMessage) error {
	// Note: gomail's SMTP sender doesn't use context yet, but we accept it for future compatibility
	return p.sender.SendEmail(message)
}

package implementations

import (
	"context"
	"fmt"
	"strconv"

	"github.com/darkrockmountain/gomail"
	gomailsmtp "github.com/darkrockmountain/gomail/providers/smtp"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
)

// SMTPEmailProvider implements the EmailProvider interface using gomail's SMTP provider
type SMTPEmailProvider struct {
	sender interface {
		SendEmail(*gomail.EmailMessage) error
	}
}

// NewSMTPEmailProvider creates a new SMTP email provider using gomail library
func NewSMTPEmailProvider(cfg *config.Config) (external.EmailProvider, error) {
	port, err := strconv.Atoi(cfg.SMTPPort)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP port: %w", err)
	}

	sender, err := gomailsmtp.NewSmtpEmailSender(
		cfg.SMTPHost,
		port,
		cfg.SMTPUsername,
		cfg.SMTPPassword,
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

package implementations

import (
	"bytes"
	"context"
	"fmt"
	"net/smtp"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
)

// EmailSenderImpl implements the EmailSender interface
type EmailSenderImpl struct {
	config *config.Config
}

// NewEmailSender creates a new SMTP client
func NewEmailSender(cfg *config.Config) external.EmailSender {
	return &EmailSenderImpl{
		config: cfg,
	}
}

// SendEmail sends an email using SMTP
func (s *EmailSenderImpl) SendEmail(ctx context.Context, req *external.EmailRequest) error {
	var buffer bytes.Buffer

	// Build email headers
	s.buildHeaders(&buffer, req)

	// Build email body
	s.buildBody(&buffer, req)

	// Send the email
	return s.sendMail(req, buffer.Bytes())
}

// buildHeaders constructs the email headers
func (s *EmailSenderImpl) buildHeaders(buffer *bytes.Buffer, req *external.EmailRequest) {
	buffer.WriteString("From: " + req.From + "\r\n")

	if len(req.To) > 0 {
		buffer.WriteString("To: ")
		for i, to := range req.To {
			if i > 0 {
				buffer.WriteString(", ")
			}
			buffer.WriteString(to)
		}
		buffer.WriteString("\r\n")
	}

	buffer.WriteString("Subject: " + req.Subject + "\r\n")

	if req.ReplyTo != "" {
		buffer.WriteString("Reply-To: " + req.ReplyTo + "\r\n")
	}

	buffer.WriteString("MIME-Version: 1.0\r\n")
}

// buildBody constructs the email body based on content type
func (s *EmailSenderImpl) buildBody(buffer *bytes.Buffer, req *external.EmailRequest) {
	hasHTML := req.HTMLBody != ""
	hasText := req.TextBody != ""

	switch {
	case hasHTML && hasText:
		s.buildMultipartBody(buffer, req)
	case hasHTML:
		s.buildHTMLBody(buffer, req)
	default:
		s.buildTextBody(buffer, req)
	}
}

// buildMultipartBody creates a multipart email with both HTML and text
func (s *EmailSenderImpl) buildMultipartBody(buffer *bytes.Buffer, req *external.EmailRequest) {
	boundary := "boundary123456789"
	buffer.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n")
	buffer.WriteString("\r\n")

	// Text part
	buffer.WriteString("--" + boundary + "\r\n")
	buffer.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buffer.WriteString("\r\n")
	buffer.WriteString(req.TextBody)
	buffer.WriteString("\r\n")

	// HTML part
	buffer.WriteString("--" + boundary + "\r\n")
	buffer.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buffer.WriteString("\r\n")
	buffer.WriteString(req.HTMLBody)
	buffer.WriteString("\r\n")

	buffer.WriteString("--" + boundary + "--\r\n")
}

// buildHTMLBody creates an HTML-only email
func (s *EmailSenderImpl) buildHTMLBody(buffer *bytes.Buffer, req *external.EmailRequest) {
	buffer.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buffer.WriteString("\r\n")
	buffer.WriteString(req.HTMLBody)
}

// buildTextBody creates a text-only email
func (s *EmailSenderImpl) buildTextBody(buffer *bytes.Buffer, req *external.EmailRequest) {
	buffer.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buffer.WriteString("\r\n")
	buffer.WriteString(req.TextBody)
}

// sendMail sends the email using SMTP
func (s *EmailSenderImpl) sendMail(req *external.EmailRequest, message []byte) error {
	auth := smtp.PlainAuth(
		"",
		s.config.Email.SMTP.Username,
		s.config.Email.SMTP.Password,
		s.config.Email.SMTP.Host,
	)

	smtpAddr := s.config.Email.SMTP.Host + ":" + s.config.Email.SMTP.Port
	err := smtp.SendMail(smtpAddr, auth, req.From, req.To, message)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

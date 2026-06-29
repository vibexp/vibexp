package services

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/darkrockmountain/gomail"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
)

//go:embed templates/email/*.html templates/email/*.txt
var templateFS embed.FS

type EmailService struct {
	provider external.EmailProvider
	cfg      *config.Config
}

// Ensure EmailService implements EmailServiceInterface
var _ EmailServiceInterface = (*EmailService)(nil)

func NewEmailService(provider external.EmailProvider, cfg *config.Config) *EmailService {
	return &EmailService{
		provider: provider,
		cfg:      cfg,
	}
}

// adminRecipient resolves the destination for support
// notification emails. It prefers the explicitly configured
// ContactRecipientAddress, falling back to the sender address
// (EmailFromAddress, then SMTPUsername) so a single-mailbox deployment works
// without extra wiring.
func (es *EmailService) adminRecipient() string {
	if es.cfg.ContactRecipientAddress != "" {
		return es.cfg.ContactRecipientAddress
	}
	if es.cfg.EmailFromAddress != "" {
		return es.cfg.EmailFromAddress
	}
	return es.cfg.SMTPUsername
}

// appBaseURL returns the configured frontend base URL with any trailing slash
// removed so it can be safely concatenated with template paths
// (e.g. "<base>/settings/notifications").
func (es *EmailService) appBaseURL() string {
	return strings.TrimRight(es.cfg.FrontendBaseURL, "/")
}

// SendSupportRequest sends a support request from an authenticated user
func (es *EmailService) SendSupportRequest(userName, userEmail string, req *models.SupportRequest) error {
	// Send notification email to the configured admin recipient.
	if err := es.sendSupportNotificationToAdmin(userName, userEmail, req); err != nil {
		return fmt.Errorf("failed to send admin notification: %w", err)
	}

	// Send acknowledgement to user if requested
	if req.Acknowledgement {
		if err := es.sendSupportAcknowledgement(userName, userEmail, req); err != nil {
			// Log but don't fail - admin notification was sent
			slog.With("error", err).Warn("Failed to send acknowledgement email")
		}
	}

	return nil
}

func (es *EmailService) sendEmail(to, subject, htmlBody, textBody string) error {
	// Resolve the from address: prefer EmailFromAddress, fall back to SMTPUsername
	// for backwards compatibility when EMAIL_PROVIDER=smtp.
	from := es.cfg.EmailFromAddress
	if from == "" {
		from = es.cfg.SMTPUsername
	}

	// Build gomail EmailMessage
	message := gomail.NewFullEmailMessage(
		from,         // from
		[]string{to}, // to
		subject,
		nil, // cc
		nil, // bcc
		"",  // replyTo
		textBody,
		htmlBody,
		nil, // attachments
	)

	// Send via provider
	ctx := context.Background()
	err := es.provider.SendEmail(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	es.logEmailSent(to, subject, htmlBody, textBody)

	return nil
}

// logEmailSent logs successful email sending
func (es *EmailService) logEmailSent(to, subject, htmlBody, textBody string) {
	slog.With(
		"to", to,
		"subject", subject,
		"email_backend", es.cfg.EmailProvider,
		"text_body", textBody != "",
		"html_body", htmlBody != "",
		"multipart", textBody != "" && htmlBody != "",
	).Info("Email sent successfully")
}

func (es *EmailService) sendSupportNotificationToAdmin(userName, userEmail string, req *models.SupportRequest) error {
	subject := fmt.Sprintf("New Support Request from %s", userEmail)

	// Build additional info for both HTML and text
	additionalInfoHTML, additionalInfoText := es.buildAdditionalInfo(req.AdditionalInfo)

	// Prepare template data
	data := struct {
		UserName           string
		UserEmail          string
		Text               string
		Acknowledgement    bool
		AdditionalInfoHTML string
		AdditionalInfoText string
		Year               int
		AppBaseURL         string
		PrivacyPolicyURL   string
	}{
		UserName:           userName,
		UserEmail:          userEmail,
		Text:               req.Text,
		Acknowledgement:    req.Acknowledgement,
		AdditionalInfoHTML: additionalInfoHTML,
		AdditionalInfoText: additionalInfoText,
		Year:               2025,
		AppBaseURL:         es.appBaseURL(),
		PrivacyPolicyURL:   es.cfg.PrivacyPolicyURL,
	}

	// Render HTML template
	htmlBody, err := es.renderTemplateFromFS(
		"templates/email/base.html",
		"templates/email/support-notification.html",
		data,
	)
	if err != nil {
		return fmt.Errorf("failed to render HTML template: %w", err)
	}

	// Render text template
	textBody, err := es.renderTextTemplateFromFS(
		"templates/email/support-notification.txt",
		data,
	)
	if err != nil {
		return fmt.Errorf("failed to render text template: %w", err)
	}

	return es.sendEmail(es.adminRecipient(), subject, htmlBody, textBody)
}

func (es *EmailService) sendSupportAcknowledgement(userName, userEmail string, req *models.SupportRequest) error {
	subject := "Thank you for contacting VibeXP Support"

	// Extract first name from full name
	firstName := es.extractFirstName(userName)

	// Prepare template data
	data := struct {
		FirstName        string
		Text             string
		Year             int
		AppBaseURL       string
		PrivacyPolicyURL string
	}{
		FirstName:        firstName,
		Text:             req.Text,
		Year:             2025,
		AppBaseURL:       es.appBaseURL(),
		PrivacyPolicyURL: es.cfg.PrivacyPolicyURL,
	}

	// Render HTML template
	htmlBody, err := es.renderTemplateFromFS(
		"templates/email/base.html",
		"templates/email/support-acknowledgement.html",
		data,
	)
	if err != nil {
		return fmt.Errorf("failed to render HTML template: %w", err)
	}

	// Render text template
	textBody, err := es.renderTextTemplateFromFS(
		"templates/email/support-acknowledgement.txt",
		data,
	)
	if err != nil {
		return fmt.Errorf("failed to render text template: %w", err)
	}

	return es.sendEmail(userEmail, subject, htmlBody, textBody)
}

// SendNotificationEmail sends a transactional notification email to the given address
func (es *EmailService) SendNotificationEmail(to, subject, htmlBody string) error {
	return es.sendEmail(to, subject, htmlBody, "")
}

// SendTeamInvitation sends a team invitation email
func (es *EmailService) SendTeamInvitation(invitation *models.TeamInvitation, teamName, inviterName string) error {
	subject := fmt.Sprintf("You have been invited to join %s on VibeXP", teamName)

	// Build accept URL
	acceptURL := fmt.Sprintf("%s/invitations/accept/%s", es.cfg.FrontendBaseURL, invitation.Token)

	// Prepare template data
	data := struct {
		TeamName         string
		InviterName      string
		Role             string
		AcceptURL        string
		ExpiryDate       string
		Year             int
		AppBaseURL       string
		PrivacyPolicyURL string
	}{
		TeamName:         teamName,
		InviterName:      inviterName,
		Role:             string(invitation.Role),
		AcceptURL:        acceptURL,
		ExpiryDate:       invitation.ExpiresAt.Format("January 2, 2006"),
		Year:             time.Now().Year(),
		AppBaseURL:       es.appBaseURL(),
		PrivacyPolicyURL: es.cfg.PrivacyPolicyURL,
	}

	// Render HTML template
	htmlBody, err := es.renderTemplateFromFS(
		"templates/email/base.html",
		"templates/email/team-invitation.html",
		data,
	)
	if err != nil {
		return fmt.Errorf("failed to render HTML template: %w", err)
	}

	// Render text template
	textBody, err := es.renderTextTemplateFromFS(
		"templates/email/team-invitation.txt",
		data,
	)
	if err != nil {
		return fmt.Errorf("failed to render text template: %w", err)
	}

	return es.sendEmail(invitation.InviteeEmail, subject, htmlBody, textBody)
}

// renderTemplateFromFS renders an HTML template from the embedded filesystem
//
//nolint:unparam // basePath is always the same but keeping parameter for potential future flexibility
func (es *EmailService) renderTemplateFromFS(basePath, contentPath string, data interface{}) (string, error) {
	// Read base template
	baseContent, err := templateFS.ReadFile(basePath)
	if err != nil {
		return "", fmt.Errorf("failed to read base template: %w", err)
	}

	// Read content template
	contentContent, err := templateFS.ReadFile(contentPath)
	if err != nil {
		return "", fmt.Errorf("failed to read content template: %w", err)
	}

	// Parse and execute templates
	tmpl, err := template.New("email").Parse(string(baseContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse base template: %w", err)
	}

	tmpl, err = tmpl.Parse(string(contentContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse content template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// renderTextTemplateFromFS renders a text template from the embedded filesystem
func (es *EmailService) renderTextTemplateFromFS(templatePath string, data interface{}) (string, error) {
	// Read template
	content, err := templateFS.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template: %w", err)
	}

	// Parse and execute template
	tmpl, err := template.New("text").Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func (es *EmailService) buildAdditionalInfo(additionalInfo map[string]string) (html, text string) {
	if len(additionalInfo) == 0 {
		return "", ""
	}

	// Build HTML version
	htmlBuf := &strings.Builder{}
	htmlBuf.WriteString(
		"<p><strong>Additional Information:</strong></p><ul style=\"list-style-type: none; padding: 0;\">",
	)
	for key, value := range additionalInfo {
		fmt.Fprintf(htmlBuf,
			"<li style=\"margin-bottom: 8px;\"><strong>%s:</strong> %s</li>\n",
			key, value,
		)
	}
	htmlBuf.WriteString("</ul>")
	html = htmlBuf.String()

	// Build text version
	textBuf := &strings.Builder{}
	textBuf.WriteString("ADDITIONAL INFORMATION:\n")
	textBuf.WriteString("-----------------------\n")
	for key, value := range additionalInfo {
		fmt.Fprintf(textBuf, "%s: %s\n", key, value)
	}
	text = strings.TrimRight(textBuf.String(), "\n")

	return html, text
}

func (es *EmailService) extractFirstName(fullName string) string {
	if fullName == "" {
		return "there"
	}

	// Split by space and take the first part
	parts := strings.Fields(fullName)
	if len(parts) > 0 {
		return parts[0]
	}

	return fullName
}

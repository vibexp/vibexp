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

// emailBaseTemplate is the shared layout template every HTML email renders inside.
const emailBaseTemplate = "templates/email/base.html"

type EmailService struct {
	// resolver decides, per send, which provider and sender identity to use: the
	// owning team's when it has configured one, otherwise the instance's. There is
	// no directly injected provider any more — every send goes through here, so a
	// team's configuration cannot be bypassed.
	resolver EmailSenderResolver
	cfg      *config.Config
}

// Ensure EmailService implements EmailServiceInterface
var _ EmailServiceInterface = (*EmailService)(nil)

func NewEmailService(resolver EmailSenderResolver, cfg *config.Config) *EmailService {
	return &EmailService{
		resolver: resolver,
		cfg:      cfg,
	}
}

// adminRecipient resolves the destination for support
// notification emails. It prefers the explicitly configured
// ContactRecipientAddress, falling back to the sender address
// (EmailFromAddress, then SMTPUsername) so a single-mailbox deployment works
// without extra wiring.
func (es *EmailService) adminRecipient() string {
	if es.cfg.Email.ContactRecipientAddress != "" {
		return es.cfg.Email.ContactRecipientAddress
	}
	if es.cfg.Email.FromAddress != "" {
		return es.cfg.Email.FromAddress
	}
	return es.cfg.Email.SMTP.Username
}

// appBaseURL returns the configured frontend base URL with any trailing slash
// removed so it can be safely concatenated with template paths
// (e.g. "<base>/settings/notifications").
func (es *EmailService) appBaseURL() string {
	return strings.TrimRight(es.cfg.Frontend.BaseURL, "/")
}

// SendSupportRequest sends a support request from an authenticated user.
//
// Support mail is INSTANCE mail, not team mail: it goes to the operator, from the
// operator, and /api/v1/support carries no team context. It therefore resolves
// with an empty team ID deliberately — routing it through a team's provider would
// send the operator's own correspondence on a tenant's credentials.
func (es *EmailService) SendSupportRequest(
	ctx context.Context, userName, userEmail string, req *models.SupportRequest,
) error {
	// Send notification email to the configured admin recipient.
	if err := es.sendSupportNotificationToAdmin(ctx, userName, userEmail, req); err != nil {
		return fmt.Errorf("failed to send admin notification: %w", err)
	}

	// Send acknowledgement to user if requested
	if req.Acknowledgement {
		if err := es.sendSupportAcknowledgement(ctx, userName, userEmail, req); err != nil {
			// Log but don't fail - admin notification was sent
			slog.With("error", err).Warn("Failed to send acknowledgement email")
		}
	}

	return nil
}

// sendEmail delivers one message on behalf of teamID.
//
// teamID selects the sender: a team that has configured its own provider sends
// through it and as itself, and any other value (including empty) falls back to
// the instance provider. Mail that is not attributable to a team — support
// requests — passes an empty teamID deliberately.
//
// A failure is never retried through the instance provider (epic #499 decision
// 7): silently re-sending a team's mail from the operator's address is worse than
// a visible failure.
func (es *EmailService) sendEmail(ctx context.Context, teamID, to, subject, htmlBody, textBody string) error {
	sender, err := es.resolver.Resolve(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to resolve the email sender: %w", err)
	}

	message := gomail.NewFullEmailMessage(
		// A BARE address only, deliberately. gomail validates this field with a
		// plain email regex and silently substitutes "" for anything else, so an
		// RFC-5322 `"Name" <addr>` form here would send with NO From header at
		// all. The display name travels separately, on the OutgoingMessage
		// below, and each provider applies it to the From header itself (#549).
		sender.FromAddress,
		[]string{to},
		subject,
		nil, // cc
		nil, // bcc
		sender.ReplyTo,
		textBody,
		htmlBody,
		nil, // attachments
	)

	// The caller's ctx reaches the provider, so a cancelled request no longer
	// leaves a send running detached.
	sendErr := sender.Provider.SendEmail(ctx, &external.OutgoingMessage{
		Message:  message,
		FromName: sender.FromName,
	})

	// Record the outcome before returning either way: health is derived by
	// comparing the success and failure timestamps, so a provider that only
	// recorded failures could never be shown as recovered. A bookkeeping failure
	// must not mask the send result, so it is logged rather than returned.
	if recordErr := es.resolver.RecordSendOutcome(ctx, sender, sendErr); recordErr != nil {
		slog.With("team_id", sender.TeamID, "error", recordErr).
			Warn("Failed to record the email delivery outcome")
	}

	if sendErr != nil {
		return fmt.Errorf("failed to send email: %w", sendErr)
	}

	es.logEmailSent(to, subject, htmlBody, textBody, sender)

	return nil
}

// logEmailSent logs successful email sending
func (es *EmailService) logEmailSent(to, subject, htmlBody, textBody string, sender *ResolvedEmailSender) {
	slog.With(
		"to", to,
		"subject", subject,
		"email_backend", es.cfg.Email.Provider,
		// Which provider actually handled the message, so an operator reading the
		// logs can tell a team send from an instance send.
		"email_sender_source", sender.Source,
		"team_id", sender.TeamID,
		"text_body", textBody != "",
		"html_body", htmlBody != "",
		"multipart", textBody != "" && htmlBody != "",
	).Info("Email sent successfully")
}

func (es *EmailService) sendSupportNotificationToAdmin(
	ctx context.Context, userName, userEmail string, req *models.SupportRequest,
) error {
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
		PrivacyPolicyURL:   es.cfg.Email.PrivacyPolicyURL,
	}

	// Render HTML template
	htmlBody, err := es.renderTemplateFromFS(
		emailBaseTemplate,
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

	// Empty team ID: instance sender (see SendSupportRequest).
	return es.sendEmail(ctx, "", es.adminRecipient(), subject, htmlBody, textBody)
}

func (es *EmailService) sendSupportAcknowledgement(
	ctx context.Context, userName, userEmail string, req *models.SupportRequest,
) error {
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
		PrivacyPolicyURL: es.cfg.Email.PrivacyPolicyURL,
	}

	// Render HTML template
	htmlBody, err := es.renderTemplateFromFS(
		emailBaseTemplate,
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

	// Empty team ID: instance sender (see SendSupportRequest).
	return es.sendEmail(ctx, "", userEmail, subject, htmlBody, textBody)
}

// SendNotificationEmail sends a transactional notification email on behalf of
// teamID. An empty teamID resolves to the instance sender.
func (es *EmailService) SendNotificationEmail(ctx context.Context, teamID, to, subject, htmlBody string) error {
	return es.sendEmail(ctx, teamID, to, subject, htmlBody, "")
}

// SendTeamInvitation sends a team invitation email from the inviting team, so the
// recipient sees the team they are being invited to rather than the operator.
func (es *EmailService) SendTeamInvitation(
	ctx context.Context, teamID string, invitation *models.TeamInvitation, teamName, inviterName string,
) error {
	subject := fmt.Sprintf("You have been invited to join %s on VibeXP", teamName)

	// Build accept URL
	acceptURL := fmt.Sprintf("%s/invitations/accept/%s", es.cfg.Frontend.BaseURL, invitation.Token)

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
		PrivacyPolicyURL: es.cfg.Email.PrivacyPolicyURL,
	}

	// Render HTML template
	htmlBody, err := es.renderTemplateFromFS(
		emailBaseTemplate,
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

	return es.sendEmail(ctx, teamID, invitation.InviteeEmail, subject, htmlBody, textBody)
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

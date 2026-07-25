package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"

	"github.com/darkrockmountain/gomail"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external/implementations"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Supported team email provider types. These are the same values
// implementations.NewEmailProvider accepts.
const (
	EmailProviderTypeSMTP     = "smtp"
	EmailProviderTypeMailgun  = "mailgun"
	EmailProviderTypePostmark = "postmark"
	EmailProviderTypeSendGrid = "sendgrid"
)

// testEmailSubject and testEmailBody are the fixed content of a test send. The
// caller supplies no message content, so the endpoint cannot be used to send
// arbitrary mail.
const (
	testEmailSubject  = "VibeXP test email"
	testEmailBodyText = "This is a test email from VibeXP confirming your team's " +
		"email provider is configured correctly."
	testEmailBodyHTML = "<p>This is a test email from VibeXP confirming your team's " +
		"email provider is configured correctly.</p>"
)

// TeamEmailProviderService owns validation, encryption and authorization for a
// team's own outbound email provider (#502, epic #499).
type TeamEmailProviderService struct {
	repo     repositories.TeamEmailProviderRepository
	userRepo repositories.UserRepository
	enc      EncryptionServiceInterface
	// guard bounds every destination built from team-supplied input (an SMTP host
	// or a Mailgun base URL), so configuring a provider cannot be used to probe
	// the deployment's internal network (#464).
	guard *ssrfGuard
	// authz gates the mutating operations: a provider row holds an encrypted
	// credential and decides what address the team's mail comes from.
	authz  AuthorizationServiceInterface
	logger *slog.Logger
}

// Ensure TeamEmailProviderService implements TeamEmailProviderServiceInterface
var _ TeamEmailProviderServiceInterface = (*TeamEmailProviderService)(nil)

// NewTeamEmailProviderService creates a new TeamEmailProviderService.
func NewTeamEmailProviderService(
	repo repositories.TeamEmailProviderRepository,
	userRepo repositories.UserRepository,
	enc EncryptionServiceInterface,
	cfg *config.Config,
	authzSvc AuthorizationServiceInterface,
	logger *slog.Logger,
) *TeamEmailProviderService {
	return &TeamEmailProviderService{
		repo:     repo,
		userRepo: userRepo,
		enc:      enc,
		guard:    ssrfGuardForConfig(cfg),
		authz:    authzSvc,
		logger:   logger,
	}
}

// authorizeMutation gates upsert/delete/test. A nil authz service is a wiring
// bug — fail closed rather than allow.
func (s *TeamEmailProviderService) authorizeMutation(ctx context.Context, userID, teamID string) error {
	if s.authz == nil {
		return fmt.Errorf("%w: authorization service is not configured", ErrPermissionDenied)
	}
	return s.authz.Can(ctx, userID, teamID, authz.TeamUpdate)
}

// encrypt delegates to the shared, fail-closed EncryptionService, preserving the
// empty-string passthrough (the shared service rejects empty input).
func (s *TeamEmailProviderService) encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if s.enc == nil {
		return "", ErrEncryptionUnavailable
	}
	return s.enc.Encrypt(plaintext)
}

func (s *TeamEmailProviderService) decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if s.enc == nil {
		return "", ErrEncryptionUnavailable
	}
	return s.enc.Decrypt(ciphertext)
}

// Get returns the team's provider. Membership is enforced by middleware; reading
// which provider a team uses is not privileged (the secret never leaves the
// server), so there is no role gate here.
func (s *TeamEmailProviderService) Get(
	ctx context.Context, _ /*userID*/, teamID string,
) (*models.TeamEmailProvider, error) {
	return s.repo.GetByTeamID(ctx, teamID)
}

// Upsert validates, encrypts and stores the team's provider.
//
// Order matters: authorize, then validate, then SSRF-check, and only then touch
// the repository — so a denied or malformed request never reaches storage and a
// disallowed destination is rejected before anything is persisted.
func (s *TeamEmailProviderService) Upsert(
	ctx context.Context, userID, teamID string, req models.UpsertTeamEmailProviderRequest,
) (*models.TeamEmailProvider, error) {
	if err := s.authorizeMutation(ctx, userID, teamID); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByTeamID(ctx, teamID)
	if err != nil && !errors.Is(err, repositories.ErrTeamEmailProviderNotFound) {
		return nil, err
	}
	isCreate := existing == nil

	if verr := validateUpsertRequest(req, isCreate); verr != nil {
		return nil, verr
	}

	if guardErr := s.guardRequestDestinations(ctx, req); guardErr != nil {
		return nil, guardErr
	}

	secretEncrypted, err := s.resolveSecret(req, existing)
	if err != nil {
		return nil, err
	}

	settings, err := marshalProviderSettings(req)
	if err != nil {
		return nil, err
	}

	provider := &models.TeamEmailProvider{
		TeamID:          teamID,
		UserID:          &userID,
		ProviderType:    normalizeProviderType(req.ProviderType),
		Settings:        settings,
		SecretEncrypted: secretEncrypted,
		FromAddress:     strings.TrimSpace(req.FromAddress),
		FromName:        trimOptional(req.FromName),
		ReplyTo:         trimOptional(req.ReplyTo),
	}

	if err := s.repo.Upsert(ctx, provider); err != nil {
		return nil, err
	}

	return provider, nil
}

// Delete removes the team's provider, reverting it to the instance provider.
func (s *TeamEmailProviderService) Delete(ctx context.Context, userID, teamID string) error {
	if err := s.authorizeMutation(ctx, userID, teamID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, teamID)
}

// Test sends a real message using the configuration in the REQUEST, not the
// stored row, so an admin can verify credentials before saving them.
//
// The recipient is always the acting user's own account email — it is never
// caller-supplied — so this cannot be used as an open relay. A delivery failure
// comes back as a result value, not an error: "no, because X" is a successful
// answer to "does this work?".
func (s *TeamEmailProviderService) Test(
	ctx context.Context, userID, teamID string, req models.TestTeamEmailProviderRequest,
) (*models.TeamEmailProviderTestResult, error) {
	if err := s.authorizeMutation(ctx, userID, teamID); err != nil {
		return nil, err
	}

	// A test send must carry a complete configuration: there is no stored row to
	// fall back on, so the secret is always required.
	if verr := validateUpsertRequest(req.UpsertTeamEmailProviderRequest, true); verr != nil {
		return nil, verr
	}

	if err := s.guardRequestDestinations(ctx, req.UpsertTeamEmailProviderRequest); err != nil {
		return nil, err
	}

	recipient, err := s.actingUserEmail(ctx, userID)
	if err != nil {
		return nil, err
	}

	secret := ""
	if req.Secret != nil {
		secret = *req.Secret
	}

	provider, err := implementations.NewEmailProvider(
		providerSpecFromRequest(req.UpsertTeamEmailProviderRequest, secret), s.logger)
	if err != nil {
		return &models.TeamEmailProviderTestResult{
			Success:   false,
			Recipient: recipient,
			Message:   "The provider could not be configured: " + err.Error(),
		}, nil
	}

	message := gomail.NewFullEmailMessage(
		strings.TrimSpace(req.FromAddress),
		[]string{recipient},
		testEmailSubject,
		nil, nil,
		optionalValue(req.ReplyTo),
		testEmailBodyText,
		testEmailBodyHTML,
		nil,
	)

	if sendErr := provider.SendEmail(ctx, message); sendErr != nil {
		return &models.TeamEmailProviderTestResult{
			Success:   false,
			Recipient: recipient,
			Message:   "Sending failed: " + sendErr.Error(),
		}, nil
	}

	return &models.TeamEmailProviderTestResult{
		Success:   true,
		Recipient: recipient,
		Message:   "Test email sent to " + recipient,
	}, nil
}

// actingUserEmail resolves the recipient of a test send. Fixing it to the acting
// user's account email is what keeps the endpoint from being a relay.
func (s *TeamEmailProviderService) actingUserEmail(ctx context.Context, userID string) (string, error) {
	if s.userRepo == nil {
		return "", fmt.Errorf("team email provider test: user repository is not configured")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("team email provider test: failed to resolve the acting user: %w", err)
	}
	if user == nil || strings.TrimSpace(user.Email) == "" {
		return "", &TeamEmailProviderValidationError{Fields: []FieldError{{
			Field:   "recipient",
			Message: "your account has no email address to send the test message to",
		}}}
	}
	return user.Email, nil
}

// resolveSecret decides what ciphertext to store: a supplied secret is
// encrypted, an omitted one keeps the stored value. Validation has already
// rejected the empty string and an omitted secret on create, so an absent secret
// here always means "keep".
func (s *TeamEmailProviderService) resolveSecret(
	req models.UpsertTeamEmailProviderRequest, existing *models.TeamEmailProvider,
) (string, error) {
	if req.Secret != nil {
		encrypted, err := s.encrypt(*req.Secret)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt the provider secret: %w", err)
		}
		return encrypted, nil
	}
	return existing.SecretEncrypted, nil
}

// guardRequestDestinations rejects a team-supplied destination in a reserved
// range before anything is stored or dialled. Only SMTP and Mailgun carry a
// caller-controlled host; Postmark and SendGrid have fixed vendor endpoints.
func (s *TeamEmailProviderService) guardRequestDestinations(
	ctx context.Context, req models.UpsertTeamEmailProviderRequest,
) error {
	switch normalizeProviderType(req.ProviderType) {
	case EmailProviderTypeSMTP:
		if req.Settings.SMTP == nil {
			return nil
		}
		// validateOutboundHost takes a URL, and an SMTP host is a bare host, so
		// give it a scheme to parse. Only the host is inspected.
		host := strings.TrimSpace(req.Settings.SMTP.Host)
		if err := s.guard.validateOutboundHost(ctx, "smtp://"+host); err != nil {
			return &TeamEmailProviderValidationError{Fields: []FieldError{{
				Field:   "settings.smtp.host",
				Message: "this host is not an allowed destination",
			}}}
		}
	case EmailProviderTypeMailgun:
		if req.Settings.Mailgun == nil || strings.TrimSpace(req.Settings.Mailgun.BaseURL) == "" {
			return nil
		}
		if err := s.guard.validateOutboundHost(ctx, strings.TrimSpace(req.Settings.Mailgun.BaseURL)); err != nil {
			return &TeamEmailProviderValidationError{Fields: []FieldError{{
				Field:   "settings.mailgun.base_url",
				Message: "this URL is not an allowed destination",
			}}}
		}
	}
	return nil
}

// providerSpecFromRequest maps a request onto the config-free ProviderSpec from
// #500. Building a spec and calling the shared factory — rather than
// constructing providers here — is what keeps this code from duplicating the
// factory's four-way switch.
func providerSpecFromRequest(
	req models.UpsertTeamEmailProviderRequest, secret string,
) implementations.ProviderSpec {
	spec := implementations.ProviderSpec{Type: normalizeProviderType(req.ProviderType)}

	switch spec.Type {
	case EmailProviderTypeSMTP:
		if req.Settings.SMTP != nil {
			spec.SMTP = implementations.SMTPSpec{
				Host:     strings.TrimSpace(req.Settings.SMTP.Host),
				Port:     strings.TrimSpace(req.Settings.SMTP.Port),
				Username: req.Settings.SMTP.Username,
				Password: secret,
			}
		}
	case EmailProviderTypeMailgun:
		if req.Settings.Mailgun != nil {
			spec.Mailgun = implementations.MailgunSpec{
				BaseURL:    strings.TrimSpace(req.Settings.Mailgun.BaseURL),
				Domain:     strings.TrimSpace(req.Settings.Mailgun.Domain),
				SendingKey: secret,
			}
		}
	case EmailProviderTypePostmark:
		stream := ""
		if req.Settings.Postmark != nil {
			stream = strings.TrimSpace(req.Settings.Postmark.MessageStream)
		}
		spec.Postmark = implementations.PostmarkSpec{ServerToken: secret, MessageStream: stream}
	case EmailProviderTypeSendGrid:
		spec.SendGrid = implementations.SendGridSpec{APIKey: secret}
	}

	return spec
}

// marshalProviderSettings serialises only the block belonging to the request's
// provider type, so a row never carries settings for a provider it does not use.
func marshalProviderSettings(req models.UpsertTeamEmailProviderRequest) (json.RawMessage, error) {
	var payload any
	switch normalizeProviderType(req.ProviderType) {
	case EmailProviderTypeSMTP:
		payload = req.Settings.SMTP
	case EmailProviderTypeMailgun:
		payload = req.Settings.Mailgun
	case EmailProviderTypePostmark:
		if req.Settings.Postmark == nil {
			return json.RawMessage(`{}`), nil
		}
		payload = req.Settings.Postmark
	default:
		// SendGrid has no non-secret settings.
		return json.RawMessage(`{}`), nil
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode provider settings: %w", err)
	}
	return encoded, nil
}

func normalizeProviderType(providerType string) string {
	return strings.ToLower(strings.TrimSpace(providerType))
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func optionalValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// validateUpsertRequest checks a request against the rules for its provider
// type. isCreate is true when there is no stored row to inherit a secret from.
func validateUpsertRequest(req models.UpsertTeamEmailProviderRequest, isCreate bool) error {
	var fields []FieldError

	providerType := normalizeProviderType(req.ProviderType)
	validator, supported := emailProviderValidators[providerType]
	if !supported {
		fields = append(fields, FieldError{
			Field: "provider_type",
			Message: fmt.Sprintf("must be one of %s, %s, %s, %s",
				EmailProviderTypeSMTP, EmailProviderTypeMailgun,
				EmailProviderTypePostmark, EmailProviderTypeSendGrid),
		})
		// Without a known type nothing else can be checked coherently.
		return &TeamEmailProviderValidationError{Fields: fields}
	}

	fields = append(fields, validateSenderIdentity(req)...)
	fields = append(fields, validateSecret(req, isCreate)...)
	fields = append(fields, validateSettingsBlocks(req, providerType)...)
	fields = append(fields, validator(req.Settings)...)

	if len(fields) > 0 {
		return &TeamEmailProviderValidationError{Fields: fields}
	}
	return nil
}

func validateSenderIdentity(req models.UpsertTeamEmailProviderRequest) []FieldError {
	var fields []FieldError

	from := strings.TrimSpace(req.FromAddress)
	if from == "" {
		fields = append(fields, FieldError{Field: "from_address", Message: "is required"})
	} else if _, err := mail.ParseAddress(from); err != nil {
		fields = append(fields, FieldError{Field: "from_address", Message: "must be a valid email address"})
	}

	if replyTo := optionalValue(req.ReplyTo); replyTo != "" {
		if _, err := mail.ParseAddress(replyTo); err != nil {
			fields = append(fields, FieldError{Field: "reply_to", Message: "must be a valid email address"})
		}
	}

	return fields
}

func validateSecret(req models.UpsertTeamEmailProviderRequest, isCreate bool) []FieldError {
	switch {
	case req.Secret == nil && isCreate:
		return []FieldError{{Field: "secret", Message: "is required"}}
	case req.Secret != nil && strings.TrimSpace(*req.Secret) == "":
		// Explicitly not treated as "clear the secret": a provider with no
		// credential cannot send, and silently disabling a team's mail is worse
		// than rejecting the request.
		return []FieldError{{
			Field:   "secret",
			Message: "cannot be empty; omit the field to keep the stored secret",
		}}
	default:
		return nil
	}
}

// validateSettingsBlocks rejects settings belonging to a provider type other
// than the one named, so a mismatched body is a validation error rather than a
// field that is silently dropped.
func validateSettingsBlocks(req models.UpsertTeamEmailProviderRequest, providerType string) []FieldError {
	present := map[string]bool{
		EmailProviderTypeSMTP:     req.Settings.SMTP != nil,
		EmailProviderTypeMailgun:  req.Settings.Mailgun != nil,
		EmailProviderTypePostmark: req.Settings.Postmark != nil,
	}

	var fields []FieldError
	for blockType, isPresent := range present {
		if isPresent && blockType != providerType {
			fields = append(fields, FieldError{
				Field:   "settings." + blockType,
				Message: fmt.Sprintf("must not be set when provider_type is %q", providerType),
			})
		}
	}
	return fields
}

// emailProviderValidators holds one validator per provider type, so the four
// heterogeneous config shapes are checked without a single sprawling switch.
var emailProviderValidators = map[string]func(models.TeamEmailProviderSettings) []FieldError{
	EmailProviderTypeSMTP:     validateSMTPSettings,
	EmailProviderTypeMailgun:  validateMailgunSettings,
	EmailProviderTypePostmark: validatePostmarkSettings,
	EmailProviderTypeSendGrid: validateSendGridSettings,
}

func validateSMTPSettings(settings models.TeamEmailProviderSettings) []FieldError {
	if settings.SMTP == nil {
		return []FieldError{{Field: "settings.smtp", Message: "is required when provider_type is smtp"}}
	}

	var fields []FieldError
	if strings.TrimSpace(settings.SMTP.Host) == "" {
		fields = append(fields, FieldError{Field: "settings.smtp.host", Message: "is required"})
	}

	port := strings.TrimSpace(settings.SMTP.Port)
	if port == "" {
		fields = append(fields, FieldError{Field: "settings.smtp.port", Message: "is required"})
		return fields
	}

	// The provider parses the port with strconv.Atoi at construction time;
	// rejecting it here turns an opaque startup error into a field error.
	if parsed, err := strconv.Atoi(port); err != nil || parsed < 1 || parsed > 65535 {
		fields = append(fields, FieldError{
			Field:   "settings.smtp.port",
			Message: "must be a number between 1 and 65535",
		})
	}

	return fields
}

func validateMailgunSettings(settings models.TeamEmailProviderSettings) []FieldError {
	if settings.Mailgun == nil {
		return []FieldError{{Field: "settings.mailgun", Message: "is required when provider_type is mailgun"}}
	}

	var fields []FieldError
	domain := strings.TrimSpace(settings.Mailgun.Domain)
	switch {
	case domain == "":
		fields = append(fields, FieldError{Field: "settings.mailgun.domain", Message: "is required"})
	case strings.Contains(domain, "://") || strings.HasSuffix(domain, "/"):
		// Mirrors NewMailgunEmailProvider's check so the caller gets a field
		// error instead of a construction failure.
		fields = append(fields, FieldError{
			Field:   "settings.mailgun.domain",
			Message: "must be a bare domain (e.g. mg.example.com), not a URL",
		})
	}

	return fields
}

// validatePostmarkSettings has nothing required: the server token is the secret
// and the message stream is optional.
func validatePostmarkSettings(models.TeamEmailProviderSettings) []FieldError { return nil }

// validateSendGridSettings has nothing to check: SendGrid's only configuration
// is its API key, which is the secret.
func validateSendGridSettings(models.TeamEmailProviderSettings) []FieldError { return nil }

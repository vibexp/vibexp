package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"strings"

	"github.com/vibexp/vibexp/internal/external/implementations"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceEmailProviderService owns validation, encryption and auditing of the
// instance's own outbound email provider (#1188, epic #1185): the stored
// replacement for config.yaml's `email:` block, editable by an instance admin
// at runtime. EmailSenderResolver reads the same row per send, so a change
// here takes effect on the next message with no restart.
//
// It follows TeamEmailProviderService step for step (validate, resolve the
// secret, marshal settings, persist) with three deliberate differences:
//   - no authorization check: instance-admin gating is the admin route
//     middleware (epic decision 6), as for the other admin services;
//   - no SSRF guard (epic decision 5), see Upsert;
//   - every write appends one redacted entry to the instance settings log.
type InstanceEmailProviderService struct {
	repo     repositories.InstanceEmailProviderRepository
	audit    repositories.InstanceSettingsAuditRepository
	userRepo repositories.UserRepository
	enc      EncryptionServiceInterface
	logger   *slog.Logger
}

var _ InstanceEmailProviderServiceInterface = (*InstanceEmailProviderService)(nil)

// errInvalidStoredEmailConfig marks a stored row whose settings cannot be
// mapped onto a provider spec (malformed settings, unsupported type).
var errInvalidStoredEmailConfig = errors.New("the stored email configuration is invalid")

// NewInstanceEmailProviderService creates a new InstanceEmailProviderService.
func NewInstanceEmailProviderService(
	repo repositories.InstanceEmailProviderRepository,
	audit repositories.InstanceSettingsAuditRepository,
	userRepo repositories.UserRepository,
	enc EncryptionServiceInterface,
	logger *slog.Logger,
) *InstanceEmailProviderService {
	return &InstanceEmailProviderService{
		repo:     repo,
		audit:    audit,
		userRepo: userRepo,
		enc:      enc,
		logger:   logger,
	}
}

// Get returns the instance's effective configuration. It never reports
// not-found: an instance without a row is Configured false. The secret is
// never part of the result, only whether one is stored.
func (s *InstanceEmailProviderService) Get(ctx context.Context) (*models.InstanceEmailProviderEffective, error) {
	row, err := s.stored(ctx)
	if err != nil {
		return nil, err
	}
	return instanceEffective(row), nil
}

// Upsert validates, encrypts and stores the instance provider, then appends one
// redacted audit entry. An omitted secret keeps the stored one when the
// provider type is unchanged.
//
// Deliberately NO ssrfGuard (epic #1185 decision 5): the instance admin is the
// operator, and localhost/internal relays are legitimate. Team-supplied hosts
// keep the guard in TeamEmailProviderService.guardRequestDestinations.
//
// The row is written before the audit entry and the two are not one
// transaction (the repositories are separate). An Append failure is returned,
// never swallowed, so the admin sees that the change is unaudited.
func (s *InstanceEmailProviderService) Upsert(
	ctx context.Context, actorUserID string, req models.UpsertInstanceEmailProviderRequest,
) (*models.InstanceEmailProviderEffective, error) {
	existing, err := s.stored(ctx)
	if err != nil {
		return nil, err
	}

	// An omitted secret keeps the stored one only for the SAME provider type:
	// a type change must carry its own secret. Unlike a test send (see
	// testConfiguration), a save may change the destination (SMTP host, Mailgun
	// base URL) and keep the secret, deliberately: a save is audited, so the
	// before/after snapshots record the destination change, whereas a test send
	// is unaudited and therefore also requires the same destination.
	keepsStoredSecret := sameStoredProviderType(existing, req.ProviderType)
	if verr := validateInstanceUpsertRequest(req, !keepsStoredSecret); verr != nil {
		return nil, verr
	}

	secretEncrypted, err := s.resolveSecret(req.Secret, existing)
	if err != nil {
		return nil, err
	}

	row, err := instanceRowFromRequest(req, secretEncrypted, actorUserID)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// The repository never touches the health columns on upsert, so carry
		// them over for the returned view rather than reporting a fresh row.
		row.LastSuccessAt, row.LastError, row.LastErrorAt = existing.LastSuccessAt, existing.LastError, existing.LastErrorAt
	}

	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, err
	}

	secretMarker := models.InstanceSettingsAuditSecretUnchanged
	if req.Secret != nil {
		secretMarker = models.InstanceSettingsAuditSecretChanged
	}
	if err := s.appendAudit(ctx, models.InstanceSettingsAuditActionUpsert, actorUserID,
		existing, row, secretMarker); err != nil {
		return nil, err
	}

	return instanceEffective(row), nil
}

// Delete removes the instance provider and appends one redacted audit entry.
// Deleting when there is no row reports repositories.ErrInstanceEmailProviderNotFound
// and writes no entry: nothing changed, and the admin endpoint answers it with
// 409 like the team endpoint does (#1189).
func (s *InstanceEmailProviderService) Delete(ctx context.Context, actorUserID string) error {
	existing, err := s.stored(ctx)
	if err != nil {
		return err
	}
	if existing == nil {
		return repositories.ErrInstanceEmailProviderNotFound
	}

	// A concurrent delete surfaces here as ErrInstanceEmailProviderNotFound too;
	// the other delete owns the audit entry, so it is returned as is.
	if err := s.repo.Delete(ctx); err != nil {
		return err
	}

	return s.appendAudit(ctx, models.InstanceSettingsAuditActionDelete, actorUserID, existing, nil, "")
}

// Test sends the fixed test message with the submitted configuration, or with
// the stored one when the request carries none. Like the team test, a failure
// to build or to deliver is a result value, not an error. Also like Upsert,
// there is no SSRF guard on this path.
func (s *InstanceEmailProviderService) Test(
	ctx context.Context, actorUserID string, req models.TestInstanceEmailProviderRequest,
) (*models.TeamEmailProviderTestResult, error) {
	recipient, err := s.testRecipient(ctx, actorUserID, req.Recipient)
	if err != nil {
		return nil, err
	}

	stored, err := s.stored(ctx)
	if err != nil {
		return nil, err
	}

	spec, sender, err := s.testConfiguration(req.Config, stored)
	if err != nil {
		if errors.Is(err, errInvalidStoredEmailConfig) {
			// A stored row that cannot be mapped is a build failure — the very
			// thing an admin tests to diagnose — so it is a result, not an error.
			return testConfigInvalid(recipient, err), nil
		}
		return nil, err
	}

	provider, err := implementations.NewEmailProvider(spec, s.logger)
	if err != nil {
		return testConfigInvalid(recipient, err), nil
	}
	// The stub accepts and discards everything, so "sent" would be a lie.
	if _, isStub := provider.(*implementations.StubEmailProvider); isStub {
		return testConfigInvalid(recipient,
			errors.New("the configuration is incomplete and would discard messages")), nil
	}

	return sendTestMessage(ctx, provider, sender, recipient), nil
}

// testConfiguration picks what a test send uses: the request's configuration
// when it has one, otherwise the stored row. A request that omits its secret
// borrows the stored one only when it targets the SAME destination (see
// sameStoredDestination), so a credential is never sent anywhere it was not
// issued for — a test send is unaudited, so a looser rule would let the
// write-only secret be recovered by pointing a test at a listener.
func (s *InstanceEmailProviderService) testConfiguration(
	config *models.UpsertInstanceEmailProviderRequest, stored *models.InstanceEmailProvider,
) (implementations.ProviderSpec, testSender, error) {
	if config == nil {
		if stored == nil {
			return implementations.ProviderSpec{}, testSender{}, &TeamEmailProviderValidationError{
				Fields: []FieldError{{Field: "config", Message: "is required when no configuration is stored"}},
			}
		}
		return s.storedTestConfiguration(stored)
	}

	canBorrowSecret := config.Secret == nil && sameStoredDestination(stored, config.UpsertTeamEmailProviderRequest)
	if verr := validateInstanceUpsertRequest(*config, !canBorrowSecret); verr != nil {
		return implementations.ProviderSpec{}, testSender{}, verr
	}

	secret := ""
	if config.Secret != nil {
		secret = *config.Secret
	} else if canBorrowSecret {
		decrypted, err := s.decrypt(derefOrEmpty(stored.SecretEncrypted))
		if err != nil {
			return implementations.ProviderSpec{}, testSender{}, fmt.Errorf(
				"failed to decrypt the instance email provider secret: %w", err)
		}
		secret = decrypted
	}

	return providerSpecFromRequest(config.UpsertTeamEmailProviderRequest, secret),
		testSenderFromRequest(config.UpsertTeamEmailProviderRequest), nil
}

func (s *InstanceEmailProviderService) storedTestConfiguration(
	stored *models.InstanceEmailProvider,
) (implementations.ProviderSpec, testSender, error) {
	secret, err := s.decrypt(derefOrEmpty(stored.SecretEncrypted))
	if err != nil {
		return implementations.ProviderSpec{}, testSender{}, fmt.Errorf(
			"failed to decrypt the instance email provider secret: %w", err)
	}
	spec, err := providerSpecFromStored(stored.ProviderType, stored.Settings, secret, instanceEmailLabel)
	if err != nil {
		return implementations.ProviderSpec{}, testSender{}, fmt.Errorf("%w: %w", errInvalidStoredEmailConfig, err)
	}
	return spec, testSender{
		FromAddress: stored.FromAddress,
		FromName:    derefOrEmpty(stored.FromName),
		ReplyTo:     derefOrEmpty(stored.ReplyTo),
	}, nil
}

// testRecipient is the requested recipient, which must be a valid address, or
// the acting admin's own email by default.
func (s *InstanceEmailProviderService) testRecipient(
	ctx context.Context, actorUserID string, requested *string,
) (string, error) {
	if recipient := optionalValue(requested); recipient != "" {
		if !isBareEmailAddress(recipient) {
			return "", &TeamEmailProviderValidationError{Fields: []FieldError{{
				Field: "recipient", Message: "must be a valid email address",
			}}}
		}
		return recipient, nil
	}
	return actingUserEmail(ctx, s.userRepo, actorUserID, "instance email provider test")
}

// instanceRowFromRequest builds the row a validated request stores, so an admin
// save and the config.yaml import (#1190) store identical rows. An empty
// actorUserID leaves UpdatedBy nil.
func instanceRowFromRequest(
	req models.UpsertInstanceEmailProviderRequest, secretEncrypted *string, actorUserID string,
) (*models.InstanceEmailProvider, error) {
	settings, err := marshalProviderSettings(req.UpsertTeamEmailProviderRequest)
	if err != nil {
		return nil, err
	}
	return &models.InstanceEmailProvider{
		ProviderType:            normalizeProviderType(req.ProviderType),
		Settings:                settings,
		SecretEncrypted:         secretEncrypted,
		FromAddress:             strings.TrimSpace(req.FromAddress),
		FromName:                trimOptional(req.FromName),
		ReplyTo:                 trimOptional(req.ReplyTo),
		ContactRecipientAddress: trimOptional(req.ContactRecipientAddress),
		PrivacyPolicyURL:        trimOptional(req.PrivacyPolicyURL),
		UpdatedBy:               optionalActor(actorUserID),
	}, nil
}

// stored returns the instance row, or nil when none is stored.
func (s *InstanceEmailProviderService) stored(ctx context.Context) (*models.InstanceEmailProvider, error) {
	row, err := s.repo.Get(ctx)
	if err != nil {
		if errors.Is(err, repositories.ErrInstanceEmailProviderNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read the instance email provider: %w", err)
	}
	return row, nil
}

// resolveSecret decides what ciphertext to store: a supplied secret is
// encrypted, an omitted one keeps the stored value. Validation has already
// rejected the empty string, and an omitted secret on create or on a change of
// provider type.
func (s *InstanceEmailProviderService) resolveSecret(
	secret *string, existing *models.InstanceEmailProvider,
) (*string, error) {
	if secret == nil {
		if existing == nil {
			return nil, nil
		}
		return existing.SecretEncrypted, nil
	}
	if s.enc == nil {
		return nil, fmt.Errorf("failed to encrypt the provider secret: %w", ErrEncryptionUnavailable)
	}
	encrypted, err := s.enc.Encrypt(*secret)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt the provider secret: %w", err)
	}
	return &encrypted, nil
}

func (s *InstanceEmailProviderService) decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if s.enc == nil {
		return "", ErrEncryptionUnavailable
	}
	return s.enc.Decrypt(ciphertext)
}

// appendAudit writes one redacted entry. before is nil on create, after is nil
// on delete; secretMarker describes the credential on the after snapshot.
func (s *InstanceEmailProviderService) appendAudit(
	ctx context.Context, action, actorUserID string,
	before, after *models.InstanceEmailProvider, secretMarker string,
) error {
	beforeDoc, err := InstanceEmailAuditSnapshot(before, "")
	if err != nil {
		return err
	}
	afterDoc, err := InstanceEmailAuditSnapshot(after, secretMarker)
	if err != nil {
		return err
	}

	entry := &models.InstanceSettingsAuditEntry{
		Setting:     models.InstanceSettingEmailProvider,
		Action:      action,
		ActorUserID: optionalActor(actorUserID),
		Before:      beforeDoc,
		After:       afterDoc,
	}
	if err := s.audit.Append(ctx, entry); err != nil {
		return fmt.Errorf("failed to audit the instance email provider change: %w", err)
	}
	return nil
}

// InstanceEmailAuditSnapshot is the redacted audit shape of an instance row:
// every field except the ciphertext (which the model never marshals), plus
// has_credential, plus — when secretMarker is non-empty — a "secret" key saying
// whether the credential changed. A nil row snapshots to nil (no document).
//
// Exported so the config.yaml import (#1190) writes the same shape.
func InstanceEmailAuditSnapshot(row *models.InstanceEmailProvider, secretMarker string) (json.RawMessage, error) {
	if row == nil {
		return nil, nil
	}

	encoded, err := json.Marshal(row)
	if err != nil {
		return nil, fmt.Errorf("failed to snapshot the instance email provider: %w", err)
	}
	var snapshot map[string]any
	if err = json.Unmarshal(encoded, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to snapshot the instance email provider: %w", err)
	}

	snapshot["has_credential"] = row.HasCredential()
	if secretMarker != "" {
		snapshot["secret"] = secretMarker
	}

	redacted, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to snapshot the instance email provider: %w", err)
	}
	return redacted, nil
}

func instanceEffective(row *models.InstanceEmailProvider) *models.InstanceEmailProviderEffective {
	if row == nil {
		return models.NewInstanceEmailProviderEffective(nil, nil)
	}
	return models.NewInstanceEmailProviderEffective(row, settingsUnionFromStored(row.ProviderType, row.Settings))
}

// sameStoredProviderType reports whether a stored row exists with the given
// provider type — the only case in which its secret may be reused.
func sameStoredProviderType(stored *models.InstanceEmailProvider, providerType string) bool {
	return stored != nil && normalizeProviderType(stored.ProviderType) == normalizeProviderType(providerType)
}

// sameStoredDestination reports whether req targets exactly the destination the
// stored secret was issued for: the same provider type and, where the caller
// chooses the endpoint, the same SMTP host/port/username or Mailgun
// base URL/domain. Postmark and SendGrid have fixed vendor endpoints, so the
// type alone identifies them.
func sameStoredDestination(stored *models.InstanceEmailProvider, req models.UpsertTeamEmailProviderRequest) bool {
	if !sameStoredProviderType(stored, req.ProviderType) {
		return false
	}
	storedSettings := settingsUnionFromStored(stored.ProviderType, stored.Settings)
	if storedSettings == nil {
		storedSettings = &models.TeamEmailProviderSettings{}
	}

	switch normalizeProviderType(req.ProviderType) {
	case EmailProviderTypeSMTP:
		return sameSMTPDestination(storedSettings.SMTP, req.Settings.SMTP)
	case EmailProviderTypeMailgun:
		return sameMailgunDestination(storedSettings.Mailgun, req.Settings.Mailgun)
	default:
		return true
	}
}

func sameSMTPDestination(have, want *models.SMTPProviderSettings) bool {
	if have == nil || want == nil {
		return false
	}
	return strings.TrimSpace(have.Host) == strings.TrimSpace(want.Host) &&
		strings.TrimSpace(have.Port) == strings.TrimSpace(want.Port) &&
		have.Username == want.Username
}

func sameMailgunDestination(have, want *models.MailgunProviderSettings) bool {
	if have == nil || want == nil {
		return false
	}
	return strings.TrimSpace(have.BaseURL) == strings.TrimSpace(want.BaseURL) &&
		strings.TrimSpace(have.Domain) == strings.TrimSpace(want.Domain)
}

func optionalActor(actorUserID string) *string {
	if strings.TrimSpace(actorUserID) == "" {
		return nil
	}
	return &actorUserID
}

// validateInstanceUpsertRequest applies the team provider's rules to the shared
// part of the request, plus the instance-only fields, and reports all field
// errors together.
func validateInstanceUpsertRequest(req models.UpsertInstanceEmailProviderRequest, isCreate bool) error {
	var fields []FieldError
	if err := validateUpsertRequest(req.UpsertTeamEmailProviderRequest, isCreate); err != nil {
		var verr *TeamEmailProviderValidationError
		if !errors.As(err, &verr) {
			return err
		}
		fields = append(fields, verr.Fields...)
	}
	fields = append(fields, validateInstanceOnlyFields(req.ContactRecipientAddress, req.PrivacyPolicyURL)...)

	if len(fields) > 0 {
		return &TeamEmailProviderValidationError{Fields: fields}
	}
	return nil
}

// validateInstanceOnlyFields checks the contact recipient (an email address)
// and the privacy policy URL (an absolute http/https URL). Both may be empty.
func validateInstanceOnlyFields(contact, privacyURL *string) []FieldError {
	var fields []FieldError

	if value := optionalValue(contact); value != "" && !isBareEmailAddress(value) {
		fields = append(fields, FieldError{
			Field: "contact_recipient_address", Message: "must be a valid email address",
		})
	}

	if value := optionalValue(privacyURL); value != "" {
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			fields = append(fields, FieldError{
				Field: "privacy_policy_url", Message: "must be an absolute http or https URL",
			})
		}
	}

	return fields
}

// isBareEmailAddress reports whether value is exactly one address with no
// display name, so a recipient can never smuggle in a second address.
func isBareEmailAddress(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}

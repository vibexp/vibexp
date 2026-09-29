package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The one-release upgrade bridge for config.yaml's deprecated `email:` section
// (#1190, epic #1185). Since #1188 the instance sender is read only from the
// instance_email_provider row, so an install upgrading with its mail configured
// in config.yaml would otherwise fall onto the discard-everything stub. At boot
// the block is imported once into that row when none exists; afterwards the row
// is authoritative and the block is ignored. #1193 removes the block.
//
// Nothing here is fatal: an unimportable block, an encryption, repository or
// audit failure each log an ERROR and boot continues. Unconfigured mail is a
// warning, not an outage (epic decision 8).

// legacyEmailSection names the config.yaml section in every log line.
const legacyEmailSection = "email"

// legacyEmailImportFailedMsg is logged by every branch that aborts the import.
const legacyEmailImportFailedMsg = "Failed to import the config.yaml email: section"

// LegacyEmailImportDeps are the collaborators of ImportLegacyEmailConfig.
type LegacyEmailImportDeps struct {
	Repo   repositories.InstanceEmailProviderRepository
	Audit  repositories.InstanceSettingsAuditRepository
	Enc    EncryptionServiceInterface
	Logger *slog.Logger
}

// LegacyEmailImportResult reports what ImportLegacyEmailConfig did, for tests.
type LegacyEmailImportResult struct {
	// Imported is true when this run inserted the instance row.
	Imported bool
	// Ignored is true when the block is populated but a row already exists.
	Ignored bool
	// Unconfigured is true when the instance has no row after the import.
	Unconfigured bool
}

// LegacyEmailPopulated reports whether the legacy block could actually have
// sent mail, as opposed to carrying only inherited defaults.
//
// It is evaluated on the effective loaded struct, where koanf has already
// merged defaults() and an unset key cannot be told apart from a default. So
// the rule is per provider:
//   - smtp: host and port are set, and either a password is set or the
//     host/port pair differs from the code defaults (an unauthenticated
//     internal relay). The baked config.docker.yaml resolves to exactly those
//     defaults (smtp.gmail.com:587, no credentials) on every install that never
//     configured mail, and that must not be imported as a real configuration;
//   - mailgun: domain and sending key are both set;
//   - postmark: the server token is set;
//   - sendgrid: the API key is set;
//   - any other provider value was chosen by the operator (the default is
//     smtp), so it counts as populated and the import then reports it as
//     unsupported.
func LegacyEmailPopulated(c config.LegacyEmailConfig) bool {
	switch legacyEmailProviderType(c) {
	case EmailProviderTypeSMTP:
		host, port := strings.TrimSpace(c.SMTP.Host), strings.TrimSpace(c.SMTP.Port)
		if host == "" || port == "" {
			return false
		}
		isDefaultDestination := host == config.DefaultLegacySMTPHost && port == config.DefaultLegacySMTPPort
		return hasLegacySecret(c.SMTP.Password) || !isDefaultDestination
	case EmailProviderTypeMailgun:
		return strings.TrimSpace(c.Mailgun.Domain) != "" && hasLegacySecret(c.Mailgun.SendingKey)
	case EmailProviderTypePostmark:
		return hasLegacySecret(c.Postmark.ServerToken)
	case EmailProviderTypeSendGrid:
		return hasLegacySecret(c.SendGrid.APIKey)
	default:
		return true
	}
}

// ImportLegacyEmailConfig runs the boot-time bridge once: it imports a
// populated legacy block when the instance has no row, and logs the
// deprecation and unconfigured-mail warnings. instanceAdmins is
// auth.instance_admins, which decides how loudly unconfigured mail is reported.
//
// Concurrent replicas are safe by construction: InsertIfAbsent lets the
// database pick one writer, and only the writer appends the audit entry.
func ImportLegacyEmailConfig(
	ctx context.Context, deps LegacyEmailImportDeps, legacy config.LegacyEmailConfig, instanceAdmins []string,
) LegacyEmailImportResult {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	populated := LegacyEmailPopulated(legacy)
	if populated {
		logger.Warn("The config.yaml email: section is deprecated and will be removed in the next minor release; "+
			"configure instance mail under Admin → Settings → Email",
			"section", legacyEmailSection)
	}

	_, err := deps.Repo.Get(ctx)
	switch {
	case err == nil:
		if populated {
			logger.Warn("The config.yaml email: section is ignored: the instance email provider stored in the "+
				"database is authoritative",
				"section", legacyEmailSection)
		}
		return LegacyEmailImportResult{Ignored: populated}
	case !errors.Is(err, repositories.ErrInstanceEmailProviderNotFound):
		// Whether the instance is configured is unknown, so neither import nor
		// report it as unconfigured.
		logger.Error("Failed to read the instance email provider; skipping the config.yaml email: import",
			"section", legacyEmailSection, "error", err)
		return LegacyEmailImportResult{}
	}

	var result LegacyEmailImportResult
	stored := false
	if populated {
		result.Imported, stored = importLegacyEmail(ctx, deps, logger, legacy)
	}
	if !stored {
		result.Unconfigured = true
		warnInstanceEmailUnconfigured(logger, instanceAdmins)
	}
	return result
}

// importLegacyEmail maps, validates, encrypts and inserts the block. It
// reports whether this run inserted the row, and whether a row is stored
// afterwards (inserted here, or by a replica that won the race).
func importLegacyEmail(
	ctx context.Context, deps LegacyEmailImportDeps, logger *slog.Logger, legacy config.LegacyEmailConfig,
) (inserted, stored bool) {
	req := legacyEmailRequest(legacy, logger)
	if err := validateInstanceUpsertRequest(req, false); err != nil {
		logger.Error("The config.yaml email: section is invalid; nothing was imported",
			"section", legacyEmailSection, "fields", validationFieldsSummary(err))
		return false, false
	}

	secretEncrypted, err := encryptLegacySecret(deps.Enc, req.Secret)
	if err != nil {
		logger.Error(legacyEmailImportFailedMsg,
			"section", legacyEmailSection, "error", err)
		return false, false
	}

	row, err := instanceRowFromRequest(req, secretEncrypted, "")
	if err != nil {
		logger.Error(legacyEmailImportFailedMsg,
			"section", legacyEmailSection, "error", err)
		return false, false
	}

	inserted, err = deps.Repo.InsertIfAbsent(ctx, row)
	if err != nil {
		logger.Error(legacyEmailImportFailedMsg,
			"section", legacyEmailSection, "error", err)
		return false, false
	}
	if !inserted {
		// Another replica imported first, or an admin saved in between: the row
		// that exists wins, and its writer owns the audit entry.
		logger.Info("The instance email provider was stored concurrently; the config.yaml email: section was not imported",
			"section", legacyEmailSection)
		return false, true
	}

	logger.Info("Imported the config.yaml email: section into the database",
		"section", legacyEmailSection, "provider_type", row.ProviderType)

	secretMarker := ""
	if req.Secret != nil {
		secretMarker = models.InstanceSettingsAuditSecretChanged
	}
	if err := appendLegacyEmailImportAudit(ctx, deps.Audit, row, secretMarker); err != nil {
		// The row is stored and serving mail; only the record of it is missing.
		logger.Error("Imported the config.yaml email: section but failed to audit it",
			"section", legacyEmailSection, "error", err)
	}
	return true, true
}

// legacyEmailRequest maps the legacy block onto the admin API's request, so the
// import is validated and stored exactly like an admin save:
//   - from_address falls back to smtp.username when that is an email address,
//     mirroring the legacy sender;
//   - the privacy policy URL is dropped when it is the defaults() placeholder,
//     which the operator never chose;
//   - only the selected provider's settings and secret are taken; another
//     provider's credential is never stored, and its drop is logged.
func legacyEmailRequest(c config.LegacyEmailConfig, logger *slog.Logger) models.UpsertInstanceEmailProviderRequest {
	providerType := legacyEmailProviderType(c)

	fromAddress := strings.TrimSpace(c.FromAddress)
	if username := strings.TrimSpace(c.SMTP.Username); fromAddress == "" && isBareEmailAddress(username) {
		fromAddress = username
	}

	req := models.UpsertInstanceEmailProviderRequest{
		UpsertTeamEmailProviderRequest: models.UpsertTeamEmailProviderRequest{
			ProviderType: providerType,
			FromAddress:  fromAddress,
		},
		ContactRecipientAddress: trimOptional(&c.ContactRecipientAddress),
	}
	if privacy := strings.TrimSpace(c.PrivacyPolicyURL); privacy != config.DefaultLegacyPrivacyPolicyURL {
		req.PrivacyPolicyURL = trimOptional(&privacy)
	}

	secrets := map[string]string{
		EmailProviderTypeSMTP:     c.SMTP.Password,
		EmailProviderTypeMailgun:  c.Mailgun.SendingKey,
		EmailProviderTypePostmark: c.Postmark.ServerToken,
		EmailProviderTypeSendGrid: c.SendGrid.APIKey,
	}
	// The secret is taken verbatim (a password may legitimately carry spaces);
	// only a blank one is treated as absent.
	if secret := secrets[providerType]; hasLegacySecret(secret) {
		req.Secret = &secret
	}
	logDroppedLegacyCredentials(logger, providerType, secrets)

	switch providerType {
	case EmailProviderTypeSMTP:
		req.Settings.SMTP = &models.SMTPProviderSettings{
			Host:     strings.TrimSpace(c.SMTP.Host),
			Port:     strings.TrimSpace(c.SMTP.Port),
			Username: c.SMTP.Username,
		}
	case EmailProviderTypeMailgun:
		req.Settings.Mailgun = &models.MailgunProviderSettings{
			Domain:  strings.TrimSpace(c.Mailgun.Domain),
			BaseURL: strings.TrimSpace(c.Mailgun.BaseURL),
		}
	case EmailProviderTypePostmark:
		req.Settings.Postmark = &models.PostmarkProviderSettings{
			MessageStream: strings.TrimSpace(c.Postmark.MessageStream),
		}
	}
	return req
}

// logDroppedLegacyCredentials notes, once, the credentials of providers other
// than the selected one: they are not imported.
func logDroppedLegacyCredentials(logger *slog.Logger, providerType string, secrets map[string]string) {
	var dropped []string
	for _, other := range []string{
		EmailProviderTypeSMTP, EmailProviderTypeMailgun, EmailProviderTypePostmark, EmailProviderTypeSendGrid,
	} {
		if other != providerType && hasLegacySecret(secrets[other]) {
			dropped = append(dropped, other)
		}
	}
	if len(dropped) > 0 {
		logger.Info("Credentials of unselected providers in the config.yaml email: section are not imported",
			"section", legacyEmailSection, "provider_type", providerType, "dropped", dropped)
	}
}

// hasLegacySecret reports whether a legacy credential is set. A blank one is
// absent, both for the populated rule and for the import, so the two agree.
func hasLegacySecret(secret string) bool {
	return strings.TrimSpace(secret) != ""
}

// legacyEmailProviderType is the selected provider, smtp when unset.
func legacyEmailProviderType(c config.LegacyEmailConfig) string {
	if providerType := normalizeProviderType(c.Provider); providerType != "" {
		return providerType
	}
	return EmailProviderTypeSMTP
}

func encryptLegacySecret(enc EncryptionServiceInterface, secret *string) (*string, error) {
	if secret == nil {
		return nil, nil
	}
	if enc == nil {
		return nil, fmt.Errorf("failed to encrypt the provider secret: %w", ErrEncryptionUnavailable)
	}
	encrypted, err := enc.Encrypt(*secret)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt the provider secret: %w", err)
	}
	return &encrypted, nil
}

// appendLegacyEmailImportAudit records the import: no actor, no before
// snapshot, and the same redacted after snapshot an admin save writes.
func appendLegacyEmailImportAudit(
	ctx context.Context, audit repositories.InstanceSettingsAuditRepository,
	row *models.InstanceEmailProvider, secretMarker string,
) error {
	after, err := InstanceEmailAuditSnapshot(row, secretMarker)
	if err != nil {
		return err
	}
	return audit.Append(ctx, &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingEmailProvider,
		Action:  models.InstanceSettingsAuditActionImport,
		After:   after,
	})
}

// warnInstanceEmailUnconfigured reports an instance with no email row: a
// warning when nobody can fix it (no instance admin, epic decision 8), a
// pointer to the settings page otherwise.
func warnInstanceEmailUnconfigured(logger *slog.Logger, instanceAdmins []string) {
	if !hasInstanceAdmin(instanceAdmins) {
		logger.Warn("Instance email is not configured and auth.instance_admins is empty, so nobody can configure it; " +
			"instance mail (invitations, notifications, digests) is discarded until an instance admin sets it " +
			"under Admin → Settings → Email")
		return
	}
	logger.Info("Instance email is not configured; an instance admin can configure it under Admin → Settings → Email")
}

// hasInstanceAdmin reports whether any auth.instance_admins entry is non-blank.
// Blank entries are accepted by the config loader and ignored by
// Config.IsInstanceAdmin, so they are not admins here either.
func hasInstanceAdmin(instanceAdmins []string) bool {
	for _, admin := range instanceAdmins {
		if strings.TrimSpace(admin) != "" {
			return true
		}
	}
	return false
}

// validationFieldsSummary renders a validation error's fields as
// "field: message" strings, or the error text for any other error.
func validationFieldsSummary(err error) []string {
	var verr *TeamEmailProviderValidationError
	if !errors.As(err, &verr) {
		return []string{err.Error()}
	}
	summary := make([]string, 0, len(verr.Fields))
	for _, field := range verr.Fields {
		summary = append(summary, field.Field+": "+field.Message)
	}
	return summary
}

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/external/implementations"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Sources reported by ResolvedEmailSender.Source.
const (
	EmailSenderSourceTeam     = "team"
	EmailSenderSourceInstance = "instance"
)

// ResolvedEmailSender is everything needed to send one message: which provider to
// send through and which identity to send as.
type ResolvedEmailSender struct {
	Provider    external.EmailProvider
	FromAddress string
	FromName    string
	ReplyTo     string
	// Source is "team" or "instance", for logging — it tells an operator reading
	// the logs whose credentials a message went out on.
	Source string
	// TeamID is the team whose provider was used; empty for the instance branch.
	TeamID string
}

// InstanceEmailIdentity is the instance's own mail identity, read from the
// instance_email_provider row: who instance mail comes from, where the contact
// form delivers, and the privacy policy linked from outbound mail. Every field
// is empty when the instance has no row.
type InstanceEmailIdentity struct {
	FromAddress             string
	ContactRecipientAddress string
	PrivacyPolicyURL        string
}

// EmailSenderResolver picks the provider and sender identity for a message.
//
// This is the codebase's first team-override -> instance-fallback resolver. The
// existing per-team provider domains have no fallback at all
// (EmbeddingProviderService.ResolveActiveProvider returns (nil, nil) and callers
// no-op), so this is new architecture rather than a clone of one.
//
// Both branches are resolved at SEND time from the database, never at wire
// time and never cached (the same reasoning as TeamSearchSettingsResolver): a
// team or instance admin's change takes effect on the next message, with no
// restart, and one primary-key read is negligible next to an SMTP or HTTP send.
type EmailSenderResolver interface {
	Resolve(ctx context.Context, teamID string) (*ResolvedEmailSender, error)
	// RecordSendOutcome stamps the delivery health of one send attempt on the
	// row the sender was built from: the team's row for a team sender, the
	// instance row for an instance sender. It is a no-op for the stub sender of
	// an unconfigured instance, which has no row to stamp.
	//
	// It lives beside Resolve because only this type knows which row a sender
	// came from — pushing that decision to the caller would invite an instance
	// send being recorded against whichever team happened to be in scope.
	RecordSendOutcome(ctx context.Context, sender *ResolvedEmailSender, sendErr error) error
	// InstanceIdentity reads the instance's mail identity for the parts of a
	// send that happen before Resolve (template rendering, the contact-form
	// recipient). It returns zero values when the instance has no row, and an
	// error only when the row could not be read.
	InstanceIdentity(ctx context.Context) (InstanceEmailIdentity, error)
}

// instanceUnconfiguredWarnInterval bounds how often an unconfigured instance is
// reported. Rate-limited rather than once per process, so a row deleted in a
// long-lived process is still reported again.
const instanceUnconfiguredWarnInterval = 15 * time.Minute

// teamEmailSenderResolver resolves a team's own provider, falling back to the
// instance provider stored in instance_email_provider.
type teamEmailSenderResolver struct {
	repo         repositories.TeamEmailProviderRepository
	instanceRepo repositories.InstanceEmailProviderRepository
	enc          EncryptionServiceInterface
	logger       *slog.Logger
	now          func() time.Time
	// lastUnconfiguredWarn is the unix-nano time of the last "instance email is
	// not configured" warning; zero when none has been logged.
	lastUnconfiguredWarn atomic.Int64
}

var _ EmailSenderResolver = (*teamEmailSenderResolver)(nil)

// NewEmailSenderResolver creates the send-time sender resolver.
func NewEmailSenderResolver(
	repo repositories.TeamEmailProviderRepository,
	instanceRepo repositories.InstanceEmailProviderRepository,
	enc EncryptionServiceInterface,
	logger *slog.Logger,
) EmailSenderResolver {
	return &teamEmailSenderResolver{
		repo:         repo,
		instanceRepo: instanceRepo,
		enc:          enc,
		logger:       logger,
		now:          time.Now,
	}
}

// Resolve returns the provider and identity to send a message for teamID with.
//
// Only two conditions select the instance provider: no team at all, and a team
// with no configured row. Every other outcome — a repository failure, a secret
// that will not decrypt, a row that will not build a provider — is returned as an
// error and never silently falls back (epic #499 decision 7). Falling back there
// would send a team's mail from the operator's address using the operator's
// credentials, which is precisely what a team configures its own provider to
// avoid.
func (r *teamEmailSenderResolver) Resolve(
	ctx context.Context, teamID string,
) (*ResolvedEmailSender, error) {
	if strings.TrimSpace(teamID) == "" {
		return r.instanceSender(ctx)
	}

	provider, err := r.repo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrTeamEmailProviderNotFound) {
			return r.instanceSender(ctx)
		}
		return nil, fmt.Errorf("failed to resolve the team email provider: %w", err)
	}

	return r.teamSender(provider)
}

// instanceSender builds the instance sender from the instance row, read per
// send.
//
// No row is the one non-error fallback: the instance has not configured mail,
// so messages go to the no-op stub and a rate-limited warning says so
// (epic #1185 decision 8, unconfigured stays non-fatal). Every other failure is
// an error, exactly as on the team branch.
func (r *teamEmailSenderResolver) instanceSender(ctx context.Context) (*ResolvedEmailSender, error) {
	row, err := r.instanceRepo.Get(ctx)
	if err != nil {
		if errors.Is(err, repositories.ErrInstanceEmailProviderNotFound) {
			r.warnUnconfigured()
			return &ResolvedEmailSender{
				Provider: &implementations.StubEmailProvider{},
				Source:   EmailSenderSourceInstance,
			}, nil
		}
		return nil, fmt.Errorf("failed to resolve the instance email provider: %w", err)
	}

	secret, err := r.decryptSecret(derefOrEmpty(row.SecretEncrypted))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt the instance email provider secret: %w", err)
	}

	built, err := buildStoredProvider(row.ProviderType, row.Settings, secret, instanceEmailLabel, r.logger)
	if err != nil {
		return nil, err
	}

	return &ResolvedEmailSender{
		Provider:    built,
		FromAddress: InstanceFromAddress(row),
		FromName:    derefOrEmpty(row.FromName),
		ReplyTo:     derefOrEmpty(row.ReplyTo),
		Source:      EmailSenderSourceInstance,
	}, nil
}

// warnUnconfigured logs that instance mail is being discarded, at most once per
// instanceUnconfiguredWarnInterval. The compare-and-swap keeps concurrent sends
// from each logging the same warning.
func (r *teamEmailSenderResolver) warnUnconfigured() {
	now := r.now()
	last := r.lastUnconfiguredWarn.Load()
	if last != 0 && now.Sub(time.Unix(0, last)) < instanceUnconfiguredWarnInterval {
		return
	}
	if !r.lastUnconfiguredWarn.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	r.logger.Warn("Instance email provider is not configured; instance mail is being discarded by the stub provider")
}

// InstanceIdentity reads the instance's mail identity. See the interface.
func (r *teamEmailSenderResolver) InstanceIdentity(ctx context.Context) (InstanceEmailIdentity, error) {
	row, err := r.instanceRepo.Get(ctx)
	if err != nil {
		if errors.Is(err, repositories.ErrInstanceEmailProviderNotFound) {
			return InstanceEmailIdentity{}, nil
		}
		return InstanceEmailIdentity{}, fmt.Errorf("failed to read the instance email identity: %w", err)
	}
	return InstanceEmailIdentity{
		FromAddress:             InstanceFromAddress(row),
		ContactRecipientAddress: derefOrEmpty(row.ContactRecipientAddress),
		PrivacyPolicyURL:        derefOrEmpty(row.PrivacyPolicyURL),
	}, nil
}

// InstanceFromAddress is the instance's envelope sender: the row's
// from_address, or empty when the instance has no row.
//
// Exported and kept in ONE place because several callers need the identical
// value — this resolver, InstanceIdentity, and the team settings endpoint that
// reports the effective from-address of a team inheriting the instance.
func InstanceFromAddress(row *models.InstanceEmailProvider) string {
	if row == nil {
		return ""
	}
	return row.FromAddress
}

// teamSender builds a provider from a stored row: decrypt the secret, map the
// row onto the ProviderSpec from #500, and let the shared factory construct it.
// Going through the factory is what keeps the four-way provider switch in one
// place.
func (r *teamEmailSenderResolver) teamSender(
	provider *models.TeamEmailProvider,
) (*ResolvedEmailSender, error) {
	secret, err := r.decryptSecret(provider.SecretEncrypted)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to decrypt the email provider secret for team %s: %w", provider.TeamID, err)
	}

	built, err := buildStoredProvider(
		provider.ProviderType, provider.Settings, secret, teamEmailLabel(provider.TeamID), r.logger)
	if err != nil {
		return nil, err
	}

	return &ResolvedEmailSender{
		Provider:    built,
		FromAddress: provider.FromAddress,
		FromName:    derefOrEmpty(provider.FromName),
		ReplyTo:     derefOrEmpty(provider.ReplyTo),
		Source:      EmailSenderSourceTeam,
		TeamID:      provider.TeamID,
	}, nil
}

// RecordSendOutcome stamps delivery health on the row the sender was built
// from.
//
// Success is recorded as well as failure, not just failure: health is derived by
// comparing last_success_at with last_error_at, so a provider that only ever
// records failures could never be shown as recovered.
func (r *teamEmailSenderResolver) RecordSendOutcome(
	ctx context.Context, sender *ResolvedEmailSender, sendErr error,
) error {
	if sender == nil {
		return nil
	}
	switch sender.Source {
	case EmailSenderSourceTeam:
		if sender.TeamID == "" {
			return nil
		}
		return r.repo.RecordSendResult(ctx, sender.TeamID, sendErr, r.now().UTC())
	case EmailSenderSourceInstance:
		return r.recordInstanceOutcome(ctx, sender, sendErr)
	default:
		return nil
	}
}

// recordInstanceOutcome stamps the instance row. The stub sender of an
// unconfigured instance has no row, and a row deleted between Resolve and this
// call is gone: both are no-ops rather than errors.
func (r *teamEmailSenderResolver) recordInstanceOutcome(
	ctx context.Context, sender *ResolvedEmailSender, sendErr error,
) error {
	if _, isStub := sender.Provider.(*implementations.StubEmailProvider); isStub {
		return nil
	}

	at := r.now().UTC()
	var err error
	if sendErr == nil {
		err = r.instanceRepo.RecordSuccess(ctx, at)
	} else {
		err = r.instanceRepo.RecordError(ctx, sendErr, at)
	}
	if errors.Is(err, repositories.ErrInstanceEmailProviderNotFound) {
		return nil
	}
	return err
}

func (r *teamEmailSenderResolver) decryptSecret(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if r.enc == nil {
		return "", ErrEncryptionUnavailable
	}
	return r.enc.Decrypt(ciphertext)
}

// instanceEmailLabel names the instance in error messages about its stored
// provider, as teamEmailLabel names a team.
const instanceEmailLabel = "the instance"

func teamEmailLabel(teamID string) string {
	return "team " + teamID
}

// buildStoredProvider builds a provider from a stored configuration (a team or
// the instance row) through the shared factory, which keeps the four-way
// provider switch in one place. label names the owner in error messages.
//
// NewEmailProvider answers an SMTP spec with no host or port with the no-op
// stub. For a STORED configuration that would accept and discard every message
// while reporting success; validation keeps such a row out via the API, so
// reaching it means the row was written some other way. It is reported rather
// than silently dropping the mail (epic #499 decision 7).
func buildStoredProvider(
	providerType string, settings json.RawMessage, secret, label string, logger *slog.Logger,
) (external.EmailProvider, error) {
	spec, err := providerSpecFromStored(providerType, settings, secret, label)
	if err != nil {
		return nil, err
	}

	built, err := implementations.NewEmailProvider(spec, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build the email provider for %s: %w", label, err)
	}

	if _, isStub := built.(*implementations.StubEmailProvider); isStub {
		return nil, fmt.Errorf(
			"%s has an incomplete email provider configuration: it would discard messages silently", label)
	}
	return built, nil
}

// providerSpecFromRow maps a stored team row plus its decrypted secret onto a
// ProviderSpec.
func providerSpecFromRow(
	provider *models.TeamEmailProvider, secret string,
) (implementations.ProviderSpec, error) {
	return providerSpecFromStored(
		provider.ProviderType, provider.Settings, secret, teamEmailLabel(provider.TeamID))
}

// providerSpecFromStored maps a stored configuration plus its decrypted secret
// onto a ProviderSpec. The settings jsonb is shaped by provider_type, so only
// the matching block is decoded. label names the owner in error messages.
func providerSpecFromStored(
	providerType string, settings json.RawMessage, secret, label string,
) (implementations.ProviderSpec, error) {
	spec := implementations.ProviderSpec{Type: normalizeProviderType(providerType)}

	switch spec.Type {
	case EmailProviderTypeSMTP:
		var block models.SMTPProviderSettings
		if err := decodeStoredSettings(settings, label, &block); err != nil {
			return spec, err
		}
		spec.SMTP = implementations.SMTPSpec{
			Host:     block.Host,
			Port:     block.Port,
			Username: block.Username,
			Password: secret,
		}
	case EmailProviderTypeMailgun:
		var block models.MailgunProviderSettings
		if err := decodeStoredSettings(settings, label, &block); err != nil {
			return spec, err
		}
		spec.Mailgun = implementations.MailgunSpec{
			BaseURL:    block.BaseURL,
			Domain:     block.Domain,
			SendingKey: secret,
		}
	case EmailProviderTypePostmark:
		var block models.PostmarkProviderSettings
		if err := decodeStoredSettings(settings, label, &block); err != nil {
			return spec, err
		}
		spec.Postmark = implementations.PostmarkSpec{
			ServerToken:   secret,
			MessageStream: block.MessageStream,
		}
	case EmailProviderTypeSendGrid:
		spec.SendGrid = implementations.SendGridSpec{APIKey: secret}
	default:
		// A row can only hold an unknown type if it was written before the type
		// was removed, or by hand. Report it rather than falling through to the
		// instance provider.
		return spec, fmt.Errorf("%s has an unsupported email provider type %q", label, providerType)
	}

	return spec, nil
}

func decodeStoredSettings(settings json.RawMessage, label string, target any) error {
	if len(settings) == 0 {
		return nil
	}
	if err := json.Unmarshal(settings, target); err != nil {
		return fmt.Errorf("failed to decode the email provider settings for %s: %w", label, err)
	}
	return nil
}

func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

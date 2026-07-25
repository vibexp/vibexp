package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vibexp/vibexp/internal/config"
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

// EmailSenderResolver picks the provider and sender identity for a message.
//
// This is the codebase's first team-override -> instance-fallback resolver. The
// existing per-team provider domains have no fallback at all
// (EmbeddingProviderService.ResolveActiveProvider returns (nil, nil) and callers
// no-op), so this is new architecture rather than a clone of one.
type EmailSenderResolver interface {
	Resolve(ctx context.Context, teamID string) (*ResolvedEmailSender, error)
}

// teamEmailSenderResolver resolves a team's own provider, falling back to the
// instance provider wired at container build time.
type teamEmailSenderResolver struct {
	repo repositories.TeamEmailProviderRepository
	enc  EncryptionServiceInterface
	// instanceProvider is the process-wide provider built from config.yaml at
	// wire time. It is the fallback, and is used as-is.
	instanceProvider external.EmailProvider
	cfg              *config.Config
	logger           *slog.Logger
}

var _ EmailSenderResolver = (*teamEmailSenderResolver)(nil)

// NewEmailSenderResolver creates the send-time sender resolver.
func NewEmailSenderResolver(
	repo repositories.TeamEmailProviderRepository,
	enc EncryptionServiceInterface,
	instanceProvider external.EmailProvider,
	cfg *config.Config,
	logger *slog.Logger,
) EmailSenderResolver {
	return &teamEmailSenderResolver{
		repo:             repo,
		enc:              enc,
		instanceProvider: instanceProvider,
		cfg:              cfg,
		logger:           logger,
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
		return r.instanceSender(), nil
	}

	provider, err := r.repo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrTeamEmailProviderNotFound) {
			return r.instanceSender(), nil
		}
		return nil, fmt.Errorf("failed to resolve the team email provider: %w", err)
	}

	return r.teamSender(provider)
}

// instanceSender reproduces today's instance behaviour exactly: the wire-time
// provider plus the cfg.Email.FromAddress -> cfg.Email.SMTP.Username from-address
// chain used by EmailService.sendEmail, so nothing changes for a team without its
// own provider.
func (r *teamEmailSenderResolver) instanceSender() *ResolvedEmailSender {
	from := ""
	if r.cfg != nil {
		from = r.cfg.Email.FromAddress
		if from == "" {
			from = r.cfg.Email.SMTP.Username
		}
	}

	return &ResolvedEmailSender{
		Provider:    r.instanceProvider,
		FromAddress: from,
		Source:      EmailSenderSourceInstance,
	}
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

	spec, err := providerSpecFromRow(provider, secret)
	if err != nil {
		return nil, err
	}

	built, err := implementations.NewEmailProvider(spec, r.logger)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to build the email provider for team %s: %w", provider.TeamID, err)
	}

	// NewEmailProvider answers an SMTP spec with no host or port with the no-op
	// stub, which is the right behaviour for an instance that has not configured
	// mail — but for a team that HAS configured one it would accept and discard
	// every message while reporting success. Validation keeps such a row out via
	// the API, so reaching this means the row was written some other way; report
	// it rather than silently dropping the team's mail (epic #499 decision 7).
	if _, isStub := built.(*implementations.StubEmailProvider); isStub {
		return nil, fmt.Errorf(
			"team %s has an incomplete email provider configuration: it would discard messages silently",
			provider.TeamID)
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

func (r *teamEmailSenderResolver) decryptSecret(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if r.enc == nil {
		return "", ErrEncryptionUnavailable
	}
	return r.enc.Decrypt(ciphertext)
}

// providerSpecFromRow maps a stored row plus its decrypted secret onto a
// ProviderSpec. The row's settings jsonb is shaped by provider_type, so only the
// matching block is decoded.
func providerSpecFromRow(
	provider *models.TeamEmailProvider, secret string,
) (implementations.ProviderSpec, error) {
	spec := implementations.ProviderSpec{Type: normalizeProviderType(provider.ProviderType)}

	switch spec.Type {
	case EmailProviderTypeSMTP:
		var settings models.SMTPProviderSettings
		if err := decodeRowSettings(provider, &settings); err != nil {
			return spec, err
		}
		spec.SMTP = implementations.SMTPSpec{
			Host:     settings.Host,
			Port:     settings.Port,
			Username: settings.Username,
			Password: secret,
		}
	case EmailProviderTypeMailgun:
		var settings models.MailgunProviderSettings
		if err := decodeRowSettings(provider, &settings); err != nil {
			return spec, err
		}
		spec.Mailgun = implementations.MailgunSpec{
			BaseURL:    settings.BaseURL,
			Domain:     settings.Domain,
			SendingKey: secret,
		}
	case EmailProviderTypePostmark:
		var settings models.PostmarkProviderSettings
		if err := decodeRowSettings(provider, &settings); err != nil {
			return spec, err
		}
		spec.Postmark = implementations.PostmarkSpec{
			ServerToken:   secret,
			MessageStream: settings.MessageStream,
		}
	case EmailProviderTypeSendGrid:
		spec.SendGrid = implementations.SendGridSpec{APIKey: secret}
	default:
		// A row can only hold an unknown type if it was written before the type
		// was removed, or by hand. Report it rather than falling through to the
		// instance provider.
		return spec, fmt.Errorf(
			"team %s has an unsupported email provider type %q",
			provider.TeamID, provider.ProviderType)
	}

	return spec, nil
}

func decodeRowSettings(provider *models.TeamEmailProvider, target any) error {
	if len(provider.Settings) == 0 {
		return nil
	}
	if err := json.Unmarshal(provider.Settings, target); err != nil {
		return fmt.Errorf(
			"failed to decode the email provider settings for team %s: %w", provider.TeamID, err)
	}
	return nil
}

func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

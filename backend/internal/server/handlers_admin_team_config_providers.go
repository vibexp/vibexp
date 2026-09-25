package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
)

// Read-only team configuration for instance admins, part 2 (#1141): the
// credential-bearing sections — model and embedding providers, the email
// provider and GitHub.
//
// Same shape as part 1 (handlers_admin_team_config.go): requireAdminTeam, then
// the existing teamID-only getter, then a field-by-field converter into an
// admin-only generated type. Redaction is by construction (epic #1131
// decision 10): the admin schema has no field for any secret, and the
// converters COMPUTE has_*, configuration_keys, webhook_configured and status
// rather than copying the values they are derived from. None of these reads
// decrypts anything or calls out to a provider or to GitHub.

// GetAdminTeamModelProviders returns the team's model providers, redacted.
func (a *adminStrictServer) GetAdminTeamModelProviders(
	ctx context.Context, request admingen.GetAdminTeamModelProvidersRequestObject,
) (admingen.GetAdminTeamModelProvidersResponseObject, error) {
	const handler = "GetAdminTeamModelProviders"
	teamID := request.Id.String()
	if err := a.requireAdminTeam(ctx, handler, teamID); err != nil {
		return nil, err
	}

	providers, err := a.s.container.ModelProviderService().GetModelProvidersByTeamID(ctx, teamID)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	// make(...,0,...): `providers` is a required array on a generated type.
	genProviders := make([]admingen.AdminModelProvider, 0, len(providers))
	for i := range providers {
		converted, cerr := toGenAdminModelProvider(&providers[i])
		if cerr != nil {
			return nil, a.adminConfigInternalError(handler, teamID, cerr)
		}
		genProviders = append(genProviders, converted)
	}

	return admingen.GetAdminTeamModelProviders200JSONResponse(admingen.AdminTeamModelProvidersConfig{
		Providers: genProviders,
	}), nil
}

func toGenAdminModelProvider(p *models.ModelProviderResponse) (admingen.AdminModelProvider, error) {
	id, err := parseAdminUUID("model provider", p.ID)
	if err != nil {
		return admingen.AdminModelProvider{}, err
	}
	return admingen.AdminModelProvider{
		Id:                id,
		Name:              p.Name,
		ProviderType:      p.ProviderType,
		Model:             p.Model,
		BaseUrl:           redactBaseURL(p.BaseURL),
		IsDefault:         p.IsDefault,
		HasApiKey:         p.HasAPIKey,
		ConfigurationKeys: configurationKeys(p.Configuration),
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}, nil
}

// GetAdminTeamEmbeddingProviders returns the team's embedding providers,
// redacted, plus its embedding coverage counts.
func (a *adminStrictServer) GetAdminTeamEmbeddingProviders(
	ctx context.Context, request admingen.GetAdminTeamEmbeddingProvidersRequestObject,
) (admingen.GetAdminTeamEmbeddingProvidersResponseObject, error) {
	const handler = "GetAdminTeamEmbeddingProviders"
	teamID := request.Id.String()
	if err := a.requireAdminTeam(ctx, handler, teamID); err != nil {
		return nil, err
	}

	providers, err := a.s.container.EmbeddingProviderService().GetEmbeddingProvidersByTeamID(ctx, teamID)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}
	coverage, err := a.s.container.EmbeddingStatusService().GetCoverage(ctx, teamID)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	// make(...,0,...): `providers` is a required array on a generated type.
	genProviders := make([]admingen.AdminEmbeddingProvider, 0, len(providers))
	for i := range providers {
		converted, cerr := toGenAdminEmbeddingProvider(&providers[i])
		if cerr != nil {
			return nil, a.adminConfigInternalError(handler, teamID, cerr)
		}
		genProviders = append(genProviders, converted)
	}

	return admingen.GetAdminTeamEmbeddingProviders200JSONResponse(admingen.AdminTeamEmbeddingProvidersConfig{
		Providers: genProviders,
		Coverage:  toGenAdminEmbeddingCoverage(coverage),
	}), nil
}

func toGenAdminEmbeddingProvider(p *models.EmbeddingProviderResponse) (admingen.AdminEmbeddingProvider, error) {
	id, err := parseAdminUUID("embedding provider", p.ID)
	if err != nil {
		return admingen.AdminEmbeddingProvider{}, err
	}
	return admingen.AdminEmbeddingProvider{
		Id:                id,
		Name:              p.Name,
		ProviderType:      p.ProviderType,
		Model:             p.Model,
		ChunkSize:         p.ChunkSize,
		ChunkOverlap:      p.ChunkOverlap,
		Concurrency:       p.Concurrency,
		QueryPrefix:       p.QueryPrefix,
		DocumentPrefix:    p.DocumentPrefix,
		BaseUrl:           redactBaseURL(p.BaseURL),
		IsDefault:         p.IsDefault,
		HasApiKey:         p.HasAPIKey,
		ConfigurationKeys: configurationKeys(p.Configuration),
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}, nil
}

func toGenAdminEmbeddingCoverage(c *models.EmbeddingCoverageResponse) admingen.AdminEmbeddingCoverage {
	// make(...,0,...): `items` is a required array on a generated type.
	items := make([]admingen.AdminEmbeddingCoverageItem, 0)
	if c == nil {
		return admingen.AdminEmbeddingCoverage{Items: items}
	}
	for _, item := range c.Coverage {
		items = append(items, admingen.AdminEmbeddingCoverageItem{
			EntityType:      item.EntityType,
			Total:           item.Total,
			Embedded:        item.Embedded,
			Pending:         item.Pending,
			EmbeddedPercent: item.EmbeddedPercent,
		})
	}
	return admingen.AdminEmbeddingCoverage{
		HasActiveProvider: c.HasActiveProvider,
		ActiveModel:       c.ActiveModel,
		Items:             items,
	}
}

// GetAdminTeamEmailProvider returns the team's effective email provider,
// without its secret, last error text or SMTP username.
func (a *adminStrictServer) GetAdminTeamEmailProvider(
	ctx context.Context, request admingen.GetAdminTeamEmailProviderRequestObject,
) (admingen.GetAdminTeamEmailProviderResponseObject, error) {
	const handler = "GetAdminTeamEmailProvider"
	teamID := request.Id.String()
	if err := a.requireAdminTeam(ctx, handler, teamID); err != nil {
		return nil, err
	}

	// GetEffective ignores its userID (TeamEmailProviderService.Get discards
	// it), so the admin read passes none.
	eff, err := a.s.container.TeamEmailProviderService().GetEffective(ctx, "", teamID)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	return admingen.GetAdminTeamEmailProvider200JSONResponse(toGenAdminEmailProvider(eff)), nil
}

// toGenAdminEmailProvider maps the effective view field by field. LastError and
// the SMTP Username are deliberately never read.
func toGenAdminEmailProvider(eff *models.TeamEmailProviderEffective) admingen.AdminTeamEmailProviderConfig {
	return admingen.AdminTeamEmailProviderConfig{
		Configured:           eff.Configured,
		Source:               admingen.AdminTeamConfigSource(eff.Source),
		EffectiveFromAddress: eff.EffectiveFromAddress,
		ProviderType:         eff.ProviderType,
		FromAddress:          eff.FromAddress,
		FromName:             eff.FromName,
		ReplyTo:              eff.ReplyTo,
		Settings:             toGenAdminEmailSettings(eff.Settings),
		HasSecret:            eff.HasCredential,
		LastSuccessAt:        eff.LastSuccessAt,
		LastErrorAt:          eff.LastErrorAt,
		Status:               adminEmailStatus(eff),
	}
}

func toGenAdminEmailSettings(s *models.TeamEmailProviderSettings) *admingen.AdminEmailSettings {
	if s == nil {
		return nil
	}
	out := &admingen.AdminEmailSettings{}
	if s.SMTP != nil {
		out.Smtp = &admingen.AdminSMTPSettings{Host: s.SMTP.Host, Port: s.SMTP.Port}
	}
	if s.Mailgun != nil {
		baseURL := s.Mailgun.BaseURL
		out.Mailgun = &admingen.AdminMailgunSettings{
			Domain:  s.Mailgun.Domain,
			BaseUrl: redactBaseURL(&baseURL),
		}
	}
	if s.Postmark != nil {
		out.Postmark = &admingen.AdminPostmarkSettings{MessageStream: s.Postmark.MessageStream}
	}
	return out
}

// adminEmailStatus derives the provider's health from its two timestamps.
// TeamEmailProvider.IsHealthy reports a never-used provider as healthy; an
// operator view must not claim health nobody observed, so that case — and the
// inherited instance provider, whose health the team row cannot speak for — is
// `unknown`.
func adminEmailStatus(eff *models.TeamEmailProviderEffective) admingen.AdminTeamEmailProviderConfigStatus {
	if !eff.Configured || (eff.LastSuccessAt == nil && eff.LastErrorAt == nil) {
		return admingen.Unknown
	}
	if eff.LastErrorAt == nil {
		return admingen.Healthy
	}
	if eff.LastSuccessAt == nil || !eff.LastSuccessAt.After(*eff.LastErrorAt) {
		return admingen.Failing
	}
	return admingen.Healthy
}

// GetAdminTeamGitHubConfig returns the team's GitHub App registration (secrets
// and webhook URL redacted) and its recorded installation status. It never
// calls GitHub: repositories are not listed because the only source for them
// mints a token, calls the API and can delete the installation row.
func (a *adminStrictServer) GetAdminTeamGitHubConfig(
	ctx context.Context, request admingen.GetAdminTeamGitHubConfigRequestObject,
) (admingen.GetAdminTeamGitHubConfigResponseObject, error) {
	const handler = "GetAdminTeamGitHubConfig"
	teamID := request.Id.String()
	if err := a.requireAdminTeam(ctx, handler, teamID); err != nil {
		return nil, err
	}

	var appConfig *admingen.AdminGitHubAppConfig
	cfg, err := a.s.container.GitHubAppConfigService().GetAppConfig(ctx, teamID)
	switch {
	case errors.Is(err, services.ErrGitHubAppNotConfigured):
		// No App registered: the documented `app_config: null`, not an error.
	case err != nil:
		return nil, a.adminConfigInternalError(handler, teamID, err)
	default:
		converted, cerr := toGenAdminGitHubAppConfig(cfg)
		if cerr != nil {
			return nil, a.adminConfigInternalError(handler, teamID, cerr)
		}
		appConfig = &converted
	}

	status, err := a.s.container.GitHubAppService().GetInstallationStatus(ctx, teamID)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	return admingen.GetAdminTeamGitHubConfig200JSONResponse(admingen.AdminTeamGitHubConfig{
		AppConfig:    appConfig,
		Installation: toGenAdminGitHubInstallation(status),
	}), nil
}

// toGenAdminGitHubAppConfig maps the App registration. WebhookURL embeds the
// routing token, so only whether one could be composed is reported.
func toGenAdminGitHubAppConfig(c *models.GitHubAppConfigResponse) (admingen.AdminGitHubAppConfig, error) {
	id, err := parseAdminUUID("github app config", c.ID)
	if err != nil {
		return admingen.AdminGitHubAppConfig{}, err
	}
	return admingen.AdminGitHubAppConfig{
		Id:                id,
		AppId:             c.AppID,
		AppSlug:           c.AppSlug,
		ClientId:          c.ClientID,
		HasPrivateKey:     c.HasPrivateKey,
		HasClientSecret:   c.HasClientSecret,
		HasWebhookSecret:  c.HasWebhookSecret,
		WebhookConfigured: c.WebhookURL != "",
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
	}, nil
}

// toGenAdminGitHubInstallation maps the recorded installation. A team with no
// installation reports nulls rather than the zero id, login and time.
func toGenAdminGitHubInstallation(s *models.GitHubInstallationStatus) admingen.AdminGitHubInstallation {
	if s == nil || !s.Installed {
		return admingen.AdminGitHubInstallation{}
	}
	login := s.AccountLogin
	installationID := s.InstallationID
	var installedAt *time.Time
	if !s.InstalledAt.IsZero() {
		at := s.InstalledAt
		installedAt = &at
	}
	return admingen.AdminGitHubInstallation{
		Installed:      true,
		AccountLogin:   &login,
		InstallationId: &installationID,
		Suspended:      s.Suspended,
		InstalledAt:    installedAt,
	}
}

// configurationKeys lists the sorted top-level keys of a provider's stored
// configuration JSON, never its values. Empty, non-object or invalid input
// yields `[]`.
func configurationKeys(raw string) []string {
	keys := make([]string, 0)
	if raw == "" {
		return keys
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return keys
	}
	for key := range decoded {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// redactBaseURL returns a free-text base URL without userinfo, query string or
// fragment, any of which could carry a credential. Anything that is not an
// absolute URL (no scheme or host — e.g. `user:pass@host`, which parses as an
// opaque URL) is `null`: it cannot be redacted with confidence.
func redactBaseURL(raw *string) *string {
	if raw == nil || *raw == "" {
		return nil
	}
	u, err := url.Parse(*raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Opaque != "" {
		return nil
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	redacted := u.String()
	return &redacted
}

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Every sensitive fixture field in this file carries one of these, so a single
// assertBodyExcludes proves none of them reached the wire. They are consts
// marshalled through Go values, never JSON literals, so gitleaks does not read
// the fixtures as real credentials.
const (
	providerKeySentinel  = "SENTINEL-APIKEY"
	providerCfgSentinel  = "SENTINEL-CFG"
	baseURLUserSentinel  = "SENTINEL-URLUSER"
	baseURLQuerySentinel = "SENTINEL-URLQUERY"
	emailSecretSentinel  = "SENTINEL-EMAILSECRET"
	emailErrorSentinel   = "SENTINEL-ERR"
	smtpUserSentinel     = "SENTINEL-USER"
	githubSecretSentinel = "SENTINEL-GHSECRET"
	githubTokenSentinel  = "SENTINEL-TOKEN"
)

var allProviderSentinels = []string{
	providerKeySentinel, providerCfgSentinel, baseURLUserSentinel, baseURLQuerySentinel,
	emailSecretSentinel, emailErrorSentinel, smtpUserSentinel, githubSecretSentinel, githubTokenSentinel,
}

// sentinelConfiguration is a provider configuration whose VALUES are all
// sentinels; only its keys may surface.
func sentinelConfiguration(t *testing.T) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"api_key":  providerCfgSentinel,
		"headers":  map[string]any{"authorization": providerCfgSentinel},
		"org_hint": providerCfgSentinel,
	})
	require.NoError(t, err)
	return string(raw)
}

// sentinelBaseURL carries a credential in both the userinfo and the query.
func sentinelBaseURL() *string {
	u := "https://" + baseURLUserSentinel + ":pw@llm.example.com/v1?key=" + baseURLQuerySentinel + "#frag"
	return &u
}

func strRef(s string) *string { return &s }

var adminModelProviderPaths = []string{
	"id", "name", "provider_type", "model", "base_url", "is_default", "has_api_key",
	"configuration_keys", "created_at", "updated_at",
}

var adminEmbeddingProviderPaths = append([]string{
	"chunk_size", "chunk_overlap", "concurrency", "query_prefix", "document_prefix",
}, adminModelProviderPaths...)

func arrayPaths(prefix string, keys []string) []string {
	out := []string{prefix}
	for _, k := range keys {
		out = append(out, prefix+"[]."+k)
	}
	return out
}

func TestGetAdminTeamModelProviders(t *testing.T) {
	teamID := uuid.NewString()
	now := time.Now().UTC()
	providerID := uuid.NewString()
	svc := servicesmocks.NewMockModelProviderServiceInterface(t)
	svc.EXPECT().GetModelProvidersByTeamID(mock.Anything, teamID).Return([]models.ModelProviderResponse{
		{
			ModelProvider: models.ModelProvider{
				ID: providerID, UserID: uuid.NewString(), TeamID: &teamID, Name: "OpenAI",
				ProviderType: "openai", Model: "gpt-5", IsDefault: true, BaseURL: sentinelBaseURL(),
				APIKeyEncrypted: strRef(providerKeySentinel), Configuration: sentinelConfiguration(t),
				CreatedAt: now, UpdatedAt: now, Version: 7,
			},
			HasAPIKey: true,
		},
		{
			ModelProvider: models.ModelProvider{
				ID: uuid.NewString(), Name: "Local", ProviderType: "ollama", Model: "llama",
				Configuration: "", CreatedAt: now, UpdatedAt: now,
			},
		},
	}, nil)

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), modelProviderService: svc,
	}, adminConfigPaths(teamID)["model-providers"])

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminTeamModelProvidersConfig
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Providers, 2)
	first := resp.Providers[0]
	assert.Equal(t, providerID, first.Id.String())
	assert.True(t, first.HasApiKey)
	assert.True(t, first.IsDefault)
	assert.Equal(t, []string{"api_key", "headers", "org_hint"}, first.ConfigurationKeys)
	require.NotNil(t, first.BaseUrl)
	assert.Equal(t, "https://llm.example.com/v1", *first.BaseUrl)
	assert.False(t, resp.Providers[1].HasApiKey)
	assert.Nil(t, resp.Providers[1].BaseUrl)
	assert.Equal(t, []string{}, resp.Providers[1].ConfigurationKeys)

	assertJSONKeyPaths(t, rr.Body.Bytes(), arrayPaths("providers", adminModelProviderPaths)...)
	assertBodyExcludes(t, rr.Body.Bytes(), allProviderSentinels...)
	assert.NotContains(t, rr.Body.String(), `"version"`)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminTeamModelProviders_Empty(t *testing.T) {
	teamID := uuid.NewString()
	svc := servicesmocks.NewMockModelProviderServiceInterface(t)
	svc.EXPECT().GetModelProvidersByTeamID(mock.Anything, teamID).Return(nil, nil)

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), modelProviderService: svc,
	}, adminConfigPaths(teamID)["model-providers"])

	require.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{"providers":[]}`, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminTeamEmbeddingProviders(t *testing.T) {
	teamID := uuid.NewString()
	now := time.Now().UTC()
	svc := servicesmocks.NewMockEmbeddingProviderServiceInterface(t)
	svc.EXPECT().GetEmbeddingProvidersByTeamID(mock.Anything, teamID).Return([]models.EmbeddingProviderResponse{
		{
			EmbeddingProvider: models.EmbeddingProvider{
				ID: uuid.NewString(), UserID: uuid.NewString(), TeamID: &teamID, Name: "Voyage",
				ProviderType: "openai", Model: "voyage-3", ChunkSize: 800, ChunkOverlap: 80, Concurrency: 4,
				QueryPrefix: strRef("query: "), DocumentPrefix: strRef("passage: "), IsDefault: true,
				BaseURL: sentinelBaseURL(), APIKeyEncrypted: strRef(providerKeySentinel),
				Configuration: sentinelConfiguration(t), CreatedAt: now, UpdatedAt: now, Version: 3,
			},
			HasAPIKey: true,
		},
	}, nil)
	coverage := servicesmocks.NewMockEmbeddingCoverageGetter(t)
	coverage.EXPECT().GetCoverage(mock.Anything, teamID).Return(&models.EmbeddingCoverageResponse{
		HasActiveProvider: true, ActiveModel: strRef("voyage-3"),
		Coverage: models.JSONArray[models.EmbeddingCoverageItem]{
			{EntityType: "prompt", Total: 10, Embedded: 7, Pending: 3, EmbeddedPercent: 70},
		},
	}, nil)

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), embeddingProviderService: svc, embeddingStatusService: coverage,
	}, adminConfigPaths(teamID)["embedding-providers"])

	require.Equal(t, http.StatusOK, rr.Code)
	var resp admingen.AdminTeamEmbeddingProvidersConfig
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Providers, 1)
	p := resp.Providers[0]
	assert.Equal(t, 800, p.ChunkSize)
	assert.Equal(t, "query: ", *p.QueryPrefix)
	assert.Equal(t, "https://llm.example.com/v1", *p.BaseUrl)
	assert.Equal(t, []string{"api_key", "headers", "org_hint"}, p.ConfigurationKeys)
	assert.True(t, resp.Coverage.HasActiveProvider)
	assert.Equal(t, "voyage-3", *resp.Coverage.ActiveModel)
	require.Len(t, resp.Coverage.Items, 1)
	assert.Equal(t, int64(3), resp.Coverage.Items[0].Pending)

	want := arrayPaths("providers", adminEmbeddingProviderPaths)
	want = append(want, "coverage", "coverage.has_active_provider", "coverage.active_model")
	want = append(want, arrayPaths("coverage.items",
		[]string{"entity_type", "total", "embedded", "pending", "embedded_percent"})...)
	assertJSONKeyPaths(t, rr.Body.Bytes(), want...)
	assertBodyExcludes(t, rr.Body.Bytes(), allProviderSentinels...)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminTeamEmbeddingProviders_NoProvider(t *testing.T) {
	teamID := uuid.NewString()
	svc := servicesmocks.NewMockEmbeddingProviderServiceInterface(t)
	svc.EXPECT().GetEmbeddingProvidersByTeamID(mock.Anything, teamID).Return(nil, nil)
	coverage := servicesmocks.NewMockEmbeddingCoverageGetter(t)
	coverage.EXPECT().GetCoverage(mock.Anything, teamID).Return(&models.EmbeddingCoverageResponse{}, nil)

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), embeddingProviderService: svc, embeddingStatusService: coverage,
	}, adminConfigPaths(teamID)["embedding-providers"])

	require.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t,
		`{"providers":[],"coverage":{"has_active_provider":false,"active_model":null,"items":[]}}`,
		rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestGetAdminTeamEmbeddingProviders_Errors(t *testing.T) {
	teamID := uuid.NewString()
	t.Run("provider list", func(t *testing.T) {
		svc := servicesmocks.NewMockEmbeddingProviderServiceInterface(t)
		svc.EXPECT().GetEmbeddingProvidersByTeamID(mock.Anything, teamID).Return(nil, errors.New("db down"))

		req, rr := serveAdminConfig(t, &adminMockContainer{
			teamRepo: adminConfigTeamRepo(t, teamID), embeddingProviderService: svc,
		}, adminConfigPaths(teamID)["embedding-providers"])

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.NotContains(t, rr.Body.String(), "db down")
		specconformance.AssertConformsToSpec(t, req, rr)
	})
	t.Run("coverage", func(t *testing.T) {
		svc := servicesmocks.NewMockEmbeddingProviderServiceInterface(t)
		svc.EXPECT().GetEmbeddingProvidersByTeamID(mock.Anything, teamID).Return(nil, nil)
		coverage := servicesmocks.NewMockEmbeddingCoverageGetter(t)
		coverage.EXPECT().GetCoverage(mock.Anything, teamID).Return(nil, errors.New("db down"))

		req, rr := serveAdminConfig(t, &adminMockContainer{
			teamRepo: adminConfigTeamRepo(t, teamID), embeddingProviderService: svc, embeddingStatusService: coverage,
		}, adminConfigPaths(teamID)["embedding-providers"])

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})
}

func TestGetAdminTeamModelProviders_ServiceError(t *testing.T) {
	teamID := uuid.NewString()
	svc := servicesmocks.NewMockModelProviderServiceInterface(t)
	svc.EXPECT().GetModelProvidersByTeamID(mock.Anything, teamID).Return(nil, errors.New("db down"))

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), modelProviderService: svc,
	}, adminConfigPaths(teamID)["model-providers"])

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.NotContains(t, rr.Body.String(), "db down")
	specconformance.AssertConformsToSpec(t, req, rr)
}

var adminEmailProviderPaths = []string{
	"configured", "source", "effective_from_address", "provider_type", "from_address", "from_name",
	"reply_to", "settings", "has_secret", "last_success_at", "last_error_at", "status",
}

// sentinelTeamEmailProvider builds the effective view the REAL model
// constructor produces from a row whose every sensitive field is a sentinel,
// so the fixture carries LastError and the SMTP username exactly as the
// service would hand them over.
func sentinelTeamEmailProvider(success, failure *time.Time) *models.TeamEmailProviderEffective {
	provider := &models.TeamEmailProvider{
		ProviderType:    "smtp",
		FromAddress:     "noreply@acme.example",
		FromName:        strRef("Acme"),
		ReplyTo:         strRef("help@acme.example"),
		SecretEncrypted: emailSecretSentinel,
		LastSuccessAt:   success,
		LastError:       strRef(emailErrorSentinel),
		LastErrorAt:     failure,
	}
	settings := &models.TeamEmailProviderSettings{
		SMTP: &models.SMTPProviderSettings{Host: "smtp.acme.example", Port: "587", Username: smtpUserSentinel},
	}
	return models.NewTeamEmailProviderEffectiveTeam(provider, settings)
}

func serveAdminEmail(t *testing.T, eff *models.TeamEmailProviderEffective) (body []byte, code int) {
	t.Helper()
	teamID := uuid.NewString()
	svc := servicesmocks.NewMockTeamEmailProviderServiceInterface(t)
	svc.EXPECT().GetEffective(mock.Anything, "", teamID).Return(eff, nil)
	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), emailProviderService: svc,
	}, adminConfigPaths(teamID)["email-provider"])
	specconformance.AssertConformsToSpec(t, req, rr)
	return rr.Body.Bytes(), rr.Code
}

func TestGetAdminTeamEmailProvider_TeamProvider(t *testing.T) {
	success := time.Now().UTC().Add(-time.Hour)
	failure := success.Add(-time.Hour)
	eff := sentinelTeamEmailProvider(&success, &failure)
	require.NotNil(t, eff.LastError, "fixture must carry last_error for the no-leak check")

	body, code := serveAdminEmail(t, eff)

	require.Equal(t, http.StatusOK, code)
	var resp admingen.AdminTeamEmailProviderConfig
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.True(t, resp.Configured)
	assert.Equal(t, admingen.AdminTeamConfigSource("team"), resp.Source)
	assert.True(t, resp.HasSecret)
	assert.Equal(t, admingen.Healthy, resp.Status)
	require.NotNil(t, resp.Settings)
	require.NotNil(t, resp.Settings.Smtp)
	assert.Equal(t, "smtp.acme.example", resp.Settings.Smtp.Host)

	assertJSONKeyPaths(t, body, append(adminEmailProviderPaths,
		"settings.smtp", "settings.smtp.host", "settings.smtp.port")...)
	assertBodyExcludes(t, body, allProviderSentinels...)
	assert.NotContains(t, string(body), "last_error\"")
	assert.NotContains(t, string(body), "username")
}

func TestGetAdminTeamEmailProvider_InstanceSource(t *testing.T) {
	body, code := serveAdminEmail(t, models.NewTeamEmailProviderEffectiveInstance("mail@vibexp.example"))

	require.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, `{
		"configured": false, "source": "instance", "effective_from_address": "mail@vibexp.example",
		"provider_type": null, "from_address": null, "from_name": null, "reply_to": null,
		"settings": null, "has_secret": false, "last_success_at": null, "last_error_at": null,
		"status": "unknown"
	}`, string(body))
}

func TestGetAdminTeamEmailProvider_MailgunAndPostmarkSettings(t *testing.T) {
	eff := sentinelTeamEmailProvider(nil, nil)
	eff.Settings = &models.TeamEmailProviderSettings{
		Mailgun:  &models.MailgunProviderSettings{Domain: "mg.acme.example", BaseURL: *sentinelBaseURL()},
		Postmark: &models.PostmarkProviderSettings{MessageStream: "outbound"},
	}

	body, code := serveAdminEmail(t, eff)

	require.Equal(t, http.StatusOK, code)
	var resp admingen.AdminTeamEmailProviderConfig
	require.NoError(t, json.Unmarshal(body, &resp))
	require.NotNil(t, resp.Settings.Mailgun)
	assert.Equal(t, "mg.acme.example", resp.Settings.Mailgun.Domain)
	assert.Equal(t, "https://llm.example.com/v1", *resp.Settings.Mailgun.BaseUrl)
	assert.Equal(t, "outbound", resp.Settings.Postmark.MessageStream)
	assertBodyExcludes(t, body, allProviderSentinels...)
}

func TestGetAdminTeamEmailProvider_UndecodableSettings(t *testing.T) {
	eff := sentinelTeamEmailProvider(nil, nil)
	eff.Settings = nil

	body, code := serveAdminEmail(t, eff)

	require.Equal(t, http.StatusOK, code)
	var resp admingen.AdminTeamEmailProviderConfig
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Nil(t, resp.Settings)
	assert.True(t, resp.Configured)
}

func TestGetAdminTeamEmailProvider_ServiceError(t *testing.T) {
	teamID := uuid.NewString()
	svc := servicesmocks.NewMockTeamEmailProviderServiceInterface(t)
	svc.EXPECT().GetEffective(mock.Anything, "", teamID).Return(nil, errors.New("db down"))

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), emailProviderService: svc,
	}, adminConfigPaths(teamID)["email-provider"])

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestAdminEmailStatus(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	tests := []struct {
		name       string
		configured bool
		success    *time.Time
		failure    *time.Time
		want       admingen.AdminTeamEmailProviderConfigStatus
	}{
		{"instance is unknown", false, &newer, nil, admingen.Unknown},
		{"never used is unknown", true, nil, nil, admingen.Unknown},
		{"only successes is healthy", true, &newer, nil, admingen.Healthy},
		{"only failures is failing", true, nil, &newer, admingen.Failing},
		{"failure after success is failing", true, &older, &newer, admingen.Failing},
		{"same instant is failing", true, &older, &older, admingen.Failing},
		{"success after failure is healthy", true, &newer, &older, admingen.Healthy},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := adminEmailStatus(&models.TeamEmailProviderEffective{
				Configured: tc.configured, LastSuccessAt: tc.success, LastErrorAt: tc.failure,
			})
			assert.Equal(t, tc.want, got)
		})
	}
}

var adminGitHubAppConfigPaths = []string{
	"app_config.id", "app_config.app_id", "app_config.app_slug", "app_config.client_id",
	"app_config.has_private_key", "app_config.has_client_secret", "app_config.has_webhook_secret",
	"app_config.webhook_configured", "app_config.created_at", "app_config.updated_at",
}

var adminGitHubInstallationPaths = []string{
	"installation", "installation.installed", "installation.account_login",
	"installation.installation_id", "installation.suspended", "installation.installed_at",
}

func serveAdminGitHub(
	t *testing.T, cfg *models.GitHubAppConfigResponse, cfgErr error, status *models.GitHubInstallationStatus,
) (body []byte, code int) {
	t.Helper()
	teamID := uuid.NewString()
	configs := servicesmocks.NewMockGitHubAppConfigServiceInterface(t)
	configs.EXPECT().GetAppConfig(mock.Anything, teamID).Return(cfg, cfgErr)
	apps := servicesmocks.NewMockGitHubAppServiceInterface(t)
	apps.EXPECT().GetInstallationStatus(mock.Anything, teamID).Return(status, nil)

	req, rr := serveAdminConfig(t, &adminMockContainer{
		teamRepo: adminConfigTeamRepo(t, teamID), githubAppConfigService: configs, githubAppService: apps,
	}, adminConfigPaths(teamID)["github"])
	specconformance.AssertConformsToSpec(t, req, rr)
	return rr.Body.Bytes(), rr.Code
}

func TestGetAdminTeamGitHubConfig_Configured(t *testing.T) {
	now := time.Now().UTC()
	configID := uuid.NewString()
	cfg := &models.GitHubAppConfigResponse{
		GitHubAppConfig: models.GitHubAppConfig{
			ID: configID, TeamID: uuid.NewString(), UserID: strRef(uuid.NewString()),
			AppID: "12345", AppSlug: "acme-vibexp", ClientID: "Iv1.abc",
			PrivateKeyEncrypted:    githubSecretSentinel + "-pk",
			ClientSecretEncrypted:  githubSecretSentinel + "-cs",
			WebhookSecretEncrypted: githubSecretSentinel + "-ws",
			WebhookToken:           githubTokenSentinel,
			CreatedAt:              now, UpdatedAt: now, Version: 2,
		},
		HasPrivateKey: true, HasClientSecret: true, HasWebhookSecret: true,
		WebhookURL: "https://vibexp.example/api/v1/webhooks/github/" + githubTokenSentinel,
	}
	installedAt := now.Add(-24 * time.Hour)
	status := &models.GitHubInstallationStatus{
		Installed: true, AccountLogin: "acme", InstallationID: 987, Suspended: true, InstalledAt: installedAt,
	}

	body, code := serveAdminGitHub(t, cfg, nil, status)

	require.Equal(t, http.StatusOK, code)
	var resp admingen.AdminTeamGitHubConfig
	require.NoError(t, json.Unmarshal(body, &resp))
	require.NotNil(t, resp.AppConfig)
	assert.Equal(t, configID, resp.AppConfig.Id.String())
	assert.True(t, resp.AppConfig.WebhookConfigured)
	assert.True(t, resp.AppConfig.HasPrivateKey)
	assert.True(t, resp.Installation.Installed)
	assert.Equal(t, "acme", *resp.Installation.AccountLogin)
	assert.Equal(t, int64(987), *resp.Installation.InstallationId)
	assert.True(t, resp.Installation.Suspended)
	assert.WithinDuration(t, installedAt, *resp.Installation.InstalledAt, time.Second)

	assertJSONKeyPaths(t, body, append(append([]string{"app_config"}, adminGitHubAppConfigPaths...),
		adminGitHubInstallationPaths...)...)
	assertBodyExcludes(t, body, allProviderSentinels...)
	assert.NotContains(t, string(body), "webhook_url")
	assert.NotContains(t, string(body), "/webhooks/github/")
}

func TestGetAdminTeamGitHubConfig_NotConfiguredNotInstalled(t *testing.T) {
	body, code := serveAdminGitHub(t, nil, services.ErrGitHubAppNotConfigured,
		&models.GitHubInstallationStatus{Installed: false})

	require.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, `{"app_config":null,"installation":{"installed":false,"account_login":null,`+
		`"installation_id":null,"suspended":false,"installed_at":null}}`, string(body))
}

func TestGetAdminTeamGitHubConfig_NoWebhookURL(t *testing.T) {
	now := time.Now().UTC()
	cfg := &models.GitHubAppConfigResponse{
		GitHubAppConfig: models.GitHubAppConfig{
			ID: uuid.NewString(), AppID: "1", AppSlug: "s", ClientID: "c", CreatedAt: now, UpdatedAt: now,
		},
	}

	body, code := serveAdminGitHub(t, cfg, nil, &models.GitHubInstallationStatus{})

	require.Equal(t, http.StatusOK, code)
	var resp admingen.AdminTeamGitHubConfig
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.False(t, resp.AppConfig.WebhookConfigured)
	assert.False(t, resp.AppConfig.HasPrivateKey)
}

func TestGetAdminTeamGitHubConfig_Errors(t *testing.T) {
	teamID := uuid.NewString()
	t.Run("app config", func(t *testing.T) {
		configs := servicesmocks.NewMockGitHubAppConfigServiceInterface(t)
		configs.EXPECT().GetAppConfig(mock.Anything, teamID).Return(nil, errors.New("db down"))

		req, rr := serveAdminConfig(t, &adminMockContainer{
			teamRepo: adminConfigTeamRepo(t, teamID), githubAppConfigService: configs,
		}, adminConfigPaths(teamID)["github"])

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})
	t.Run("installation status", func(t *testing.T) {
		configs := servicesmocks.NewMockGitHubAppConfigServiceInterface(t)
		configs.EXPECT().GetAppConfig(mock.Anything, teamID).Return(nil, services.ErrGitHubAppNotConfigured)
		apps := servicesmocks.NewMockGitHubAppServiceInterface(t)
		apps.EXPECT().GetInstallationStatus(mock.Anything, teamID).Return(nil, errors.New("db down"))

		req, rr := serveAdminConfig(t, &adminMockContainer{
			teamRepo: adminConfigTeamRepo(t, teamID), githubAppConfigService: configs, githubAppService: apps,
		}, adminConfigPaths(teamID)["github"])

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		specconformance.AssertConformsToSpec(t, req, rr)
	})
}

func TestConfigurationKeys(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", []string{}},
		{"empty object", "{}", []string{}},
		{"invalid", "{not json", []string{}},
		{"array", `["a"]`, []string{}},
		{"null", "null", []string{}},
		{"nested keys are not listed", `{"b":{"inner":1},"a":2}`, []string{"a", "b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, configurationKeys(tc.raw))
		})
	}
}

func TestRedactBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  *string
		want *string
	}{
		{"nil", nil, nil},
		{"empty", strRef(""), nil},
		{"plain", strRef("https://api.openai.com/v1"), strRef("https://api.openai.com/v1")},
		{"keeps port and path", strRef("http://localhost:11434/api"), strRef("http://localhost:11434/api")},
		{"strips userinfo", strRef("https://u:p@host.example/v1"), strRef("https://host.example/v1")},
		{"strips query and fragment", strRef("https://host.example/v1?key=s#x"), strRef("https://host.example/v1")},
		{"bare question mark", strRef("https://host.example/v1?"), strRef("https://host.example/v1")},
		{"no scheme", strRef("user@host.example/v1"), nil},
		{"opaque userinfo form", strRef("user:pass@host.example"), nil},
		{"unparseable", strRef("https://host.example/%zz"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, redactBaseURL(tc.raw))
		})
	}
}

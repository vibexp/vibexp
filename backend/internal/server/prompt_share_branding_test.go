package server

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// The server-rendered shared-prompt page, which link-preview crawlers and
// no-script clients receive, carries the instance's branding (#1361).

// sharedPromptBrandingContainer serves a shared prompt and the site info the
// branding is read from.
type sharedPromptBrandingContainer struct {
	*adminMockContainer
	shares services.PromptShareServiceInterface
}

func (c *sharedPromptBrandingContainer) PromptShareService() services.PromptShareServiceInterface {
	return c.shares
}

// crawlSharedPrompt requests the shared prompt as a link-preview crawler over
// the given stored site info (nil: nothing stored) and returns the HTML.
func crawlSharedPrompt(t *testing.T, row *models.InstanceSiteInfo, readErr error) string {
	t.Helper()
	shares := servicesmocks.NewMockPromptShareServiceInterface(t)
	shares.EXPECT().GetSharedPrompt("tok-123", (*string)(nil)).Return(&models.SharedPromptResponse{
		Prompt:    models.Prompt{Name: "Release Notes Writer", Description: "Drafts release notes"},
		ShareType: "public",
	}, nil)

	repo := &fakeSiteInfoRepo{fakeSingleton[models.InstanceSiteInfo]{
		audit:  &fakeInstanceAuditRepo{clock: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)},
		row:    row,
		getErr: readErr,
	}}
	cfg := &config.Config{Frontend: config.FrontendConfig{BaseURL: "https://vibexp.acme.test"}}
	srv := newAdminTestServer(cfg, &adminMockContainer{})
	srv.container = &sharedPromptBrandingContainer{
		adminMockContainer: &adminMockContainer{
			instanceSiteInfoService: services.NewInstanceSiteInfoService(repo, slog.New(slog.DiscardHandler)),
		},
		shares: shares,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/shared/prompts/tok-123", nil)
	req.Header.Set("User-Agent", "Slackbot-LinkExpanding 1.0")
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Contains(t, rr.Header().Get("Content-Type"), "text/html")
	return rr.Body.String()
}

func TestGetSharedPrompt_CrawlerHTML_UsesCompanyBranding(t *testing.T) {
	name, logo := "Acme Inc.", "https://cdn.acme.test/logo.png"

	html := crawlSharedPrompt(t, &models.InstanceSiteInfo{CompanyName: &name, LogoURL: &logo}, nil)

	assert.Contains(t, html, "<title>Release Notes Writer | Shared Prompt | Acme Inc.</title>")
	assert.Contains(t, html, `property="og:site_name" content="Acme Inc."`)
	assert.Contains(t, html, `property="og:image" content="https://cdn.acme.test/logo.png"`)
	assert.Contains(t, html, `alt="Acme Inc."`)
}

func TestGetSharedPrompt_CrawlerHTML_UnbrandedIsVibeXP(t *testing.T) {
	html := crawlSharedPrompt(t, nil, nil)

	assert.Contains(t, html, "<title>Release Notes Writer | Shared Prompt | VibeXP</title>")
	assert.Contains(t, html, `property="og:site_name" content="VibeXP"`)
	assert.Contains(t, html, `property="og:image" content="https://vibexp.acme.test/logo_rounded.png"`)
}

// A company name without a logo keeps the bundled image; a logo without a name
// keeps the product name.
func TestGetSharedPrompt_CrawlerHTML_PartialBranding(t *testing.T) {
	t.Run("name only", func(t *testing.T) {
		name := "Acme Inc."

		html := crawlSharedPrompt(t, &models.InstanceSiteInfo{CompanyName: &name}, nil)

		assert.Contains(t, html, `property="og:site_name" content="Acme Inc."`)
		assert.Contains(t, html, `property="og:image" content="https://vibexp.acme.test/logo_rounded.png"`)
	})

	t.Run("logo only", func(t *testing.T) {
		logo := "https://cdn.acme.test/logo.png"

		html := crawlSharedPrompt(t, &models.InstanceSiteInfo{LogoURL: &logo}, nil)

		assert.Contains(t, html, `property="og:site_name" content="VibeXP"`)
		assert.Contains(t, html, `property="og:image" content="https://cdn.acme.test/logo.png"`)
	})
}

// A branding read failure never breaks a link preview: the page renders with
// the product's values and does not leak the error.
func TestGetSharedPrompt_CrawlerHTML_SiteInfoReadFailureFallsBack(t *testing.T) {
	name := "Acme Inc."

	html := crawlSharedPrompt(t, &models.InstanceSiteInfo{CompanyName: &name},
		errors.New("pq: connection reset by peer"))

	assert.Contains(t, html, `property="og:site_name" content="VibeXP"`)
	assert.NotContains(t, html, "Acme Inc.")
	assert.NotContains(t, html, "connection reset")
}

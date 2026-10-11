package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	publicSiteInfoPath = "/api/v1/site-info"
	configJSSiteInfo   = "window.__VIBEXP_SITE_INFO__ = "
)

// publicSiteInfoKeys is the complete key set of the public shape, on both the
// endpoint and /config.js.
var publicSiteInfoKeys = []string{
	"company_name", "company_email", "website", "support_url",
	"terms_url", "privacy_url", "logo_url", "logo_dark_url",
}

func publicSiteInfoPtr(s string) *string { return &s }

// newPublicSiteInfoServer builds the FULL router over the real site info
// service and an in-memory repository holding row (nil: nothing stored). The
// base URL is production-like, so the rate limiter is live when limits sets
// one.
func newPublicSiteInfoServer(
	t *testing.T, row *models.InstanceSiteInfo, limits config.RateLimitConfig,
) (*Server, *fakeSiteInfoRepo) {
	t.Helper()
	repo := &fakeSiteInfoRepo{fakeSingleton[models.InstanceSiteInfo]{
		audit: &fakeInstanceAuditRepo{clock: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)},
		row:   row,
	}}
	cfg := &config.Config{
		Frontend:  config.FrontendConfig{BaseURL: "https://vibexp.acme.test"},
		RateLimit: limits,
	}
	srv := newAdminTestServer(cfg, &adminMockContainer{
		instanceSiteInfoService: services.NewInstanceSiteInfoService(repo, slog.New(slog.DiscardHandler)),
	})
	return srv, repo
}

// storedPublicSiteInfo sets a name, a website and a logo; the rest is not
// configured.
func storedPublicSiteInfo() *models.InstanceSiteInfo {
	editor := "11111111-1111-4111-8111-111111111111"
	return &models.InstanceSiteInfo{
		CompanyName: publicSiteInfoPtr("Acme Inc."),
		Website:     publicSiteInfoPtr("https://acme.test"),
		LogoURL:     publicSiteInfoPtr("https://cdn.acme.test/logo.svg"),
		UpdatedBy:   &editor,
		Version:     7,
	}
}

// getPublicSiteInfo sends an anonymous GET through the full router and asserts
// the response conforms to the spec.
func getPublicSiteInfo(t *testing.T, srv *Server) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, publicSiteInfoPath, nil)
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)
	specconformance.AssertConformsToSpec(t, req, rr)
	return rr
}

func allNullSiteInfo() map[string]any {
	want := map[string]any{}
	for _, k := range publicSiteInfoKeys {
		want[k] = nil
	}
	return want
}

// An anonymous caller gets a 200 with every value null when nothing is stored.
func TestGetSiteInfo_NothingStoredIsAllNull(t *testing.T) {
	srv, _ := newPublicSiteInfoServer(t, nil, config.RateLimitConfig{})

	rr := getPublicSiteInfo(t, srv)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, allNullSiteInfo(), decodeJSONMap(t, rr))
}

// Stored values come back as strings and unset ones as null. The body is
// exactly the eight values: no version, editor or timestamp of the stored row.
func TestGetSiteInfo_ReturnsOnlyTheEightValues(t *testing.T) {
	srv, _ := newPublicSiteInfoServer(t, storedPublicSiteInfo(), config.RateLimitConfig{})

	rr := getPublicSiteInfo(t, srv)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertJSONKeyPaths(t, rr.Body.Bytes(), publicSiteInfoKeys...)
	want := allNullSiteInfo()
	want["company_name"] = "Acme Inc."
	want["website"] = "https://acme.test"
	want["logo_url"] = "https://cdn.acme.test/logo.svg"
	assert.Equal(t, want, decodeJSONMap(t, rr))
	assert.NotContains(t, rr.Body.String(), "11111111-1111-4111-8111-111111111111", "the editor is not public")
}

// A failed read must never break the sign-in page: the endpoint fails open to
// "nothing configured" and never echoes the error.
func TestGetSiteInfo_FailsOpenOnReadError(t *testing.T) {
	srv, repo := newPublicSiteInfoServer(t, storedPublicSiteInfo(), config.RateLimitConfig{})
	repo.getErr = errors.New("pq: connection reset by peer")

	rr := getPublicSiteInfo(t, srv)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, allNullSiteInfo(), decodeJSONMap(t, rr))
	assert.NotContains(t, rr.Body.String(), "connection reset")
}

// A server built without a database (its read panics), or a container with no
// site info service, still answers "nothing configured".
func TestPublicSiteInfo_FailsOpenWithoutAService(t *testing.T) {
	t.Run("no database", func(t *testing.T) {
		srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))

		rr := getPublicSiteInfo(t, srv)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, allNullSiteInfo(), decodeJSONMap(t, rr))
	})

	t.Run("no service", func(t *testing.T) {
		srv := newAdminTestServer(&config.Config{}, &adminMockContainer{})

		rr := getPublicSiteInfo(t, srv)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, allNullSiteInfo(), decodeJSONMap(t, rr))
	})
}

// The endpoint takes the general per-IP API limit, not the strict auth limit:
// with a tight auth limit and a roomy API limit it is not throttled, and it is
// throttled once the API limit itself is exceeded.
func TestGetSiteInfo_IsRateLimitedAtTheAPILimit(t *testing.T) {
	getFrom := func(srv *Server, ip string) int {
		req := httptest.NewRequest(http.MethodGet, publicSiteInfoPath, nil)
		req.RemoteAddr = ip + ":40000"
		rr := httptest.NewRecorder()
		srv.router.ServeHTTP(rr, req)
		return rr.Code
	}

	t.Run("the auth limit does not apply", func(t *testing.T) {
		srv, _ := newPublicSiteInfoServer(t, nil, config.RateLimitConfig{AuthPerMinute: 1, APIPerMinute: 1000})

		for range 5 {
			require.Equal(t, http.StatusOK, getFrom(srv, "203.0.113.7"))
		}
	})

	t.Run("the API limit does", func(t *testing.T) {
		srv, _ := newPublicSiteInfoServer(t, nil, config.RateLimitConfig{AuthPerMinute: 1000, APIPerMinute: 2})

		assert.Equal(t, http.StatusOK, getFrom(srv, "203.0.113.7"))
		assert.Equal(t, http.StatusOK, getFrom(srv, "203.0.113.7"))
		assert.Equal(t, http.StatusTooManyRequests, getFrom(srv, "203.0.113.7"), "the third request from one IP is throttled")
		assert.Equal(t, http.StatusOK, getFrom(srv, "203.0.113.8"), "another client has its own budget")
	})
}

func TestToPublicSiteInfo_MapsEveryValueAndEmptyToNull(t *testing.T) {
	full := models.InstanceSiteInfoValues{
		CompanyName: "Acme Inc.", CompanyEmail: "hello@acme.test", Website: "https://acme.test",
		SupportURL: "https://acme.test/help", TermsURL: "https://acme.test/terms",
		PrivacyURL: "https://acme.test/privacy", LogoURL: "https://cdn.acme.test/logo.svg",
		LogoDarkURL: "https://cdn.acme.test/logo-dark.svg",
	}

	raw, err := json.Marshal(toPublicSiteInfo(full))
	require.NoError(t, err)
	assert.JSONEq(t, `{"company_name":"Acme Inc.","company_email":"hello@acme.test",
		"website":"https://acme.test","support_url":"https://acme.test/help",
		"terms_url":"https://acme.test/terms","privacy_url":"https://acme.test/privacy",
		"logo_url":"https://cdn.acme.test/logo.svg",
		"logo_dark_url":"https://cdn.acme.test/logo-dark.svg"}`, string(raw))

	raw, err = json.Marshal(toPublicSiteInfo(models.InstanceSiteInfoValues{}))
	require.NoError(t, err)
	var empty map[string]any
	require.NoError(t, json.Unmarshal(raw, &empty))
	assert.Equal(t, allNullSiteInfo(), empty)
}

// --- /config.js ---------------------------------------------------------------

// configJSSiteInfoObject serves /config.js and returns its body and the decoded
// site info assignment.
func configJSSiteInfoObject(t *testing.T, srv *Server) (string, map[string]any) {
	t.Helper()
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/config.js", nil))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, "no-cache", rr.Header().Get("Cache-Control"))

	body := rr.Body.String()
	lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	require.Len(t, lines, 2, "one line per assignment: %q", body)
	assert.True(t, strings.HasPrefix(lines[0], "window.__VIBEXP_ENV__ = "), "the env assignment stays first")
	require.True(t, strings.HasPrefix(lines[1], configJSSiteInfo), body)
	require.True(t, strings.HasSuffix(lines[1], ";"), body)

	var got map[string]any
	doc := strings.TrimSuffix(strings.TrimPrefix(lines[1], configJSSiteInfo), ";")
	require.NoError(t, json.Unmarshal([]byte(doc), &got), doc)
	return body, got
}

// /config.js carries the same eight values, in the same shape, as the public
// endpoint.
func TestHandleConfigJS_InlinesSiteInfo(t *testing.T) {
	srv, _ := newPublicSiteInfoServer(t, storedPublicSiteInfo(), config.RateLimitConfig{})

	_, got := configJSSiteInfoObject(t, srv)

	assert.Equal(t, decodeJSONMap(t, getPublicSiteInfo(t, srv)), got)
	assert.Equal(t, "Acme Inc.", got["company_name"])
	assert.Nil(t, got["support_url"])
}

// The app must still load when the branding cannot be read, or when the
// server has no database at all.
func TestHandleConfigJS_SiteInfoFailsOpen(t *testing.T) {
	t.Run("read error", func(t *testing.T) {
		srv, repo := newPublicSiteInfoServer(t, storedPublicSiteInfo(), config.RateLimitConfig{})
		repo.getErr = errors.New("pq: connection reset by peer")

		body, got := configJSSiteInfoObject(t, srv)

		assert.Equal(t, allNullSiteInfo(), got)
		assert.NotContains(t, body, "connection reset")
	})

	t.Run("no database", func(t *testing.T) {
		srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))

		_, got := configJSSiteInfoObject(t, srv)

		assert.Equal(t, allNullSiteInfo(), got)
	})
}

// A stored value is admin-entered text inlined into a script: it must not be
// able to close the script element, open an HTML comment or end the statement.
func TestHandleConfigJS_SiteInfoIsScriptSafe(t *testing.T) {
	hostile := "</script><script>alert(1)</script><!-- \" '     &"
	row := storedPublicSiteInfo()
	row.CompanyName = &hostile
	srv, _ := newPublicSiteInfoServer(t, row, config.RateLimitConfig{})

	body, got := configJSSiteInfoObject(t, srv)

	assert.Equal(t, hostile, got["company_name"], "the value round-trips exactly once decoded")
	for _, raw := range []string{"</script", "<script", "<!--", " ", " ", "&"} {
		assert.NotContains(t, body, raw, "raw %q must be escaped", raw)
	}
}

// HEAD still writes headers only.
func TestHandleConfigJS_HeadHasNoBody(t *testing.T) {
	srv, _ := newPublicSiteInfoServer(t, storedPublicSiteInfo(), config.RateLimitConfig{})

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodHead, "/config.js", nil))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Body.String())
}

// --- a stalled or panicking read ----------------------------------------------

// stalledSiteInfoRepo never answers: Get blocks until its context ends, the way
// a query does against an exhausted pool or an unreachable database.
type stalledSiteInfoRepo struct{ fakeSiteInfoRepo }

func (r *stalledSiteInfoRepo) Get(ctx context.Context) (*models.InstanceSiteInfo, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

var _ repositories.InstanceSiteInfoRepository = (*stalledSiteInfoRepo)(nil)

// A database that stalls must cost a short wait, not the page: both public
// surfaces answer "nothing configured" once siteInfoReadTimeout passes, long
// before the router's own request timeout.
func TestPublicSiteInfo_StalledReadIsBoundedAndFailsOpen(t *testing.T) {
	prev := siteInfoReadTimeout
	siteInfoReadTimeout = 50 * time.Millisecond
	t.Cleanup(func() { siteInfoReadTimeout = prev })
	const bound = 5 * time.Second // generous for a loaded CI box, far below the 60s router timeout

	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		instanceSiteInfoService: services.NewInstanceSiteInfoService(
			&stalledSiteInfoRepo{}, slog.New(slog.DiscardHandler)),
	})

	t.Run("the endpoint", func(t *testing.T) {
		start := time.Now()
		rr := getPublicSiteInfo(t, srv)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, allNullSiteInfo(), decodeJSONMap(t, rr))
		assert.Less(t, time.Since(start), bound)
	})

	t.Run("config.js", func(t *testing.T) {
		start := time.Now()
		_, got := configJSSiteInfoObject(t, srv)

		assert.Equal(t, allNullSiteInfo(), got)
		assert.Less(t, time.Since(start), bound)
	})
}

// The default bound is short enough that a stall cannot hold the first paint
// for long.
func TestSiteInfoReadTimeout_IsShort(t *testing.T) {
	assert.LessOrEqual(t, siteInfoReadTimeout, 5*time.Second)
	assert.Positive(t, siteInfoReadTimeout)
}

// panickingSiteInfoRepo stands in for a real bug in the read path.
type panickingSiteInfoRepo struct{ fakeSiteInfoRepo }

func (r *panickingSiteInfoRepo) Get(context.Context) (*models.InstanceSiteInfo, error) {
	panic("site info read exploded")
}

// A panic in the read still fails open, and is logged the way the panic
// middleware logs one (event, message, stack), so it is never silent.
func TestPublicSiteInfo_PanicFailsOpenAndIsLoggedWithItsStack(t *testing.T) {
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		instanceSiteInfoService: services.NewInstanceSiteInfoService(
			&panickingSiteInfoRepo{}, slog.New(slog.DiscardHandler)),
	})
	logger, hook := logtest.New()
	ctx := context.WithValue(context.Background(), contextkeys.Logger, logger)

	raw, err := json.Marshal(srv.publicSiteInfo(ctx))

	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, allNullSiteInfo(), got)

	entry := hook.LastEntry()
	require.NotNil(t, entry, "the panic must be logged")
	assert.Equal(t, "panic", entry.Data["event"])
	assert.Equal(t, "site info read exploded", entry.Data["panic.message"])
	stack, _ := entry.Data["panic.stack"].(string)
	assert.Contains(t, stack, "panickingSiteInfoRepo", "the stack names where it panicked")
}

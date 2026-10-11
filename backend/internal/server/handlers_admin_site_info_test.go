package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	siteInfoPath      = "/api/v1/admin/settings/site-info"
	siteInfoAuditPath = siteInfoPath + "/audit"
)

// siteInfoResponseKeyPaths is the complete key set of the GET and PUT
// responses.
func siteInfoResponseKeyPaths() []string {
	paths := []string{
		"values", "limits", "updated_at", "updated_by_user_id", "updated_by_name", "version",
		"limits.company_name_max_length", "limits.company_email_max_length", "limits.url_max_length",
	}
	for _, k := range siteInfoAuditSnapshotKeys {
		paths = append(paths, "values."+k)
	}
	return paths
}

// fakeSiteInfoRepo is the in-memory InstanceSiteInfoRepository the handlers
// run over, with the REAL service on top, so the real validator decides every
// 400 and PUT → GET → DELETE → GET is a genuine round trip.
type fakeSiteInfoRepo struct {
	fakeSingleton[models.InstanceSiteInfo]
}

func (r *fakeSiteInfoRepo) Get(context.Context) (*models.InstanceSiteInfo, error) {
	row, err := r.get()
	if err == nil && row == nil {
		err = repositories.ErrInstanceSiteInfoNotFound
	}
	return row, err
}

func (r *fakeSiteInfoRepo) UpsertAudited(
	_ context.Context, s *models.InstanceSiteInfo, v *int64, audit repositories.InstanceSiteInfoAuditFunc,
) error {
	return r.upsertAudited(s, v, audit)
}

func (r *fakeSiteInfoRepo) DeleteAudited(
	_ context.Context, audit repositories.InstanceSiteInfoAuditFunc,
) (bool, error) {
	return r.deleteAudited(audit)
}

type siteInfoFixture struct {
	router   http.Handler
	audit    *fakeInstanceAuditRepo
	repo     *fakeSiteInfoRepo
	userRepo *repomocks.MockUserRepository
}

func newSiteInfoFixture(t *testing.T) *siteInfoFixture {
	t.Helper()
	audit := &fakeInstanceAuditRepo{clock: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)}
	f := &siteInfoFixture{
		audit: audit,
		repo: &fakeSiteInfoRepo{fakeSingleton[models.InstanceSiteInfo]{
			audit: audit,
			meta: func(r *models.InstanceSiteInfo) (*int64, *time.Time, *time.Time) {
				return &r.Version, &r.CreatedAt, &r.UpdatedAt
			},
		}},
		userRepo: repomocks.NewMockUserRepository(t),
	}
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		userRepo:                f.userRepo,
		instanceAuditRepo:       audit,
		instanceSiteInfoService: services.NewInstanceSiteInfoService(f.repo, slog.New(slog.DiscardHandler)),
	})
	f.router = mountAdminStrictRouter(srv)
	return f
}

// resolveAdminName lets the acting admin's display name resolve. Tests that
// expect no name lookup at all simply do not call it.
func (f *siteInfoFixture) resolveAdminName() {
	f.userRepo.On("GetNamesByIDs", mock.Anything, []string{instanceEmailActingAdmin}).
		Return(map[string]string{instanceEmailActingAdmin: instanceSettingsAdminName}, nil).Maybe()
}

// serve sends one request as the acting admin, asserts the response conforms
// to the spec and returns it.
func (f *siteInfoFixture) serve(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := instanceEmailRequest(t, method, path, body)
	rr := serveInstanceEmail(t, f.router, req)
	specconformance.AssertConformsToSpec(t, req, rr)
	return rr
}

// siteInfoBody is a valid whole-row PUT body; variant makes successive bodies
// distinguishable.
func siteInfoBody(variant int) map[string]any {
	return map[string]any{
		"company_name":  "Acme " + strconv.Itoa(variant),
		"company_email": "hello@acme.test",
		"website":       "https://acme.test",
		"support_url":   "https://acme.test/help",
		"terms_url":     "https://acme.test/terms",
		"privacy_url":   "https://acme.test/privacy",
		"logo_url":      "https://cdn.acme.test/logo.svg",
		"logo_dark_url": "",
	}
}

func emptySiteInfoValues() map[string]any {
	values := map[string]any{}
	for _, k := range siteInfoAuditSnapshotKeys {
		values[k] = ""
	}
	return values
}

// With nothing stored the GET is still a 200: every value is an empty string,
// the row metadata is null, and no updater name is looked up.
func TestAdminSiteInfo_GetWithNothingStored(t *testing.T) {
	f := newSiteInfoFixture(t)

	rr := f.serve(t, http.MethodGet, siteInfoPath, nil)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertJSONKeyPaths(t, rr.Body.Bytes(), siteInfoResponseKeyPaths()...)
	got := decodeJSONMap(t, rr)
	assert.Equal(t, emptySiteInfoValues(), got["values"])
	for _, k := range []string{"updated_at", "updated_by_user_id", "updated_by_name", "version"} {
		assert.Nil(t, got[k], k)
	}
}

// The published limits are the constants the validator and the CHECK
// constraints enforce.
func TestAdminSiteInfo_LimitsMatchTheModelConstants(t *testing.T) {
	f := newSiteInfoFixture(t)

	got := decodeJSONMap(t, f.serve(t, http.MethodGet, siteInfoPath, nil))

	assert.Equal(t, map[string]any{
		"company_name_max_length":  float64(models.MaxSiteInfoCompanyNameLength),
		"company_email_max_length": float64(models.MaxSiteInfoEmailLength),
		"url_max_length":           float64(models.MaxSiteInfoURLLength),
	}, got["limits"])
}

// TestAdminSiteInfo_RoundTrip walks the whole lifecycle: PUT → GET (updater
// resolved, version 1), a compare-and-set PUT (version 2), a stale PUT (409,
// nothing changes), DELETE → GET (nothing configured) and a no-op DELETE.
// Exactly one audit entry is written per real change.
func TestAdminSiteInfo_RoundTrip(t *testing.T) {
	f := newSiteInfoFixture(t)
	f.resolveAdminName()

	first := siteInfoBody(1)
	rr := f.serve(t, http.MethodPut, siteInfoPath, first)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertJSONKeyPaths(t, rr.Body.Bytes(), siteInfoResponseKeyPaths()...)

	got := decodeJSONMap(t, f.serve(t, http.MethodGet, siteInfoPath, nil))
	assert.Equal(t, first, got["values"])
	assert.Equal(t, instanceEmailActingAdmin, got["updated_by_user_id"])
	assert.Equal(t, instanceSettingsAdminName, got["updated_by_name"])
	assert.NotNil(t, got["updated_at"])
	assert.Equal(t, float64(1), got["version"])

	second := siteInfoBody(2)
	second["expected_version"] = 1
	rr = f.serve(t, http.MethodPut, siteInfoPath, second)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, float64(2), decodeJSONMap(t, rr)["version"])

	stale := siteInfoBody(3)
	stale["expected_version"] = 1
	rr = f.serve(t, http.MethodPut, siteInfoPath, stale)
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "INSTANCE_SETTINGS_VERSION_CONFLICT")

	got = decodeJSONMap(t, f.serve(t, http.MethodGet, siteInfoPath, nil))
	delete(second, "expected_version")
	assert.Equal(t, second, got["values"], "a stale PUT changes nothing")
	assert.Equal(t, float64(2), got["version"])

	require.Equal(t, http.StatusNoContent, f.serve(t, http.MethodDelete, siteInfoPath, nil).Code)
	got = decodeJSONMap(t, f.serve(t, http.MethodGet, siteInfoPath, nil))
	assert.Equal(t, emptySiteInfoValues(), got["values"], "a reset leaves nothing configured")
	assert.Nil(t, got["updated_at"])
	assert.Nil(t, got["version"])

	require.Equal(t, http.StatusNoContent, f.serve(t, http.MethodDelete, siteInfoPath, nil).Code,
		"resetting with nothing stored is still a 204")

	require.Len(t, f.audit.entries, 3, "two upserts and one delete; the stale PUT and no-op reset audit nothing")
	actions := []string{f.audit.entries[0].Action, f.audit.entries[1].Action, f.audit.entries[2].Action}
	assert.Equal(t, []string{
		models.InstanceSettingsAuditActionUpsert, models.InstanceSettingsAuditActionUpsert,
		models.InstanceSettingsAuditActionDelete,
	}, actions)
	for _, e := range f.audit.entries {
		assert.Equal(t, models.InstanceSettingSiteInfo, e.Setting)
		require.NotNil(t, e.ActorUserID)
		assert.Equal(t, instanceEmailActingAdmin, *e.ActorUserID)
	}
	assert.Nil(t, f.audit.entries[0].Before, "the first save creates the row")
	assert.NotNil(t, f.audit.entries[1].Before)
	assert.Nil(t, f.audit.entries[2].After, "a reset removes the row")
}

// Values are trimmed before they are stored, and a blank one reads back as
// the empty string.
func TestAdminSiteInfo_PutTrimsValues(t *testing.T) {
	f := newSiteInfoFixture(t)
	f.resolveAdminName()
	body := emptySiteInfoValues()
	body["company_name"] = "  Acme Inc.  "
	body["website"] = " https://acme.test\n"
	body["support_url"] = "   "

	rr := f.serve(t, http.MethodPut, siteInfoPath, body)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	want := emptySiteInfoValues()
	want["company_name"] = "Acme Inc."
	want["website"] = "https://acme.test"
	assert.Equal(t, want, decodeJSONMap(t, rr)["values"])
}

// expected_version 0 saves only while nothing is stored; a positive version
// with nothing stored is a conflict; null is last-write-wins.
func TestAdminSiteInfo_ExpectedVersionRules(t *testing.T) {
	put := func(f *siteInfoFixture, variant int, expected any) *httptest.ResponseRecorder {
		body := siteInfoBody(variant)
		body["expected_version"] = expected
		return f.serve(t, http.MethodPut, siteInfoPath, body)
	}

	t.Run("a positive version with nothing stored is 409", func(t *testing.T) {
		f := newSiteInfoFixture(t)

		require.Equal(t, http.StatusConflict, put(f, 1, 1).Code)
		assert.Empty(t, f.audit.entries)
	})

	t.Run("zero expects nothing stored", func(t *testing.T) {
		f := newSiteInfoFixture(t)
		f.resolveAdminName()

		require.Equal(t, http.StatusOK, put(f, 1, 0).Code)
		require.Equal(t, http.StatusConflict, put(f, 2, 0).Code, "a row is stored now")
		require.Len(t, f.audit.entries, 1)

		require.Equal(t, http.StatusNoContent, f.serve(t, http.MethodDelete, siteInfoPath, nil).Code)
		require.Equal(t, http.StatusOK, put(f, 3, 0).Code, "and it works again after a reset")
	})

	t.Run("null is last-write-wins", func(t *testing.T) {
		f := newSiteInfoFixture(t)
		f.resolveAdminName()

		require.Equal(t, http.StatusOK, put(f, 1, nil).Code)
		rr := put(f, 2, nil)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, float64(2), decodeJSONMap(t, rr)["version"])
	})
}

// The REAL validator decides: each case is a 400 naming the offending field,
// and nothing is stored or audited.
func TestAdminSiteInfo_InvalidValueIs400(t *testing.T) {
	cases := []struct {
		name, field, value, wantCode string
	}{
		{"a javascript URL", "website", "javascript:alert(1)", services.SettingsFieldInvalidValue},
		{"a data URL logo", "logo_url", "data:image/svg+xml;base64,PHN2Zz4=", services.SettingsFieldInvalidValue},
		{"a relative URL", "terms_url", "/terms", services.SettingsFieldInvalidValue},
		{"a mailto support URL", "support_url", "mailto:help@acme.test", services.SettingsFieldInvalidValue},
		{"a malformed email", "company_email", "not-an-email", services.SettingsFieldInvalidValue},
		{"a name over the cap", "company_name",
			strings.Repeat("a", models.MaxSiteInfoCompanyNameLength+1), services.SettingsFieldOutOfRange},
		{"a URL over the cap", "privacy_url",
			"https://acme.test/" + strings.Repeat("a", models.MaxSiteInfoURLLength), services.SettingsFieldOutOfRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSiteInfoFixture(t)
			body := siteInfoBody(1)
			body[tc.field] = tc.value

			rr := f.serve(t, http.MethodPut, siteInfoPath, body)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			var problem struct {
				Code             string `json:"code"`
				ValidationErrors []struct {
					Field string `json:"field"`
					Code  string `json:"code"`
				} `json:"validation_errors"`
			}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &problem))
			assert.Equal(t, "INSTANCE_SETTINGS_VALIDATION_FAILED", problem.Code)
			require.Len(t, problem.ValidationErrors, 1)
			assert.Equal(t, tc.field, problem.ValidationErrors[0].Field)
			assert.Equal(t, tc.wantCode, problem.ValidationErrors[0].Code)
			assert.Empty(t, f.audit.entries)
			assert.Nil(t, f.repo.row, "nothing is stored")
		})
	}
}

// The body guard rejects a key the schema does not have before the handler
// runs.
func TestAdminSiteInfo_UnknownFieldIs400(t *testing.T) {
	f := newSiteInfoFixture(t)
	body := siteInfoBody(1)
	body["favicon_url"] = "https://acme.test/favicon.ico"

	rr := f.serve(t, http.MethodPut, siteInfoPath, body)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "favicon_url")
	assert.Empty(t, f.audit.entries)
}

// A PUT is a whole-row replace, so dropping any value field, or sending it as
// null, is a 400 naming it: never a silently cleared value. expected_version
// stays optional (the round trip omits it).
func TestAdminSiteInfo_MissingOrNullFieldIs400(t *testing.T) {
	for _, key := range siteInfoAuditSnapshotKeys {
		for _, mode := range []string{"missing", "null"} {
			t.Run(mode+" "+key, func(t *testing.T) {
				f := newSiteInfoFixture(t)
				body := siteInfoBody(1)
				delete(body, key)
				if mode == "null" {
					body[key] = nil
				}

				rr := f.serve(t, http.MethodPut, siteInfoPath, body)

				require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
				assert.Contains(t, rr.Body.String(), "Missing required field(s): "+key)
				assert.Empty(t, f.audit.entries)
			})
		}
	}
}

// Repository failures are logged 500s that never echo the underlying error.
func TestAdminSiteInfo_FailuresAre500(t *testing.T) {
	boom := errors.New("pq: connection reset by peer")
	cases := []struct {
		name   string
		method string
		path   string
		body   any
		break_ func(f *siteInfoFixture)
	}{
		{"get", http.MethodGet, siteInfoPath, nil, func(f *siteInfoFixture) { f.repo.getErr = boom }},
		{"put", http.MethodPut, siteInfoPath, siteInfoBody(1), func(f *siteInfoFixture) { f.repo.upsertErr = boom }},
		{"delete", http.MethodDelete, siteInfoPath, nil, func(f *siteInfoFixture) { f.repo.deleteErr = boom }},
		{"audit", http.MethodGet, siteInfoAuditPath, nil, func(f *siteInfoFixture) { f.audit.listErr = boom }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSiteInfoFixture(t)
			tc.break_(f)

			rr := f.serve(t, tc.method, tc.path, tc.body)

			require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "connection reset")
		})
	}
}

// A failed updater-name lookup degrades to a null name rather than failing
// the read.
func TestAdminSiteInfo_UpdaterNameLookupFailureDegrades(t *testing.T) {
	f := newSiteInfoFixture(t)
	f.userRepo.On("GetNamesByIDs", mock.Anything, mock.Anything).Return(nil, errors.New("db down"))

	rr := f.serve(t, http.MethodPut, siteInfoPath, siteInfoBody(1))

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	got := decodeJSONMap(t, rr)
	assert.Equal(t, instanceEmailActingAdmin, got["updated_by_user_id"])
	assert.Nil(t, got["updated_by_name"])
}

// The audit list returns only site_info entries, newest first, and pages.
func TestAdminSiteInfo_AuditListPages(t *testing.T) {
	f := newSiteInfoFixture(t)
	f.resolveAdminName()
	for i := 1; i <= 3; i++ {
		require.Equal(t, http.StatusOK, f.serve(t, http.MethodPut, siteInfoPath, siteInfoBody(i)).Code)
	}
	// An entry for another setting must never appear here.
	require.NoError(t, f.audit.Append(context.Background(), &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingSearch, Action: models.InstanceSettingsAuditActionDelete,
	}))

	var page struct {
		Entries []struct {
			ID        string         `json:"id"`
			Setting   string         `json:"setting"`
			Action    string         `json:"action"`
			ActorName *string        `json:"actor_name"`
			Before    map[string]any `json:"before"`
			After     map[string]any `json:"after"`
		} `json:"entries"`
		NextCursor *string `json:"next_cursor"`
	}
	first := f.serve(t, http.MethodGet, siteInfoAuditPath, nil)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &page))
	require.Len(t, page.Entries, 3)
	assert.Nil(t, page.NextCursor)
	for _, e := range page.Entries {
		assert.Equal(t, models.InstanceSettingSiteInfo, e.Setting)
		assert.Equal(t, models.InstanceSettingsAuditActionUpsert, e.Action)
		require.NotNil(t, e.ActorName)
		assert.Equal(t, instanceSettingsAdminName, *e.ActorName)
	}
	assert.Equal(t, siteInfoBody(3), page.Entries[0].After, "newest first")
	assert.Equal(t, siteInfoBody(2), page.Entries[0].Before)
	assert.Nil(t, page.Entries[2].Before, "the oldest entry created the row")

	seen := map[string]bool{}
	path := siteInfoAuditPath + "?limit=1"
	for range 3 {
		rr := f.serve(t, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
		require.Len(t, page.Entries, 1)
		assert.False(t, seen[page.Entries[0].ID], "an entry must not repeat across pages")
		seen[page.Entries[0].ID] = true
		if page.NextCursor != nil {
			path = siteInfoAuditPath + "?limit=1&cursor=" + *page.NextCursor
		}
	}
	assert.Len(t, seen, 3)
	assert.Nil(t, page.NextCursor, "the third page is the last")
}

// A key outside the allowlist never reaches the audit surface.
func TestAdminSiteInfo_AuditSnapshotsAreAllowlisted(t *testing.T) {
	f := newSiteInfoFixture(t)
	after := siteInfoBody(1)
	after["unreviewed_key"] = "must-not-leak"
	raw, err := json.Marshal(after)
	require.NoError(t, err)
	require.NoError(t, f.audit.Append(context.Background(), &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingSiteInfo, Action: models.InstanceSettingsAuditActionUpsert, After: raw,
	}))

	rr := f.serve(t, http.MethodGet, siteInfoAuditPath, nil)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.NotContains(t, rr.Body.String(), "must-not-leak")
	assert.Contains(t, rr.Body.String(), "Acme 1")
}

func TestAdminSiteInfo_AuditBadParamsAre400(t *testing.T) {
	for _, query := range []string{"?limit=0", "?limit=" + strconv.Itoa(adminInstanceAuditMaxLimit+1), "?cursor=not-a-cursor"} {
		t.Run(query, func(t *testing.T) {
			f := newSiteInfoFixture(t)

			rr := f.serve(t, http.MethodGet, siteInfoAuditPath+query, nil)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		})
	}
}

// The stored setting constant is in the audit entry's published enum, and the
// snapshot allowlist is exactly the values' JSON shape.
func TestSiteInfoAuditVocabularyMatchesSpecAndModel(t *testing.T) {
	assert.True(t, admingen.AdminInstanceSettingsAuditEntrySetting(models.InstanceSettingSiteInfo).Valid())

	raw, err := json.Marshal(models.InstanceSiteInfoValues{})
	require.NoError(t, err)
	var shape map[string]any
	require.NoError(t, json.Unmarshal(raw, &shape))
	keys := make([]string, 0, len(shape))
	for k := range shape {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, siteInfoAuditSnapshotKeys, keys)
}

// TestAdminSiteInfoRoutes_Gate404 drives the FULL router (setupAdminRoutes:
// optionalAuthMiddleware + instanceAdminMiddleware) for every site info
// operation: an anonymous caller and an authenticated non-admin (API key) both
// get 404, and the service and audit mocks carry no expectations, so reaching
// a handler fails the test.
func TestAdminSiteInfoRoutes_Gate404(t *testing.T) {
	const nonAdminID = "33333333-3333-4333-8333-333333333333"
	cfg := &config.Config{Auth: config.AuthConfig{InstanceAdmins: config.EnvStringSlice{instanceEmailAdminEmail}}}
	routes := []struct{ method, path string }{
		{http.MethodGet, siteInfoPath},
		{http.MethodPut, siteInfoPath},
		{http.MethodDelete, siteInfoPath},
		{http.MethodGet, siteInfoAuditPath},
	}

	for _, route := range routes {
		for _, caller := range []string{"anonymous", "non-admin"} {
			t.Run(caller+" "+route.method+" "+route.path, func(t *testing.T) {
				authSvc := servicesmocks.NewMockAuthServiceInterface(t)
				keySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
				srv := newAdminTestServer(cfg, &adminMockContainer{
					authService:             authSvc,
					apiKeyService:           keySvc,
					instanceAuditRepo:       repomocks.NewMockInstanceSettingsAuditRepository(t),
					instanceSiteInfoService: servicesmocks.NewMockInstanceSiteInfoServiceInterface(t),
				})

				body, err := json.Marshal(siteInfoBody(1))
				require.NoError(t, err)
				req := httptest.NewRequest(route.method, route.path, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				if caller == "non-admin" {
					keySvc.On("ValidateAPIKey", mock.Anything, "vxk_non-admin").
						Return(&models.APIKey{ID: "key-1", UserID: nonAdminID}, nil)
					authSvc.On("GetUserByID", mock.Anything, nonAdminID).
						Return(&models.User{ID: nonAdminID, Email: "member@instance.test"}, nil)
					req.Header.Set("Authorization", "Bearer vxk_non-admin")
				}
				rr := httptest.NewRecorder()
				srv.router.ServeHTTP(rr, req)

				require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
				specconformance.AssertConformsToSpec(t, req, rr)
			})
		}
	}
}

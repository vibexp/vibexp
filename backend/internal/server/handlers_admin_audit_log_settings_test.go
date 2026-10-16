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
	auditLogSettingsPath      = "/api/v1/admin/settings/audit-log"
	auditLogSettingsAuditPath = auditLogSettingsPath + "/audit"
)

// auditLogSettingsResponseKeyPaths is the complete key set of the GET and PUT
// responses.
func auditLogSettingsResponseKeyPaths() []string {
	paths := []string{
		"values", "source", "built_in_defaults", "limits",
		"updated_at", "updated_by_user_id", "updated_by_name", "version",
		"limits.retention_weeks_min", "limits.retention_weeks_max",
	}
	for _, k := range auditLogSettingsAuditSnapshotKeys {
		paths = append(paths, "values."+k, "built_in_defaults."+k)
	}
	return paths
}

// fakeAuditSettingsRepo is the in-memory InstanceAuditSettingsRepository the
// handlers run over, with the REAL service on top, so the real validator
// decides every 400 and PUT → GET → DELETE → GET is a genuine round trip.
type fakeAuditSettingsRepo struct {
	fakeSingleton[models.InstanceAuditSettings]
}

func (r *fakeAuditSettingsRepo) Get(context.Context) (*models.InstanceAuditSettings, error) {
	row, err := r.get()
	if err == nil && row == nil {
		err = repositories.ErrInstanceAuditSettingsNotFound
	}
	return row, err
}

func (r *fakeAuditSettingsRepo) UpsertAudited(
	_ context.Context, s *models.InstanceAuditSettings, v *int64,
	audit repositories.InstanceAuditSettingsAuditFunc,
) error {
	return r.upsertAudited(s, v, audit)
}

func (r *fakeAuditSettingsRepo) DeleteAudited(
	_ context.Context, audit repositories.InstanceAuditSettingsAuditFunc,
) (bool, error) {
	return r.deleteAudited(audit)
}

func (r *fakeAuditSettingsRepo) TryPurgeLock(context.Context) (bool, func() error, error) {
	return false, func() error { return nil }, nil
}

type auditLogSettingsFixture struct {
	router   http.Handler
	audit    *fakeInstanceAuditRepo
	repo     *fakeAuditSettingsRepo
	service  *services.InstanceAuditSettingsService
	capture  *services.AuditClientInfoCapture
	userRepo *repomocks.MockUserRepository
}

func newAuditLogSettingsFixture(t *testing.T) *auditLogSettingsFixture {
	t.Helper()
	audit := &fakeInstanceAuditRepo{clock: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	f := &auditLogSettingsFixture{
		audit: audit,
		repo: &fakeAuditSettingsRepo{fakeSingleton[models.InstanceAuditSettings]{
			audit: audit,
			meta: func(r *models.InstanceAuditSettings) (*int64, *time.Time, *time.Time) {
				return &r.Version, &r.CreatedAt, &r.UpdatedAt
			},
		}},
		userRepo: repomocks.NewMockUserRepository(t),
	}
	f.capture = services.NewAuditClientInfoCapture(f.repo, slog.New(slog.DiscardHandler))
	f.service = services.NewInstanceAuditSettingsService(f.repo, f.capture, slog.New(slog.DiscardHandler))
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		userRepo:                     f.userRepo,
		instanceAuditRepo:            audit,
		instanceAuditSettingsService: f.service,
	})
	f.router = mountAdminStrictRouter(srv)
	return f
}

// resolveAdminName lets the acting admin's display name resolve. Tests that
// expect no name lookup at all simply do not call it.
func (f *auditLogSettingsFixture) resolveAdminName() {
	f.userRepo.On("GetNamesByIDs", mock.Anything, []string{instanceEmailActingAdmin}).
		Return(map[string]string{instanceEmailActingAdmin: instanceSettingsAdminName}, nil).Maybe()
}

// serve sends one request as the acting admin, asserts the response conforms
// to the spec and returns it.
func (f *auditLogSettingsFixture) serve(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := instanceEmailRequest(t, method, path, body)
	rr := serveInstanceEmail(t, f.router, req)
	specconformance.AssertConformsToSpec(t, req, rr)
	return rr
}

func auditLogSettingsBody(weeks int, capture bool) map[string]any {
	return map[string]any{"retention_weeks": weeks, "capture_ip_ua": capture}
}

func defaultAuditLogSettingsValues() map[string]any {
	return map[string]any{
		"retention_weeks": float64(models.DefaultAuditRetentionWeeks),
		"capture_ip_ua":   models.DefaultAuditCaptureClientInfo,
	}
}

// With nothing stored the GET is still a 200: the built-in defaults, marked
// as such, with the row metadata null and no updater name looked up.
func TestAdminAuditLogSettings_GetWithNothingStored(t *testing.T) {
	f := newAuditLogSettingsFixture(t)

	rr := f.serve(t, http.MethodGet, auditLogSettingsPath, nil)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertJSONKeyPaths(t, rr.Body.Bytes(), auditLogSettingsResponseKeyPaths()...)
	got := decodeJSONMap(t, rr)
	assert.Equal(t, defaultAuditLogSettingsValues(), got["values"])
	assert.Equal(t, defaultAuditLogSettingsValues(), got["built_in_defaults"])
	assert.Equal(t, "default", got["source"])
	for _, k := range []string{"updated_at", "updated_by_user_id", "updated_by_name", "version"} {
		assert.Nil(t, got[k], k)
	}
}

// The published limits are the constants the validator and the CHECK
// constraint enforce.
func TestAdminAuditLogSettings_LimitsMatchTheModelConstants(t *testing.T) {
	f := newAuditLogSettingsFixture(t)

	got := decodeJSONMap(t, f.serve(t, http.MethodGet, auditLogSettingsPath, nil))

	assert.Equal(t, map[string]any{
		"retention_weeks_min": float64(models.MinAuditRetentionWeeks),
		"retention_weeks_max": float64(models.MaxAuditRetentionWeeks),
	}, got["limits"])
}

// TestAdminAuditLogSettings_RoundTrip walks the whole lifecycle: PUT → GET
// (updater resolved, version 1), a compare-and-set PUT (version 2), a stale
// PUT (409, nothing changes), DELETE → GET (defaults) and a no-op DELETE.
// Exactly one settings audit entry is written per real change.
func TestAdminAuditLogSettings_RoundTrip(t *testing.T) {
	f := newAuditLogSettingsFixture(t)
	f.resolveAdminName()

	rr := f.serve(t, http.MethodPut, auditLogSettingsPath, auditLogSettingsBody(4, false))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertJSONKeyPaths(t, rr.Body.Bytes(), auditLogSettingsResponseKeyPaths()...)

	got := decodeJSONMap(t, f.serve(t, http.MethodGet, auditLogSettingsPath, nil))
	assert.Equal(t, map[string]any{"retention_weeks": float64(4), "capture_ip_ua": false}, got["values"])
	assert.Equal(t, defaultAuditLogSettingsValues(), got["built_in_defaults"])
	assert.Equal(t, "instance", got["source"])
	assert.Equal(t, instanceEmailActingAdmin, got["updated_by_user_id"])
	assert.Equal(t, instanceSettingsAdminName, got["updated_by_name"])
	assert.NotNil(t, got["updated_at"])
	assert.Equal(t, float64(1), got["version"])

	second := auditLogSettingsBody(52, true)
	second["expected_version"] = 1
	rr = f.serve(t, http.MethodPut, auditLogSettingsPath, second)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, float64(2), decodeJSONMap(t, rr)["version"])

	stale := auditLogSettingsBody(1, false)
	stale["expected_version"] = 1
	rr = f.serve(t, http.MethodPut, auditLogSettingsPath, stale)
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "INSTANCE_SETTINGS_VERSION_CONFLICT")

	got = decodeJSONMap(t, f.serve(t, http.MethodGet, auditLogSettingsPath, nil))
	assert.Equal(t, map[string]any{"retention_weeks": float64(52), "capture_ip_ua": true}, got["values"],
		"a stale PUT changes nothing")
	assert.Equal(t, float64(2), got["version"])

	require.Equal(t, http.StatusNoContent, f.serve(t, http.MethodDelete, auditLogSettingsPath, nil).Code)
	got = decodeJSONMap(t, f.serve(t, http.MethodGet, auditLogSettingsPath, nil))
	assert.Equal(t, defaultAuditLogSettingsValues(), got["values"], "a reset restores the built-in defaults")
	assert.Equal(t, "default", got["source"])
	assert.Nil(t, got["updated_at"])
	assert.Nil(t, got["version"])

	require.Equal(t, http.StatusNoContent, f.serve(t, http.MethodDelete, auditLogSettingsPath, nil).Code,
		"resetting with nothing stored is still a 204")

	require.Len(t, f.audit.entries, 3, "two upserts and one delete; the stale PUT and no-op reset audit nothing")
	assert.Equal(t, []string{
		models.InstanceSettingsAuditActionUpsert, models.InstanceSettingsAuditActionUpsert,
		models.InstanceSettingsAuditActionDelete,
	}, []string{f.audit.entries[0].Action, f.audit.entries[1].Action, f.audit.entries[2].Action})
	for _, e := range f.audit.entries {
		assert.Equal(t, models.InstanceSettingAuditLog, e.Setting)
		require.NotNil(t, e.ActorUserID)
		assert.Equal(t, instanceEmailActingAdmin, *e.ActorUserID)
	}
	assert.Nil(t, f.audit.entries[0].Before, "the first save creates the row")
	assert.JSONEq(t, `{"retention_weeks":4,"capture_ip_ua":false}`, string(f.audit.entries[1].Before))
	assert.JSONEq(t, `{"retention_weeks":52,"capture_ip_ua":true}`, string(f.audit.entries[1].After))
	assert.Nil(t, f.audit.entries[2].After, "a reset removes the row")
	// Each of these entries reaches the audit log as one audit.settings.updated
	// event through the instance settings observer once it commits (#1515);
	// the repository integration tests cover that hook.
}

// A PUT through the API switches the capture the recorder consults at once,
// with no wait for the cache to expire, and a reset switches it back.
func TestAdminAuditLogSettings_PutAppliesToCaptureImmediately(t *testing.T) {
	f := newAuditLogSettingsFixture(t)
	f.resolveAdminName()
	ctx := context.Background()
	require.True(t, f.capture.CaptureClientInfo(ctx), "capture is on by default")

	require.Equal(t, http.StatusOK,
		f.serve(t, http.MethodPut, auditLogSettingsPath, auditLogSettingsBody(2, false)).Code)
	assert.False(t, f.capture.CaptureClientInfo(ctx))

	require.Equal(t, http.StatusNoContent, f.serve(t, http.MethodDelete, auditLogSettingsPath, nil).Code)
	assert.True(t, f.capture.CaptureClientInfo(ctx))
}

// expected_version 0 saves only while nothing is stored; a positive version
// with nothing stored is a conflict; null is last-write-wins.
func TestAdminAuditLogSettings_ExpectedVersionRules(t *testing.T) {
	put := func(f *auditLogSettingsFixture, weeks int, expected any) *httptest.ResponseRecorder {
		body := auditLogSettingsBody(weeks, true)
		body["expected_version"] = expected
		return f.serve(t, http.MethodPut, auditLogSettingsPath, body)
	}

	t.Run("a positive version with nothing stored is 409", func(t *testing.T) {
		f := newAuditLogSettingsFixture(t)

		require.Equal(t, http.StatusConflict, put(f, 3, 1).Code)
		assert.Empty(t, f.audit.entries)
	})

	t.Run("zero expects nothing stored", func(t *testing.T) {
		f := newAuditLogSettingsFixture(t)
		f.resolveAdminName()

		require.Equal(t, http.StatusOK, put(f, 3, 0).Code)
		require.Equal(t, http.StatusConflict, put(f, 4, 0).Code, "a row is stored now")
		require.Len(t, f.audit.entries, 1)

		require.Equal(t, http.StatusNoContent, f.serve(t, http.MethodDelete, auditLogSettingsPath, nil).Code)
		require.Equal(t, http.StatusOK, put(f, 5, 0).Code, "and it works again after a reset")
	})

	t.Run("null is last-write-wins", func(t *testing.T) {
		f := newAuditLogSettingsFixture(t)
		f.resolveAdminName()

		require.Equal(t, http.StatusOK, put(f, 3, nil).Code)
		rr := put(f, 4, nil)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, float64(2), decodeJSONMap(t, rr)["version"])
	})
}

// The REAL validator decides: a retention outside 1–52 is a 400 naming the
// field, and nothing is stored or audited. The bounds themselves are accepted.
func TestAdminAuditLogSettings_RetentionBounds(t *testing.T) {
	cases := []struct {
		weeks int
		want  int
	}{
		{models.MinAuditRetentionWeeks - 1, http.StatusBadRequest},
		{models.MinAuditRetentionWeeks, http.StatusOK},
		{models.MaxAuditRetentionWeeks, http.StatusOK},
		{models.MaxAuditRetentionWeeks + 1, http.StatusBadRequest},
		{-1, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.weeks), func(t *testing.T) {
			f := newAuditLogSettingsFixture(t)
			f.resolveAdminName()

			rr := f.serve(t, http.MethodPut, auditLogSettingsPath, auditLogSettingsBody(tc.weeks, true))

			require.Equal(t, tc.want, rr.Code, rr.Body.String())
			if tc.want == http.StatusOK {
				return
			}
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
			assert.Equal(t, "retention_weeks", problem.ValidationErrors[0].Field)
			assert.Equal(t, services.SettingsFieldOutOfRange, problem.ValidationErrors[0].Code)
			assert.Empty(t, f.audit.entries)
			assert.Nil(t, f.repo.row, "nothing is stored")
		})
	}
}

// The body guard rejects a key the schema does not have before the handler
// runs.
func TestAdminAuditLogSettings_UnknownFieldIs400(t *testing.T) {
	f := newAuditLogSettingsFixture(t)
	body := auditLogSettingsBody(2, true)
	body["retention_days"] = 14

	rr := f.serve(t, http.MethodPut, auditLogSettingsPath, body)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "retention_days")
	assert.Empty(t, f.audit.entries)
}

// A PUT is a whole-row replace, so dropping a setting, or sending it as null,
// is a 400 naming it: never a silently zeroed value (a missing capture_ip_ua
// would otherwise switch capture off). expected_version stays optional.
func TestAdminAuditLogSettings_MissingOrNullFieldIs400(t *testing.T) {
	for _, key := range auditLogSettingsAuditSnapshotKeys {
		for _, mode := range []string{"missing", "null"} {
			t.Run(mode+" "+key, func(t *testing.T) {
				f := newAuditLogSettingsFixture(t)
				body := auditLogSettingsBody(2, true)
				delete(body, key)
				if mode == "null" {
					body[key] = nil
				}

				rr := f.serve(t, http.MethodPut, auditLogSettingsPath, body)

				require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
				assert.Contains(t, rr.Body.String(), "Missing required field(s): "+key)
				assert.Empty(t, f.audit.entries)
			})
		}
	}
}

// Repository failures are logged 500s that never echo the underlying error.
func TestAdminAuditLogSettings_FailuresAre500(t *testing.T) {
	boom := errors.New("pq: connection reset by peer")
	cases := []struct {
		name   string
		method string
		path   string
		body   any
		break_ func(f *auditLogSettingsFixture)
	}{
		{"get", http.MethodGet, auditLogSettingsPath, nil,
			func(f *auditLogSettingsFixture) { f.repo.getErr = boom }},
		{"put", http.MethodPut, auditLogSettingsPath, auditLogSettingsBody(2, true),
			func(f *auditLogSettingsFixture) { f.repo.upsertErr = boom }},
		{"delete", http.MethodDelete, auditLogSettingsPath, nil,
			func(f *auditLogSettingsFixture) { f.repo.deleteErr = boom }},
		{"audit", http.MethodGet, auditLogSettingsAuditPath, nil,
			func(f *auditLogSettingsFixture) { f.audit.listErr = boom }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuditLogSettingsFixture(t)
			tc.break_(f)

			rr := f.serve(t, tc.method, tc.path, tc.body)

			require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "connection reset")
		})
	}
}

// A failed updater-name lookup degrades to a null name rather than failing
// the request.
func TestAdminAuditLogSettings_UpdaterNameLookupFailureDegrades(t *testing.T) {
	f := newAuditLogSettingsFixture(t)
	f.userRepo.On("GetNamesByIDs", mock.Anything, mock.Anything).Return(nil, errors.New("db down"))

	rr := f.serve(t, http.MethodPut, auditLogSettingsPath, auditLogSettingsBody(3, true))

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	got := decodeJSONMap(t, rr)
	assert.Equal(t, instanceEmailActingAdmin, got["updated_by_user_id"])
	assert.Nil(t, got["updated_by_name"])
}

// The audit list returns only audit_log entries, newest first, and pages.
func TestAdminAuditLogSettings_AuditListPages(t *testing.T) {
	f := newAuditLogSettingsFixture(t)
	f.resolveAdminName()
	for weeks := 1; weeks <= 3; weeks++ {
		require.Equal(t, http.StatusOK,
			f.serve(t, http.MethodPut, auditLogSettingsPath, auditLogSettingsBody(weeks, true)).Code)
	}
	// An entry for another setting must never appear here.
	require.NoError(t, f.audit.Append(context.Background(), &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingSiteInfo, Action: models.InstanceSettingsAuditActionDelete,
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
	first := f.serve(t, http.MethodGet, auditLogSettingsAuditPath, nil)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &page))
	require.Len(t, page.Entries, 3)
	assert.Nil(t, page.NextCursor)
	for _, e := range page.Entries {
		assert.Equal(t, models.InstanceSettingAuditLog, e.Setting)
		assert.Equal(t, models.InstanceSettingsAuditActionUpsert, e.Action)
		require.NotNil(t, e.ActorName)
		assert.Equal(t, instanceSettingsAdminName, *e.ActorName)
	}
	assert.Equal(t, map[string]any{"retention_weeks": float64(3), "capture_ip_ua": true},
		page.Entries[0].After, "newest first")
	assert.Equal(t, map[string]any{"retention_weeks": float64(2), "capture_ip_ua": true}, page.Entries[0].Before)
	assert.Nil(t, page.Entries[2].Before, "the oldest entry created the row")

	seen := map[string]bool{}
	path := auditLogSettingsAuditPath + "?limit=1"
	for range 3 {
		rr := f.serve(t, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
		require.Len(t, page.Entries, 1)
		assert.False(t, seen[page.Entries[0].ID], "an entry must not repeat across pages")
		seen[page.Entries[0].ID] = true
		if page.NextCursor != nil {
			path = auditLogSettingsAuditPath + "?limit=1&cursor=" + *page.NextCursor
		}
	}
	assert.Len(t, seen, 3)
	assert.Nil(t, page.NextCursor, "the third page is the last")
}

// A key outside the allowlist never reaches the audit surface.
func TestAdminAuditLogSettings_AuditSnapshotsAreAllowlisted(t *testing.T) {
	f := newAuditLogSettingsFixture(t)
	after := auditLogSettingsBody(9, true)
	after["unreviewed_key"] = "must-not-leak"
	raw, err := json.Marshal(after)
	require.NoError(t, err)
	require.NoError(t, f.audit.Append(context.Background(), &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingAuditLog, Action: models.InstanceSettingsAuditActionUpsert, After: raw,
	}))

	rr := f.serve(t, http.MethodGet, auditLogSettingsAuditPath, nil)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.NotContains(t, rr.Body.String(), "must-not-leak")
	assert.Contains(t, rr.Body.String(), `"retention_weeks":9`)
}

func TestAdminAuditLogSettings_AuditBadParamsAre400(t *testing.T) {
	for _, query := range []string{
		"?limit=0", "?limit=" + strconv.Itoa(adminInstanceAuditMaxLimit+1), "?cursor=not-a-cursor",
	} {
		t.Run(query, func(t *testing.T) {
			f := newAuditLogSettingsFixture(t)

			rr := f.serve(t, http.MethodGet, auditLogSettingsAuditPath+query, nil)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		})
	}
}

// The stored setting constant is in the audit entry's published enum, and the
// snapshot allowlist is exactly the values' JSON shape.
func TestAuditLogSettingsAuditVocabularyMatchesSpecAndModel(t *testing.T) {
	assert.True(t, admingen.AdminInstanceSettingsAuditEntrySetting(models.InstanceSettingAuditLog).Valid())

	raw, err := json.Marshal(models.InstanceAuditSettingsValues{})
	require.NoError(t, err)
	var shape map[string]any
	require.NoError(t, json.Unmarshal(raw, &shape))
	keys := make([]string, 0, len(shape))
	for k := range shape {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, auditLogSettingsAuditSnapshotKeys, keys)
}

// TestAdminAuditLogSettingsRoutes_Gate404 drives the FULL router
// (setupAdminRoutes: optionalAuthMiddleware + instanceAdminMiddleware) for
// every audit-log settings operation: an anonymous caller and an authenticated
// non-admin (API key) both get 404, and the service and audit mocks carry no
// expectations, so reaching a handler fails the test.
func TestAdminAuditLogSettingsRoutes_Gate404(t *testing.T) {
	const nonAdminID = "33333333-3333-4333-8333-333333333333"
	cfg := &config.Config{Auth: config.AuthConfig{InstanceAdmins: config.EnvStringSlice{instanceEmailAdminEmail}}}
	routes := []struct{ method, path string }{
		{http.MethodGet, auditLogSettingsPath},
		{http.MethodPut, auditLogSettingsPath},
		{http.MethodDelete, auditLogSettingsPath},
		{http.MethodGet, auditLogSettingsAuditPath},
	}

	for _, route := range routes {
		for _, caller := range []string{"anonymous", "non-admin"} {
			t.Run(caller+" "+route.method+" "+route.path, func(t *testing.T) {
				authSvc := servicesmocks.NewMockAuthServiceInterface(t)
				keySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
				srv := newAdminTestServer(cfg, &adminMockContainer{
					authService:                  authSvc,
					apiKeyService:                keySvc,
					instanceAuditRepo:            repomocks.NewMockInstanceSettingsAuditRepository(t),
					instanceAuditSettingsService: servicesmocks.NewMockInstanceAuditSettingsServiceInterface(t),
				})

				body, err := json.Marshal(auditLogSettingsBody(2, true))
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

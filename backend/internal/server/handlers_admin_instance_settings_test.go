package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
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
	instanceSearchSettingsPath    = "/api/v1/admin/settings/search"
	instanceAISummarySettingsPath = "/api/v1/admin/settings/ai-summary"
	instanceSettingsAdminName     = "Ada Admin"
	instanceSettingsTeamOverrides = 3
)

// instanceSettingsResponseKeys is the complete top-level key set of both GET
// responses; the values/built_in_defaults/limits key sets are per section.
var instanceSettingsResponseKeys = []string{
	"values", "source", "built_in_defaults", "limits", "teams_with_override",
	"updated_at", "updated_by_user_id", "updated_by_name", "version",
}

// --- in-memory stores ---------------------------------------------------------
//
// The handlers run over the REAL instance settings services (so the real
// validators decide every 400) and these in-memory repositories, which keep
// the singleton row, bump its version and append audit entries the way the
// postgres repositories do. That makes PUT → GET → DELETE → GET a genuine
// round trip. The postgres compare-and-set itself is covered in the
// repositories/postgres tests.

// fakeInstanceAuditRepo implements repositories.InstanceSettingsAuditRepository
// over a slice, with the same newest-first keyset paging contract.
type fakeInstanceAuditRepo struct {
	entries []*models.InstanceSettingsAuditEntry
	clock   time.Time
	listErr error
}

func (f *fakeInstanceAuditRepo) tick() time.Time {
	f.clock = f.clock.Add(time.Minute)
	return f.clock
}

func (f *fakeInstanceAuditRepo) Append(_ context.Context, entry *models.InstanceSettingsAuditEntry) error {
	entry.ID = uuid.NewString()
	entry.CreatedAt = f.tick()
	f.entries = append(f.entries, entry)
	return nil
}

func (f *fakeInstanceAuditRepo) List(
	_ context.Context, setting string, limit int, cursor *models.InstanceSettingsAuditCursor,
) ([]*models.InstanceSettingsAuditEntry, *models.InstanceSettingsAuditCursor, error) {
	if f.listErr != nil {
		return nil, nil, f.listErr
	}
	var matching []*models.InstanceSettingsAuditEntry
	for _, e := range f.entries {
		if e.Setting == setting && (cursor == nil || e.CreatedAt.Before(cursor.CreatedAt)) {
			matching = append(matching, e)
		}
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].CreatedAt.After(matching[j].CreatedAt) })
	if len(matching) <= limit {
		return matching, nil, nil
	}
	last := matching[limit-1]
	return matching[:limit], &models.InstanceSettingsAuditCursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
}

// fakeSingleton is the shared core of the two in-memory settings repositories.
// meta exposes a row's version and timestamps so one implementation serves
// both row types.
type fakeSingleton[T any] struct {
	audit     *fakeInstanceAuditRepo
	row       *T
	meta      func(*T) (version *int64, createdAt, updatedAt *time.Time)
	getErr    error
	upsertErr error
	deleteErr error
}

func (f *fakeSingleton[T]) get() (*T, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.row == nil {
		return nil, nil
	}
	c := *f.row
	return &c, nil
}

func (f *fakeSingleton[T]) upsertAudited(
	s *T, expected *int64, audit func(before, after *T) (*models.InstanceSettingsAuditEntry, error),
) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	var before *T
	var stored *int64
	if f.row != nil {
		c := *f.row
		before = &c
		stored, _, _ = f.meta(before)
	}
	if expected != nil && (stored == nil || *stored != *expected) {
		return repositories.ErrInstanceSettingsVersionConflict
	}
	version, createdAt, updatedAt := f.meta(s)
	now := f.audit.tick()
	*version, *createdAt, *updatedAt = 1, now, now
	if stored != nil {
		_, beforeCreated, _ := f.meta(before)
		*version, *createdAt = *stored+1, *beforeCreated
	}
	entry, err := audit(before, s)
	if err != nil {
		return err
	}
	if err := f.audit.Append(context.Background(), entry); err != nil {
		return err
	}
	c := *s
	f.row = &c
	return nil
}

func (f *fakeSingleton[T]) deleteAudited(
	audit func(before, after *T) (*models.InstanceSettingsAuditEntry, error),
) (bool, error) {
	if f.deleteErr != nil {
		return false, f.deleteErr
	}
	if f.row == nil {
		return false, nil
	}
	entry, err := audit(f.row, nil)
	if err != nil {
		return false, err
	}
	if err := f.audit.Append(context.Background(), entry); err != nil {
		return false, err
	}
	f.row = nil
	return true, nil
}

var errFakeUnused = errors.New("not used by the instance settings handlers")

type fakeInstanceSearchRepo struct {
	fakeSingleton[models.InstanceSearchSettings]
}

func (r *fakeInstanceSearchRepo) Get(context.Context) (*models.InstanceSearchSettings, error) {
	row, err := r.get()
	if err == nil && row == nil {
		err = repositories.ErrInstanceSearchSettingsNotFound
	}
	return row, err
}
func (r *fakeInstanceSearchRepo) Upsert(context.Context, *models.InstanceSearchSettings) error {
	return errFakeUnused
}
func (r *fakeInstanceSearchRepo) InsertIfAbsent(context.Context, *models.InstanceSearchSettings) (bool, error) {
	return false, errFakeUnused
}
func (r *fakeInstanceSearchRepo) Delete(context.Context) error { return errFakeUnused }
func (r *fakeInstanceSearchRepo) UpsertAudited(
	_ context.Context, s *models.InstanceSearchSettings, v *int64, audit repositories.InstanceSearchSettingsAuditFunc,
) error {
	return r.upsertAudited(s, v, audit)
}
func (r *fakeInstanceSearchRepo) DeleteAudited(
	_ context.Context, audit repositories.InstanceSearchSettingsAuditFunc,
) (bool, error) {
	return r.deleteAudited(audit)
}

type fakeInstanceAISummaryRepo struct {
	fakeSingleton[models.InstanceAISummarySettings]
}

func (r *fakeInstanceAISummaryRepo) Get(context.Context) (*models.InstanceAISummarySettings, error) {
	row, err := r.get()
	if err == nil && row == nil {
		err = repositories.ErrInstanceAISummarySettingsNotFound
	}
	return row, err
}
func (r *fakeInstanceAISummaryRepo) Upsert(context.Context, *models.InstanceAISummarySettings) error {
	return errFakeUnused
}
func (r *fakeInstanceAISummaryRepo) InsertIfAbsent(context.Context, *models.InstanceAISummarySettings) (bool, error) {
	return false, errFakeUnused
}
func (r *fakeInstanceAISummaryRepo) Delete(context.Context) error { return errFakeUnused }
func (r *fakeInstanceAISummaryRepo) UpsertAudited(
	_ context.Context, s *models.InstanceAISummarySettings, v *int64,
	audit repositories.InstanceAISummarySettingsAuditFunc,
) error {
	return r.upsertAudited(s, v, audit)
}
func (r *fakeInstanceAISummaryRepo) DeleteAudited(
	_ context.Context, audit repositories.InstanceAISummarySettingsAuditFunc,
) (bool, error) {
	return r.deleteAudited(audit)
}

// --- fixture ------------------------------------------------------------------

type instanceSettingsFixture struct {
	router    http.Handler
	audit     *fakeInstanceAuditRepo
	search    *fakeInstanceSearchRepo
	aiSummary *fakeInstanceAISummaryRepo
	admin     *servicesmocks.MockAdminServiceInterface
	userRepo  *repomocks.MockUserRepository
}

func newInstanceSettingsFixture(t *testing.T) *instanceSettingsFixture {
	t.Helper()
	audit := &fakeInstanceAuditRepo{clock: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
	f := &instanceSettingsFixture{
		audit: audit,
		search: &fakeInstanceSearchRepo{fakeSingleton[models.InstanceSearchSettings]{
			audit: audit,
			meta: func(r *models.InstanceSearchSettings) (*int64, *time.Time, *time.Time) {
				return &r.Version, &r.CreatedAt, &r.UpdatedAt
			},
		}},
		aiSummary: &fakeInstanceAISummaryRepo{fakeSingleton[models.InstanceAISummarySettings]{
			audit: audit,
			meta: func(r *models.InstanceAISummarySettings) (*int64, *time.Time, *time.Time) {
				return &r.Version, &r.CreatedAt, &r.UpdatedAt
			},
		}},
		admin:    servicesmocks.NewMockAdminServiceInterface(t),
		userRepo: repomocks.NewMockUserRepository(t),
	}
	f.admin.On("CountTeamsWithSearchSettingsOverride", mock.Anything).
		Return(instanceSettingsTeamOverrides, nil).Maybe()
	f.admin.On("CountTeamsWithAISummarySettingsOverride", mock.Anything).
		Return(instanceSettingsTeamOverrides, nil).Maybe()
	f.userRepo.On("GetNamesByIDs", mock.Anything, []string{instanceEmailActingAdmin}).
		Return(map[string]string{instanceEmailActingAdmin: instanceSettingsAdminName}, nil).Maybe()

	logger := slog.New(slog.DiscardHandler)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		adminService:             f.admin,
		userRepo:                 f.userRepo,
		instanceAuditRepo:        audit,
		instanceSearchService:    services.NewInstanceSearchSettingsService(f.search, logger),
		instanceAISummaryService: services.NewInstanceAISummarySettingsService(f.aiSummary, logger),
	})
	f.router = mountAdminStrictRouter(srv)
	return f
}

// serve sends one request as the acting admin, asserts the response conforms
// to the spec and returns it.
func (f *instanceSettingsFixture) serve(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := instanceEmailRequest(t, method, path, body)
	rr := serveInstanceEmail(t, f.router, req)
	specconformance.AssertConformsToSpec(t, req, rr)
	return rr
}

func decodeJSONMap(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got), rr.Body.String())
	return got
}

// instanceSettingsSection describes one of the two settings domains, so every
// contract test runs against both.
type instanceSettingsSection struct {
	name    string
	path    string
	setting string
	// body returns a valid whole-row PUT body; variant makes successive
	// bodies distinguishable.
	body func(variant int) map[string]any
	// builtIn is the built-in defaults as the API renders them.
	builtIn   map[string]any
	valueKeys []string
	limitKeys []string
}

func (s instanceSettingsSection) keyPaths() []string {
	paths := append([]string{}, instanceSettingsResponseKeys...)
	for _, k := range s.valueKeys {
		paths = append(paths, "values."+k, "built_in_defaults."+k)
	}
	for _, k := range s.limitKeys {
		paths = append(paths, "limits."+k)
	}
	return paths
}

var instanceSettingsSections = []instanceSettingsSection{
	{
		name:    "search",
		path:    instanceSearchSettingsPath,
		setting: models.InstanceSettingSearch,
		body: func(variant int) map[string]any {
			return map[string]any{
				"recency_ranking_enabled": true,
				"rank_weight_relevance":   0.6,
				"rank_weight_created":     0.2,
				"rank_weight_updated":     0.2,
				"rank_half_life_days":     30,
				"rank_candidate_cap":      300 + variant,
			}
		},
		builtIn: map[string]any{
			"recency_ranking_enabled": false,
			"rank_weight_relevance":   0.5,
			"rank_weight_created":     0.3,
			"rank_weight_updated":     0.2,
			"rank_half_life_days":     float64(90),
			"rank_candidate_cap":      float64(200),
		},
		valueKeys: instanceSearchAuditSnapshotKeys,
		limitKeys: []string{
			"rank_weight_min", "rank_half_life_days_max", "rank_candidate_cap_min", "rank_candidate_cap_max",
		},
	},
	{
		name:    "ai summary",
		path:    instanceAISummarySettingsPath,
		setting: models.InstanceSettingAISummary,
		body: func(variant int) map[string]any {
			return map[string]any{
				"enabled":             false,
				"top_n":               3,
				"style":               "detailed",
				"max_output_tokens":   1200 + variant,
				"per_document_chars":  4000,
				"total_context_chars": 16000,
				"request_timeout_ms":  45000,
			}
		},
		builtIn: map[string]any{
			"enabled":             true,
			"top_n":               float64(5),
			"style":               "balanced",
			"max_output_tokens":   float64(800),
			"per_document_chars":  float64(8000),
			"total_context_chars": float64(32000),
			"request_timeout_ms":  float64(60000),
		},
		valueKeys: instanceAISummaryAuditSnapshotKeys,
		limitKeys: []string{
			"top_n_min", "top_n_max", "max_output_tokens_min", "max_output_tokens_max",
			"chars_min", "chars_max", "request_timeout_ms_min", "request_timeout_ms_max",
		},
	},
}

// asJSONNumbers renders a body the way it reads back from a response.
func asJSONNumbers(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// --- GET ----------------------------------------------------------------------

// TestAdminInstanceSettings_GetDefaults: with nothing stored, both GETs report
// source default, values equal to the built-in defaults, null row metadata,
// and exactly the documented key set (no max_top_n / ceiling fields).
func TestAdminInstanceSettings_GetDefaults(t *testing.T) {
	for _, sec := range instanceSettingsSections {
		t.Run(sec.name, func(t *testing.T) {
			f := newInstanceSettingsFixture(t)

			rr := f.serve(t, http.MethodGet, sec.path, nil)

			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assertJSONKeyPaths(t, rr.Body.Bytes(), sec.keyPaths()...)
			assert.NotContains(t, rr.Body.String(), "max_top_n")
			got := decodeJSONMap(t, rr)
			assert.Equal(t, "default", got["source"])
			assert.Equal(t, sec.builtIn, got["values"])
			assert.Equal(t, sec.builtIn, got["built_in_defaults"])
			assert.Equal(t, float64(instanceSettingsTeamOverrides), got["teams_with_override"])
			for _, k := range []string{"updated_at", "updated_by_user_id", "updated_by_name", "version"} {
				assert.Nil(t, got[k], k)
			}
		})
	}
}

// TestAdminInstanceSettings_Limits pins the published bounds to the code
// constants the validators enforce.
func TestAdminInstanceSettings_Limits(t *testing.T) {
	f := newInstanceSettingsFixture(t)

	search := decodeJSONMap(t, f.serve(t, http.MethodGet, instanceSearchSettingsPath, nil))
	assert.Equal(t, map[string]any{
		"rank_weight_min":         float64(0),
		"rank_half_life_days_max": float64(models.MaxSearchRankHalfLifeDays),
		"rank_candidate_cap_min":  float64(1),
		"rank_candidate_cap_max":  float64(models.MaxSearchRankCandidateCap),
	}, search["limits"])

	summary := decodeJSONMap(t, f.serve(t, http.MethodGet, instanceAISummarySettingsPath, nil))
	assert.Equal(t, map[string]any{
		"top_n_min":              float64(1),
		"top_n_max":              float64(models.MaxAISummaryTopN),
		"max_output_tokens_min":  float64(1),
		"max_output_tokens_max":  float64(models.MaxAISummaryOutputTokens),
		"chars_min":              float64(1),
		"chars_max":              float64(math.MaxInt32),
		"request_timeout_ms_min": float64(1),
		"request_timeout_ms_max": float64(math.MaxInt32),
	}, summary["limits"])
}

// --- PUT / DELETE round trip --------------------------------------------------

// TestAdminInstanceSettings_RoundTrip walks the whole lifecycle of each
// section: PUT → GET (source instance, updater resolved, version 1), a
// compare-and-set PUT (version 2), a stale PUT (409, nothing changes), DELETE
// → GET (source default, built-in values) and a no-op DELETE. Exactly one
// audit entry is written per real change.
func TestAdminInstanceSettings_RoundTrip(t *testing.T) {
	for _, sec := range instanceSettingsSections {
		t.Run(sec.name, func(t *testing.T) {
			f := newInstanceSettingsFixture(t)

			first := sec.body(1)
			rr := f.serve(t, http.MethodPut, sec.path, first)
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assertJSONKeyPaths(t, rr.Body.Bytes(), sec.keyPaths()...)

			got := decodeJSONMap(t, f.serve(t, http.MethodGet, sec.path, nil))
			assert.Equal(t, "instance", got["source"])
			assert.Equal(t, asJSONNumbers(t, first), got["values"])
			assert.Equal(t, sec.builtIn, got["built_in_defaults"])
			assert.Equal(t, instanceEmailActingAdmin, got["updated_by_user_id"])
			assert.Equal(t, instanceSettingsAdminName, got["updated_by_name"])
			assert.NotNil(t, got["updated_at"])
			assert.Equal(t, float64(1), got["version"])

			second := sec.body(2)
			second["expected_version"] = 1
			rr = f.serve(t, http.MethodPut, sec.path, second)
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assert.Equal(t, float64(2), decodeJSONMap(t, rr)["version"])

			stale := sec.body(3)
			stale["expected_version"] = 1
			rr = f.serve(t, http.MethodPut, sec.path, stale)
			require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), "INSTANCE_SETTINGS_VERSION_CONFLICT")

			got = decodeJSONMap(t, f.serve(t, http.MethodGet, sec.path, nil))
			delete(second, "expected_version")
			assert.Equal(t, asJSONNumbers(t, second), got["values"], "a stale PUT changes nothing")
			assert.Equal(t, float64(2), got["version"])

			require.Equal(t, http.StatusNoContent, f.serve(t, http.MethodDelete, sec.path, nil).Code)
			got = decodeJSONMap(t, f.serve(t, http.MethodGet, sec.path, nil))
			assert.Equal(t, "default", got["source"])
			assert.Equal(t, got["built_in_defaults"], got["values"])
			assert.Nil(t, got["updated_at"])
			assert.Nil(t, got["version"])

			require.Equal(t, http.StatusNoContent, f.serve(t, http.MethodDelete, sec.path, nil).Code,
				"resetting an already-default section is still a 204")

			require.Len(t, f.audit.entries, 3, "two upserts and one delete; the stale PUT and no-op reset audit nothing")
			actions := []string{f.audit.entries[0].Action, f.audit.entries[1].Action, f.audit.entries[2].Action}
			assert.Equal(t, []string{
				models.InstanceSettingsAuditActionUpsert, models.InstanceSettingsAuditActionUpsert,
				models.InstanceSettingsAuditActionDelete,
			}, actions)
			for _, e := range f.audit.entries {
				assert.Equal(t, sec.setting, e.Setting)
				require.NotNil(t, e.ActorUserID)
				assert.Equal(t, instanceEmailActingAdmin, *e.ActorUserID)
			}
			assert.Nil(t, f.audit.entries[0].Before, "the first save creates the row")
			assert.NotNil(t, f.audit.entries[1].Before)
			assert.Nil(t, f.audit.entries[2].After, "a reset removes the row")
		})
	}
}

// TestAdminInstanceSettings_ExpectedVersionWithNothingStoredIs409: a
// compare-and-set against a row that does not exist is a conflict.
func TestAdminInstanceSettings_ExpectedVersionWithNothingStoredIs409(t *testing.T) {
	for _, sec := range instanceSettingsSections {
		t.Run(sec.name, func(t *testing.T) {
			f := newInstanceSettingsFixture(t)
			body := sec.body(1)
			body["expected_version"] = 1

			rr := f.serve(t, http.MethodPut, sec.path, body)

			require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
			assert.Empty(t, f.audit.entries)
		})
	}
}

// TestAdminInstanceSettings_NullExpectedVersionIsLastWriteWins: an explicit
// null behaves like an omitted expected_version.
func TestAdminInstanceSettings_NullExpectedVersionIsLastWriteWins(t *testing.T) {
	f := newInstanceSettingsFixture(t)
	body := instanceSettingsSections[0].body(1)
	body["expected_version"] = nil

	rr := f.serve(t, http.MethodPut, instanceSearchSettingsPath, body)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

// --- validation ---------------------------------------------------------------

// TestAdminInstanceSettings_OutOfBoundsIs400 drives every bound through the
// REAL validators: each case is a 400 naming the offending field(s), and
// nothing is stored or audited.
func TestAdminInstanceSettings_OutOfBoundsIs400(t *testing.T) {
	weights := []string{"rank_weight_relevance", "rank_weight_created", "rank_weight_updated"}
	cases := []struct {
		name       string
		section    instanceSettingsSection
		set        map[string]any
		wantFields []string
		// wantCode is the validation error code; empty means OUT_OF_RANGE.
		wantCode string
	}{
		{"negative weight", instanceSettingsSections[0],
			map[string]any{"rank_weight_created": -0.1}, []string{"rank_weight_created"}, ""},
		{"all weights zero", instanceSettingsSections[0],
			map[string]any{"rank_weight_relevance": 0, "rank_weight_created": 0, "rank_weight_updated": 0}, weights,
			services.SettingsFieldInvalidValue},
		{"half-life zero", instanceSettingsSections[0],
			map[string]any{"rank_half_life_days": 0}, []string{"rank_half_life_days"}, ""},
		{"half-life too long", instanceSettingsSections[0],
			map[string]any{"rank_half_life_days": 36501}, []string{"rank_half_life_days"}, ""},
		{"candidate cap zero", instanceSettingsSections[0],
			map[string]any{"rank_candidate_cap": 0}, []string{"rank_candidate_cap"}, ""},
		{"candidate cap too large", instanceSettingsSections[0],
			map[string]any{"rank_candidate_cap": 5001}, []string{"rank_candidate_cap"}, ""},
		{"top_n too large", instanceSettingsSections[1],
			map[string]any{"top_n": 11}, []string{"top_n"}, ""},
		{"max_output_tokens too large", instanceSettingsSections[1],
			map[string]any{"max_output_tokens": 32769}, []string{"max_output_tokens"}, ""},
		{"unknown style", instanceSettingsSections[1],
			map[string]any{"style": "verbose"}, []string{"style"}, services.SettingsFieldInvalidValue},
		{"per_document_chars zero", instanceSettingsSections[1],
			map[string]any{"per_document_chars": 0}, []string{"per_document_chars"}, ""},
		{"total below per-document", instanceSettingsSections[1],
			map[string]any{"total_context_chars": 3999}, []string{"total_context_chars"}, ""},
		{"timeout zero", instanceSettingsSections[1],
			map[string]any{"request_timeout_ms": 0}, []string{"request_timeout_ms"}, ""},
		{"timeout past the column", instanceSettingsSections[1],
			map[string]any{"request_timeout_ms": int64(math.MaxInt32) + 1}, []string{"request_timeout_ms"}, ""},
		{"timeout that would overflow nanoseconds", instanceSettingsSections[1],
			map[string]any{"request_timeout_ms": int64(math.MaxInt64 / 1000)}, []string{"request_timeout_ms"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newInstanceSettingsFixture(t)
			body := tc.section.body(1)
			for k, v := range tc.set {
				body[k] = v
			}

			rr := f.serve(t, http.MethodPut, tc.section.path, body)

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
			wantCode := tc.wantCode
			if wantCode == "" {
				wantCode = services.SettingsFieldOutOfRange
			}
			var fields []string
			for _, ve := range problem.ValidationErrors {
				fields = append(fields, ve.Field)
				assert.Equal(t, wantCode, ve.Code, ve.Field)
			}
			assert.ElementsMatch(t, tc.wantFields, fields)
			assert.Empty(t, f.audit.entries)
		})
	}
}

// TestAdminInstanceSettings_UnknownFieldIs400: the body guard rejects a key
// the schema does not have (e.g. the dropped max_top_n ceiling) before the
// handler runs.
func TestAdminInstanceSettings_UnknownFieldIs400(t *testing.T) {
	for _, sec := range instanceSettingsSections {
		t.Run(sec.name, func(t *testing.T) {
			f := newInstanceSettingsFixture(t)
			body := sec.body(1)
			body["max_top_n"] = 10

			rr := f.serve(t, http.MethodPut, sec.path, body)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), "max_top_n")
			assert.Empty(t, f.audit.entries)
		})
	}
}

// TestAdminInstanceSettings_MissingOrNullFieldIs400: a PUT is a whole-row
// replace, so dropping any value field, or sending it as null, is a 400 naming
// it — never a silently stored zero value. expected_version stays optional.
func TestAdminInstanceSettings_MissingOrNullFieldIs400(t *testing.T) {
	for _, sec := range instanceSettingsSections {
		for _, key := range sec.valueKeys {
			for _, mode := range []string{"missing", "null"} {
				t.Run(sec.name+" "+mode+" "+key, func(t *testing.T) {
					f := newInstanceSettingsFixture(t)
					body := sec.body(1)
					delete(body, key)
					if mode == "null" {
						body[key] = nil
					}

					rr := f.serve(t, http.MethodPut, sec.path, body)

					require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
					assert.Contains(t, rr.Body.String(), "Missing required field(s): "+key)
					assert.Empty(t, f.audit.entries)
				})
			}
		}
	}
}

// --- failures -----------------------------------------------------------------

// TestAdminInstanceSettings_FailuresAre500: repository and count failures are
// logged 500s that never echo the underlying error.
func TestAdminInstanceSettings_FailuresAre500(t *testing.T) {
	boom := errors.New("db down")
	cases := []struct {
		name    string
		method  string
		path    string
		body    func() map[string]any
		breakIt func(f *instanceSettingsFixture)
	}{
		{"search get", http.MethodGet, instanceSearchSettingsPath, nil,
			func(f *instanceSettingsFixture) { f.search.getErr = boom }},
		{"search put", http.MethodPut, instanceSearchSettingsPath,
			func() map[string]any { return instanceSettingsSections[0].body(1) },
			func(f *instanceSettingsFixture) { f.search.upsertErr = boom }},
		{"search reset", http.MethodDelete, instanceSearchSettingsPath, nil,
			func(f *instanceSettingsFixture) { f.search.deleteErr = boom }},
		{"search audit", http.MethodGet, instanceSearchSettingsPath + "/audit", nil,
			func(f *instanceSettingsFixture) { f.audit.listErr = boom }},
		{"ai summary get", http.MethodGet, instanceAISummarySettingsPath, nil,
			func(f *instanceSettingsFixture) { f.aiSummary.getErr = boom }},
		{"ai summary put", http.MethodPut, instanceAISummarySettingsPath,
			func() map[string]any { return instanceSettingsSections[1].body(1) },
			func(f *instanceSettingsFixture) { f.aiSummary.upsertErr = boom }},
		{"ai summary reset", http.MethodDelete, instanceAISummarySettingsPath, nil,
			func(f *instanceSettingsFixture) { f.aiSummary.deleteErr = boom }},
		{"ai summary audit", http.MethodGet, instanceAISummarySettingsPath + "/audit", nil,
			func(f *instanceSettingsFixture) { f.audit.listErr = boom }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newInstanceSettingsFixture(t)
			tc.breakIt(f)
			var body any
			if tc.body != nil {
				body = tc.body()
			}

			rr := f.serve(t, tc.method, tc.path, body)

			require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "db down")
		})
	}
}

// TestAdminInstanceSettings_CountFailureIs500: teams_with_override is part of
// the documented response, so a failed count fails the read rather than
// reporting a made-up zero.
func TestAdminInstanceSettings_CountFailureIs500(t *testing.T) {
	for _, sec := range instanceSettingsSections {
		t.Run(sec.name, func(t *testing.T) {
			admin := servicesmocks.NewMockAdminServiceInterface(t)
			admin.On("CountTeamsWithSearchSettingsOverride", mock.Anything).Return(0, errors.New("db down")).Maybe()
			admin.On("CountTeamsWithAISummarySettingsOverride", mock.Anything).Return(0, errors.New("db down")).Maybe()
			f := newInstanceSettingsFixture(t)
			logger := slog.New(slog.DiscardHandler)
			srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
				adminService:             admin,
				instanceSearchService:    services.NewInstanceSearchSettingsService(f.search, logger),
				instanceAISummaryService: services.NewInstanceAISummarySettingsService(f.aiSummary, logger),
			})
			f.router = mountAdminStrictRouter(srv)

			rr := f.serve(t, http.MethodGet, sec.path, nil)

			require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
		})
	}
}

// TestAdminInstanceSettings_UpdaterNameLookupFailureDegrades: an updater whose
// name cannot be resolved reads as a null name, not a failed GET.
func TestAdminInstanceSettings_UpdaterNameLookupFailureDegrades(t *testing.T) {
	f := newInstanceSettingsFixture(t)
	require.Equal(t, http.StatusOK,
		f.serve(t, http.MethodPut, instanceSearchSettingsPath, instanceSettingsSections[0].body(1)).Code)
	userRepo := repomocks.NewMockUserRepository(t)
	userRepo.On("GetNamesByIDs", mock.Anything, []string{instanceEmailActingAdmin}).Return(nil, errors.New("db down"))
	logger := slog.New(slog.DiscardHandler)
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		adminService:          f.admin,
		userRepo:              userRepo,
		instanceSearchService: services.NewInstanceSearchSettingsService(f.search, logger),
	})
	f.router = mountAdminStrictRouter(srv)

	got := decodeJSONMap(t, f.serve(t, http.MethodGet, instanceSearchSettingsPath, nil))

	assert.Equal(t, instanceEmailActingAdmin, got["updated_by_user_id"])
	assert.Nil(t, got["updated_by_name"])
}

// --- audit list ---------------------------------------------------------------

// TestAdminInstanceSettings_AuditListIsPerSectionAndPages: each section's
// audit route returns only its own setting's entries, newest first, with the
// actor name resolved and the before/after values; paging with the cursor
// visits every entry exactly once.
func TestAdminInstanceSettings_AuditListIsPerSectionAndPages(t *testing.T) {
	f := newInstanceSettingsFixture(t)
	for i := 1; i <= 3; i++ {
		for _, sec := range instanceSettingsSections {
			require.Equal(t, http.StatusOK, f.serve(t, http.MethodPut, sec.path, sec.body(i)).Code)
		}
	}
	// An entry for another setting must never appear under either section.
	require.NoError(t, f.audit.Append(context.Background(), &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingEmailProvider, Action: models.InstanceSettingsAuditActionDelete,
	}))

	for _, sec := range instanceSettingsSections {
		t.Run(sec.name, func(t *testing.T) {
			first := f.serve(t, http.MethodGet, sec.path+"/audit", nil)
			require.Equal(t, http.StatusOK, first.Code, first.Body.String())
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
			require.NoError(t, json.Unmarshal(first.Body.Bytes(), &page))
			require.Len(t, page.Entries, 3)
			assert.Nil(t, page.NextCursor)
			for _, e := range page.Entries {
				assert.Equal(t, sec.setting, e.Setting)
				assert.Equal(t, models.InstanceSettingsAuditActionUpsert, e.Action)
				require.NotNil(t, e.ActorName)
				assert.Equal(t, instanceSettingsAdminName, *e.ActorName)
			}
			assert.Equal(t, asJSONNumbers(t, sec.body(3)), page.Entries[0].After, "newest first")
			assert.Equal(t, asJSONNumbers(t, sec.body(2)), page.Entries[0].Before)
			assert.Nil(t, page.Entries[2].Before, "the oldest entry created the row")

			seen := map[string]bool{}
			path := sec.path + "/audit?limit=1"
			for range 3 {
				rr := f.serve(t, http.MethodGet, path, nil)
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
				require.Len(t, page.Entries, 1)
				assert.False(t, seen[page.Entries[0].ID], "an entry must not repeat across pages")
				seen[page.Entries[0].ID] = true
				if page.NextCursor != nil {
					path = sec.path + "/audit?limit=1&cursor=" + *page.NextCursor
				}
			}
			assert.Len(t, seen, 3)
			assert.Nil(t, page.NextCursor, "the third page is the last")
		})
	}
}

// TestAdminInstanceSettings_AuditSnapshotsAreAllowlisted: a stored snapshot
// key outside the section's allowlist never reaches the response.
func TestAdminInstanceSettings_AuditSnapshotsAreAllowlisted(t *testing.T) {
	for _, sec := range instanceSettingsSections {
		t.Run(sec.name, func(t *testing.T) {
			f := newInstanceSettingsFixture(t)
			after := asJSONNumbers(t, sec.body(1))
			after["unreviewed_key"] = "must-not-leak"
			raw, err := json.Marshal(after)
			require.NoError(t, err)
			require.NoError(t, f.audit.Append(context.Background(), &models.InstanceSettingsAuditEntry{
				Setting: sec.setting, Action: models.InstanceSettingsAuditActionImport, After: raw,
			}))

			rr := f.serve(t, http.MethodGet, sec.path+"/audit", nil)

			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "must-not-leak")
			assert.Contains(t, rr.Body.String(), `"actor_name":null`, "an import has no actor")
		})
	}
}

// TestAdminInstanceSettings_AuditBadParamsAre400: a limit outside [1, 100] or a
// malformed cursor is a 400 on both sections.
func TestAdminInstanceSettings_AuditBadParamsAre400(t *testing.T) {
	for _, sec := range instanceSettingsSections {
		for _, query := range []string{"?limit=0", "?limit=" + strconv.Itoa(adminInstanceAuditMaxLimit+1), "?cursor=not-a-cursor"} {
			t.Run(sec.name+" "+query, func(t *testing.T) {
				f := newInstanceSettingsFixture(t)

				rr := f.serve(t, http.MethodGet, sec.path+"/audit"+query, nil)

				require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			})
		}
	}
}

// TestRequestTimeoutFromMS pins the clamp: in-range values convert exactly and
// out-of-range ones land just outside the validator's bounds.
func TestRequestTimeoutFromMS(t *testing.T) {
	assert.Equal(t, 45*time.Second, requestTimeoutFromMS(45000))
	assert.Equal(t, time.Duration(0), requestTimeoutFromMS(-5))
	assert.Equal(t, time.Duration(math.MaxInt32+1)*time.Millisecond, requestTimeoutFromMS(math.MaxInt64))
}

// TestInstanceSettingsAuditVocabularyMatchesSpec pins the stored setting and
// action constants to the audit entry's published enums, so a constant added
// or renamed in models cannot silently produce an off-spec response.
func TestInstanceSettingsAuditVocabularyMatchesSpec(t *testing.T) {
	for _, setting := range []string{
		models.InstanceSettingEmailProvider, models.InstanceSettingSearch, models.InstanceSettingAISummary,
	} {
		assert.True(t, admingen.AdminInstanceSettingsAuditEntrySetting(setting).Valid(), setting)
	}
	for _, action := range []string{
		models.InstanceSettingsAuditActionUpsert, models.InstanceSettingsAuditActionDelete,
		models.InstanceSettingsAuditActionImport,
	} {
		assert.True(t, admingen.AdminInstanceSettingsAuditEntryAction(action).Valid(), action)
	}
}

// --- the gate -----------------------------------------------------------------

// adminInstanceSettingsRoutes lists every instance settings operation (email,
// search, AI summary), for the 404 gate tests.
var adminInstanceSettingsRoutes = []struct{ method, path string }{
	{http.MethodGet, instanceEmailSettingsPath},
	{http.MethodPut, instanceEmailSettingsPath},
	{http.MethodDelete, instanceEmailSettingsPath},
	{http.MethodPost, instanceEmailSettingsPath + "/test"},
	{http.MethodGet, instanceEmailSettingsPath + "/audit"},
	// Instance search + AI summary settings (#1200).
	{http.MethodGet, instanceSearchSettingsPath},
	{http.MethodPut, instanceSearchSettingsPath},
	{http.MethodDelete, instanceSearchSettingsPath},
	{http.MethodGet, instanceSearchSettingsPath + "/audit"},
	{http.MethodGet, instanceAISummarySettingsPath},
	{http.MethodPut, instanceAISummarySettingsPath},
	{http.MethodDelete, instanceAISummarySettingsPath},
	{http.MethodGet, instanceAISummarySettingsPath + "/audit"},
}

// TestAdminInstanceSettingsRoutes_Gate404 drives the FULL router
// (setupAdminRoutes: optionalAuthMiddleware + instanceAdminMiddleware) for
// every instance settings operation: an anonymous caller and an authenticated
// non-admin (API key) both get 404, and the service and audit mocks carry no
// expectations, so reaching a handler fails the test.
func TestAdminInstanceSettingsRoutes_Gate404(t *testing.T) {
	const nonAdminID = "33333333-3333-4333-8333-333333333333"
	cfg := &config.Config{Auth: config.AuthConfig{InstanceAdmins: config.EnvStringSlice{instanceEmailAdminEmail}}}

	for _, route := range adminInstanceSettingsRoutes {
		for _, caller := range []string{"anonymous", "non-admin"} {
			t.Run(caller+" "+route.method+" "+route.path, func(t *testing.T) {
				authSvc := servicesmocks.NewMockAuthServiceInterface(t)
				keySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
				srv := newAdminTestServer(cfg, &adminMockContainer{
					authService:          authSvc,
					apiKeyService:        keySvc,
					instanceEmailService: servicesmocks.NewMockInstanceEmailProviderServiceInterface(t),
					instanceAuditRepo:    repomocks.NewMockInstanceSettingsAuditRepository(t),
					instanceSearchService: servicesmocks.
						NewMockInstanceSearchSettingsServiceInterface(t),
					instanceAISummaryService: servicesmocks.
						NewMockInstanceAISummarySettingsServiceInterface(t),
				})

				body, err := json.Marshal(smtpUpsertBody(nil))
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

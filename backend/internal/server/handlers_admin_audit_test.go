package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/server/openapispec"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	adminAuditEventsPath = "/api/v1/admin/audit-events"
	adminAuditExportPath = adminAuditEventsPath + "/export"
	auditActingAdmin     = "44444444-4444-4444-8444-444444444444"
)

func auditStrPtr(s string) *string { return &s }

// fullAuditEvent has every optional field set, so the response exercises the
// whole schema.
func fullAuditEvent(id string, at time.Time) models.AuditEvent {
	return models.AuditEvent{
		ID:            id,
		CreatedAt:     at,
		ActorUserID:   auditStrPtr(auditActingAdmin),
		ActorEmail:    "ada@example.com",
		ActorName:     "Ada",
		ActorType:     models.AuditActorUser,
		TeamID:        auditStrPtr(uuid.NewString()),
		TeamName:      auditStrPtr("Platform"),
		ProjectID:     auditStrPtr(uuid.NewString()),
		ProjectName:   auditStrPtr("Core"),
		Operation:     models.AuditOpPromptUpdate,
		ResourceType:  auditStrPtr("prompt"),
		ResourceID:    auditStrPtr(uuid.NewString()),
		ResourceName:  auditStrPtr("Release notes"),
		Source:        models.AuditSourceMCP,
		AssistantName: auditStrPtr("claude-code"),
		APIKeyID:      auditStrPtr(uuid.NewString()),
		APIKeyName:    auditStrPtr("laptop"),
		Outcome:       models.AuditOutcomeSuccess,
		IP:            auditStrPtr("203.0.113.7"),
		UserAgent:     auditStrPtr("claude-code/1.0"),
		RequestID:     auditStrPtr("req-1"),
		Metadata:      json.RawMessage(`{"changed":["content"]}`),
	}
}

// systemAuditEvent has every optional field nil and no metadata.
func systemAuditEvent(id string, at time.Time) models.AuditEvent {
	return models.AuditEvent{
		ID: id, CreatedAt: at, ActorType: models.AuditActorSystem,
		Operation: models.AuditOpProjectMigrated, Source: models.AuditSourceSystem,
		Outcome: models.AuditOutcomeSuccess,
	}
}

func newAuditTestRouter(t *testing.T, svc services.AdminAuditServiceInterface, activity *MockActivityService) http.Handler {
	t.Helper()
	return newAuditTestRouterWithSpy(t, svc, activity, nil)
}

func newAuditTestRouterWithSpy(
	t *testing.T, svc services.AdminAuditServiceInterface, activity *MockActivityService, spy *auditCapture,
) http.Handler {
	t.Helper()
	container := &adminMockContainer{adminAuditService: svc}
	if spy != nil {
		container.auditRecorder = spy
	}
	if activity != nil {
		container.activityService = activity
	}
	return mountAdminStrictRouter(newAdminTestServer(&config.Config{}, container))
}

func serveAudit(t *testing.T, router http.Handler, path string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, auditActingAdmin))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return req, rr
}

// Every filter is mapped onto the repository filter, and the response carries
// every field of the event.
func TestListAdminAuditEvents_MapsEveryFilter(t *testing.T) {
	svc := servicesmocks.NewMockAdminAuditServiceInterface(t)
	actor, team, project := uuid.NewString(), uuid.NewString(), uuid.NewString()
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	want := models.AuditEventFilter{
		ActorUserID:  &actor,
		TeamID:       &team,
		ProjectID:    &project,
		ResourceType: auditStrPtr("prompt"),
		Operations:   []models.AuditOperation{models.AuditOpPromptUpdate, models.AuditOpAuthLogin},
		Sources:      []models.AuditSource{models.AuditSourceMCP, models.AuditSourceCLI},
		Outcomes:     []models.AuditOutcome{models.AuditOutcomeDenied},
		From:         &from,
		To:           &to,
		Query:        auditStrPtr("10%_off"),
		Limit:        25,
	}
	event := fullAuditEvent(uuid.NewString(), from.Add(time.Hour))
	svc.EXPECT().List(mock.Anything, want).Return([]models.AuditEvent{event}, nil, nil)

	q := url.Values{}
	q.Set("limit", "25")
	q.Set("actor_user_id", actor)
	q.Set("team_id", team)
	q.Set("project_id", project)
	q.Set("resource_type", "prompt")
	q.Add("operation", "prompt.update")
	q.Add("operation", "auth.login")
	q.Add("source", "mcp")
	q.Add("source", "cli")
	q.Set("outcome", "denied")
	q.Set("from", from.Format(time.RFC3339))
	q.Set("to", to.Format(time.RFC3339))
	q.Set("q", "10%_off")
	req, rr := serveAudit(t, newAuditTestRouter(t, svc, nil), adminAuditEventsPath+"?"+q.Encode())

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
	var page admingen.AdminAuditEventPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	assert.Nil(t, page.NextCursor)
	got := page.Items[0]
	assert.Equal(t, event.ID, got.Id.String())
	assert.Equal(t, admingen.AdminAuditOperationPromptUpdate, got.Operation)
	assert.Equal(t, admingen.AdminAuditSourceMcp, got.Source)
	assert.Equal(t, "Release notes", *got.ResourceName)
	assert.Equal(t, "claude-code", *got.AssistantName)
	assert.Equal(t, "laptop", *got.ApiKeyName)
	assert.Equal(t, "203.0.113.7", *got.Ip)
	assert.Equal(t, map[string]interface{}{"changed": []interface{}{"content"}}, got.Metadata)
}

// With no params the default page size applies and nothing filters; an empty
// page is `items: []`, never null, and a system event's nullable fields and
// missing metadata serialize as null and {}.
func TestListAdminAuditEvents_DefaultsAndEmptyShapes(t *testing.T) {
	t.Run("empty page", func(t *testing.T) {
		svc := servicesmocks.NewMockAdminAuditServiceInterface(t)
		svc.EXPECT().List(mock.Anything, models.AuditEventFilter{Limit: adminAuditDefaultLimit}).Return(nil, nil, nil)

		req, rr := serveAudit(t, newAuditTestRouter(t, svc, nil), adminAuditEventsPath)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		assert.JSONEq(t, `{"items":[],"next_cursor":null}`, rr.Body.String())
	})
	t.Run("system event", func(t *testing.T) {
		svc := servicesmocks.NewMockAdminAuditServiceInterface(t)
		event := systemAuditEvent(uuid.NewString(), time.Now().UTC())
		svc.EXPECT().List(mock.Anything, mock.Anything).Return([]models.AuditEvent{event}, nil, nil)

		req, rr := serveAudit(t, newAuditTestRouter(t, svc, nil), adminAuditEventsPath)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		specconformance.AssertConformsToSpec(t, req, rr)
		got := decodeJSONMap(t, rr)["items"].([]any)[0].(map[string]any)
		assert.Equal(t, map[string]any{}, got["metadata"])
		for _, k := range []string{"actor_user_id", "team_id", "ip", "user_agent", "request_id", "api_key_id"} {
			assert.Nil(t, got[k], k)
		}
	})
}

// The next_cursor of page 1 fed back as cursor makes page 2 start strictly
// after page 1's last event.
func TestListAdminAuditEvents_CursorRoundTrips(t *testing.T) {
	svc := servicesmocks.NewMockAdminAuditServiceInterface(t)
	last := time.Date(2026, 10, 15, 9, 30, 0, 123456789, time.UTC)
	lastID := uuid.NewString()
	next := &models.AuditCursor{CreatedAt: last, ID: lastID}
	svc.EXPECT().List(mock.Anything, models.AuditEventFilter{Limit: 1}).
		Return([]models.AuditEvent{systemAuditEvent(lastID, last)}, next, nil).Once()
	router := newAuditTestRouter(t, svc, nil)

	req, rr := serveAudit(t, router, adminAuditEventsPath+"?limit=1")
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
	var page admingen.AdminAuditEventPage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &page))
	require.NotNil(t, page.NextCursor)
	assert.NotContains(t, *page.NextCursor, "=", "the cursor is unpadded, so it never needs URL-encoding")

	svc.EXPECT().List(mock.Anything, mock.MatchedBy(func(f models.AuditEventFilter) bool {
		return f.After != nil && f.After.ID == lastID && f.After.CreatedAt.Equal(last) && f.Limit == 1
	})).Return(nil, nil, nil).Once()
	req, rr = serveAudit(t, router, adminAuditEventsPath+"?limit=1&cursor="+*page.NextCursor)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

// Every malformed param is a 400 and never reaches the service (the strict
// mock fails on any call).
func TestListAdminAuditEvents_InvalidParamsAre400(t *testing.T) {
	cursorOf := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	queries := map[string]string{
		"limit zero":             "limit=0",
		"limit over max":         "limit=201",
		"cursor not base64":      "cursor=%21%21",
		"cursor without id":      "cursor=" + cursorOf("2026-10-15T00:00:00Z"),
		"cursor bad time":        "cursor=" + cursorOf("yesterday|"+uuid.NewString()),
		"cursor bad id":          "cursor=" + cursorOf("2026-10-15T00:00:00Z|nope"),
		"unknown operation":      "operation=prompt.explode",
		"unknown source":         "source=fax",
		"unknown outcome":        "outcome=maybe",
		"malformed actor id":     "actor_user_id=not-a-uuid",
		"malformed instant":      "from=yesterday",
		"from equals to":         "from=2026-10-01T00:00:00Z&to=2026-10-01T00:00:00Z",
		"from after to":          "from=2026-10-02T00:00:00Z&to=2026-10-01T00:00:00Z",
		"q over 200 characters":  "q=" + strings.Repeat("a", adminAuditMaxQueryLength+1),
		"resource_type too long": "resource_type=" + strings.Repeat("a", adminAuditMaxResourceTypeLength+1),
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			svc := servicesmocks.NewMockAdminAuditServiceInterface(t)

			req, rr := serveAudit(t, newAuditTestRouter(t, svc, nil), adminAuditEventsPath+"?"+query)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// q at the limit, counted in characters, is accepted; a blank q filters
// nothing.
func TestListAdminAuditEvents_QueryBounds(t *testing.T) {
	svc := servicesmocks.NewMockAdminAuditServiceInterface(t)
	atCap := strings.Repeat("é", adminAuditMaxQueryLength)
	svc.EXPECT().List(mock.Anything, models.AuditEventFilter{Limit: adminAuditDefaultLimit, Query: &atCap}).
		Return(nil, nil, nil).Once()
	svc.EXPECT().List(mock.Anything, models.AuditEventFilter{Limit: adminAuditDefaultLimit}).
		Return(nil, nil, nil).Once()
	router := newAuditTestRouter(t, svc, nil)

	_, rr := serveAudit(t, router, adminAuditEventsPath+"?q="+url.QueryEscape(atCap))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	_, rr = serveAudit(t, router, adminAuditEventsPath+"?q=%20%20")
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

// Service failures, and an event the response cannot represent, are 500s that
// never echo the cause.
func TestListAdminAuditEvents_FailuresAre500(t *testing.T) {
	cases := map[string]func(*servicesmocks.MockAdminAuditServiceInterface){
		"service error": func(svc *servicesmocks.MockAdminAuditServiceInterface) {
			svc.EXPECT().List(mock.Anything, mock.Anything).Return(nil, nil, errors.New("pq: connection reset"))
		},
		"bad stored id": func(svc *servicesmocks.MockAdminAuditServiceInterface) {
			svc.EXPECT().List(mock.Anything, mock.Anything).
				Return([]models.AuditEvent{systemAuditEvent("connection reset", time.Now())}, nil, nil)
		},
		"bad stored metadata": func(svc *servicesmocks.MockAdminAuditServiceInterface) {
			e := systemAuditEvent(uuid.NewString(), time.Now())
			e.Metadata = json.RawMessage(`[1,2]`)
			svc.EXPECT().List(mock.Anything, mock.Anything).Return([]models.AuditEvent{e}, nil, nil)
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			svc := servicesmocks.NewMockAdminAuditServiceInterface(t)
			arrange(svc)

			req, rr := serveAudit(t, newAuditTestRouter(t, svc, nil), adminAuditEventsPath)

			require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "connection reset")
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

// A stored JSON null metadata reads back as {}.
func TestToGenAdminAuditEvent_NullMetadataIsEmptyObject(t *testing.T) {
	e := systemAuditEvent(uuid.NewString(), time.Now())
	e.Metadata = json.RawMessage(`null`)

	got, err := toGenAdminAuditEvent(&e)

	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{}, got.Metadata)
}

// The export binds the same filter as the list, streams the CSV with the
// export headers and records the admin_audit_exported activity once the CSV
// went out.
func TestExportAdminAuditEvents_StreamsWithTheListFilter(t *testing.T) {
	svc := servicesmocks.NewMockAdminAuditServiceInterface(t)
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	want := models.AuditEventFilter{
		Operations: []models.AuditOperation{models.AuditOpAuditExport},
		Outcomes:   []models.AuditOutcome{models.AuditOutcomeDenied},
		From:       &from,
		Query:      auditStrPtr("ada"),
		Limit:      adminAuditDefaultLimit,
	}
	body := "created_at,operation\r\n2026-10-01T00:00:00Z,audit.export\r\n"
	svc.EXPECT().Export(mock.Anything, want).Return(fakeAdminExport(9, false, body, 1, nil), nil)
	activity := &MockActivityService{}
	activity.On("RecordResourceActivity",
		mock.Anything, auditActingAdmin, activities.ActivityTypeAdminAuditExported, activities.EntityTypeSystem,
		(*string)(nil), mock.Anything,
		mock.MatchedBy(func(md map[string]interface{}) bool {
			filters, ok := md["filters"].(map[string]interface{})
			return ok && md["list"] == "audit" && md["row_count"] == 1 && md["total_count"] == 9 &&
				md["truncated"] == false && filters["q"] == "ada" && filters["outcome"] != nil
		})).Return(nil).Once()

	spy := &auditCapture{}
	req, rr := serveAudit(t, newAuditTestRouterWithSpy(t, svc, activity, spy),
		adminAuditExportPath+"?operation=audit.export&outcome=denied&from=2026-10-01T00:00:00Z&q=ada")

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, "text/csv", rr.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="admin-audit-`+time.Now().UTC().Format("20060102")+`.csv"`,
		rr.Header().Get("Content-Disposition"))
	assert.Equal(t, "9", rr.Header().Get("X-Export-Total-Count"))
	assert.Equal(t, "false", rr.Header().Get("X-Export-Truncated"))
	assert.Equal(t, body, rr.Body.String())
	activity.AssertExpectations(t)
	assertOneExport(t, spy, "audit", 1, 9, false)
	assert.Equal(t, "ada", spy.only(t).event.Metadata["filters"].(map[string]interface{})["q"])
	specconformance.AssertConformsToSpec(t, req, rr)
}

// An invalid filter is a 400 before anything streams, and records nothing.
func TestExportAdminAuditEvents_InvalidParamsAre400(t *testing.T) {
	for _, query := range []string{"operation=nope", "from=2026-10-02T00:00:00Z&to=2026-10-01T00:00:00Z",
		"q=" + strings.Repeat("a", adminAuditMaxQueryLength+1)} {
		t.Run(query, func(t *testing.T) {
			svc := servicesmocks.NewMockAdminAuditServiceInterface(t)
			activity := &MockActivityService{}

			req, rr := serveAudit(t, newAuditTestRouter(t, svc, activity), adminAuditExportPath+"?"+query)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))
			activity.AssertNotCalled(t, "RecordResourceActivity")
			specconformance.AssertConformsToSpec(t, req, rr)
		})
	}
}

func TestExportAdminAuditEvents_PrepareFailureIs500(t *testing.T) {
	svc := servicesmocks.NewMockAdminAuditServiceInterface(t)
	svc.EXPECT().Export(mock.Anything, mock.Anything).Return(services.AdminExport{}, errors.New("count failed"))

	req, rr := serveAudit(t, newAuditTestRouter(t, svc, &MockActivityService{}), adminAuditExportPath)

	require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
	specconformance.AssertConformsToSpec(t, req, rr)
}

// The full router hides both operations from anonymous callers and
// authenticated non-admins, and never reaches the service.
func TestAdminAuditRoutes_Gate404(t *testing.T) {
	const nonAdminID = "33333333-3333-4333-8333-333333333333"
	cfg := &config.Config{Auth: config.AuthConfig{InstanceAdmins: config.EnvStringSlice{instanceEmailAdminEmail}}}
	for _, path := range []string{adminAuditEventsPath, adminAuditExportPath} {
		for _, caller := range []string{"anonymous", "non-admin"} {
			t.Run(caller+" "+path, func(t *testing.T) {
				authSvc := servicesmocks.NewMockAuthServiceInterface(t)
				keySvc := servicesmocks.NewMockAPIKeyServiceInterface(t)
				srv := newAdminTestServer(cfg, &adminMockContainer{
					authService:       authSvc,
					apiKeyService:     keySvc,
					adminAuditService: servicesmocks.NewMockAdminAuditServiceInterface(t),
				})
				req := httptest.NewRequest(http.MethodGet, path, bytes.NewReader(nil))
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

// The export params copy onto the list params field by field.
func TestAdminExportParamsToList_AuditCopiesEveryField(t *testing.T) {
	var e admingen.ExportAdminAuditEventsParams
	setEveryField(t, reflect.ValueOf(&e).Elem())
	l, err := adminExportParamsToList[admingen.ExportAdminAuditEventsParams, admingen.ListAdminAuditEventsParams](e)
	require.NoError(t, err)
	assertEveryListFieldSet(t, reflect.ValueOf(l))
}

// The spec's operation enum is exactly the server's vocabulary, in both
// directions: a Go operation the API cannot filter by, or a published value no
// event can carry, fails here.
func TestAdminAuditOperationEnumMatchesModel(t *testing.T) {
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Enum []string `json:"enum"`
			} `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(openapispec.JSON, &spec))

	published := spec.Components.Schemas["AdminAuditOperation"].Enum
	model := make([]string, 0, len(models.AllAuditOperations()))
	for _, op := range models.AllAuditOperations() {
		model = append(model, string(op))
		assert.True(t, admingen.AdminAuditOperation(op).Valid(), op)
	}
	assert.ElementsMatch(t, model, published)

	for name, pair := range map[string][2][]string{
		"AdminAuditSource": {spec.Components.Schemas["AdminAuditSource"].Enum,
			{"web", "cli", "mcp", "api", "system"}},
		"AdminAuditOutcome": {spec.Components.Schemas["AdminAuditOutcome"].Enum,
			{"success", "denied", "failure"}},
		"AdminAuditActorType": {spec.Components.Schemas["AdminAuditActorType"].Enum,
			{"user", "system", "anonymous"}},
	} {
		assert.ElementsMatch(t, pair[1], pair[0], name)
	}
	for _, s := range []models.AuditSource{models.AuditSourceWeb, models.AuditSourceCLI, models.AuditSourceMCP,
		models.AuditSourceAPI, models.AuditSourceSystem} {
		assert.True(t, admingen.AdminAuditSource(s).Valid(), s)
	}
	for _, o := range []models.AuditOutcome{models.AuditOutcomeSuccess, models.AuditOutcomeDenied,
		models.AuditOutcomeFailure} {
		assert.True(t, admingen.AdminAuditOutcome(o).Valid(), o)
	}
	for _, a := range []models.AuditActorType{models.AuditActorUser, models.AuditActorSystem,
		models.AuditActorAnonymous} {
		assert.True(t, admingen.AdminAuditActorType(a).Valid(), a)
	}
}

// A stream that ends in a reader error records no activity: only a complete
// download counts.
func TestExportAdminAuditEvents_MidStreamFailureRecordsNothing(t *testing.T) {
	svc := servicesmocks.NewMockAdminAuditServiceInterface(t)
	svc.EXPECT().Export(mock.Anything, mock.Anything).
		Return(fakeAdminExport(3, false, "created_at\r\n", 1, errors.New("list failed")), nil)
	activity := &MockActivityService{}
	spy := &auditCapture{}
	srv := newAdminTestServer(&config.Config{}, &adminMockContainer{
		adminAuditService: svc, activityService: activity, auditRecorder: spy,
	})

	resp, err := (&adminStrictServer{s: srv}).ExportAdminAuditEvents(context.Background(),
		admingen.ExportAdminAuditEventsRequestObject{})
	require.NoError(t, err)
	body := resp.(admingen.ExportAdminAuditEvents200TextcsvResponse).Body
	_, err = io.ReadAll(body)

	require.Error(t, err)
	activity.AssertNotCalled(t, "RecordResourceActivity")
	assert.Empty(t, spy.events(), "an aborted export is not audited")
}

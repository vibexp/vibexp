package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// spyResourceAccessService captures events passed to RecordAccess so tests can
// assert what (if anything) was recorded. The unused interface methods satisfy
// the ResourceAccessService contract but are never exercised here.
type spyResourceAccessService struct {
	mu     sync.Mutex
	events []*models.ResourceAccessEvent
}

func (s *spyResourceAccessService) RecordAccess(event *models.ResourceAccessEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *spyResourceAccessService) GetMetrics(
	_ context.Context, _, _, _ string, _ int,
) (*resourceaccess.MetricsResult, error) {
	return nil, nil
}

func (s *spyResourceAccessService) GetTeamMetrics(
	_ context.Context, _ string, _ int,
) (*resourceaccess.MetricsResult, error) {
	return nil, nil
}

func (s *spyResourceAccessService) GetTopAccessedResources(
	_ context.Context, _ string, _ int, _ string, _ int,
) ([]models.TopAccessedResource, error) {
	return nil, nil
}

func (s *spyResourceAccessService) RunRetentionJob(_ context.Context) error { return nil }

func (s *spyResourceAccessService) calls() []*models.ResourceAccessEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.events
}

// resourceAccessTestContainer injects a spy ResourceAccessService while
// defaulting everything else to nil via BaseMockContainer.
type resourceAccessTestContainer struct {
	BaseMockContainer
	svc resourceaccess.ResourceAccessService
}

func (c *resourceAccessTestContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return c.svc
}

const (
	recordTestTeamID = "550e8400-e29b-41d4-a716-446655440000"
	recordTestUserID = "user-1"
)

// mountRecordMiddleware mounts the recording middleware on a chi router with a
// {team_id} URL param so chi.URLParam resolves, routing all requests to handler.
func mountRecordMiddleware(srv *Server, resourceType string, handler http.HandlerFunc) http.Handler {
	r := chi.NewRouter()
	r.With(srv.recordResourceAccess(resourceType)).Get("/api/v1/{team_id}/things/{id}", handler)
	return r
}

// requestWithAuth builds a GET request carrying the auth-context values the auth
// middleware would have set for recordTestUserID.
func requestWithAuth(authType, apiKeyID, userAgent string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/"+recordTestTeamID+"/things/abc", nil)
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	ctx := context.WithValue(req.Context(), contextkeys.UserID, recordTestUserID)
	if authType != "" {
		ctx = context.WithValue(ctx, contextkeys.AuthType, authType)
	}
	if apiKeyID != "" {
		ctx = context.WithValue(ctx, contextkeys.APIKeyID, apiKeyID)
	}
	return req.WithContext(ctx)
}

func TestRecordResourceAccess_RecordsOn200(t *testing.T) {
	spy := &spyResourceAccessService{}
	srv := newServerWithNullLogger(t)
	srv.container = &resourceAccessTestContainer{svc: spy}

	handler := func(w http.ResponseWriter, r *http.Request) {
		contextkeys.SetAccessedResourceID(r.Context(), "resolved-uuid")
		w.WriteHeader(http.StatusOK)
	}

	req := requestWithAuth("cookie", "", "Mozilla/5.0")
	rec := httptest.NewRecorder()
	mountRecordMiddleware(srv, resourceTypePrompt, handler).ServeHTTP(rec, req)

	calls := spy.calls()
	require.Len(t, calls, 1)
	event := calls[0]
	assert.Equal(t, recordTestTeamID, event.TeamID)
	assert.Equal(t, resourceTypePrompt, event.ResourceType)
	assert.Equal(t, "resolved-uuid", event.ResourceID)
	assert.Equal(t, resourceaccess.SourceWeb, event.Source)
	require.NotNil(t, event.UserID)
	assert.Equal(t, recordTestUserID, *event.UserID)
}

func TestRecordResourceAccess_SkipsOn404(t *testing.T) {
	spy := &spyResourceAccessService{}
	srv := newServerWithNullLogger(t)
	srv.container = &resourceAccessTestContainer{svc: spy}

	handler := func(w http.ResponseWriter, r *http.Request) {
		// The handler sets an id but still fails: no event must be recorded.
		contextkeys.SetAccessedResourceID(r.Context(), "resolved-uuid")
		w.WriteHeader(http.StatusNotFound)
	}

	req := requestWithAuth("cookie", "", "Mozilla/5.0")
	rec := httptest.NewRecorder()
	mountRecordMiddleware(srv, resourceTypeMemory, handler).ServeHTTP(rec, req)

	assert.Empty(t, spy.calls())
}

func TestRecordResourceAccess_SkipsWhenNoResourceID(t *testing.T) {
	spy := &spyResourceAccessService{}
	srv := newServerWithNullLogger(t)
	srv.container = &resourceAccessTestContainer{svc: spy}

	handler := func(w http.ResponseWriter, _ *http.Request) {
		// 200 but the handler never resolved an id (e.g. slug-keyed early return).
		w.WriteHeader(http.StatusOK)
	}

	req := requestWithAuth("cookie", "", "Mozilla/5.0")
	rec := httptest.NewRecorder()
	mountRecordMiddleware(srv, resourceTypeProject, handler).ServeHTTP(rec, req)

	assert.Empty(t, spy.calls())
}

func TestRecordResourceAccess_InvalidSourceIPIsDropped(t *testing.T) {
	spy := &spyResourceAccessService{}
	srv := newServerWithNullLogger(t)
	srv.container = &resourceAccessTestContainer{svc: spy}

	handler := func(w http.ResponseWriter, r *http.Request) {
		contextkeys.SetAccessedResourceID(r.Context(), "resolved-uuid")
		w.WriteHeader(http.StatusOK)
	}

	// A non-IP value must not reach the INET column; the event is still
	// recorded, just without a source_ip. clientIPMiddleware cannot produce
	// one, so inject it directly to exercise the guard.
	req := requestWithAuth("cookie", "", "Mozilla/5.0")
	req = req.WithContext(context.WithValue(req.Context(), contextkeys.ClientIP, "not-an-ip"))
	rec := httptest.NewRecorder()
	mountRecordMiddleware(srv, resourceTypePrompt, handler).ServeHTTP(rec, req)

	calls := spy.calls()
	require.Len(t, calls, 1)
	assert.Nil(t, calls[0].SourceIP)
}

// TestRecordResourceAccess_SourceIPIsTheResolvedClientIP verifies access events
// key off the same resolved IP as the rate limiter (#465), rather than
// re-reading X-Forwarded-For themselves.
func TestRecordResourceAccess_SourceIPIsTheResolvedClientIP(t *testing.T) {
	spy := &spyResourceAccessService{}
	srv := newServerWithNullLogger(t)
	srv.container = &resourceAccessTestContainer{svc: spy}

	handler := func(w http.ResponseWriter, r *http.Request) {
		contextkeys.SetAccessedResourceID(r.Context(), "resolved-uuid")
		w.WriteHeader(http.StatusOK)
	}

	req := requestWithAuth("cookie", "", "Mozilla/5.0")
	req = req.WithContext(context.WithValue(req.Context(), contextkeys.ClientIP, "203.0.113.7"))
	rec := httptest.NewRecorder()
	mountRecordMiddleware(srv, resourceTypePrompt, handler).ServeHTTP(rec, req)

	calls := spy.calls()
	require.Len(t, calls, 1)
	require.NotNil(t, calls[0].SourceIP)
	assert.Equal(t, "203.0.113.7", *calls[0].SourceIP)
}

// TestRecordResourceAccess_ForgedForwardedForIsNotRecorded is the audit-trail
// half of #465: a spoofed header used to land in the access log verbatim, so
// incident response read attacker-chosen addresses. With no trusted proxies the
// peer address is recorded instead.
func TestRecordResourceAccess_ForgedForwardedForIsNotRecorded(t *testing.T) {
	spy := &spyResourceAccessService{}
	srv := newServerWithNullLogger(t)
	srv.container = &resourceAccessTestContainer{svc: spy}

	handler := func(w http.ResponseWriter, r *http.Request) {
		contextkeys.SetAccessedResourceID(r.Context(), "resolved-uuid")
		w.WriteHeader(http.StatusOK)
	}

	req := requestWithAuth("cookie", "", "Mozilla/5.0")
	req.RemoteAddr = "198.51.100.20:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.7") // forged
	rec := httptest.NewRecorder()

	// Full production chain: no trusted proxies configured.
	clientIPMiddleware(nil)(
		mountRecordMiddleware(srv, resourceTypePrompt, handler),
	).ServeHTTP(rec, req)

	calls := spy.calls()
	require.Len(t, calls, 1)
	require.NotNil(t, calls[0].SourceIP)
	assert.Equal(t, "198.51.100.20", *calls[0].SourceIP,
		"the audit trail must record the peer, not the forged header")
}

func TestRecordResourceAccess_NilServiceIsSafe(t *testing.T) {
	srv := newServerWithNullLogger(t)
	srv.container = &resourceAccessTestContainer{svc: nil}

	handler := func(w http.ResponseWriter, r *http.Request) {
		contextkeys.SetAccessedResourceID(r.Context(), "resolved-uuid")
		w.WriteHeader(http.StatusOK)
	}

	req := requestWithAuth("cookie", "", "Mozilla/5.0")
	rec := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		mountRecordMiddleware(srv, resourceTypeAgent, handler).ServeHTTP(rec, req)
	})
}

func TestRecordResourceAccess_SourceDerivation(t *testing.T) {
	tests := []struct {
		name       string
		authType   string
		userAgent  string
		apiKeyID   string
		wantSource string
		wantAPIKey bool
	}{
		{
			name:       "cookie auth derives web",
			authType:   "cookie",
			userAgent:  "Mozilla/5.0",
			wantSource: resourceaccess.SourceWeb,
		},
		{
			name:       "api_key with CLI user agent derives cli",
			authType:   "api_key",
			userAgent:  "VibeXP-CLI/1.2.3",
			apiKeyID:   "key-123",
			wantSource: resourceaccess.SourceCLI,
			wantAPIKey: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyResourceAccessService{}
			srv := newServerWithNullLogger(t)
			srv.container = &resourceAccessTestContainer{svc: spy}

			handler := func(w http.ResponseWriter, r *http.Request) {
				contextkeys.SetAccessedResourceID(r.Context(), "resolved-uuid")
				w.WriteHeader(http.StatusOK)
			}

			req := requestWithAuth(tc.authType, tc.apiKeyID, tc.userAgent)
			rec := httptest.NewRecorder()
			mountRecordMiddleware(srv, resourceTypeBlueprint, handler).ServeHTTP(rec, req)

			calls := spy.calls()
			require.Len(t, calls, 1)
			assert.Equal(t, tc.wantSource, calls[0].Source)
			if tc.wantAPIKey {
				require.NotNil(t, calls[0].APIKeyID)
				assert.Equal(t, tc.apiKeyID, *calls[0].APIKeyID)
			}
		})
	}
}

func TestRecordMCPResourceAccess(t *testing.T) {
	spy := &spyResourceAccessService{}
	srv := newServerWithNullLogger(t)
	srv.container = &resourceAccessTestContainer{svc: spy}

	ctx := context.WithValue(context.Background(), contextkeys.APIKeyID, "key-xyz")
	srv.recordMCPResourceAccess(ctx, recordTestTeamID, "user-9", resourceTypeArtifact, "artifact-uuid")

	calls := spy.calls()
	require.Len(t, calls, 1)
	event := calls[0]
	assert.Equal(t, resourceaccess.SourceMCP, event.Source)
	assert.Equal(t, recordTestTeamID, event.TeamID)
	assert.Equal(t, resourceTypeArtifact, event.ResourceType)
	assert.Equal(t, "artifact-uuid", event.ResourceID)
	require.NotNil(t, event.UserID)
	assert.Equal(t, "user-9", *event.UserID)
	require.NotNil(t, event.APIKeyID)
	assert.Equal(t, "key-xyz", *event.APIKeyID)
}

func TestRecordMCPResourceAccess_NilServiceIsSafe(t *testing.T) {
	srv := newServerWithNullLogger(t)
	srv.container = &resourceAccessTestContainer{svc: nil}

	assert.NotPanics(t, func() {
		srv.recordMCPResourceAccess(context.Background(), recordTestTeamID, "u", resourceTypeMemory, "id")
	})
}

package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	relationsgen "github.com/vibexp/vibexp/internal/server/gen/relations"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/audit"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// stubSeedService is a RelationSeedServiceInterface double whose Backfill runs a
// scripted callback (used to synchronize with the fire-and-forget goroutine).
type stubSeedService struct {
	backfill func()
	// backfillCtx, when set, receives the context the run was started with.
	backfillCtx func(ctx context.Context)
}

func (s stubSeedService) Backfill(ctx context.Context, _, _ string) (models.RelationSeedSummary, error) {
	if s.backfill != nil {
		s.backfill()
	}
	if s.backfillCtx != nil {
		s.backfillCtx(ctx)
	}
	return models.RelationSeedSummary{}, nil
}

// stubAuthz is an AuthorizationServiceInterface double: Can returns canErr; any
// other method panics via the embedded nil interface (the seed handler only
// calls Can).
type stubAuthz struct {
	services.AuthorizationServiceInterface
	canErr error
}

func (s stubAuthz) Can(_ context.Context, _, _ string, _ authz.Permission) error {
	return s.canErr
}

func createSeedRelationsServer(seed services.RelationSeedServiceInterface, az services.AuthorizationServiceInterface) *Server {
	logger := slog.New(slog.DiscardHandler)
	r := chi.NewRouter()
	srv := &Server{
		container: &MockRelationsContainer{relationSeedService: seed, authzService: az},
		logger:    logger,
		config:    &config.Config{},
		router:    r,
	}
	strict := relationsgen.NewStrictHandlerWithOptions(
		&relationsStrictServer{s: srv},
		nil,
		relationsgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  srv.relationsBindErrorHandler,
			ResponseErrorHandlerFunc: srv.relationsResponseErrorHandler,
		},
	)
	relationsgen.HandlerWithOptions(strict, relationsgen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: srv.relationsBindErrorHandler,
	})
	return srv
}

func TestSeedRelations_Accepted(t *testing.T) {
	done := make(chan struct{})
	seed := stubSeedService{backfill: func() { close(done) }}
	srv := createSeedRelationsServer(seed, stubAuthz{canErr: nil})

	req := makeRelationsRequest("POST", "/api/v1/"+testRelationsTeamID+"/relations/seed", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	require.Equal(t, http.StatusAccepted, w.Code)
	// The trigger is fire-and-forget; wait for the background run to prove it fired.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("seed backfill goroutine did not run")
	}
}

func TestSeedRelations_Forbidden(t *testing.T) {
	seed := stubSeedService{backfill: func() { t.Error("backfill must not run when authz denies") }}
	srv := createSeedRelationsServer(seed, stubAuthz{canErr: fmt.Errorf("%w", services.ErrPermissionDenied)})

	req := makeRelationsRequest("POST", "/api/v1/"+testRelationsTeamID+"/relations/seed", "")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assertCommentsProblem(t, w, http.StatusForbidden, "FORBIDDEN")
	// Give any (erroneous) goroutine a chance to run and fail the test.
	time.Sleep(50 * time.Millisecond)
}

// A second trigger while a run is in flight coalesces via the per-team guard —
// only one backfill runs.
func TestEnqueueTeamRelationSeed_Coalesces(t *testing.T) {
	var calls int32
	started := make(chan struct{})
	release := make(chan struct{})
	seed := stubSeedService{backfill: func() {
		atomic.AddInt32(&calls, 1)
		close(started) // first (only) run has started and holds the guard
		<-release      // block so the guard stays held while the second trigger fires
	}}
	srv := createSeedRelationsServer(seed, stubAuthz{})

	srv.enqueueTeamRelationSeed(context.Background(), "team-x", "user-1")
	<-started // the first run is in flight

	srv.enqueueTeamRelationSeed(context.Background(), "team-x", "user-1") // coalesced (guard held)
	close(release)
	time.Sleep(50 * time.Millisecond) // let the first run finish + release the guard

	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

// The backfill outlives the request but keeps its attribution (#1548): an
// audit event recorded inside the run, after the request has ended, carries
// the trigger's channel, client IP, user agent and request id. The router
// runs the real request-id, client-IP and user-agent middleware, and the
// event goes through the real recorder.
func TestSeedRelations_BackfillKeepsRequestAttribution(t *testing.T) {
	var (
		mu   sync.Mutex
		rows []models.AuditEvent
	)
	repo := repomocks.NewMockAuditEventRepository(t)
	repo.EXPECT().Insert(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, e *models.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			rows = append(rows, *e)
			return nil
		})
	users := repomocks.NewMockUserRepository(t)
	users.EXPECT().GetByID(mock.Anything, testRelationsUserID).
		Return(&models.User{ID: testRelationsUserID, Email: "u@example.com", Name: "U"}, nil).Maybe()
	recorder := audit.NewSyncRecorder(audit.RecorderDeps{
		Repo: repo, Users: users, Logger: slog.New(slog.DiscardHandler),
	})

	requestDone := make(chan struct{})
	ran := make(chan error, 1)
	seed := stubSeedService{backfillCtx: func(ctx context.Context) {
		<-requestDone // the request is over (and cancelled) before the run records anything
		userID := testRelationsUserID
		recorder.Record(ctx, audit.Event{
			Operation: models.AuditOpAuthzDenied, Outcome: models.AuditOutcomeDenied,
			Actor: &audit.Actor{UserID: &userID}, TeamID: testRelationsTeamID, TeamName: "Relations Team",
		})
		ran <- ctx.Err()
	}}
	srv := createSeedRelationsServer(seed, stubAuthz{})
	handler := RequestIDMiddleware(srv.logger)(clientIPMiddleware(nil)(userAgentMiddleware(srv)))

	req := makeRelationsRequest("POST", "/api/v1/"+testRelationsTeamID+"/relations/seed", "")
	req.RemoteAddr = "203.0.113.9:4242"
	req.Header.Set("User-Agent", "Mozilla/5.0 (seed test)")
	req.Header.Set("X-Request-ID", "req-seed-1548")
	reqCtx, cancel := context.WithCancel(context.WithValue(req.Context(), contextkeys.AuthType, "cookie"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req.WithContext(reqCtx))
	require.Equal(t, http.StatusAccepted, w.Code)
	cancel()
	close(requestDone)

	select {
	case err := <-ran:
		require.NoError(t, err, "the run is not cancelled with the request")
	case <-time.After(2 * time.Second):
		t.Fatal("seed backfill goroutine did not run")
	}
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, models.AuditSourceWeb, row.Source)
	require.NotNil(t, row.RequestID)
	require.Equal(t, "req-seed-1548", *row.RequestID)
	require.NotNil(t, row.IP)
	require.Equal(t, "203.0.113.9", *row.IP)
	require.NotNil(t, row.UserAgent)
	require.Equal(t, "Mozilla/5.0 (seed test)", *row.UserAgent)
}

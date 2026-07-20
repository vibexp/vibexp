package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	relationsgen "github.com/vibexp/vibexp/internal/server/gen/relations"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// stubSeedService is a RelationSeedServiceInterface double whose Backfill runs a
// scripted callback (used to synchronize with the fire-and-forget goroutine).
type stubSeedService struct {
	backfill func()
}

func (s stubSeedService) Backfill(_ context.Context, _, _ string) (models.RelationSeedSummary, error) {
	if s.backfill != nil {
		s.backfill()
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

	srv.enqueueTeamRelationSeed("team-x", "user-1")
	<-started // the first run is in flight

	srv.enqueueTeamRelationSeed("team-x", "user-1") // coalesced (guard held)
	close(release)
	time.Sleep(50 * time.Millisecond) // let the first run finish + release the guard

	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

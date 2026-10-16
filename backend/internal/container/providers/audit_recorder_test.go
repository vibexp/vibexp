package providers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/audit"
	"github.com/vibexp/vibexp/pkg/events"
)

// TestProvideAuditRecorder_IsAsynchronous pins that the server's recorder never
// writes on the caller's goroutine: Record returns while the insert is still
// blocked. audit.NewSyncRecorder (the recovery CLI's, #1535) must never be
// what this provider returns, since Record runs on request paths.
func TestProvideAuditRecorder_IsAsynchronous(t *testing.T) {
	pool := events.NewWorkerPool(1)
	release := make(chan struct{})
	t.Cleanup(pool.Stop)
	t.Cleanup(func() { close(release) }) // runs first: unblocks the insert before Stop

	inserting := make(chan struct{})
	repo := repomocks.NewMockAuditEventRepository(t)
	repo.EXPECT().Insert(mock.Anything, mock.Anything).
		RunAndReturn(func(context.Context, *models.AuditEvent) error {
			close(inserting)
			<-release
			return nil
		}).Maybe()

	recorder := ProvideAuditRecorder(audit.RecorderDeps{Repo: repo, Pool: pool})

	returned := make(chan struct{})
	go func() {
		recorder.Record(context.Background(), audit.Event{Operation: models.AuditOpAuditExport})
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Record blocked on the insert: the server's recorder must be asynchronous")
	}
	select {
	case <-inserting:
	case <-time.After(5 * time.Second):
		t.Fatal("the event was never persisted")
	}
	require.NotNil(t, recorder)
}

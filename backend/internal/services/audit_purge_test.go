package services_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
)

var auditPurgeNow = time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)

type auditPurgeHarness struct {
	svc      *services.AuditPurgeService
	settings *repomocks.MockInstanceAuditSettingsRepository
	events   *repomocks.MockAuditEventRepository
	released int
}

func newAuditPurgeHarness(t *testing.T, logs *bytes.Buffer) *auditPurgeHarness {
	t.Helper()
	h := &auditPurgeHarness{
		settings: repomocks.NewMockInstanceAuditSettingsRepository(t),
		events:   repomocks.NewMockAuditEventRepository(t),
	}
	h.svc = services.NewAuditPurgeService(h.settings, h.events, testLogger(logs))
	h.svc.SetClockForTest(func() time.Time { return auditPurgeNow })
	return h
}

// lockAcquired makes TryPurgeLock succeed and counts the releases; release
// returns releaseErr.
func (h *auditPurgeHarness) lockAcquired(releaseErr error) {
	h.settings.EXPECT().TryPurgeLock(mock.Anything).Return(true, func() error {
		h.released++
		return releaseErr
	}, nil).Once()
}

// The cutoff is now minus the stored retention, in whole weeks, and the purge
// deletes in batches of AuditPurgeBatchSize.
func TestAuditPurgeService_DeletesOlderThanTheStoredRetention(t *testing.T) {
	h := newAuditPurgeHarness(t, nil)
	h.lockAcquired(nil)
	h.settings.EXPECT().Get(mock.Anything).
		Return(&models.InstanceAuditSettings{RetentionWeeks: 6, CaptureIPUA: true}, nil)
	wantCutoff := auditPurgeNow.Add(-6 * 7 * 24 * time.Hour)
	h.events.EXPECT().PurgeOlderThan(mock.Anything, wantCutoff, services.AuditPurgeBatchSize).Return(2500, nil)

	deleted, skipped, err := h.svc.Purge(context.Background())

	require.NoError(t, err)
	assert.False(t, skipped)
	assert.Equal(t, int64(2500), deleted)
	assert.Equal(t, 1, h.released, "the lock is released exactly once")
}

func TestAuditPurgeService_NoStoredSettingsUsesTheDefaultRetention(t *testing.T) {
	h := newAuditPurgeHarness(t, nil)
	h.lockAcquired(nil)
	h.settings.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuditSettingsNotFound)
	wantCutoff := auditPurgeNow.Add(-models.DefaultAuditRetentionWeeks * 7 * 24 * time.Hour)
	h.events.EXPECT().PurgeOlderThan(mock.Anything, wantCutoff, services.AuditPurgeBatchSize).Return(0, nil)

	deleted, skipped, err := h.svc.Purge(context.Background())

	require.NoError(t, err)
	assert.False(t, skipped)
	assert.Zero(t, deleted)
}

// Another replica holds the lock: skip, read nothing, delete nothing (the
// strict mocks fail on any other call).
func TestAuditPurgeService_SkipsWhenTheLockIsHeld(t *testing.T) {
	h := newAuditPurgeHarness(t, nil)
	h.settings.EXPECT().TryPurgeLock(mock.Anything).Return(false, func() error { return nil }, nil)

	deleted, skipped, err := h.svc.Purge(context.Background())

	require.NoError(t, err)
	assert.True(t, skipped)
	assert.Zero(t, deleted)
}

func TestAuditPurgeService_LockErrorIsReturned(t *testing.T) {
	h := newAuditPurgeHarness(t, nil)
	boom := errors.New("pool exhausted")
	h.settings.EXPECT().TryPurgeLock(mock.Anything).Return(false, nil, boom)

	_, skipped, err := h.svc.Purge(context.Background())

	assert.ErrorIs(t, err, boom)
	assert.False(t, skipped)
}

// An unreadable retention deletes nothing: guessing the default could destroy
// a year of events an admin chose to keep.
func TestAuditPurgeService_SettingsReadFailureDeletesNothing(t *testing.T) {
	h := newAuditPurgeHarness(t, nil)
	h.lockAcquired(nil)
	boom := errors.New("db down")
	h.settings.EXPECT().Get(mock.Anything).Return(nil, boom)

	deleted, _, err := h.svc.Purge(context.Background())

	assert.ErrorIs(t, err, boom)
	assert.Zero(t, deleted)
	assert.Equal(t, 1, h.released, "the lock is released on the error path too")
}

// A purge that fails part-way reports what its completed batches deleted.
func TestAuditPurgeService_PurgeErrorReportsPartialProgress(t *testing.T) {
	h := newAuditPurgeHarness(t, nil)
	h.lockAcquired(nil)
	h.settings.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuditSettingsNotFound)
	boom := errors.New("statement timeout")
	h.events.EXPECT().PurgeOlderThan(mock.Anything, mock.Anything, mock.Anything).Return(1000, boom)

	deleted, _, err := h.svc.Purge(context.Background())

	assert.ErrorIs(t, err, boom)
	assert.Equal(t, int64(1000), deleted)
	assert.Equal(t, 1, h.released)
}

// A failed release is logged, never turned into a purge failure.
func TestAuditPurgeService_ReleaseFailureIsLogged(t *testing.T) {
	var logs bytes.Buffer
	h := newAuditPurgeHarness(t, &logs)
	h.lockAcquired(errors.New("conn closed"))
	h.settings.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuditSettingsNotFound)
	h.events.EXPECT().PurgeOlderThan(mock.Anything, mock.Anything, mock.Anything).Return(3, nil)

	deleted, _, err := h.svc.Purge(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(3), deleted)
	assert.Contains(t, logs.String(), "failed to release the audit purge lock")
}

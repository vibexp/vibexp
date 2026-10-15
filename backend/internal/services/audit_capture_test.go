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
	"github.com/vibexp/vibexp/internal/services"
)

// fakeClock is a settable clock for the capture cache.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func TestAuditClientInfoCapture_DefaultsToOn(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	repo := h.repo
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuditSettingsNotFound).Once()

	assert.True(t, h.capture.CaptureClientInfo(context.Background()))
}

func TestAuditClientInfoCapture_CachesUntilTTL(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	repo := h.repo
	clock := &fakeClock{now: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	h.capture.SetClockForTest(clock.Now)
	repo.EXPECT().Get(mock.Anything).Return(storedAuditSettingsRow(), nil).Once()
	ctx := context.Background()

	assert.False(t, h.capture.CaptureClientInfo(ctx))
	clock.Advance(services.AuditCaptureCacheTTL - time.Second)
	assert.False(t, h.capture.CaptureClientInfo(ctx), "served from the cache: the mock allows one read only")

	on := storedAuditSettingsRow()
	on.CaptureIPUA = true
	repo.EXPECT().Get(mock.Anything).Return(on, nil).Once()
	clock.Advance(time.Second)
	assert.True(t, h.capture.CaptureClientInfo(ctx), "an expired cache reads again")
}

func TestAuditClientInfoCapture_WriteInvalidates(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	svc, repo := h.svc, h.repo
	ctx := context.Background()
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuditSettingsNotFound).Once()
	require.True(t, h.capture.CaptureClientInfo(ctx))

	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	_, err := svc.Update(ctx, testInstanceAdminID,
		models.InstanceAuditSettingsValues{RetentionWeeks: 2, CaptureIPUA: false}, nil)
	require.NoError(t, err)

	repo.EXPECT().Get(mock.Anything).Return(storedAuditSettingsRow(), nil).Once()
	assert.False(t, h.capture.CaptureClientInfo(ctx), "the update must drop the cached true")

	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(true, nil).Once()
	require.NoError(t, svc.Reset(ctx, testInstanceAdminID))
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuditSettingsNotFound).Once()
	assert.True(t, h.capture.CaptureClientInfo(ctx), "the reset must drop the cached false")
}

func TestAuditClientInfoCapture_NoOpResetKeepsCache(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	svc, repo := h.svc, h.repo
	ctx := context.Background()
	repo.EXPECT().Get(mock.Anything).Return(storedAuditSettingsRow(), nil).Once()
	require.False(t, h.capture.CaptureClientInfo(ctx))

	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, nil).Once()
	require.NoError(t, svc.Reset(ctx, testInstanceAdminID))

	assert.False(t, h.capture.CaptureClientInfo(ctx), "served from the cache: no second read")
}

func TestAuditClientInfoCapture_FailsClosed(t *testing.T) {
	var logs bytes.Buffer
	h := newAuditSettingsHarness(t, &logs)
	repo := h.repo
	repo.EXPECT().Get(mock.Anything).Return(nil, errors.New("db down")).Once()
	ctx := context.Background()

	assert.False(t, h.capture.CaptureClientInfo(ctx))
	assert.False(t, h.capture.CaptureClientInfo(ctx), "the fallback is cached: one failed read per TTL")
	assert.Contains(t, logs.String(), "not capturing client info")
}

func TestAuditClientInfoCapture_RacingWriteIsNotCached(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	svc, repo := h.svc, h.repo
	ctx := context.Background()

	repo.EXPECT().Get(mock.Anything).RunAndReturn(
		func(context.Context) (*models.InstanceAuditSettings, error) {
			// The write lands while this read is in flight.
			_, err := svc.Update(ctx, testInstanceAdminID,
				models.InstanceAuditSettingsValues{RetentionWeeks: 2, CaptureIPUA: false}, nil)
			require.NoError(t, err)
			return nil, repositories.ErrInstanceAuditSettingsNotFound
		}).Once()
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	assert.True(t, h.capture.CaptureClientInfo(ctx), "the in-flight read answers with what it read")

	repo.EXPECT().Get(mock.Anything).Return(storedAuditSettingsRow(), nil).Once()
	assert.False(t, h.capture.CaptureClientInfo(ctx), "but did not cache it over the write")
}

func TestAuditClientInfoCapture_CallerCancellationDoesNotPoisonCache(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	repo := h.repo
	on := storedAuditSettingsRow()
	on.CaptureIPUA = true
	repo.EXPECT().Get(mock.Anything).RunAndReturn(
		func(ctx context.Context) (*models.InstanceAuditSettings, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			_, hasDeadline := ctx.Deadline()
			assert.True(t, hasDeadline, "the detached read is still bounded")
			return on, nil
		}).Once()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	assert.True(t, h.capture.CaptureClientInfo(cancelled), "a cancelled caller still gets the real value")
	assert.True(t, h.capture.CaptureClientInfo(context.Background()), "and the cache holds it, not a fallback")
}

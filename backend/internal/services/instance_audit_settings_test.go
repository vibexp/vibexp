package services_test

import (
	"bytes"
	"context"
	"encoding/json"
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

func newInstanceAuditSettingsService(t *testing.T, logs *bytes.Buffer) (
	*services.InstanceAuditSettingsService, *repomocks.MockInstanceAuditSettingsRepository,
) {
	t.Helper()
	repo := repomocks.NewMockInstanceAuditSettingsRepository(t)
	return services.NewInstanceAuditSettingsService(repo, testLogger(logs)), repo
}

func storedAuditSettingsRow() *models.InstanceAuditSettings {
	updatedBy := testInstanceAdminID
	return &models.InstanceAuditSettings{
		RetentionWeeks: 6,
		CaptureIPUA:    false,
		UpdatedAt:      time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC),
		UpdatedBy:      &updatedBy,
		Version:        3,
	}
}

// fakeClock is a settable clock for the capture cache.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func TestInstanceAuditSettingsService_Get_NoRowIsTheDefaults(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuditSettingsNotFound)

	view, err := svc.Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, models.InstanceAuditSettingsValues{
		RetentionWeeks: models.DefaultAuditRetentionWeeks, CaptureIPUA: true,
	}, view.Values)
	assert.True(t, view.IsDefault)
	assert.Nil(t, view.Version)
	assert.Nil(t, view.UpdatedAt)
	assert.Nil(t, view.UpdatedBy)
}

func TestInstanceAuditSettingsService_Get_StoredRow(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	stored := storedAuditSettingsRow()
	repo.EXPECT().Get(mock.Anything).Return(stored, nil)

	view, err := svc.Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, models.InstanceAuditSettingsValues{RetentionWeeks: 6, CaptureIPUA: false}, view.Values)
	assert.False(t, view.IsDefault)
	require.NotNil(t, view.Version)
	assert.Equal(t, int64(3), *view.Version)
	require.NotNil(t, view.UpdatedAt)
	assert.True(t, view.UpdatedAt.Equal(stored.UpdatedAt))
	assert.Equal(t, stored.UpdatedBy, view.UpdatedBy)
}

// Get is the admin read: a repository fault is an error, never the defaults.
func TestInstanceAuditSettingsService_Get_RepositoryErrorIsReturned(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	boom := errors.New("db down")
	repo.EXPECT().Get(mock.Anything).Return(nil, boom)

	view, err := svc.Get(context.Background())

	assert.Nil(t, view)
	assert.ErrorIs(t, err, boom)
}

func TestValidateInstanceAuditSettings(t *testing.T) {
	cases := []struct {
		weeks int
		valid bool
	}{
		{models.MinAuditRetentionWeeks - 1, false},
		{models.MinAuditRetentionWeeks, true},
		{models.DefaultAuditRetentionWeeks, true},
		{models.MaxAuditRetentionWeeks, true},
		{models.MaxAuditRetentionWeeks + 1, false},
		{-5, false},
	}
	for _, tc := range cases {
		err := services.ValidateInstanceAuditSettings(models.InstanceAuditSettingsValues{RetentionWeeks: tc.weeks})
		if tc.valid {
			assert.NoError(t, err, tc.weeks)
			continue
		}
		require.ErrorIs(t, err, services.ErrInvalidAuditSettings, tc.weeks)
		var fieldErr *services.SettingsFieldError
		require.ErrorAs(t, err, &fieldErr)
		assert.Equal(t, []string{"retention_weeks"}, fieldErr.Fields)
		assert.Equal(t, services.SettingsFieldOutOfRange, fieldErr.Code)
	}
}

// An invalid value is refused before the repository is touched (the strict
// mock fails on any call).
func TestInstanceAuditSettingsService_Update_InvalidWritesNothing(t *testing.T) {
	svc, _ := newInstanceAuditSettingsService(t, nil)

	view, err := svc.Update(context.Background(), testInstanceAdminID,
		models.InstanceAuditSettingsValues{RetentionWeeks: 53, CaptureIPUA: true}, nil)

	assert.Nil(t, view)
	assert.ErrorIs(t, err, services.ErrInvalidAuditSettings)
}

// Update stores the values with the actor and the expected version, and the
// audit entry it hands the repository snapshots before and after under the
// audit_log setting.
func TestInstanceAuditSettingsService_Update_StoresAndAudits(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	expected := int64(3)
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, &expected, mock.Anything).
		RunAndReturn(func(_ context.Context, s *models.InstanceAuditSettings, _ *int64,
			audit repositories.InstanceAuditSettingsAuditFunc,
		) error {
			assert.Equal(t, 12, s.RetentionWeeks)
			assert.True(t, s.CaptureIPUA)
			require.NotNil(t, s.UpdatedBy)
			assert.Equal(t, testInstanceAdminID, *s.UpdatedBy)
			s.Version = 4
			s.UpdatedAt = time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC)
			var err error
			entry, err = audit(storedAuditSettingsRow(), s)
			return err
		})

	view, err := svc.Update(context.Background(), testInstanceAdminID,
		models.InstanceAuditSettingsValues{RetentionWeeks: 12, CaptureIPUA: true}, &expected)

	require.NoError(t, err)
	assert.Equal(t, models.InstanceAuditSettingsValues{RetentionWeeks: 12, CaptureIPUA: true}, view.Values)
	assert.False(t, view.IsDefault)
	require.NotNil(t, view.Version)
	assert.Equal(t, int64(4), *view.Version)

	require.NotNil(t, entry)
	assert.Equal(t, models.InstanceSettingAuditLog, entry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionUpsert, entry.Action)
	require.NotNil(t, entry.ActorUserID)
	assert.Equal(t, testInstanceAdminID, *entry.ActorUserID)
	assert.JSONEq(t, `{"retention_weeks":6,"capture_ip_ua":false}`, string(entry.Before))
	assert.JSONEq(t, `{"retention_weeks":12,"capture_ip_ua":true}`, string(entry.After))
}

func TestInstanceAuditSettingsService_Update_ConflictIsPassedThrough(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(repositories.ErrInstanceSettingsVersionConflict)

	_, err := svc.Update(context.Background(), testInstanceAdminID,
		models.InstanceAuditSettingsValues{RetentionWeeks: 2, CaptureIPUA: true}, nil)

	assert.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)
}

func TestInstanceAuditSettingsService_Reset_AuditsTheDelete(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, audit repositories.InstanceAuditSettingsAuditFunc) (bool, error) {
			var err error
			entry, err = audit(storedAuditSettingsRow(), nil)
			return true, err
		})

	require.NoError(t, svc.Reset(context.Background(), testInstanceAdminID))

	require.NotNil(t, entry)
	assert.Equal(t, models.InstanceSettingAuditLog, entry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entry.Action)
	assert.JSONEq(t, `{"retention_weeks":6,"capture_ip_ua":false}`, string(entry.Before))
	assert.Nil(t, entry.After)
}

func TestInstanceAuditSettingsService_Reset_Error(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	boom := errors.New("db down")
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, boom)

	assert.ErrorIs(t, svc.Reset(context.Background(), testInstanceAdminID), boom)
}

// The audit snapshot is the values' exact JSON shape, which the admin audit
// allowlist mirrors.
func TestInstanceAuditSettingsValues_SnapshotShape(t *testing.T) {
	raw, err := json.Marshal(models.DefaultInstanceAuditSettingsValues())
	require.NoError(t, err)
	assert.JSONEq(t, `{"retention_weeks":2,"capture_ip_ua":true}`, string(raw))
}

func TestInstanceAuditSettingsService_CaptureClientInfo_DefaultsToOn(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuditSettingsNotFound).Once()

	assert.True(t, svc.CaptureClientInfo(context.Background()))
}

// The switch is read once and served from memory until the TTL passes.
func TestInstanceAuditSettingsService_CaptureClientInfo_CachesUntilTTL(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	clock := &fakeClock{now: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	svc.SetClockForTest(clock.Now)
	repo.EXPECT().Get(mock.Anything).Return(storedAuditSettingsRow(), nil).Once()
	ctx := context.Background()

	assert.False(t, svc.CaptureClientInfo(ctx))
	clock.Advance(services.AuditCaptureCacheTTL - time.Second)
	assert.False(t, svc.CaptureClientInfo(ctx), "served from the cache: the mock allows one read only")

	on := storedAuditSettingsRow()
	on.CaptureIPUA = true
	repo.EXPECT().Get(mock.Anything).Return(on, nil).Once()
	clock.Advance(time.Second)
	assert.True(t, svc.CaptureClientInfo(ctx), "an expired cache reads again")
}

// A write through this process drops the cached switch, so the very next event
// follows it, as does a reset.
func TestInstanceAuditSettingsService_CaptureClientInfo_WriteInvalidates(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	ctx := context.Background()
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuditSettingsNotFound).Once()
	require.True(t, svc.CaptureClientInfo(ctx))

	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	_, err := svc.Update(ctx, testInstanceAdminID,
		models.InstanceAuditSettingsValues{RetentionWeeks: 2, CaptureIPUA: false}, nil)
	require.NoError(t, err)

	repo.EXPECT().Get(mock.Anything).Return(storedAuditSettingsRow(), nil).Once()
	assert.False(t, svc.CaptureClientInfo(ctx), "the update must drop the cached true")

	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(true, nil).Once()
	require.NoError(t, svc.Reset(ctx, testInstanceAdminID))
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAuditSettingsNotFound).Once()
	assert.True(t, svc.CaptureClientInfo(ctx), "the reset must drop the cached false")
}

// A reset with nothing stored changes nothing, so it keeps the cache.
func TestInstanceAuditSettingsService_CaptureClientInfo_NoOpResetKeepsCache(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
	ctx := context.Background()
	repo.EXPECT().Get(mock.Anything).Return(storedAuditSettingsRow(), nil).Once()
	require.False(t, svc.CaptureClientInfo(ctx))

	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, nil).Once()
	require.NoError(t, svc.Reset(ctx, testInstanceAdminID))

	assert.False(t, svc.CaptureClientInfo(ctx), "served from the cache: no second read")
}

// An unreadable setting must never switch capture back on behind an admin who
// turned it off: the fallback is false, logged, and cached like a read.
func TestInstanceAuditSettingsService_CaptureClientInfo_FailsClosed(t *testing.T) {
	var logs bytes.Buffer
	svc, repo := newInstanceAuditSettingsService(t, &logs)
	repo.EXPECT().Get(mock.Anything).Return(nil, errors.New("db down")).Once()
	ctx := context.Background()

	assert.False(t, svc.CaptureClientInfo(ctx))
	assert.False(t, svc.CaptureClientInfo(ctx), "the fallback is cached: one failed read per TTL")
	assert.Contains(t, logs.String(), "not capturing client info")
}

// A read that raced a write must not cache what it read: the write's
// invalidation wins.
func TestInstanceAuditSettingsService_CaptureClientInfo_RacingWriteIsNotCached(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
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

	assert.True(t, svc.CaptureClientInfo(ctx), "the in-flight read answers with what it read")

	repo.EXPECT().Get(mock.Anything).Return(storedAuditSettingsRow(), nil).Once()
	assert.False(t, svc.CaptureClientInfo(ctx), "but did not cache it over the write")
}

// One caller going away mid-read must not poison the shared cache: the read
// runs detached from the caller's cancellation, so a cancelled request still
// resolves the real value.
func TestInstanceAuditSettingsService_CaptureClientInfo_CallerCancellationDoesNotPoisonCache(t *testing.T) {
	svc, repo := newInstanceAuditSettingsService(t, nil)
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

	assert.True(t, svc.CaptureClientInfo(cancelled), "a cancelled caller still gets the real value")
	assert.True(t, svc.CaptureClientInfo(context.Background()), "and the cache holds it, not a fallback")
}

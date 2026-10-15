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
	"github.com/vibexp/vibexp/internal/services/audit"
)

// auditSettingsHarness is the REAL settings service and the REAL capture switch
// over one repository mock, with a recorder that keeps what it is given.
type auditSettingsHarness struct {
	svc      *services.InstanceAuditSettingsService
	repo     *repomocks.MockInstanceAuditSettingsRepository
	capture  *services.AuditClientInfoCapture
	recorder *recordingAuditRecorder
}

func newAuditSettingsHarness(t *testing.T, logs *bytes.Buffer) *auditSettingsHarness {
	t.Helper()
	h := &auditSettingsHarness{
		repo:     repomocks.NewMockInstanceAuditSettingsRepository(t),
		recorder: &recordingAuditRecorder{},
	}
	h.capture = services.NewAuditClientInfoCapture(h.repo, testLogger(logs))
	h.svc = services.NewInstanceAuditSettingsService(h.repo, h.capture, h.recorder, testLogger(logs))
	return h
}

// recordingAuditRecorder is an audit.Recorder that keeps every event.
type recordingAuditRecorder struct {
	events []audit.Event
}

func (r *recordingAuditRecorder) Record(_ context.Context, e audit.Event) {
	r.events = append(r.events, e)
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

func TestInstanceAuditSettingsService_Get_NoRowIsTheDefaults(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	svc, repo := h.svc, h.repo
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
	h := newAuditSettingsHarness(t, nil)
	svc, repo := h.svc, h.repo
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
	h := newAuditSettingsHarness(t, nil)
	svc, repo := h.svc, h.repo
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
	h := newAuditSettingsHarness(t, nil)
	svc := h.svc

	view, err := svc.Update(context.Background(), testInstanceAdminID,
		models.InstanceAuditSettingsValues{RetentionWeeks: 53, CaptureIPUA: true}, nil)

	assert.Nil(t, view)
	assert.ErrorIs(t, err, services.ErrInvalidAuditSettings)
}

// Update stores the values with the actor and the expected version, and the
// audit entry it hands the repository snapshots before and after under the
// audit_log setting.
func TestInstanceAuditSettingsService_Update_StoresAndAudits(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	svc, repo := h.svc, h.repo
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

	require.Len(t, h.recorder.events, 1, "the change is recorded in the audit log itself")
	event := h.recorder.events[0]
	assert.Equal(t, models.AuditOpAuditSettingsUpdated, event.Operation)
	assert.Equal(t, map[string]any{
		"before": models.InstanceAuditSettingsValues{RetentionWeeks: 6, CaptureIPUA: false},
		"after":  models.InstanceAuditSettingsValues{RetentionWeeks: 12, CaptureIPUA: true},
	}, event.Metadata, "the event reports the rows the settings audit entry stored")
}

func TestInstanceAuditSettingsService_Update_ConflictIsPassedThrough(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	svc, repo := h.svc, h.repo
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(repositories.ErrInstanceSettingsVersionConflict)

	_, err := svc.Update(context.Background(), testInstanceAdminID,
		models.InstanceAuditSettingsValues{RetentionWeeks: 2, CaptureIPUA: true}, nil)

	assert.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict)
}

func TestInstanceAuditSettingsService_Reset_AuditsTheDelete(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	svc, repo := h.svc, h.repo
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

	require.Len(t, h.recorder.events, 1)
	assert.Equal(t, models.AuditOpAuditSettingsUpdated, h.recorder.events[0].Operation)
	assert.Equal(t, map[string]any{
		"before": models.InstanceAuditSettingsValues{RetentionWeeks: 6, CaptureIPUA: false},
		"after":  nil,
	}, h.recorder.events[0].Metadata, "after a reset the built-in defaults apply: after is null")
}

func TestInstanceAuditSettingsService_Reset_Error(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	svc, repo := h.svc, h.repo
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

// The switch is read once and served from memory until the TTL passes.
// A write through this process drops the cached switch, so the very next event
// follows it, as does a reset.
// A reset with nothing stored changes nothing, so it keeps the cache.
// An unreadable setting must never switch capture back on behind an admin who
// turned it off: the fallback is false, logged, and cached like a read.
// A read that raced a write must not cache what it read: the write's
// invalidation wins.
// One caller going away mid-read must not poison the shared cache: the read
// runs detached from the caller's cancellation, so a cancelled request still
// resolves the real value.
// A write that did not happen is not an event: a rejected value, a version
// conflict, a failed reset and a reset with nothing stored record nothing.
func TestInstanceAuditSettingsService_RecordsOnlyCommittedChanges(t *testing.T) {
	h := newAuditSettingsHarness(t, nil)
	ctx := context.Background()

	_, err := h.svc.Update(ctx, testInstanceAdminID, models.InstanceAuditSettingsValues{RetentionWeeks: 0}, nil)
	require.Error(t, err)

	h.repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(repositories.ErrInstanceSettingsVersionConflict).Once()
	_, err = h.svc.Update(ctx, testInstanceAdminID,
		models.InstanceAuditSettingsValues{RetentionWeeks: 2, CaptureIPUA: true}, nil)
	require.Error(t, err)

	h.repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, errors.New("db down")).Once()
	require.Error(t, h.svc.Reset(ctx, testInstanceAdminID))

	h.repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, nil).Once()
	require.NoError(t, h.svc.Reset(ctx, testInstanceAdminID))

	assert.Empty(t, h.recorder.events)
}

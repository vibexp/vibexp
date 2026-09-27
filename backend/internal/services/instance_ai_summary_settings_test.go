package services_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
)

func newInstanceAISummaryService(t *testing.T, logs *bytes.Buffer) (
	*services.InstanceAISummarySettingsService, *repomocks.MockInstanceAISummarySettingsRepository,
) {
	t.Helper()
	repo := repomocks.NewMockInstanceAISummarySettingsRepository(t)
	return services.NewInstanceAISummarySettingsService(repo, testLogger(logs)), repo
}

func storedInstanceAISummaryRow() *models.InstanceAISummarySettings {
	updatedBy := testInstanceAdminID
	return &models.InstanceAISummarySettings{
		Enabled:           false,
		TopN:              7,
		Style:             models.AISummaryStyleDetailed,
		MaxOutputTokens:   2000,
		PerDocumentChars:  4000,
		TotalContextChars: 20000,
		RequestTimeout:    45 * time.Second,
		UpdatedAt:         time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
		UpdatedBy:         &updatedBy,
		Version:           2,
	}
}

func validInstanceAISummaryValues() models.InstanceAISummarySettingsValues {
	return models.InstanceAISummarySettingsValues{
		Enabled:           true,
		TopN:              3,
		Style:             models.AISummaryStyleConcise,
		MaxOutputTokens:   600,
		PerDocumentChars:  5000,
		TotalContextChars: 15000,
		RequestTimeout:    30 * time.Second,
	}
}

// With no instance row, the built-in defaults are exactly the values the
// `ai_summary:` config defaulted to before #1199.
func TestDefaultInstanceAISummarySettings_AreTodaysValues(t *testing.T) {
	assert.Equal(t, models.InstanceAISummarySettingsValues{
		Enabled:           true,
		TopN:              5,
		Style:             models.AISummaryStyleBalanced,
		MaxOutputTokens:   800,
		PerDocumentChars:  8000,
		TotalContextChars: 32000,
		RequestTimeout:    60 * time.Second,
	}, models.DefaultInstanceAISummarySettings())
}

// config.yaml's `ai_summary:` defaults are built from the same values while the
// block still exists (until #1203): the boot-time import (#1201) compares
// against them.
func TestDefaultInstanceAISummarySettings_MatchConfigDefaults(t *testing.T) {
	// Only the one required field; everything else inherits config defaults().
	body := "security:\n  encryption_key: " + strings.Repeat("k", 32) + "\n"
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Equal(t, models.InstanceAISummarySettingsValues{
		Enabled:           bool(cfg.AISummary.Enabled),
		TopN:              int(cfg.AISummary.TopN),
		Style:             cfg.AISummary.Style,
		MaxOutputTokens:   cfg.AISummary.MaxOutputTokens,
		PerDocumentChars:  cfg.AISummary.PerDocumentChars,
		TotalContextChars: cfg.AISummary.TotalContextChars,
		RequestTimeout:    cfg.AISummary.RequestTimeout,
	}, models.DefaultInstanceAISummarySettings())
}

func TestInstanceAISummarySettingsService_Resolve_NoRowReturnsBuiltInDefaults(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAISummarySettingsNotFound)

	assert.Equal(t, models.DefaultInstanceAISummarySettings(), svc.Resolve(context.Background()))
}

func TestInstanceAISummarySettingsService_Resolve_StoredRowIsUsed(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(storedInstanceAISummaryRow(), nil)

	assert.Equal(t, models.InstanceAISummarySettingsValues{
		Enabled:           false,
		TopN:              7,
		Style:             models.AISummaryStyleDetailed,
		MaxOutputTokens:   2000,
		PerDocumentChars:  4000,
		TotalContextChars: 20000,
		RequestTimeout:    45 * time.Second,
	}, svc.Resolve(context.Background()))
}

// No cache: every Resolve reads the row, so a change applies to the next call.
func TestInstanceAISummarySettingsService_Resolve_SeesAChangeOnTheNextCall(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAISummarySettingsNotFound).Once()
	repo.EXPECT().Get(mock.Anything).Return(storedInstanceAISummaryRow(), nil).Once()

	first := svc.Resolve(context.Background())
	second := svc.Resolve(context.Background())

	assert.Equal(t, models.DefaultInstanceAISummarySettings(), first)
	assert.Equal(t, 4000, second.PerDocumentChars)
	assert.NotEqual(t, first, second)
}

func TestInstanceAISummarySettingsService_Resolve_RepositoryErrorFailsOpen(t *testing.T) {
	var logs bytes.Buffer
	svc, repo := newInstanceAISummaryService(t, &logs)
	repo.EXPECT().Get(mock.Anything).Return(nil, errors.New("connection refused"))

	assert.Equal(t, models.DefaultInstanceAISummarySettings(), svc.Resolve(context.Background()),
		"a settings read failure must fall back to the built-in defaults, not fail the summary")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	assert.Equal(t, "WARN", entry["level"])
	assert.Contains(t, entry["error"], "connection refused")
}

func TestInstanceAISummarySettingsService_Get_NoRowReportsDefaultSource(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceAISummarySettingsNotFound)

	view, err := svc.Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, &models.InstanceAISummarySettingsView{
		Source: models.InstanceAISummarySettingsSourceDefault,
		Values: models.DefaultInstanceAISummarySettings(),
	}, view)
}

func TestInstanceAISummarySettingsService_Get_StoredRowReportsInstanceSource(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	row := storedInstanceAISummaryRow()
	repo.EXPECT().Get(mock.Anything).Return(row, nil)

	view, err := svc.Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, models.InstanceAISummarySettingsSourceInstance, view.Source)
	assert.Equal(t, 45*time.Second, view.Values.RequestTimeout)
	require.NotNil(t, view.UpdatedAt)
	assert.Equal(t, row.UpdatedAt, *view.UpdatedAt)
	assert.Equal(t, row.UpdatedBy, view.UpdatedBy)
}

func TestInstanceAISummarySettingsService_Get_RepositoryErrorPropagates(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, errors.New("boom"))

	_, err := svc.Get(context.Background())

	assert.Error(t, err, "unlike Resolve, Get must not fail open: an admin must see real state")
}

func TestInstanceAISummarySettingsService_Update_StoresAndAudits(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	before := storedInstanceAISummaryRow()
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			_ context.Context, s *models.InstanceAISummarySettings, audit repositories.InstanceAISummarySettingsAuditFunc,
		) error {
			assert.Equal(t, 5000, s.PerDocumentChars)
			assert.Equal(t, 30*time.Second, s.RequestTimeout)
			require.NotNil(t, s.UpdatedBy)
			assert.Equal(t, testInstanceAdminID, *s.UpdatedBy)
			s.UpdatedAt = time.Date(2026, 9, 27, 13, 0, 0, 0, time.UTC)
			var err error
			entry, err = audit(before, s)
			return err
		})

	view, err := svc.Update(context.Background(), testInstanceAdminID, validInstanceAISummaryValues())

	require.NoError(t, err)
	assert.Equal(t, models.InstanceAISummarySettingsSourceInstance, view.Source)
	assert.Equal(t, validInstanceAISummaryValues(), view.Values)
	require.NotNil(t, view.UpdatedAt)
	assert.Equal(t, time.Date(2026, 9, 27, 13, 0, 0, 0, time.UTC), *view.UpdatedAt)

	require.NotNil(t, entry)
	assert.Equal(t, models.InstanceSettingAISummary, entry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionUpsert, entry.Action)
	require.NotNil(t, entry.ActorUserID)
	assert.Equal(t, testInstanceAdminID, *entry.ActorUserID)
	assert.JSONEq(t, `{"enabled":false,"top_n":7,"style":"detailed","max_output_tokens":2000,
		"per_document_chars":4000,"total_context_chars":20000,"request_timeout_ms":45000}`, string(entry.Before))
	assert.JSONEq(t, `{"enabled":true,"top_n":3,"style":"concise","max_output_tokens":600,
		"per_document_chars":5000,"total_context_chars":15000,"request_timeout_ms":30000}`, string(entry.After))
}

// A first save has no previous row: the entry's before is empty.
func TestInstanceAISummarySettingsService_Update_FirstSaveHasNoBefore(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			_ context.Context, s *models.InstanceAISummarySettings, audit repositories.InstanceAISummarySettingsAuditFunc,
		) error {
			var err error
			entry, err = audit(nil, s)
			return err
		})

	_, err := svc.Update(context.Background(), testInstanceAdminID, validInstanceAISummaryValues())

	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Nil(t, entry.Before)
	assert.NotNil(t, entry.After)
}

// A system change (no actor) records no actor rather than a blank user id.
func TestInstanceAISummarySettingsService_Update_BlankActorIsNil(t *testing.T) {
	for _, actor := range []string{"", "   "} {
		svc, repo := newInstanceAISummaryService(t, nil)
		var entry *models.InstanceSettingsAuditEntry
		repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(
				_ context.Context, s *models.InstanceAISummarySettings, audit repositories.InstanceAISummarySettingsAuditFunc,
			) error {
				assert.Nil(t, s.UpdatedBy)
				var err error
				entry, err = audit(nil, s)
				return err
			})

		_, err := svc.Update(context.Background(), actor, validInstanceAISummaryValues())

		require.NoError(t, err)
		assert.Nil(t, entry.ActorUserID, "actor %q", actor)
	}
}

// The row and its audit entry are one transaction (the repository's
// UpsertAudited contract): an audit append that fails surfaces as the error of
// the whole save, and the service reports nothing as saved.
func TestInstanceAISummarySettingsService_Update_RepositoryErrorPropagates(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("audit append failed; upsert rolled back"))

	view, err := svc.Update(context.Background(), testInstanceAdminID, validInstanceAISummaryValues())

	assert.ErrorContains(t, err, "rolled back")
	assert.Nil(t, view)
}

func TestInstanceAISummarySettingsService_Update_RejectsInvalidValuesWithoutWriting(t *testing.T) {
	// No repo expectations: an invalid set must write no row and no audit entry.
	svc, _ := newInstanceAISummaryService(t, nil)
	invalid := validInstanceAISummaryValues()
	invalid.TopN = models.MaxAISummaryTopN + 1

	_, err := svc.Update(context.Background(), testInstanceAdminID, invalid)

	assert.ErrorIs(t, err, services.ErrInvalidInstanceAISummarySettings)
}

func TestInstanceAISummarySettingsService_Reset_DeletesAndAudits(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, audit repositories.InstanceAISummarySettingsAuditFunc) (bool, error) {
			var err error
			entry, err = audit(storedInstanceAISummaryRow(), nil)
			return true, err
		})

	require.NoError(t, svc.Reset(context.Background(), testInstanceAdminID))

	require.NotNil(t, entry)
	assert.Equal(t, models.InstanceSettingAISummary, entry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entry.Action)
	assert.NotNil(t, entry.Before)
	assert.Nil(t, entry.After)
}

// With no stored row the repository deletes nothing and never builds an
// entry; the service treats that as success.
func TestInstanceAISummarySettingsService_Reset_NoRowIsANoOp(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, nil)

	assert.NoError(t, svc.Reset(context.Background(), testInstanceAdminID))
}

func TestInstanceAISummarySettingsService_Reset_RepositoryErrorPropagates(t *testing.T) {
	svc, repo := newInstanceAISummaryService(t, nil)
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, errors.New("boom"))

	assert.ErrorContains(t, svc.Reset(context.Background(), testInstanceAdminID), "boom")
}

// Each bound of the instance validator, on both sides where it has two.
func TestValidateInstanceAISummarySettings_Bounds(t *testing.T) {
	with := func(mutate func(*models.InstanceAISummarySettingsValues)) models.InstanceAISummarySettingsValues {
		v := validInstanceAISummaryValues()
		mutate(&v)
		return v
	}
	cases := []struct {
		name    string
		values  models.InstanceAISummarySettingsValues
		wantErr bool
	}{
		{"valid", validInstanceAISummaryValues(), false},
		{"top_n 0", with(func(v *models.InstanceAISummarySettingsValues) { v.TopN = 0 }), true},
		{"top_n 1", with(func(v *models.InstanceAISummarySettingsValues) { v.TopN = 1 }), false},
		{"top_n 10", with(func(v *models.InstanceAISummarySettingsValues) { v.TopN = models.MaxAISummaryTopN }), false},
		{"top_n 11", with(func(v *models.InstanceAISummarySettingsValues) { v.TopN = 11 }), true},
		{"max_output_tokens 0", with(func(v *models.InstanceAISummarySettingsValues) { v.MaxOutputTokens = 0 }), true},
		{"max_output_tokens 1", with(func(v *models.InstanceAISummarySettingsValues) { v.MaxOutputTokens = 1 }), false},
		{"max_output_tokens 32768", with(func(v *models.InstanceAISummarySettingsValues) {
			v.MaxOutputTokens = models.MaxAISummaryOutputTokens
		}), false},
		{"max_output_tokens 32769", with(func(v *models.InstanceAISummarySettingsValues) {
			v.MaxOutputTokens = 32769
		}), true},
		{"per_document_chars 0", with(func(v *models.InstanceAISummarySettingsValues) { v.PerDocumentChars = 0 }), true},
		{"per_document_chars 1", with(func(v *models.InstanceAISummarySettingsValues) {
			v.PerDocumentChars, v.TotalContextChars = 1, 1
		}), false},
		{"total below per-document", with(func(v *models.InstanceAISummarySettingsValues) {
			v.TotalContextChars = v.PerDocumentChars - 1
		}), true},
		{"total equal to per-document", with(func(v *models.InstanceAISummarySettingsValues) {
			v.TotalContextChars = v.PerDocumentChars
		}), false},
		{"request_timeout 0", with(func(v *models.InstanceAISummarySettingsValues) { v.RequestTimeout = 0 }), true},
		{"request_timeout negative", with(func(v *models.InstanceAISummarySettingsValues) {
			v.RequestTimeout = -time.Second
		}), true},
		{"request_timeout 1ms", with(func(v *models.InstanceAISummarySettingsValues) {
			v.RequestTimeout = time.Millisecond
		}), false},
		// Positive, but stored in whole milliseconds it would truncate to 0.
		{"request_timeout 999µs", with(func(v *models.InstanceAISummarySettingsValues) {
			v.RequestTimeout = 999 * time.Microsecond
		}), true},
		{"request_timeout at the int32 ms limit", with(func(v *models.InstanceAISummarySettingsValues) {
			v.RequestTimeout = time.Duration(math.MaxInt32) * time.Millisecond
		}), false},
		{"request_timeout past the int32 ms limit", with(func(v *models.InstanceAISummarySettingsValues) {
			v.RequestTimeout = time.Duration(math.MaxInt32)*time.Millisecond + time.Millisecond
		}), true},
		{"per_document_chars past int32", with(func(v *models.InstanceAISummarySettingsValues) {
			v.PerDocumentChars, v.TotalContextChars = math.MaxInt32+1, math.MaxInt32+1
		}), true},
		{"total_context_chars past int32", with(func(v *models.InstanceAISummarySettingsValues) {
			v.TotalContextChars = math.MaxInt32 + 1
		}), true},
		{"budgets at int32", with(func(v *models.InstanceAISummarySettingsValues) {
			v.PerDocumentChars, v.TotalContextChars = math.MaxInt32, math.MaxInt32
		}), false},
		{"unknown style", with(func(v *models.InstanceAISummarySettingsValues) { v.Style = "verbose" }), true},
		{"empty style", with(func(v *models.InstanceAISummarySettingsValues) { v.Style = "" }), true},
		{"detailed style", with(func(v *models.InstanceAISummarySettingsValues) {
			v.Style = models.AISummaryStyleDetailed
		}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := services.ValidateInstanceAISummarySettings(tc.values)
			if tc.wantErr {
				assert.ErrorIs(t, err, services.ErrInvalidInstanceAISummarySettings)
				assert.NotErrorIs(t, err, services.ErrInvalidAISummarySettings,
					"the instance sentinel is its own: #1200 maps it to 422, the team one to 400")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// The built-in defaults must themselves pass the validator.
func TestValidateInstanceAISummarySettings_AcceptsBuiltInDefaults(t *testing.T) {
	assert.NoError(t, services.ValidateInstanceAISummarySettings(models.DefaultInstanceAISummarySettings()))
}

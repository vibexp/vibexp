package services_test

import (
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

// fieldsOf returns the fields a validation error is attributed to.
func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	var fieldErr *services.SettingsFieldError
	require.ErrorAs(t, err, &fieldErr)
	return fieldErr.Fields
}

// codeOf returns the class of rule a validation error reports.
func codeOf(t *testing.T, err error) string {
	t.Helper()
	var fieldErr *services.SettingsFieldError
	require.ErrorAs(t, err, &fieldErr)
	return fieldErr.Code
}

// TestValidateInstanceSearchSettings_AttributesFields: every rejection names
// the request field(s) at fault, keeps its sentinel, and keeps the message
// wording the team settings API already returns.
func TestValidateInstanceSearchSettings_AttributesFields(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(v *models.InstanceSearchSettingsValues)
		wantFields []string
		wantMsg    string
		wantCode   string
	}{
		{"negative weights", func(v *models.InstanceSearchSettingsValues) {
			v.RankWeightRelevance, v.RankWeightUpdated = -1, -0.5
		}, []string{"rank_weight_relevance", "rank_weight_updated"}, "rank_weight_* must be non-negative",
			services.SettingsFieldOutOfRange},
		{"all weights zero", func(v *models.InstanceSearchSettingsValues) {
			v.RankWeightRelevance, v.RankWeightCreated, v.RankWeightUpdated = 0, 0, 0
		}, []string{"rank_weight_relevance", "rank_weight_created", "rank_weight_updated"}, "rank_weight_* must not all be zero",
			services.SettingsFieldInvalidValue},
		{"half-life zero", func(v *models.InstanceSearchSettingsValues) { v.RankHalfLifeDays = 0 },
			[]string{"rank_half_life_days"}, "rank_half_life_days must be positive", services.SettingsFieldOutOfRange},
		{"half-life too long", func(v *models.InstanceSearchSettingsValues) { v.RankHalfLifeDays = 36501 },
			[]string{"rank_half_life_days"}, "rank_half_life_days must be <= 36500", services.SettingsFieldOutOfRange},
		{"candidate cap zero", func(v *models.InstanceSearchSettingsValues) { v.RankCandidateCap = 0 },
			[]string{"rank_candidate_cap"}, "rank_candidate_cap must be >= 1", services.SettingsFieldOutOfRange},
		{"candidate cap too large", func(v *models.InstanceSearchSettingsValues) { v.RankCandidateCap = 5001 },
			[]string{"rank_candidate_cap"}, "rank_candidate_cap must be <= 5000", services.SettingsFieldOutOfRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := validInstanceValues()
			tc.mutate(&v)

			err := services.ValidateInstanceSearchSettings(v)

			require.ErrorIs(t, err, services.ErrInvalidSearchSettings)
			assert.Equal(t, tc.wantFields, fieldsOf(t, err))
			assert.Equal(t, tc.wantCode, codeOf(t, err))
			assert.Contains(t, err.Error(), "invalid search settings: "+tc.wantMsg)
		})
	}
}

// TestValidateInstanceAISummarySettings_AttributesFields: as above, for the AI
// summary bounds, including the ones shared with the team profile.
func TestValidateInstanceAISummarySettings_AttributesFields(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(v *models.InstanceAISummarySettingsValues)
		wantField string
	}{
		{"top_n", func(v *models.InstanceAISummarySettingsValues) { v.TopN = 11 }, "top_n"},
		{"max_output_tokens", func(v *models.InstanceAISummarySettingsValues) { v.MaxOutputTokens = 32769 },
			"max_output_tokens"},
		{"style", func(v *models.InstanceAISummarySettingsValues) { v.Style = "verbose" }, "style"},
		{"per_document_chars", func(v *models.InstanceAISummarySettingsValues) { v.PerDocumentChars = 0 },
			"per_document_chars"},
		{"total_context_chars", func(v *models.InstanceAISummarySettingsValues) {
			v.TotalContextChars = v.PerDocumentChars - 1
		}, "total_context_chars"},
		{"request timeout", func(v *models.InstanceAISummarySettingsValues) { v.RequestTimeout = time.Microsecond },
			"request_timeout_ms"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := validInstanceAISummaryValues()
			tc.mutate(&v)

			err := services.ValidateInstanceAISummarySettings(v)

			require.ErrorIs(t, err, services.ErrInvalidInstanceAISummarySettings)
			assert.Equal(t, []string{tc.wantField}, fieldsOf(t, err))
			wantCode := services.SettingsFieldOutOfRange
			if tc.wantField == "style" {
				wantCode = services.SettingsFieldInvalidValue
			}
			assert.Equal(t, wantCode, codeOf(t, err))
		})
	}
}

// TestValidateAISummarySettings_TeamErrorsCarryFields: the team validator
// shares the profile bounds, so its errors are attributed too while keeping
// their sentinel.
func TestValidateAISummarySettings_TeamErrorsCarryFields(t *testing.T) {
	err := services.ValidateAISummarySettings(models.TeamAISummarySettingsValues{
		Enabled: true, TopN: 0, Style: models.AISummaryStyleBalanced, MaxOutputTokens: 100,
	})

	require.ErrorIs(t, err, services.ErrInvalidAISummarySettings)
	assert.Equal(t, []string{"top_n"}, fieldsOf(t, err))
}

// TestInstanceSettingsServices_UpdatePassesExpectedVersion: Update hands the
// caller's expected version to the repository, surfaces its conflict
// untouched, and reports the stored version on success.
func TestInstanceSettingsServices_UpdatePassesExpectedVersion(t *testing.T) {
	expected := int64(7)

	t.Run("search", func(t *testing.T) {
		svc, repo := newInstanceSearchService(t, nil)
		repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, &expected, mock.Anything).
			Return(repositories.ErrInstanceSettingsVersionConflict).Once()
		repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, &expected, mock.Anything).
			RunAndReturn(func(
				_ context.Context, s *models.InstanceSearchSettings, _ *int64, _ repositories.InstanceSearchSettingsAuditFunc,
			) error {
				s.Version = 8
				return nil
			}).Once()

		_, err := svc.Update(context.Background(), testInstanceAdminID, validInstanceValues(), &expected)
		require.True(t, errors.Is(err, repositories.ErrInstanceSettingsVersionConflict))

		view, err := svc.Update(context.Background(), testInstanceAdminID, validInstanceValues(), &expected)
		require.NoError(t, err)
		require.NotNil(t, view.Version)
		assert.Equal(t, int64(8), *view.Version)
	})

	t.Run("ai summary", func(t *testing.T) {
		svc, repo := newInstanceAISummaryService(t, nil)
		repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, &expected, mock.Anything).
			Return(repositories.ErrInstanceSettingsVersionConflict).Once()
		repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, &expected, mock.Anything).
			RunAndReturn(func(
				_ context.Context, s *models.InstanceAISummarySettings, _ *int64,
				_ repositories.InstanceAISummarySettingsAuditFunc,
			) error {
				s.Version = 8
				return nil
			}).Once()

		_, err := svc.Update(context.Background(), testInstanceAdminID, validInstanceAISummaryValues(), &expected)
		require.True(t, errors.Is(err, repositories.ErrInstanceSettingsVersionConflict))

		view, err := svc.Update(context.Background(), testInstanceAdminID, validInstanceAISummaryValues(), &expected)
		require.NoError(t, err)
		require.NotNil(t, view.Version)
		assert.Equal(t, int64(8), *view.Version)
	})
}

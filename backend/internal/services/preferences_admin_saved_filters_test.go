package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func savedPreset(name string) models.AdminSavedFilterPreset {
	return models.AdminSavedFilterPreset{Name: name, Query: map[string]string{"status": "active"}}
}

func TestGetAdminSavedFilters_ValidatesList(t *testing.T) {
	repo := new(MockUserPreferencesRepository)
	svc := NewUserPreferencesService(repo)

	_, _, err := svc.GetAdminSavedFilters(context.Background(), "u1", "widgets")

	var invalid *ErrAdminSavedFiltersInvalid
	require.ErrorAs(t, err, &invalid)
	assert.Contains(t, invalid.Error(), "widgets")
	repo.AssertNotCalled(t, "GetAdminSavedFilters", mock.Anything, mock.Anything, mock.Anything)
}

func TestGetAdminSavedFilters_PassesThrough(t *testing.T) {
	repo := new(MockUserPreferencesRepository)
	want := []models.AdminSavedFilterPreset{{ID: uuid.NewString(), Name: "A"}}
	repo.On("GetAdminSavedFilters", mock.Anything, "u1", "teams").Return(want, int64(7), nil)
	svc := NewUserPreferencesService(repo)

	got, version, err := svc.GetAdminSavedFilters(context.Background(), "u1", "teams")

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.EqualValues(t, 7, version)
}

// TestReplaceAdminSavedFilters_Normalizes: names trimmed, sent ids kept (in
// canonical form), missing ids assigned, and the row seed is the defaults.
func TestReplaceAdminSavedFilters_Normalizes(t *testing.T) {
	keptID := uuid.New()
	repo := new(MockUserPreferencesRepository)
	var stored []models.AdminSavedFilterPreset
	repo.On("ReplaceAdminSavedFilters", mock.Anything, "u1", "users", mock.Anything,
		models.DefaultPreferences(), int64(0)).
		Run(func(args mock.Arguments) { stored = args.Get(3).([]models.AdminSavedFilterPreset) }).
		Return(int64(1), nil)
	svc := NewUserPreferencesService(repo)

	in := []models.AdminSavedFilterPreset{
		{ID: strings.ToUpper(keptID.String()), Name: "  Kept ", Query: nil},
		savedPreset("Fresh"),
	}
	got, version, err := svc.ReplaceAdminSavedFilters(context.Background(), "u1", "users", in, 0)

	require.NoError(t, err)
	assert.EqualValues(t, 1, version)
	require.Len(t, got, 2)
	assert.Equal(t, keptID.String(), got[0].ID)
	assert.Equal(t, "Kept", got[0].Name)
	assert.NotNil(t, got[0].Query)
	_, parseErr := uuid.Parse(got[1].ID)
	require.NoError(t, parseErr)
	assert.Equal(t, got, stored)
	assert.Equal(t, "  Kept ", in[0].Name, "the caller's slice is not modified")
}

func TestReplaceAdminSavedFilters_EmptyListIsAllowed(t *testing.T) {
	repo := new(MockUserPreferencesRepository)
	repo.On("ReplaceAdminSavedFilters", mock.Anything, "u1", "projects",
		[]models.AdminSavedFilterPreset{}, mock.Anything, int64(3)).Return(int64(4), nil)
	svc := NewUserPreferencesService(repo)

	got, version, err := svc.ReplaceAdminSavedFilters(context.Background(), "u1", "projects", nil, 3)

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.EqualValues(t, 4, version)
}

func TestReplaceAdminSavedFilters_ConflictPassesThrough(t *testing.T) {
	repo := new(MockUserPreferencesRepository)
	repo.On("ReplaceAdminSavedFilters", mock.Anything, "u1", "users", mock.Anything, mock.Anything, int64(2)).
		Return(int64(0), repositories.ErrUserPreferencesVersionConflict)
	svc := NewUserPreferencesService(repo)

	_, _, err := svc.ReplaceAdminSavedFilters(context.Background(), "u1", "users",
		[]models.AdminSavedFilterPreset{savedPreset("A")}, 2)

	require.ErrorIs(t, err, repositories.ErrUserPreferencesVersionConflict)
}

// TestReplaceAdminSavedFilters_Rejects covers every validation rule; the repo
// is never called, so nothing is persisted.
func TestReplaceAdminSavedFilters_Rejects(t *testing.T) {
	tooMany := make([]models.AdminSavedFilterPreset, 0, 21)
	for i := range 21 {
		tooMany = append(tooMany, savedPreset(fmt.Sprintf("P%d", i)))
	}
	bigQuery := map[string]string{}
	for i := range 41 {
		bigQuery[fmt.Sprintf("k%d", i)] = "v"
	}
	dup := uuid.NewString()

	tests := []struct {
		name    string
		list    string
		version int64
		presets []models.AdminSavedFilterPreset
		wantMsg string
	}{
		{"unknown list", "widgets", 0, nil, "unknown saved-filter list"},
		{"negative version", "users", -1, nil, "version"},
		{"21 presets", "users", 0, tooMany, "too many presets"},
		{"case-insensitive duplicate", "users", 0,
			[]models.AdminSavedFilterPreset{savedPreset("Power users"), savedPreset(" POWER users")}, "used more than once"},
		{"empty name", "users", 0, []models.AdminSavedFilterPreset{savedPreset(" \t ")}, "name must be"},
		{"81-rune name", "users", 0, []models.AdminSavedFilterPreset{savedPreset(strings.Repeat("ü", 81))}, "name must be"},
		{"duplicate id", "users", 0, []models.AdminSavedFilterPreset{
			{ID: dup, Name: "A"}, {ID: dup, Name: "B"},
		}, "used more than once"},
		{"malformed id", "users", 0, []models.AdminSavedFilterPreset{{ID: "nope", Name: "A"}}, "not a UUID"},
		{"bad query key", "users", 0, []models.AdminSavedFilterPreset{
			{Name: "A", Query: map[string]string{"UPPER": "x"}},
		}, "query key"},
		{"long query key", "users", 0, []models.AdminSavedFilterPreset{
			{Name: "A", Query: map[string]string{strings.Repeat("a", 65): "x"}},
		}, "query key"},
		{"oversize query value", "users", 0, []models.AdminSavedFilterPreset{
			{Name: "A", Query: map[string]string{"q": strings.Repeat("é", 513)}},
		}, "exceeds 512"},
		{"too many query keys", "users", 0, []models.AdminSavedFilterPreset{{Name: "A", Query: bigQuery}}, "at most 40"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockUserPreferencesRepository)
			svc := NewUserPreferencesService(repo)

			_, _, err := svc.ReplaceAdminSavedFilters(context.Background(), "u1", tc.list, tc.presets, tc.version)

			var invalid *ErrAdminSavedFiltersInvalid
			require.ErrorAs(t, err, &invalid)
			assert.Contains(t, invalid.Detail, tc.wantMsg)
			repo.AssertNotCalled(t, "ReplaceAdminSavedFilters",
				mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// TestReplaceAdminSavedFilters_LimitsAreInclusive: exactly 20 presets, an
// 80-rune name, 40 keys and a 512-rune value are all accepted.
func TestReplaceAdminSavedFilters_LimitsAreInclusive(t *testing.T) {
	query := map[string]string{"q": strings.Repeat("é", 512)}
	for i := range 39 {
		query[fmt.Sprintf("k%d", i)] = "v"
	}
	presets := []models.AdminSavedFilterPreset{{Name: strings.Repeat("ü", 80), Query: query}}
	for i := range 19 {
		presets = append(presets, savedPreset(fmt.Sprintf("P%d", i)))
	}
	repo := new(MockUserPreferencesRepository)
	repo.On("ReplaceAdminSavedFilters", mock.Anything, "u1", "teams", mock.Anything, mock.Anything, int64(0)).
		Return(int64(1), nil)
	svc := NewUserPreferencesService(repo)

	got, _, err := svc.ReplaceAdminSavedFilters(context.Background(), "u1", "teams", presets, 0)

	require.NoError(t, err)
	assert.Len(t, got, 20)
}

func TestReplaceAdminSavedFilters_RepoErrorPassesThrough(t *testing.T) {
	repo := new(MockUserPreferencesRepository)
	repo.On("ReplaceAdminSavedFilters", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything).Return(int64(0), errors.New("db down"))
	svc := NewUserPreferencesService(repo)

	_, _, err := svc.ReplaceAdminSavedFilters(context.Background(), "u1", "users", nil, 0)

	require.EqualError(t, err, "db down")
}

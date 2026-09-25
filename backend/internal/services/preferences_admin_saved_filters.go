package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/models"
)

// Admin saved filter presets (#1147): an instance admin's named filter presets
// per admin list, stored in their own preferences row.

const (
	adminSavedFilterNameMaxRunes  = 80
	adminSavedFilterQueryMaxKeys  = 40
	adminSavedFilterValueMaxRunes = 512
)

// adminSavedFilterQueryKey matches every admin list's URL filter param
// (snake_case), which is all a preset's query ever holds.
var adminSavedFilterQueryKey = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// ErrAdminSavedFiltersInvalid reports a preset list that breaks a validation
// rule. Nothing is persisted when it is returned.
type ErrAdminSavedFiltersInvalid struct {
	Detail string
}

func (e *ErrAdminSavedFiltersInvalid) Error() string { return e.Detail }

func invalidAdminSavedFilters(format string, args ...any) error {
	return &ErrAdminSavedFiltersInvalid{Detail: fmt.Sprintf(format, args...)}
}

func validateAdminSavedFilterList(list string) error {
	switch list {
	case models.AdminSavedFilterListUsers, models.AdminSavedFilterListTeams, models.AdminSavedFilterListProjects:
		return nil
	}
	return invalidAdminSavedFilters("unknown saved-filter list %q: must be users, teams or projects", list)
}

// GetAdminSavedFilters returns the caller's presets for one admin list.
func (s *UserPreferencesService) GetAdminSavedFilters(
	ctx context.Context, userID, list string,
) ([]models.AdminSavedFilterPreset, int64, error) {
	if err := validateAdminSavedFilterList(list); err != nil {
		return nil, 0, err
	}
	return s.repo.GetAdminSavedFilters(ctx, userID, list)
}

// ReplaceAdminSavedFilters validates the presets, normalises them (trimmed
// names, a fresh id for each preset sent without one) and replaces the list
// under the version lock. A row created here is seeded with the default
// preferences so the admin's notification settings are not zeroed.
func (s *UserPreferencesService) ReplaceAdminSavedFilters(
	ctx context.Context, userID, list string, presets []models.AdminSavedFilterPreset, expectedVersion int64,
) ([]models.AdminSavedFilterPreset, int64, error) {
	if err := validateAdminSavedFilterList(list); err != nil {
		return nil, 0, err
	}
	if expectedVersion < 0 {
		return nil, 0, invalidAdminSavedFilters("version must not be negative")
	}
	normalized, err := normalizeAdminSavedFilters(presets)
	if err != nil {
		return nil, 0, err
	}

	version, err := s.repo.ReplaceAdminSavedFilters(
		ctx, userID, list, normalized, models.DefaultPreferences(), expectedVersion,
	)
	if err != nil {
		return nil, 0, err
	}
	return normalized, version, nil
}

// normalizeAdminSavedFilters applies every validation rule and returns the
// presets as they will be stored. The input is not modified.
func normalizeAdminSavedFilters(presets []models.AdminSavedFilterPreset) ([]models.AdminSavedFilterPreset, error) {
	if len(presets) > models.MaxAdminSavedFilterPresets {
		return nil, invalidAdminSavedFilters(
			"too many presets: %d, at most %d are allowed per list", len(presets), models.MaxAdminSavedFilterPresets,
		)
	}

	out := make([]models.AdminSavedFilterPreset, 0, len(presets))
	names := make(map[string]struct{}, len(presets))
	ids := make(map[string]struct{}, len(presets))
	for i, p := range presets {
		preset, err := normalizeAdminSavedFilter(i, p)
		if err != nil {
			return nil, err
		}

		nameKey := strings.ToLower(preset.Name)
		if _, dup := names[nameKey]; dup {
			return nil, invalidAdminSavedFilters("preset %d: name %q is used more than once", i, preset.Name)
		}
		names[nameKey] = struct{}{}

		if _, dup := ids[preset.ID]; dup {
			return nil, invalidAdminSavedFilters("preset %d: id %s is used more than once", i, preset.ID)
		}
		ids[preset.ID] = struct{}{}

		out = append(out, preset)
	}
	return out, nil
}

func normalizeAdminSavedFilter(i int, p models.AdminSavedFilterPreset) (models.AdminSavedFilterPreset, error) {
	name := strings.TrimSpace(p.Name)
	if n := utf8.RuneCountInString(name); n == 0 || n > adminSavedFilterNameMaxRunes {
		return models.AdminSavedFilterPreset{}, invalidAdminSavedFilters(
			"preset %d: name must be 1-%d characters after trimming", i, adminSavedFilterNameMaxRunes,
		)
	}

	id := p.ID
	if id == "" {
		id = uuid.NewString()
	} else if parsed, err := uuid.Parse(id); err != nil {
		return models.AdminSavedFilterPreset{}, invalidAdminSavedFilters("preset %d: id %q is not a UUID", i, id)
	} else {
		id = parsed.String()
	}

	if err := validateAdminSavedFilterQuery(i, p.Query); err != nil {
		return models.AdminSavedFilterPreset{}, err
	}
	query := make(map[string]string, len(p.Query))
	for k, v := range p.Query {
		query[k] = v
	}

	return models.AdminSavedFilterPreset{ID: id, Name: name, Query: query}, nil
}

func validateAdminSavedFilterQuery(i int, query map[string]string) error {
	if len(query) > adminSavedFilterQueryMaxKeys {
		return invalidAdminSavedFilters(
			"preset %d: query has %d keys, at most %d are allowed", i, len(query), adminSavedFilterQueryMaxKeys,
		)
	}
	for k, v := range query {
		if !adminSavedFilterQueryKey.MatchString(k) {
			return invalidAdminSavedFilters("preset %d: query key %q must match ^[a-z0-9_]{1,64}$", i, k)
		}
		if utf8.RuneCountInString(v) > adminSavedFilterValueMaxRunes {
			return invalidAdminSavedFilters(
				"preset %d: query value for %q exceeds %d characters", i, k, adminSavedFilterValueMaxRunes,
			)
		}
	}
	return nil
}

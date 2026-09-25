package server

import (
	"context"
	"errors"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
)

// Admin saved filter presets (#1147): the calling admin's own named presets per
// admin list, stored in their preferences row. No audit row is written — these
// are the admin's own preferences, not an action on another account.

const adminSavedFiltersConflictDetail = "Saved filters were changed elsewhere; reload and try again."

// GetAdminSavedFilters returns the caller's presets for one admin list.
func (a *adminStrictServer) GetAdminSavedFilters(
	ctx context.Context, request admingen.GetAdminSavedFiltersRequestObject,
) (admingen.GetAdminSavedFiltersResponseObject, error) {
	// The generated binder does not enforce a path-param enum.
	if !request.List.Valid() {
		return nil, apierrors.NewBadRequestError(unknownSavedFilterListDetail(request.List))
	}

	presets, version, err := a.s.container.UserPreferencesService().GetAdminSavedFilters(
		ctx, a.actingAdminID(ctx), string(request.List),
	)
	if err != nil {
		return nil, a.mapSavedFiltersError(err, "GetAdminSavedFilters")
	}

	out, err := toGenAdminSavedFilters(request.List, presets, version)
	if err != nil {
		return nil, a.mapSavedFiltersError(err, "GetAdminSavedFilters")
	}
	return admingen.GetAdminSavedFilters200JSONResponse(out), nil
}

// ReplaceAdminSavedFilters replaces the caller's presets for one admin list
// under the preferences row's version lock. Bodies carrying an unknown field
// are rejected before this runs, by rejectUnknownAdminBodyFields.
func (a *adminStrictServer) ReplaceAdminSavedFilters(
	ctx context.Context, request admingen.ReplaceAdminSavedFiltersRequestObject,
) (admingen.ReplaceAdminSavedFiltersResponseObject, error) {
	if !request.List.Valid() {
		return nil, apierrors.NewBadRequestError(unknownSavedFilterListDetail(request.List))
	}
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("Request body is required")
	}

	inputs := make([]models.AdminSavedFilterPreset, 0, len(request.Body.Presets))
	for _, p := range request.Body.Presets {
		preset := models.AdminSavedFilterPreset{Name: p.Name, Query: p.Query}
		if p.Id != nil {
			preset.ID = p.Id.String()
		}
		inputs = append(inputs, preset)
	}

	saved, version, err := a.s.container.UserPreferencesService().ReplaceAdminSavedFilters(
		ctx, a.actingAdminID(ctx), string(request.List), inputs, request.Body.Version,
	)
	if err != nil {
		return nil, a.mapSavedFiltersError(err, "ReplaceAdminSavedFilters")
	}

	out, err := toGenAdminSavedFilters(request.List, saved, version)
	if err != nil {
		return nil, a.mapSavedFiltersError(err, "ReplaceAdminSavedFilters")
	}
	return admingen.ReplaceAdminSavedFilters200JSONResponse(out), nil
}

func unknownSavedFilterListDetail(list admingen.AdminSavedFilterListName) string {
	return "Unknown saved-filter list \"" + string(list) + "\": must be users, teams or projects"
}

// mapSavedFiltersError turns validation failures into 400, a lost version race
// into 409, and anything else into a logged 500.
func (a *adminStrictServer) mapSavedFiltersError(err error, handler string) error {
	var invalid *services.ErrAdminSavedFiltersInvalid
	if errors.As(err, &invalid) {
		return apierrors.NewBadRequestError(invalid.Detail)
	}
	if errors.Is(err, repositories.ErrUserPreferencesVersionConflict) {
		return adminConflictError(adminSavedFiltersConflictDetail)
	}

	a.s.logger.With(
		"service", serverLogServiceName, "handler", handler, "error", err,
	).Error("Admin saved filters request failed")
	return apierrors.NewInternalError(adminMsgInternalError)
}

// toGenAdminSavedFilters converts the stored presets, with the required
// `presets` array and every `query` object built non-nil (#125).
func toGenAdminSavedFilters(
	list admingen.AdminSavedFilterListName, presets []models.AdminSavedFilterPreset, version int64,
) (admingen.AdminSavedFilters, error) {
	out := make([]admingen.AdminSavedFilterPreset, 0, len(presets))
	for _, p := range presets {
		id, err := parseAdminUUID("saved filter preset", p.ID)
		if err != nil {
			return admingen.AdminSavedFilters{}, err
		}
		query := admingen.AdminSavedFilterQuery{}
		for k, v := range p.Query {
			query[k] = v
		}
		out = append(out, admingen.AdminSavedFilterPreset{Id: id, Name: p.Name, Query: query})
	}
	return admingen.AdminSavedFilters{List: list, Presets: out, Version: version}, nil
}

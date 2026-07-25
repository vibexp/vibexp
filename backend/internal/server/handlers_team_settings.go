package server

import (
	"context"
	"errors"
	"net/http"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	teamsettingsgen "github.com/vibexp/vibexp/internal/server/gen/teamsettings"
	"github.com/vibexp/vibexp/internal/services"
)

const teamSettingsMsgInternalError = "Internal server error"

// teamSettingsMsgForbidden is what a caller without team.settings.update is told.
// Team settings change ranking for everyone in the team, so they are owner/admin
// surface (epic #487).
const teamSettingsMsgForbidden = "You do not have permission to change this team's settings."

// teamSettingsStrictServer implements teamsettingsgen.StrictServerInterface
// (epic #487): the per-team search ranking settings singleton at
// /api/v1/{team_id}/settings/search.
type teamSettingsStrictServer struct {
	s *Server
}

var _ teamsettingsgen.StrictServerInterface = (*teamSettingsStrictServer)(nil)

// GetTeamSearchSettings handles GET /api/v1/{team_id}/settings/search. Any team
// member may read; membership is enforced by the tenancy middleware, so there is
// no permission check here.
func (ts *teamSettingsStrictServer) GetTeamSearchSettings(
	ctx context.Context, request teamsettingsgen.GetTeamSearchSettingsRequestObject,
) (teamsettingsgen.GetTeamSearchSettingsResponseObject, error) {
	view, err := ts.s.container.TeamSearchSettingsService().Get(ctx, request.TeamId.String())
	if err != nil {
		return nil, ts.teamSettingsError("GetTeamSearchSettings", err)
	}
	return teamsettingsgen.GetTeamSearchSettings200JSONResponse(toGenTeamSearchSettings(view)), nil
}

// UpdateTeamSearchSettings handles PUT /api/v1/{team_id}/settings/search.
func (ts *teamSettingsStrictServer) UpdateTeamSearchSettings(
	ctx context.Context, request teamsettingsgen.UpdateTeamSearchSettingsRequestObject,
) (teamsettingsgen.UpdateTeamSearchSettingsResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}

	view, err := ts.s.container.TeamSearchSettingsService().Update(
		ctx, userID, request.TeamId.String(), valuesFromGenRequest(request.Body))
	if err != nil {
		return nil, ts.teamSettingsError("UpdateTeamSearchSettings", err)
	}
	return teamsettingsgen.UpdateTeamSearchSettings200JSONResponse(toGenTeamSearchSettings(view)), nil
}

// ResetTeamSearchSettings handles DELETE /api/v1/{team_id}/settings/search.
func (ts *teamSettingsStrictServer) ResetTeamSearchSettings(
	ctx context.Context, request teamsettingsgen.ResetTeamSearchSettingsRequestObject,
) (teamsettingsgen.ResetTeamSearchSettingsResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := ts.s.container.TeamSearchSettingsService().Reset(
		ctx, userID, request.TeamId.String()); err != nil {
		return nil, ts.teamSettingsError("ResetTeamSearchSettings", err)
	}
	return teamsettingsgen.ResetTeamSearchSettings204Response{}, nil
}

// valuesFromGenRequest projects the generated request body onto the domain
// profile. There is deliberately no rank_candidate_cap here — the request schema
// has no such field, so the cap cannot be set through this endpoint by
// construction rather than by a check that could be forgotten.
func valuesFromGenRequest(
	body *teamsettingsgen.UpdateTeamSearchSettingsJSONRequestBody,
) models.TeamSearchSettingsValues {
	return models.TeamSearchSettingsValues{
		RecencyRankingEnabled: body.RecencyRankingEnabled,
		RankWeightRelevance:   body.RankWeightRelevance,
		RankWeightCreated:     body.RankWeightCreated,
		RankWeightUpdated:     body.RankWeightUpdated,
		RankHalfLifeDays:      body.RankHalfLifeDays,
	}
}

// toGenTeamSearchSettings converts the service read model to the generated
// response type.
func toGenTeamSearchSettings(view *models.TeamSearchSettingsView) teamsettingsgen.TeamSearchSettings {
	return teamsettingsgen.TeamSearchSettings{
		Source:                teamsettingsgen.TeamSearchSettingsSource(view.Source),
		RecencyRankingEnabled: view.Values.RecencyRankingEnabled,
		RankWeightRelevance:   view.Values.RankWeightRelevance,
		RankWeightCreated:     view.Values.RankWeightCreated,
		RankWeightUpdated:     view.Values.RankWeightUpdated,
		RankHalfLifeDays:      view.Values.RankHalfLifeDays,
		InstanceDefaults:      toGenTeamSearchSettingsValues(view.InstanceDefaults),
		RankCandidateCap:      view.RankCandidateCap,
	}
}

func toGenTeamSearchSettingsValues(
	values models.TeamSearchSettingsValues,
) teamsettingsgen.TeamSearchSettingsValues {
	return teamsettingsgen.TeamSearchSettingsValues{
		RecencyRankingEnabled: values.RecencyRankingEnabled,
		RankWeightRelevance:   values.RankWeightRelevance,
		RankWeightCreated:     values.RankWeightCreated,
		RankWeightUpdated:     values.RankWeightUpdated,
		RankHalfLifeDays:      values.RankHalfLifeDays,
	}
}

// teamSettingsError maps a service error to the RFC 9457 error the strict
// response handler will write. An authorization failure must surface as 403 and
// a degenerate profile as 400 — reporting either as a generic 500 would leave
// the caller unable to tell a role problem from an outage.
func (ts *teamSettingsStrictServer) teamSettingsError(op string, err error) error {
	switch {
	case errors.Is(err, services.ErrPermissionDenied):
		return apierrors.NewForbiddenError(teamSettingsMsgForbidden)
	case errors.Is(err, services.ErrInvalidSearchSettings):
		return apierrors.NewBadRequestError(err.Error())
	default:
		ts.s.logger.With("error", err, "operation", op).Error("Team settings request failed")
		return apierrors.NewInternalError(teamSettingsMsgInternalError)
	}
}

// teamSettingsBindErrorHandler translates parameter-binding failures from the
// generated layer into this domain's RFC 9457 400 responses.
func (s *Server) teamSettingsBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()

	var invalidParam *teamsettingsgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) && invalidParam.ParamName == "team_id" {
		msg = "team_id must be a valid UUID"
	}

	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// teamSettingsResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) teamSettingsResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("TeamSettings strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(teamSettingsMsgInternalError))
}

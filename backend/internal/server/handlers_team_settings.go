package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	teamsettingsgen "github.com/vibexp/vibexp/internal/server/gen/teamsettings"
	"github.com/vibexp/vibexp/internal/services"
)

const teamSettingsMsgInternalError = "Internal server error"

// teamSettingsMsgForbidden is what a caller without team.settings.update is told.
// Team settings change ranking for everyone in the team, so they are owner/admin
// surface (epic #487).
//
// "manage" rather than "change" because team.settings.update now also gates a
// READ — the settings audit log (#832) — and telling a member they may not
// change something they only tried to list sends them looking for a write they
// never attempted. The word has to cover both, since teamSettingsError maps
// every ErrPermissionDenied on this tag through here.
const teamSettingsMsgForbidden = "You do not have permission to manage this team's settings."

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

// searchSettingsBodyFields is the exact set of JSON keys the update request
// accepts — the five writable profile fields, and nothing else.
var searchSettingsBodyFields = []string{
	"recency_ranking_enabled",
	"rank_weight_relevance",
	"rank_weight_created",
	"rank_weight_updated",
	"rank_half_life_days",
}

// searchSettingsBodyProblem returns the 400 message for a decoded update body
// that is not a complete, exact profile, or "" when the body is acceptable.
// Split out of the middleware so the rule is a pure function over the decoded
// keys, testable and simple on its own.
func searchSettingsBodyProblem(fields map[string]json.RawMessage) string {
	allowed := make(map[string]bool, len(searchSettingsBodyFields))
	for _, f := range searchSettingsBodyFields {
		allowed[f] = true
	}

	var unknown []string
	for key := range fields {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Sprintf(
			"Unknown field(s): %s. Only %s may be set; rank_candidate_cap is instance-owned.",
			strings.Join(unknown, ", "), strings.Join(searchSettingsBodyFields, ", "))
	}

	var missing []string
	for _, f := range searchSettingsBodyFields {
		if _, ok := fields[f]; !ok {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return fmt.Sprintf(
			"Missing required field(s): %s. This endpoint replaces the whole profile, "+
				"so every field must be supplied.", strings.Join(missing, ", "))
	}

	return ""
}

// requireCompleteSearchSettingsBody enforces on the wire what the spec declares
// for UpdateTeamSearchSettingsRequest: `additionalProperties: false` and all five
// fields required. oapi-codegen honours neither (the same gap that motivated
// rejectUnknownAdminBodyFields for the Admin domain, #455), and here silence
// would be worse than usual:
//
//   - an unknown key such as rank_candidate_cap would be dropped, so a caller
//     could believe they had raised an instance-owned limit;
//   - a MISSTYPED key means its real field is absent, and the generated struct's
//     non-pointer float leaves it 0 — which passes validation as long as the
//     other weights sum above zero. The team would end up with a profile blending
//     their values and silent zeroes, which is exactly the partial override this
//     epic's whole-row design forbids.
//
// Applied to PUT only; GET and DELETE carry no body.
func (s *Server) requireCompleteSearchSettingsBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Failed to read request body"))
			return
		}
		// Restore the body for the generated decoder regardless of the outcome.
		r.Body = io.NopCloser(bytes.NewReader(raw))

		// An empty or non-object body is the generated decoder's problem, not
		// ours; let it produce its usual error.
		var fields map[string]json.RawMessage
		if len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, &fields) != nil {
			next.ServeHTTP(w, r)
			return
		}

		if problem := searchSettingsBodyProblem(fields); problem != "" {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(problem))
			return
		}

		next.ServeHTTP(w, r)
	})
}

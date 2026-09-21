package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	teamsettingsgen "github.com/vibexp/vibexp/internal/server/gen/teamsettings"
	"github.com/vibexp/vibexp/internal/services"
)

// teamAISummarySettingsMsgInternalError mirrors teamSettingsMsgInternalError;
// kept separate so a future divergence in wording doesn't require touching the
// search-settings constant.
const teamAISummarySettingsMsgInternalError = "Internal server error"

// GetTeamAISummarySettings handles GET /api/v1/{team_id}/settings/ai-summary.
// Any team member may read; membership is enforced by the tenancy middleware,
// so there is no permission check here — mirrors GetTeamSearchSettings.
func (ts *teamSettingsStrictServer) GetTeamAISummarySettings(
	ctx context.Context, request teamsettingsgen.GetTeamAISummarySettingsRequestObject,
) (teamsettingsgen.GetTeamAISummarySettingsResponseObject, error) {
	view, err := ts.s.container.TeamAISummarySettingsService().Get(ctx, request.TeamId.String())
	if err != nil {
		return nil, ts.teamAISummarySettingsError("GetTeamAISummarySettings", err)
	}
	return teamsettingsgen.GetTeamAISummarySettings200JSONResponse(toGenTeamAISummarySettings(view)), nil
}

// UpdateTeamAISummarySettings handles PUT /api/v1/{team_id}/settings/ai-summary.
func (ts *teamSettingsStrictServer) UpdateTeamAISummarySettings(
	ctx context.Context, request teamsettingsgen.UpdateTeamAISummarySettingsRequestObject,
) (teamsettingsgen.UpdateTeamAISummarySettingsResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}

	view, err := ts.s.container.TeamAISummarySettingsService().Update(
		ctx, userID, request.TeamId.String(), aiSummaryValuesFromGenRequest(request.Body))
	if err != nil {
		return nil, ts.teamAISummarySettingsError("UpdateTeamAISummarySettings", err)
	}
	return teamsettingsgen.UpdateTeamAISummarySettings200JSONResponse(toGenTeamAISummarySettings(view)), nil
}

// ResetTeamAISummarySettings handles DELETE /api/v1/{team_id}/settings/ai-summary.
func (ts *teamSettingsStrictServer) ResetTeamAISummarySettings(
	ctx context.Context, request teamsettingsgen.ResetTeamAISummarySettingsRequestObject,
) (teamsettingsgen.ResetTeamAISummarySettingsResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := ts.s.container.TeamAISummarySettingsService().Reset(
		ctx, userID, request.TeamId.String()); err != nil {
		return nil, ts.teamAISummarySettingsError("ResetTeamAISummarySettings", err)
	}
	return teamsettingsgen.ResetTeamAISummarySettings204Response{}, nil
}

// aiSummaryValuesFromGenRequest projects the generated request body onto the
// domain profile.
func aiSummaryValuesFromGenRequest(
	body *teamsettingsgen.UpdateTeamAISummarySettingsJSONRequestBody,
) models.TeamAISummarySettingsValues {
	return models.TeamAISummarySettingsValues{
		Enabled:         body.Enabled,
		ModelProviderID: uuidPtrToStringPtr(body.ModelProviderId),
		TopN:            body.TopN,
		Style:           string(body.Style),
		MaxOutputTokens: body.MaxOutputTokens,
	}
}

// toGenTeamAISummarySettings converts the service read model to the generated
// response type.
func toGenTeamAISummarySettings(view *models.TeamAISummarySettingsView) teamsettingsgen.TeamAISummarySettings {
	return teamsettingsgen.TeamAISummarySettings{
		Source:           teamsettingsgen.TeamAISummarySettingsSource(view.Source),
		Values:           toGenTeamAISummarySettingsValues(view.Values),
		InstanceDefaults: toGenTeamAISummarySettingsValues(view.InstanceDefaults),
		MaxTopN:          view.MaxTopN,
		Available:        view.Available,
	}
}

func toGenTeamAISummarySettingsValues(
	values models.TeamAISummarySettingsValues,
) teamsettingsgen.TeamAISummarySettingsValues {
	return teamsettingsgen.TeamAISummarySettingsValues{
		Enabled:         values.Enabled,
		ModelProviderId: parseOptionalUUID(values.ModelProviderID),
		TopN:            values.TopN,
		Style:           teamsettingsgen.TeamAISummarySettingsValuesStyle(values.Style),
		MaxOutputTokens: values.MaxOutputTokens,
	}
}

// teamAISummarySettingsError maps a service error to the RFC 9457 error the
// strict response handler will write, mirroring teamSettingsStrictServer's
// teamSettingsError for the search-settings surface: an authorization failure
// is 403, a degenerate profile is 400, anything else is a logged 500.
func (ts *teamSettingsStrictServer) teamAISummarySettingsError(op string, err error) error {
	switch {
	case errors.Is(err, services.ErrPermissionDenied):
		return apierrors.NewForbiddenError(teamSettingsMsgForbidden)
	case errors.Is(err, services.ErrInvalidAISummarySettings):
		return apierrors.NewBadRequestError(err.Error())
	default:
		ts.s.logger.With("error", err, "operation", op).Error("Team AI summary settings request failed")
		return apierrors.NewInternalError(teamAISummarySettingsMsgInternalError)
	}
}

// aiSummarySettingsBodyFields is the exact set of JSON keys the update request
// accepts — the five writable profile fields, and nothing else.
var aiSummarySettingsBodyFields = []string{
	"enabled",
	"model_provider_id",
	"top_n",
	"style",
	"max_output_tokens",
}

// aiSummarySettingsBodyProblem returns the 400 message for a decoded update
// body that is not a complete, exact profile, or "" when the body is
// acceptable. Mirrors searchSettingsBodyProblem for this domain's field set.
func aiSummarySettingsBodyProblem(fields map[string]json.RawMessage) string {
	var unknown []string
	for key := range fields {
		if !slices.Contains(aiSummarySettingsBodyFields, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return "Unknown field(s): " + strings.Join(unknown, ", ") +
			". Only " + strings.Join(aiSummarySettingsBodyFields, ", ") +
			" may be set; max_top_n and available are computed."
	}

	var missing []string
	for _, f := range aiSummarySettingsBodyFields {
		if _, ok := fields[f]; !ok {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return "Missing required field(s): " + strings.Join(missing, ", ") +
			". This endpoint replaces the whole profile, so every field must be supplied."
	}

	return ""
}

// requireCompleteAISummarySettingsBody enforces on the wire what the spec
// declares for UpdateTeamAISummarySettingsRequest: `additionalProperties:
// false` and all five fields required (including model_provider_id, which is
// nullable but must still be PRESENT — a client clears it by sending `null`,
// not by omitting the key). oapi-codegen honours neither (see
// requireCompleteSettingsBodyFor), and this route group also serves the
// search-settings PUT, so this middleware only acts on the ai-summary path —
// applying it unconditionally would 400 every search-settings PUT using this
// domain's field names instead.
func (s *Server) requireCompleteAISummarySettingsBody(next http.Handler) http.Handler {
	return requireCompleteSettingsBodyFor("/settings/ai-summary", aiSummarySettingsBodyProblem)(next)
}

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
)

// Instance search ranking and AI summary settings for instance admins (#1200,
// epic #1196): thin adapters over InstanceSearchSettingsService (#1198) and
// InstanceAISummarySettingsService (#1199).
//
// Authorization is instanceAdminMiddleware, which 404s every non-admin and
// anonymous caller for the whole /api/v1/admin surface; the team-scoped authz
// matrix does not apply to the instance's own settings. The acting admin's id
// always comes from the auth context, never from a request body.

const (
	adminMsgInstanceSearchInvalid    = "The search settings are invalid"
	adminMsgInstanceAISummaryInvalid = "The AI summary settings are invalid"
)

// instanceSearchAuditSnapshotKeys is the allowlist of keys a search settings
// audit snapshot may surface (models.InstanceSearchSettingsValues' JSON shape).
var instanceSearchAuditSnapshotKeys = []string{
	"recency_ranking_enabled", "rank_weight_relevance", "rank_weight_created",
	"rank_weight_updated", "rank_half_life_days", "rank_candidate_cap",
}

// instanceAISummaryAuditSnapshotKeys is the allowlist of keys an AI summary
// settings audit snapshot may surface (the services package's audit shape,
// which records the timeout as request_timeout_ms).
var instanceAISummaryAuditSnapshotKeys = []string{
	"enabled", "top_n", "style", "max_output_tokens",
	"per_document_chars", "total_context_chars", "request_timeout_ms",
}

// --- search -------------------------------------------------------------------

// GetAdminSearchSettings returns the instance search ranking defaults in
// effect. It is never a 404: with nothing stored, the built-in defaults apply.
func (a *adminStrictServer) GetAdminSearchSettings(
	ctx context.Context, _ admingen.GetAdminSearchSettingsRequestObject,
) (admingen.GetAdminSearchSettingsResponseObject, error) {
	const handler = "GetAdminSearchSettings"

	view, err := a.s.container.InstanceSearchSettingsService().Get(ctx)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	resp, err := a.toGenAdminInstanceSearchSettings(ctx, view)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.GetAdminSearchSettings200JSONResponse(resp), nil
}

// UpdateAdminSearchSettings replaces the instance search ranking defaults.
func (a *adminStrictServer) UpdateAdminSearchSettings(
	ctx context.Context, request admingen.UpdateAdminSearchSettingsRequestObject,
) (admingen.UpdateAdminSearchSettingsResponseObject, error) {
	const handler = "UpdateAdminSearchSettings"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	view, err := a.s.container.InstanceSearchSettingsService().Update(
		ctx, a.actingAdminID(ctx), toModelInstanceSearchValues(*request.Body), request.Body.ExpectedVersion)
	if err != nil {
		return nil, a.mapInstanceSettingsError(handler, adminMsgInstanceSearchInvalid, err)
	}
	resp, err := a.toGenAdminInstanceSearchSettings(ctx, view)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.UpdateAdminSearchSettings200JSONResponse(resp), nil
}

// ResetAdminSearchSettings drops the stored defaults so the built-in ones
// apply. With nothing stored it is still a 204 and audits nothing.
func (a *adminStrictServer) ResetAdminSearchSettings(
	ctx context.Context, _ admingen.ResetAdminSearchSettingsRequestObject,
) (admingen.ResetAdminSearchSettingsResponseObject, error) {
	if err := a.s.container.InstanceSearchSettingsService().Reset(ctx, a.actingAdminID(ctx)); err != nil {
		return nil, a.adminInternalError("ResetAdminSearchSettings", err)
	}
	return admingen.ResetAdminSearchSettings204Response{}, nil
}

// ListAdminSearchSettingsAudit returns one page of the search settings' audit
// log, newest first.
func (a *adminStrictServer) ListAdminSearchSettingsAudit(
	ctx context.Context, request admingen.ListAdminSearchSettingsAuditRequestObject,
) (admingen.ListAdminSearchSettingsAuditResponseObject, error) {
	page, err := a.listInstanceSettingsAudit(ctx, "ListAdminSearchSettingsAudit",
		models.InstanceSettingSearch, request.Params.Limit, request.Params.Cursor,
		filterInstanceSearchAuditSnapshot)
	if err != nil {
		return nil, err
	}
	return admingen.ListAdminSearchSettingsAudit200JSONResponse(page), nil
}

// --- AI summary ---------------------------------------------------------------

// GetAdminAISummarySettings returns the instance AI summary defaults and
// budgets in effect. It is never a 404: with nothing stored, the built-in
// defaults apply.
func (a *adminStrictServer) GetAdminAISummarySettings(
	ctx context.Context, _ admingen.GetAdminAISummarySettingsRequestObject,
) (admingen.GetAdminAISummarySettingsResponseObject, error) {
	const handler = "GetAdminAISummarySettings"

	view, err := a.s.container.InstanceAISummarySettingsService().Get(ctx)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	resp, err := a.toGenAdminInstanceAISummarySettings(ctx, view)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.GetAdminAISummarySettings200JSONResponse(resp), nil
}

// UpdateAdminAISummarySettings replaces the instance AI summary defaults and
// budgets.
func (a *adminStrictServer) UpdateAdminAISummarySettings(
	ctx context.Context, request admingen.UpdateAdminAISummarySettingsRequestObject,
) (admingen.UpdateAdminAISummarySettingsResponseObject, error) {
	const handler = "UpdateAdminAISummarySettings"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	view, err := a.s.container.InstanceAISummarySettingsService().Update(
		ctx, a.actingAdminID(ctx), toModelInstanceAISummaryValues(*request.Body), request.Body.ExpectedVersion)
	if err != nil {
		return nil, a.mapInstanceSettingsError(handler, adminMsgInstanceAISummaryInvalid, err)
	}
	resp, err := a.toGenAdminInstanceAISummarySettings(ctx, view)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.UpdateAdminAISummarySettings200JSONResponse(resp), nil
}

// ResetAdminAISummarySettings drops the stored settings so the built-in ones
// apply. With nothing stored it is still a 204 and audits nothing.
func (a *adminStrictServer) ResetAdminAISummarySettings(
	ctx context.Context, _ admingen.ResetAdminAISummarySettingsRequestObject,
) (admingen.ResetAdminAISummarySettingsResponseObject, error) {
	if err := a.s.container.InstanceAISummarySettingsService().Reset(ctx, a.actingAdminID(ctx)); err != nil {
		return nil, a.adminInternalError("ResetAdminAISummarySettings", err)
	}
	return admingen.ResetAdminAISummarySettings204Response{}, nil
}

// ListAdminAISummarySettingsAudit returns one page of the AI summary settings'
// audit log, newest first.
func (a *adminStrictServer) ListAdminAISummarySettingsAudit(
	ctx context.Context, request admingen.ListAdminAISummarySettingsAuditRequestObject,
) (admingen.ListAdminAISummarySettingsAuditResponseObject, error) {
	page, err := a.listInstanceSettingsAudit(ctx, "ListAdminAISummarySettingsAudit",
		models.InstanceSettingAISummary, request.Params.Limit, request.Params.Cursor,
		filterInstanceAISummaryAuditSnapshot)
	if err != nil {
		return nil, err
	}
	return admingen.ListAdminAISummarySettingsAudit200JSONResponse(page), nil
}

// --- errors -------------------------------------------------------------------

// mapInstanceSettingsError turns an Update error into its HTTP shape: invalid
// values are a 400 naming the offending fields, a stale expected_version is a
// 409, and anything else is a logged 500 (the error, never the request body).
func (a *adminStrictServer) mapInstanceSettingsError(handler, invalidMsg string, err error) error {
	switch {
	case errors.Is(err, services.ErrInvalidSearchSettings),
		errors.Is(err, services.ErrInvalidInstanceAISummarySettings):
		return apierrors.NewInstanceSettingsValidationError(invalidMsg, instanceSettingsValidationErrors(err))
	case errors.Is(err, repositories.ErrInstanceSettingsVersionConflict):
		return apierrors.NewInstanceSettingsVersionConflictError()
	default:
		return a.adminInternalError(handler, err)
	}
}

// instanceSettingsValidationErrors lists one validation error per field the
// validator attributed the failure to. A rule spanning several fields (the
// weights must not all be zero) names each of them with the same message.
func instanceSettingsValidationErrors(err error) []apierrors.ValidationError {
	var fieldErr *services.SettingsFieldError
	if !errors.As(err, &fieldErr) {
		return nil
	}
	out := make([]apierrors.ValidationError, 0, len(fieldErr.Fields))
	for _, field := range fieldErr.Fields {
		out = append(out, apierrors.NewFieldValidationError(field, fieldErr.Message, "OUT_OF_RANGE"))
	}
	return out
}

// --- request mapping ----------------------------------------------------------

func toModelInstanceSearchValues(body admingen.AdminInstanceSearchSettingsUpdate) models.InstanceSearchSettingsValues {
	return models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: body.RecencyRankingEnabled,
		RankWeightRelevance:   body.RankWeightRelevance,
		RankWeightCreated:     body.RankWeightCreated,
		RankWeightUpdated:     body.RankWeightUpdated,
		RankHalfLifeDays:      body.RankHalfLifeDays,
		RankCandidateCap:      body.RankCandidateCap,
	}
}

func toModelInstanceAISummaryValues(
	body admingen.AdminInstanceAISummarySettingsUpdate,
) models.InstanceAISummarySettingsValues {
	return models.InstanceAISummarySettingsValues{
		Enabled:           body.Enabled,
		TopN:              body.TopN,
		Style:             string(body.Style),
		MaxOutputTokens:   body.MaxOutputTokens,
		PerDocumentChars:  body.PerDocumentChars,
		TotalContextChars: body.TotalContextChars,
		RequestTimeout:    requestTimeoutFromMS(body.RequestTimeoutMs),
	}
}

// requestTimeoutFromMS converts request_timeout_ms to a duration for the
// validator. The value is first clamped to [0, math.MaxInt32+1] — each end one
// step outside the storable range — so the validator still rejects an
// out-of-range value, naming the field, while a huge one cannot overflow the
// nanosecond conversion into a value that looks valid.
func requestTimeoutFromMS(ms int) time.Duration {
	clamped := min(max(ms, 0), math.MaxInt32+1)
	return time.Duration(clamped) * time.Millisecond
}

// --- response mapping ---------------------------------------------------------

func (a *adminStrictServer) toGenAdminInstanceSearchSettings(
	ctx context.Context, view *models.InstanceSearchSettingsView,
) (admingen.AdminInstanceSearchSettings, error) {
	teams, err := a.s.container.AdminService().CountTeamsWithSearchSettingsOverride(ctx)
	if err != nil {
		return admingen.AdminInstanceSearchSettings{}, err
	}
	updater, err := a.instanceSettingsUpdater(ctx, view.UpdatedBy)
	if err != nil {
		return admingen.AdminInstanceSearchSettings{}, err
	}
	return admingen.AdminInstanceSearchSettings{
		Values:          toGenAdminInstanceSearchValues(view.Values),
		Source:          admingen.AdminInstanceSettingsSource(view.Source),
		BuiltInDefaults: toGenAdminInstanceSearchValues(services.BuiltInSearchDefaults()),
		Limits: admingen.AdminInstanceSearchLimits{
			RankWeightMin:       0,
			RankHalfLifeDaysMax: models.MaxSearchRankHalfLifeDays,
			RankCandidateCapMin: 1,
			RankCandidateCapMax: models.MaxSearchRankCandidateCap,
		},
		TeamsWithOverride: teams,
		UpdatedAt:         view.UpdatedAt,
		UpdatedByUserId:   updater.id,
		UpdatedByName:     updater.name,
		Version:           view.Version,
	}, nil
}

func toGenAdminInstanceSearchValues(v models.InstanceSearchSettingsValues) admingen.AdminInstanceSearchValues {
	return admingen.AdminInstanceSearchValues{
		RecencyRankingEnabled: v.RecencyRankingEnabled,
		RankWeightRelevance:   v.RankWeightRelevance,
		RankWeightCreated:     v.RankWeightCreated,
		RankWeightUpdated:     v.RankWeightUpdated,
		RankHalfLifeDays:      v.RankHalfLifeDays,
		RankCandidateCap:      v.RankCandidateCap,
	}
}

func (a *adminStrictServer) toGenAdminInstanceAISummarySettings(
	ctx context.Context, view *models.InstanceAISummarySettingsView,
) (admingen.AdminInstanceAISummarySettings, error) {
	teams, err := a.s.container.AdminService().CountTeamsWithAISummarySettingsOverride(ctx)
	if err != nil {
		return admingen.AdminInstanceAISummarySettings{}, err
	}
	updater, err := a.instanceSettingsUpdater(ctx, view.UpdatedBy)
	if err != nil {
		return admingen.AdminInstanceAISummarySettings{}, err
	}
	return admingen.AdminInstanceAISummarySettings{
		Values:          toGenAdminInstanceAISummaryValues(view.Values),
		Source:          admingen.AdminInstanceSettingsSource(view.Source),
		BuiltInDefaults: toGenAdminInstanceAISummaryValues(models.DefaultInstanceAISummarySettings()),
		Limits: admingen.AdminInstanceAISummaryLimits{
			TopNMin:             1,
			TopNMax:             models.MaxAISummaryTopN,
			MaxOutputTokensMin:  1,
			MaxOutputTokensMax:  models.MaxAISummaryOutputTokens,
			CharsMin:            1,
			CharsMax:            math.MaxInt32,
			RequestTimeoutMsMin: 1,
			RequestTimeoutMsMax: math.MaxInt32,
		},
		TeamsWithOverride: teams,
		UpdatedAt:         view.UpdatedAt,
		UpdatedByUserId:   updater.id,
		UpdatedByName:     updater.name,
		Version:           view.Version,
	}, nil
}

func toGenAdminInstanceAISummaryValues(v models.InstanceAISummarySettingsValues) admingen.AdminInstanceAISummaryValues {
	return admingen.AdminInstanceAISummaryValues{
		Enabled:           v.Enabled,
		TopN:              v.TopN,
		Style:             admingen.AdminInstanceAISummaryStyle(v.Style),
		MaxOutputTokens:   v.MaxOutputTokens,
		PerDocumentChars:  v.PerDocumentChars,
		TotalContextChars: v.TotalContextChars,
		RequestTimeoutMs:  int(v.RequestTimeout.Milliseconds()),
	}
}

// instanceSettingsUpdaterRef is the id and resolved display name of the admin
// who last stored an instance setting.
type instanceSettingsUpdaterRef struct {
	id   *openapi_types.UUID
	name *string
}

// instanceSettingsUpdater resolves updated_by to its id and display name. A
// name lookup failure degrades to a null name rather than failing the read, as
// the audit list's actor names do.
func (a *adminStrictServer) instanceSettingsUpdater(
	ctx context.Context, updatedBy *string,
) (instanceSettingsUpdaterRef, error) {
	id, err := optionalGenUUID(updatedBy)
	if err != nil {
		return instanceSettingsUpdaterRef{}, fmt.Errorf("updated_by: %w", err)
	}
	if id == nil {
		return instanceSettingsUpdaterRef{}, nil
	}
	ref := instanceSettingsUpdaterRef{id: id}
	names, err := a.s.container.UserRepository().GetNamesByIDs(ctx, []string{*updatedBy})
	if err != nil {
		a.s.logger.Warn("Failed to resolve instance settings updater name",
			"service", serverLogServiceName, "error", err)
		return ref, nil
	}
	if name, ok := names[*updatedBy]; ok {
		ref.name = &name
	}
	return ref, nil
}

// --- audit snapshots ----------------------------------------------------------

// filterInstanceSearchAuditSnapshot keeps only the search snapshot's
// allowlisted keys, so a key added to the stored shape later cannot reach this
// surface unreviewed.
func filterInstanceSearchAuditSnapshot(raw json.RawMessage) *map[string]interface{} {
	return filterInstanceAuditSnapshot(raw, instanceSearchAuditSnapshotKeys)
}

// filterInstanceAISummaryAuditSnapshot keeps only the AI summary snapshot's
// allowlisted keys.
func filterInstanceAISummaryAuditSnapshot(raw json.RawMessage) *map[string]interface{} {
	return filterInstanceAuditSnapshot(raw, instanceAISummaryAuditSnapshotKeys)
}

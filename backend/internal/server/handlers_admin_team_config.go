package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
)

// Read-only team configuration for instance admins (#1140, epic #1131
// decision 9), one op per settings section.
//
// Every response is an ADMIN-ONLY DTO mapped field by field from the service
// read model — never a team-scoped response type re-used or re-marshalled — so
// a field added to a team settings response cannot reach this surface until it
// is deliberately added to schemas/admin.yaml and to a converter below. The
// exact key set of every response is pinned by admin_noleak_test.go.
//
// The getters these handlers call take only a team id and carry no authz of
// their own; that is safe here because instanceAdminMiddleware is the boundary
// for the whole /api/v1/admin surface. The settings audit log is the exception
// in the service layer — TeamSettingsAuditService.ListAudit role-checks — so
// the audit op reads the repository directly and shares the extracted name
// resolver instead of bypassing or weakening that check.

// adminArtifactResourceType is the only resource type TypeService supports.
const adminArtifactResourceType = "artifacts"

// requireAdminTeam confirms the team exists before a config read. Without it
// the settings getters would answer an unknown id with the instance defaults
// (`source: instance`) instead of a 404.
func (a *adminStrictServer) requireAdminTeam(ctx context.Context, handler, teamID string) error {
	if _, err := a.s.container.TeamRepository().GetByID(ctx, teamID); err != nil {
		if errors.Is(err, repositories.ErrTeamNotFound) {
			return apierrors.NewResourceNotFoundError("team", "Team not found")
		}
		return a.adminConfigInternalError(handler, teamID, err)
	}
	return nil
}

// adminConfigInternalError logs an unexpected failure and returns the generic
// problem detail.
func (a *adminStrictServer) adminConfigInternalError(handler, teamID string, err error) error {
	a.s.logger.With(
		"service", serverLogServiceName,
		"handler", handler,
		"team_id", teamID,
		"error", err,
	).Error("Failed to read admin team configuration")
	return apierrors.NewInternalError(adminMsgInternalError)
}

// GetAdminTeamSearchConfig returns the team's effective search ranking settings.
func (a *adminStrictServer) GetAdminTeamSearchConfig(
	ctx context.Context, request admingen.GetAdminTeamSearchConfigRequestObject,
) (admingen.GetAdminTeamSearchConfigResponseObject, error) {
	const handler = "GetAdminTeamSearchConfig"
	teamID := request.Id.String()
	if err := a.requireAdminTeam(ctx, handler, teamID); err != nil {
		return nil, err
	}

	view, err := a.s.container.TeamSearchSettingsService().Get(ctx, teamID)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	return admingen.GetAdminTeamSearchConfig200JSONResponse(admingen.AdminTeamSearchConfig{
		Source:           admingen.AdminTeamConfigSource(view.Source),
		Values:           toGenAdminSearchValues(view.Values),
		InstanceDefaults: toGenAdminSearchValues(view.InstanceDefaults),
		RankCandidateCap: view.RankCandidateCap,
	}), nil
}

func toGenAdminSearchValues(v models.TeamSearchSettingsValues) admingen.AdminSearchValues {
	return admingen.AdminSearchValues{
		RecencyRankingEnabled: v.RecencyRankingEnabled,
		RankWeightRelevance:   v.RankWeightRelevance,
		RankWeightCreated:     v.RankWeightCreated,
		RankWeightUpdated:     v.RankWeightUpdated,
		RankHalfLifeDays:      v.RankHalfLifeDays,
	}
}

// GetAdminTeamAISummaryConfig returns the team's effective AI summary settings
// plus the selected model provider's name.
func (a *adminStrictServer) GetAdminTeamAISummaryConfig(
	ctx context.Context, request admingen.GetAdminTeamAISummaryConfigRequestObject,
) (admingen.GetAdminTeamAISummaryConfigResponseObject, error) {
	const handler = "GetAdminTeamAISummaryConfig"
	teamID := request.Id.String()
	if err := a.requireAdminTeam(ctx, handler, teamID); err != nil {
		return nil, err
	}

	view, err := a.s.container.TeamAISummarySettingsService().Get(ctx, teamID)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	values, err := toGenAdminAISummaryValues(view.Values)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}
	defaults, err := toGenAdminAISummaryValues(view.InstanceDefaults)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	providerName, err := a.adminModelProviderName(ctx, teamID, view.Values.ModelProviderID)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	return admingen.GetAdminTeamAISummaryConfig200JSONResponse(admingen.AdminTeamAISummaryConfig{
		Source:                 admingen.AdminTeamConfigSource(view.Source),
		Values:                 values,
		InstanceDefaults:       defaults,
		ModelProviderName:      providerName,
		MaxTopN:                view.MaxTopN,
		MaxOutputTokensCeiling: view.MaxOutputTokensCeiling,
		Available:              view.Available,
	}), nil
}

// adminModelProviderName resolves the selected provider's display name. Only
// the name is read off the provider row — never its config or credential. A
// provider that no longer resolves is the documented `null`, not an error.
func (a *adminStrictServer) adminModelProviderName(
	ctx context.Context, teamID string, providerID *string,
) (*string, error) {
	if providerID == nil || *providerID == "" {
		return nil, nil
	}
	provider, err := a.s.container.ModelProviderRepository().GetByID(ctx, teamID, *providerID)
	if err != nil {
		if errors.Is(err, repositories.ErrModelProviderNotFound) {
			return nil, nil
		}
		return nil, err
	}
	name := provider.Name
	return &name, nil
}

func toGenAdminAISummaryValues(v models.TeamAISummarySettingsValues) (admingen.AdminAISummaryValues, error) {
	providerID, err := optionalGenUUID(v.ModelProviderID)
	if err != nil {
		return admingen.AdminAISummaryValues{}, fmt.Errorf("model_provider_id: %w", err)
	}
	return admingen.AdminAISummaryValues{
		Enabled:         v.Enabled,
		ModelProviderId: providerID,
		TopN:            v.TopN,
		Style:           admingen.AdminAISummaryValuesStyle(v.Style),
		MaxOutputTokens: v.MaxOutputTokens,
	}, nil
}

// GetAdminTeamFreshnessConfig returns the team's freshness settings and rules.
func (a *adminStrictServer) GetAdminTeamFreshnessConfig(
	ctx context.Context, request admingen.GetAdminTeamFreshnessConfigRequestObject,
) (admingen.GetAdminTeamFreshnessConfigResponseObject, error) {
	const handler = "GetAdminTeamFreshnessConfig"
	teamID := request.Id.String()
	if err := a.requireAdminTeam(ctx, handler, teamID); err != nil {
		return nil, err
	}

	freshness := a.s.container.FreshnessService()
	view, err := freshness.GetSettings(ctx, teamID)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}
	rules, err := freshness.ListRules(ctx, teamID)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	genRules, err := toGenAdminFreshnessRules(rules)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	return admingen.GetAdminTeamFreshnessConfig200JSONResponse(admingen.AdminTeamFreshnessConfig{
		Source:   admingen.AdminTeamConfigSource(view.Source),
		Values:   toGenAdminFreshnessValues(view.Values),
		Defaults: toGenAdminFreshnessValues(view.Defaults),
		Rules:    genRules,
	}), nil
}

func toGenAdminFreshnessValues(v models.FreshnessSettingsValues) admingen.AdminFreshnessValues {
	return admingen.AdminFreshnessValues{
		IntervalSeconds:      v.IntervalSeconds,
		ReversibilityEnabled: v.ReversibilityEnabled,
	}
}

func toGenAdminFreshnessRule(r *models.FreshnessRule) (admingen.AdminFreshnessRule, error) {
	id, err := parseAdminUUID("freshness rule", r.ID)
	if err != nil {
		return admingen.AdminFreshnessRule{}, err
	}
	projectID, err := optionalGenUUID(r.ProjectID)
	if err != nil {
		return admingen.AdminFreshnessRule{}, fmt.Errorf("freshness rule project_id: %w", err)
	}
	return admingen.AdminFreshnessRule{
		Id:            id,
		ProjectId:     projectID,
		ResourceTypes: nonNilStrings(r.ResourceTypes),
		Mediums:       nonNilStrings(r.Mediums),
		ThresholdDays: r.ThresholdDays,
		Enabled:       r.Enabled,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}, nil
}

// toGenAdminFreshnessRules converts rules through toGenAdminFreshnessRule. The
// result is make(...,0): both config arrays are required, so an empty rule set
// serializes as `[]`, not `null`.
func toGenAdminFreshnessRules(rules []*models.FreshnessRule) ([]admingen.AdminFreshnessRule, error) {
	out := make([]admingen.AdminFreshnessRule, 0, len(rules))
	for _, rule := range rules {
		converted, err := toGenAdminFreshnessRule(rule)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

// GetAdminTeamArtifactTypes returns the system and custom artifact types the
// team sees.
func (a *adminStrictServer) GetAdminTeamArtifactTypes(
	ctx context.Context, request admingen.GetAdminTeamArtifactTypesRequestObject,
) (admingen.GetAdminTeamArtifactTypesResponseObject, error) {
	const handler = "GetAdminTeamArtifactTypes"
	teamID := request.Id.String()
	if err := a.requireAdminTeam(ctx, handler, teamID); err != nil {
		return nil, err
	}

	types, err := a.s.container.TypeService().List(ctx, teamID, adminArtifactResourceType)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}

	// make(...,0,...): `types` is a required array on a generated type.
	genTypes := make([]admingen.AdminArtifactType, 0, len(types))
	for _, t := range types {
		id, perr := parseAdminUUID("type", t.ID)
		if perr != nil {
			return nil, a.adminConfigInternalError(handler, teamID, perr)
		}
		genTypes = append(genTypes, admingen.AdminArtifactType{
			Id:        id,
			Slug:      t.Slug,
			Name:      t.Name,
			IsSystem:  t.IsSystem,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
		})
	}

	return admingen.GetAdminTeamArtifactTypes200JSONResponse(admingen.AdminTeamArtifactTypes{
		Types: genTypes,
	}), nil
}

// settingsAuditDetailAllowlist is the closed set of `detail` keys each copy
// surface writes today (model_provider_copy.go, embedding_provider_copy.go,
// type.go). Anything else stored on a row is dropped on this surface —
// fail-closed, so a future writer that puts something sensitive in `detail`
// cannot leak it to the admin view by accident. An unknown surface keeps no
// keys at all.
var settingsAuditDetailAllowlist = map[string][]string{
	models.SettingsAuditSurfaceModelProvider: {
		"source_name", "created_name", "provider_type", "model", "has_api_key",
	},
	models.SettingsAuditSurfaceEmbeddingProvider: {
		"source_name", "created_name", "provider_type", "model", "has_api_key",
		"becomes_active", "displaced_model", "displaced_embedded_resources",
	},
	models.SettingsAuditSurfaceCustomTypes: {
		"added_ids", "added_slugs", "skipped_slugs",
	},
}

// filterSettingsAuditDetail decodes a stored detail and keeps only the
// surface's allowlisted keys. It always returns a non-nil map, since `detail`
// is a required, non-nullable object.
func filterSettingsAuditDetail(surface string, raw json.RawMessage) map[string]interface{} {
	allowed := settingsAuditDetailAllowlist[surface]
	decoded := auditDetailObject(raw)
	filtered := make(map[string]interface{}, len(allowed))
	for key, value := range decoded {
		if slices.Contains(allowed, key) {
			filtered[key] = value
		}
	}
	return filtered
}

// ListAdminTeamSettingsAudit returns one page of the team's settings audit log.
func (a *adminStrictServer) ListAdminTeamSettingsAudit(
	ctx context.Context, request admingen.ListAdminTeamSettingsAuditRequestObject,
) (admingen.ListAdminTeamSettingsAuditResponseObject, error) {
	const handler = "ListAdminTeamSettingsAudit"
	teamID := request.Id.String()

	page, limit, err := boundSettingsAuditPaging(request.Params.Page, request.Params.Limit)
	if err != nil {
		return nil, err
	}
	if guardErr := a.requireAdminTeam(ctx, handler, teamID); guardErr != nil {
		return nil, guardErr
	}

	rows, total, err := a.s.container.TeamSettingsAuditRepository().
		ListByTeam(ctx, teamID, limit, (page-1)*limit)
	if err != nil {
		return nil, a.adminConfigInternalError(handler, teamID, err)
	}
	views := services.ResolveSettingsAuditNames(ctx,
		a.s.container.UserRepository(), a.s.container.TeamRepository(),
		a.s.logger, teamID, rows)

	// make(...,0,...): `entries` is a required array on a generated type.
	entries := make([]admingen.AdminTeamSettingsAuditEntry, 0, len(views))
	for _, view := range views {
		converted, cerr := toGenAdminTeamSettingsAuditEntry(view)
		if cerr != nil {
			return nil, a.adminConfigInternalError(handler, teamID, cerr)
		}
		entries = append(entries, converted)
	}

	return admingen.ListAdminTeamSettingsAudit200JSONResponse(admingen.AdminTeamSettingsAuditListResponse{
		Entries:    entries,
		TotalCount: total,
		Page:       page,
		PerPage:    limit,
		TotalPages: totalPagesFor(total, limit),
	}), nil
}

func toGenAdminTeamSettingsAuditEntry(
	view *models.TeamSettingsAuditEntryView,
) (admingen.AdminTeamSettingsAuditEntry, error) {
	entry := view.Entry

	id, err := parseAdminUUID("settings audit entry", entry.ID)
	if err != nil {
		return admingen.AdminTeamSettingsAuditEntry{}, err
	}
	actorID, err := optionalGenUUID(entry.ActorUserID)
	if err != nil {
		return admingen.AdminTeamSettingsAuditEntry{}, fmt.Errorf("actor_user_id: %w", err)
	}
	sourceTeamID, err := optionalGenUUID(entry.SourceTeamID)
	if err != nil {
		return admingen.AdminTeamSettingsAuditEntry{}, fmt.Errorf("source_team_id: %w", err)
	}
	sourceResourceID, err := optionalGenUUID(entry.SourceResourceID)
	if err != nil {
		return admingen.AdminTeamSettingsAuditEntry{}, fmt.Errorf("source_resource_id: %w", err)
	}
	createdResourceID, err := optionalGenUUID(entry.CreatedResourceID)
	if err != nil {
		return admingen.AdminTeamSettingsAuditEntry{}, fmt.Errorf("created_resource_id: %w", err)
	}

	return admingen.AdminTeamSettingsAuditEntry{
		Id:                id,
		Surface:           admingen.AdminTeamSettingsAuditEntrySurface(entry.Surface),
		ActorUserId:       actorID,
		ActorName:         view.ActorName,
		SourceTeamId:      sourceTeamID,
		SourceTeamName:    view.SourceTeamName,
		SourceResourceId:  sourceResourceID,
		CreatedResourceId: createdResourceID,
		Detail:            filterSettingsAuditDetail(entry.Surface, entry.Detail),
		CreatedAt:         entry.CreatedAt,
	}, nil
}

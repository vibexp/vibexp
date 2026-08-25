package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	teamsettingsgen "github.com/vibexp/vibexp/internal/server/gen/teamsettings"
)

// Settings audit log read path (issue #832, epic #827).
//
// It lives beside handlers_team_settings.go rather than in it because it is a
// second surface on the same tag with its own paging and conversion helpers —
// the same split handlers_freshness_metrics.go makes from handlers_freshness.go.
//
// Unlike the search-settings GET next door, this read is NOT satisfied by the
// tenancy middleware alone: the service authorizes team.settings.update on top
// of it, so a plain member gets a 403 rather than the log of who copied which
// credential-bearing configuration in from where.

const (
	// defaultSettingsAuditLimit and maxSettingsAuditLimit mirror the `limit`
	// query parameter's default and maximum in the spec; maxSettingsAuditPage
	// mirrors the `page` maximum. oapi-codegen generates neither defaults nor
	// bound checks for query parameters, so the handler applies both — the
	// spec would otherwise document a contract nothing enforces.
	defaultSettingsAuditLimit = 20
	maxSettingsAuditLimit     = 100
	maxSettingsAuditPage      = 1_000_000

	settingsAuditMsgInvalidPage  = "page must be between 1 and 1000000"
	settingsAuditMsgInvalidLimit = "limit must be between 1 and 100"
)

// ListTeamSettingsAudit returns one page of the team's settings audit log.
func (ts *teamSettingsStrictServer) ListTeamSettingsAudit(
	ctx context.Context, request teamsettingsgen.ListTeamSettingsAuditRequestObject,
) (teamsettingsgen.ListTeamSettingsAuditResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	page, limit, err := settingsAuditPaging(request.Params)
	if err != nil {
		return nil, err
	}

	result, err := ts.s.container.TeamSettingsAuditService().
		ListAudit(ctx, userID, request.TeamId.String(), page, limit)
	if err != nil {
		return nil, ts.teamSettingsError("ListTeamSettingsAudit", err)
	}

	// make(...,0,...) rather than a nil slice: `entries` is a required array,
	// and the generated type cannot carry the models.JSONArray shim that
	// guarantees `[]` for the hand-marshaled payloads, so an empty page would
	// otherwise serialize as `null`. See required_array_null_test.go.
	entries := make([]teamsettingsgen.TeamSettingsAuditEntry, 0, len(result.Entries))
	for _, view := range result.Entries {
		converted, cerr := toGenTeamSettingsAuditEntry(view)
		if cerr != nil {
			return nil, ts.teamSettingsError("ListTeamSettingsAudit", cerr)
		}
		entries = append(entries, converted)
	}

	return teamsettingsgen.ListTeamSettingsAudit200JSONResponse{
		Entries:    entries,
		TotalCount: result.TotalCount,
		Page:       result.Page,
		PerPage:    result.PerPage,
		TotalPages: totalPagesFor(result.TotalCount, result.PerPage),
	}, nil
}

// settingsAuditPaging applies the spec's defaults and bounds to the optional
// page/limit query parameters.
func settingsAuditPaging(params teamsettingsgen.ListTeamSettingsAuditParams) (page, limit int, err error) {
	page, limit = 1, defaultSettingsAuditLimit
	if params.Page != nil {
		page = *params.Page
	}
	if params.Limit != nil {
		limit = *params.Limit
	}

	if page < 1 || page > maxSettingsAuditPage {
		return 0, 0, apierrors.NewBadRequestError(settingsAuditMsgInvalidPage)
	}
	if limit < 1 || limit > maxSettingsAuditLimit {
		return 0, 0, apierrors.NewBadRequestError(settingsAuditMsgInvalidLimit)
	}
	return page, limit, nil
}

// toGenTeamSettingsAuditEntry converts one stored entry plus its resolved names
// into the generated payload.
func toGenTeamSettingsAuditEntry(
	view *models.TeamSettingsAuditEntryView,
) (teamsettingsgen.TeamSettingsAuditEntry, error) {
	entry := view.Entry

	id, err := uuid.Parse(entry.ID)
	if err != nil {
		return teamsettingsgen.TeamSettingsAuditEntry{},
			fmt.Errorf("settings audit entry id %q is not a uuid: %w", entry.ID, err)
	}

	actorID, err := optionalGenUUID(entry.ActorUserID)
	if err != nil {
		return teamsettingsgen.TeamSettingsAuditEntry{}, fmt.Errorf("actor_user_id: %w", err)
	}
	sourceTeamID, err := optionalGenUUID(entry.SourceTeamID)
	if err != nil {
		return teamsettingsgen.TeamSettingsAuditEntry{}, fmt.Errorf("source_team_id: %w", err)
	}
	sourceResourceID, err := optionalGenUUID(entry.SourceResourceID)
	if err != nil {
		return teamsettingsgen.TeamSettingsAuditEntry{}, fmt.Errorf("source_resource_id: %w", err)
	}
	createdResourceID, err := optionalGenUUID(entry.CreatedResourceID)
	if err != nil {
		return teamsettingsgen.TeamSettingsAuditEntry{}, fmt.Errorf("created_resource_id: %w", err)
	}

	return teamsettingsgen.TeamSettingsAuditEntry{
		Id:                id,
		Surface:           teamsettingsgen.TeamSettingsAuditEntrySurface(entry.Surface),
		ActorUserId:       actorID,
		ActorName:         view.ActorName,
		SourceTeamId:      sourceTeamID,
		SourceTeamName:    view.SourceTeamName,
		SourceResourceId:  sourceResourceID,
		CreatedResourceId: createdResourceID,
		Detail:            auditDetailObject(entry.Detail),
		CreatedAt:         entry.CreatedAt,
	}, nil
}

// optionalGenUUID parses a nullable id column. A nil or empty value is the
// documented `null`, not an error — the actor may be deleted and the source
// team may never have been recorded.
func optionalGenUUID(id *string) (*openapi_types.UUID, error) {
	if id == nil || *id == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(*id)
	if err != nil {
		return nil, fmt.Errorf("%q is not a uuid: %w", *id, err)
	}
	return &parsed, nil
}

// auditDetailObject decodes the jsonb detail into the payload's object.
//
// `detail` is a required, non-nullable object, and a nil Go map marshals to
// `null`, so every failure path here yields an EMPTY object rather than nil: a
// row whose detail somehow did not decode still renders as a legible audit
// entry instead of failing the whole page or violating the schema. The
// repository already substitutes `{}` on write, so this is the belt to that
// braces rather than an expected path.
func auditDetailObject(raw json.RawMessage) map[string]interface{} {
	detail := map[string]interface{}{}
	if len(raw) == 0 {
		return detail
	}
	if err := json.Unmarshal(raw, &detail); err != nil || detail == nil {
		return map[string]interface{}{}
	}
	return detail
}

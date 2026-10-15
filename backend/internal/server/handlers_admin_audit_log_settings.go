package server

import (
	"context"
	"encoding/json"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
)

// Instance audit-log settings for instance admins (#1512, epic #1510): thin
// adapters over InstanceAuditSettingsService.
//
// Authorization is instanceAdminMiddleware, which 404s every non-admin and
// anonymous caller on these routes. The path is deliberately absent from
// adminSetupSessionRoutes, so a setup session cannot reach it either. The
// acting admin's id always comes from the auth context, never from a request
// body.

const adminMsgAuditLogSettingsInvalid = "The audit-log settings are invalid"

// auditLogSettingsAuditSnapshotKeys is the allowlist of keys an audit-log
// settings audit snapshot may surface (models.InstanceAuditSettingsValues'
// JSON shape).
var auditLogSettingsAuditSnapshotKeys = []string{"retention_weeks", "capture_ip_ua"}

// GetAdminAuditLogSettings returns the instance's audit-log settings. It is
// never a 404: with nothing stored, the built-in defaults apply.
func (a *adminStrictServer) GetAdminAuditLogSettings(
	ctx context.Context, _ admingen.GetAdminAuditLogSettingsRequestObject,
) (admingen.GetAdminAuditLogSettingsResponseObject, error) {
	const handler = "GetAdminAuditLogSettings"

	view, err := a.s.container.InstanceAuditSettingsService().Get(ctx)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	resp, err := a.toGenAdminAuditLogSettings(ctx, view)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.GetAdminAuditLogSettings200JSONResponse(resp), nil
}

// UpdateAdminAuditLogSettings replaces the instance's audit-log settings.
func (a *adminStrictServer) UpdateAdminAuditLogSettings(
	ctx context.Context, request admingen.UpdateAdminAuditLogSettingsRequestObject,
) (admingen.UpdateAdminAuditLogSettingsResponseObject, error) {
	const handler = "UpdateAdminAuditLogSettings"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	values := models.InstanceAuditSettingsValues{
		RetentionWeeks: request.Body.RetentionWeeks,
		CaptureIPUA:    request.Body.CaptureIpUa,
	}
	view, err := a.s.container.InstanceAuditSettingsService().Update(
		ctx, a.actingAdminID(ctx), values, request.Body.ExpectedVersion)
	if err != nil {
		return nil, a.mapInstanceSettingsError(handler, adminMsgAuditLogSettingsInvalid, err)
	}
	resp, err := a.toGenAdminAuditLogSettings(ctx, view)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.UpdateAdminAuditLogSettings200JSONResponse(resp), nil
}

// ResetAdminAuditLogSettings drops the stored settings so the built-in
// defaults apply. With nothing stored it is still a 204 and audits nothing.
func (a *adminStrictServer) ResetAdminAuditLogSettings(
	ctx context.Context, _ admingen.ResetAdminAuditLogSettingsRequestObject,
) (admingen.ResetAdminAuditLogSettingsResponseObject, error) {
	if err := a.s.container.InstanceAuditSettingsService().Reset(ctx, a.actingAdminID(ctx)); err != nil {
		return nil, a.adminInternalError("ResetAdminAuditLogSettings", err)
	}
	return admingen.ResetAdminAuditLogSettings204Response{}, nil
}

// ListAdminAuditLogSettingsAudit returns one page of the audit-log settings'
// change history, newest first.
func (a *adminStrictServer) ListAdminAuditLogSettingsAudit(
	ctx context.Context, request admingen.ListAdminAuditLogSettingsAuditRequestObject,
) (admingen.ListAdminAuditLogSettingsAuditResponseObject, error) {
	page, err := a.listInstanceSettingsAudit(ctx, "ListAdminAuditLogSettingsAudit",
		models.InstanceSettingAuditLog, request.Params.Limit, request.Params.Cursor,
		filterAuditLogSettingsAuditSnapshot)
	if err != nil {
		return nil, err
	}
	return admingen.ListAdminAuditLogSettingsAudit200JSONResponse(page), nil
}

func toGenAuditLogSettingsValues(v models.InstanceAuditSettingsValues) admingen.AdminAuditLogSettingsValues {
	return admingen.AdminAuditLogSettingsValues{
		RetentionWeeks: v.RetentionWeeks,
		CaptureIpUa:    v.CaptureIPUA,
	}
}

func (a *adminStrictServer) toGenAdminAuditLogSettings(
	ctx context.Context, view *models.InstanceAuditSettingsView,
) (admingen.AdminAuditLogSettings, error) {
	updater, err := a.instanceSettingsUpdater(ctx, view.UpdatedBy)
	if err != nil {
		return admingen.AdminAuditLogSettings{}, err
	}
	source := admingen.AdminInstanceSettingsSourceInstance
	if view.IsDefault {
		source = admingen.AdminInstanceSettingsSourceDefault
	}
	return admingen.AdminAuditLogSettings{
		Values:          toGenAuditLogSettingsValues(view.Values),
		Source:          source,
		BuiltInDefaults: toGenAuditLogSettingsValues(models.DefaultInstanceAuditSettingsValues()),
		Limits: admingen.AdminAuditLogSettingsLimits{
			RetentionWeeksMin: models.MinAuditRetentionWeeks,
			RetentionWeeksMax: models.MaxAuditRetentionWeeks,
		},
		UpdatedAt:       view.UpdatedAt,
		UpdatedByUserId: updater.id,
		UpdatedByName:   updater.name,
		Version:         view.Version,
	}, nil
}

// filterAuditLogSettingsAuditSnapshot keeps only the snapshot's allowlisted
// keys, so a key added to the stored shape later cannot reach this surface
// unreviewed.
func filterAuditLogSettingsAuditSnapshot(raw json.RawMessage) *map[string]interface{} {
	return filterInstanceAuditSnapshot(raw, auditLogSettingsAuditSnapshotKeys)
}

package server

import (
	"context"
	"encoding/json"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
)

// Instance site info for instance admins (#1354, epic #1352): thin adapters
// over InstanceSiteInfoService (#1353).
//
// Authorization is instanceAdminMiddleware, which 404s every non-admin and
// anonymous caller on these routes. The path is deliberately absent from
// adminSetupSessionRoutes, so a setup session cannot reach it either. The
// acting admin's id always comes from the auth context, never from a request
// body.

const adminMsgSiteInfoInvalid = "The site info is invalid"

// siteInfoAuditSnapshotKeys is the allowlist of keys a site info audit
// snapshot may surface (models.InstanceSiteInfoValues' JSON shape).
var siteInfoAuditSnapshotKeys = []string{
	"company_name", "company_email", "website", "support_url",
	"terms_url", "privacy_url", "logo_url", "logo_dark_url",
}

// GetAdminSiteInfo returns the instance's company branding. It is never a 404:
// with nothing stored, every value is empty.
func (a *adminStrictServer) GetAdminSiteInfo(
	ctx context.Context, _ admingen.GetAdminSiteInfoRequestObject,
) (admingen.GetAdminSiteInfoResponseObject, error) {
	const handler = "GetAdminSiteInfo"

	view, err := a.s.container.InstanceSiteInfoService().Get(ctx)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	resp, err := a.toGenAdminSiteInfo(ctx, view)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.GetAdminSiteInfo200JSONResponse(resp), nil
}

// UpdateAdminSiteInfo replaces the instance's company branding.
func (a *adminStrictServer) UpdateAdminSiteInfo(
	ctx context.Context, request admingen.UpdateAdminSiteInfoRequestObject,
) (admingen.UpdateAdminSiteInfoResponseObject, error) {
	const handler = "UpdateAdminSiteInfo"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	view, err := a.s.container.InstanceSiteInfoService().Update(
		ctx, a.actingAdminID(ctx), toModelSiteInfoValues(*request.Body), request.Body.ExpectedVersion)
	if err != nil {
		return nil, a.mapInstanceSettingsError(handler, adminMsgSiteInfoInvalid, err)
	}
	resp, err := a.toGenAdminSiteInfo(ctx, view)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.UpdateAdminSiteInfo200JSONResponse(resp), nil
}

// ResetAdminSiteInfo drops the stored branding so nothing is configured. With
// nothing stored it is still a 204 and audits nothing.
func (a *adminStrictServer) ResetAdminSiteInfo(
	ctx context.Context, _ admingen.ResetAdminSiteInfoRequestObject,
) (admingen.ResetAdminSiteInfoResponseObject, error) {
	if err := a.s.container.InstanceSiteInfoService().Reset(ctx, a.actingAdminID(ctx)); err != nil {
		return nil, a.adminInternalError("ResetAdminSiteInfo", err)
	}
	return admingen.ResetAdminSiteInfo204Response{}, nil
}

// ListAdminSiteInfoAudit returns one page of the site info's audit log, newest
// first.
func (a *adminStrictServer) ListAdminSiteInfoAudit(
	ctx context.Context, request admingen.ListAdminSiteInfoAuditRequestObject,
) (admingen.ListAdminSiteInfoAuditResponseObject, error) {
	page, err := a.listInstanceSettingsAudit(ctx, "ListAdminSiteInfoAudit",
		models.InstanceSettingSiteInfo, request.Params.Limit, request.Params.Cursor,
		filterSiteInfoAuditSnapshot)
	if err != nil {
		return nil, err
	}
	return admingen.ListAdminSiteInfoAudit200JSONResponse(page), nil
}

func toModelSiteInfoValues(body admingen.AdminSiteInfoUpdate) models.InstanceSiteInfoValues {
	return models.InstanceSiteInfoValues{
		CompanyName:  body.CompanyName,
		CompanyEmail: body.CompanyEmail,
		Website:      body.Website,
		SupportURL:   body.SupportUrl,
		TermsURL:     body.TermsUrl,
		PrivacyURL:   body.PrivacyUrl,
		LogoURL:      body.LogoUrl,
		LogoDarkURL:  body.LogoDarkUrl,
	}
}

func (a *adminStrictServer) toGenAdminSiteInfo(
	ctx context.Context, view *models.InstanceSiteInfoView,
) (admingen.AdminSiteInfo, error) {
	updater, err := a.instanceSettingsUpdater(ctx, view.UpdatedBy)
	if err != nil {
		return admingen.AdminSiteInfo{}, err
	}
	return admingen.AdminSiteInfo{
		Values: admingen.AdminSiteInfoValues{
			CompanyName:  view.Values.CompanyName,
			CompanyEmail: view.Values.CompanyEmail,
			Website:      view.Values.Website,
			SupportUrl:   view.Values.SupportURL,
			TermsUrl:     view.Values.TermsURL,
			PrivacyUrl:   view.Values.PrivacyURL,
			LogoUrl:      view.Values.LogoURL,
			LogoDarkUrl:  view.Values.LogoDarkURL,
		},
		Limits: admingen.AdminSiteInfoLimits{
			CompanyNameMaxLength:  models.MaxSiteInfoCompanyNameLength,
			CompanyEmailMaxLength: models.MaxSiteInfoEmailLength,
			UrlMaxLength:          models.MaxSiteInfoURLLength,
		},
		UpdatedAt:       view.UpdatedAt,
		UpdatedByUserId: updater.id,
		UpdatedByName:   updater.name,
		Version:         view.Version,
	}, nil
}

// filterSiteInfoAuditSnapshot keeps only the site info snapshot's allowlisted
// keys, so a key added to the stored shape later cannot reach this surface
// unreviewed.
func filterSiteInfoAuditSnapshot(raw json.RawMessage) *map[string]interface{} {
	return filterInstanceAuditSnapshot(raw, siteInfoAuditSnapshotKeys)
}

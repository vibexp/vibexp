package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/vibexp/vibexp/internal/contextkeys"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
)

// Instance authentication settings for instance admins (#1238, epic #1230):
// thin adapters over InstanceAuthSettingsService.
//
// Authorization is adminRouteGuard. The provider and allowlist operations admit
// an instance admin OR a first-run setup session (#1236); the admin and audit
// operations are instance-admin only, and granting or revoking an admin is
// root-only, enforced by the service. On a setup session there is no acting
// user, so actingAdminID is empty and the change is audited with no actor.
//
// Secret safety holds by construction: no response schema has a field able to
// carry a client secret, the audit snapshots are filtered to an allowlist, and
// no request body is ever logged.

const (
	adminMsgAuthProviderInvalid  = "The sign-in provider is invalid"
	adminMsgAuthAllowlistInvalid = "The access allowlist is invalid"
	adminMsgLockoutConfirm       = "; send confirm_lockout_risk to apply it anyway"

	authProviderResource = "auth provider"
)

// adminSetupSessionRoutes matches exactly the instance authentication settings
// paths a setup session may reach: /providers, /providers/test,
// /providers/{uuid}, /allowlist and /allowlist/preview. Each sub-path is spelled
// out, so nothing else on the admin surface matches — not the admin list, not
// the audit log, and not a route added later under either prefix.
// TestAdminSetupSessionRoutes_MatchExactlyTheMountedSetupRoutes pins the
// pattern against the mounted route table.
var adminSetupSessionRoutes = regexp.MustCompile(`^/api/v1/admin/settings/auth/(` +
	`providers(/test|/` + uuidPattern + `)?|` +
	`allowlist(/preview)?` +
	`)$`)

// uuidPattern matches one UUID in its canonical textual form.
const uuidPattern = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

// adminRouteGuard authenticates the whole /api/v1/admin surface. Every route is
// instance-admin only (optionalAuthMiddleware + instanceAdminMiddleware, so
// anonymous and non-admin callers alike get 404) except the ones matching
// adminSetupSessionRoutes, which setupSessionOrInstanceAdmin guards instead so
// that a setup session can configure sign-in before any admin can sign in.
//
// The choice is made per route, by path, and defaults to admin-only: a route
// added later is never reachable on a setup session unless it is added to
// adminSetupSessionRoutes.
func (s *Server) adminRouteGuard(next http.Handler) http.Handler {
	adminOnly := s.optionalAuthMiddleware(s.instanceAdminMiddleware(next))
	setupOrAdmin := s.setupSessionOrInstanceAdmin(s.withSessionProvider(next))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if adminSetupSessionRoutes.MatchString(r.URL.Path) {
			setupOrAdmin.ServeHTTP(w, r)
			return
		}
		adminOnly.ServeHTTP(w, r)
	})
}

// withSessionProvider records, for the lockout guard, the slug of the identity
// provider the caller's cookie session was issued by. Only a cookie-authenticated
// request has one: an API key, a setup session and a dev-login session carry no
// provider, and the guard then judges the enabled set alone.
func (s *Server) withSessionProvider(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authType, _ := r.Context().Value(contextkeys.AuthType).(string); authType == authTypeCookie &&
			s.sessionManager != nil {
			if sess, err := s.sessionManager.Read(r); err == nil && sess.Provider != "" {
				r = r.WithContext(context.WithValue(r.Context(), contextkeys.SessionProvider, sess.Provider))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// sessionProviderFromContext returns the slug withSessionProvider recorded, or
// "" when the caller has no provider-issued session.
func sessionProviderFromContext(ctx context.Context) string {
	provider, _ := ctx.Value(contextkeys.SessionProvider).(string)
	return provider
}

// --- providers ----------------------------------------------------------------

// ListAdminAuthProviders returns every stored sign-in provider with its health
// and the shared settings version.
func (a *adminStrictServer) ListAdminAuthProviders(
	ctx context.Context, _ admingen.ListAdminAuthProvidersRequestObject,
) (admingen.ListAdminAuthProvidersResponseObject, error) {
	const handler = "ListAdminAuthProviders"

	list, err := a.s.container.InstanceAuthSettingsService().ListProviders(ctx)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	// make(...,0): `providers` is a required array on a generated type.
	providers := make([]admingen.AdminAuthProvider, 0, len(list.Providers))
	for _, view := range list.Providers {
		provider, err := toGenAdminAuthProvider(view)
		if err != nil {
			return nil, a.adminInternalError(handler, err)
		}
		providers = append(providers, provider)
	}
	return admingen.ListAdminAuthProviders200JSONResponse{Providers: providers, Version: list.Version}, nil
}

// CreateAdminAuthProvider stores a new sign-in provider.
func (a *adminStrictServer) CreateAdminAuthProvider(
	ctx context.Context, request admingen.CreateAdminAuthProviderRequestObject,
) (admingen.CreateAdminAuthProviderResponseObject, error) {
	const handler = "CreateAdminAuthProvider"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}
	body := request.Body
	// enabled defaults to true and sort_order to 0, as the schema documents.
	sortOrder := 0
	if body.SortOrder != nil {
		sortOrder = *body.SortOrder
	}

	saved, err := a.s.container.InstanceAuthSettingsService().CreateProvider(ctx, a.actingAdminID(ctx),
		models.CreateInstanceAuthProviderRequest{
			Type:         models.InstanceAuthProviderType(body.Type),
			Slug:         body.Slug,
			DisplayName:  body.DisplayName,
			Enabled:      body.Enabled == nil || *body.Enabled,
			SortOrder:    sortOrder,
			ClientID:     body.ClientId,
			ClientSecret: body.ClientSecret,
			IssuerURL:    body.IssuerUrl,
			// A null expected_version decodes to nil, like an omitted one.
			ExpectedVersion: body.ExpectedVersion,
		})
	if err != nil {
		return nil, a.mapInstanceAuthError(handler, adminMsgAuthProviderInvalid, err)
	}
	resp, err := toGenAdminAuthProviderSaved(saved)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.CreateAdminAuthProvider201JSONResponse(resp), nil
}

// UpdateAdminAuthProvider replaces a provider's editable fields. An omitted
// client secret reaches the service as nil, which keeps the stored one under
// the service's rules.
func (a *adminStrictServer) UpdateAdminAuthProvider(
	ctx context.Context, request admingen.UpdateAdminAuthProviderRequestObject,
) (admingen.UpdateAdminAuthProviderResponseObject, error) {
	const handler = "UpdateAdminAuthProvider"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}
	body := request.Body

	saved, err := a.s.container.InstanceAuthSettingsService().UpdateProvider(ctx, a.actingAdminID(ctx),
		request.Id.String(), models.UpdateInstanceAuthProviderRequest{
			DisplayName:     body.DisplayName,
			Enabled:         body.Enabled,
			SortOrder:       body.SortOrder,
			ClientID:        body.ClientId,
			ClientSecret:    body.ClientSecret,
			IssuerURL:       body.IssuerUrl,
			ExpectedVersion: body.ExpectedVersion,
			Lockout:         lockoutContext(ctx, body.ConfirmLockoutRisk),
		})
	if err != nil {
		return nil, a.mapInstanceAuthError(handler, adminMsgAuthProviderInvalid, err)
	}
	resp, err := toGenAdminAuthProviderSaved(saved)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.UpdateAdminAuthProvider200JSONResponse(resp), nil
}

// DeleteAdminAuthProvider removes a sign-in provider.
func (a *adminStrictServer) DeleteAdminAuthProvider(
	ctx context.Context, request admingen.DeleteAdminAuthProviderRequestObject,
) (admingen.DeleteAdminAuthProviderResponseObject, error) {
	err := a.s.container.InstanceAuthSettingsService().DeleteProvider(ctx, a.actingAdminID(ctx),
		request.Id.String(), models.DeleteInstanceAuthProviderRequest{
			ExpectedVersion: request.Params.ExpectedVersion,
			Lockout:         lockoutContext(ctx, request.Params.ConfirmLockoutRisk),
		})
	if err != nil {
		return nil, a.mapInstanceAuthError("DeleteAdminAuthProvider", adminMsgAuthProviderInvalid, err)
	}
	return admingen.DeleteAdminAuthProvider204Response{}, nil
}

// TestAdminAuthProvider checks a stored provider or a candidate. Nothing is
// stored; a failed check is a 200 with is_valid false.
func (a *adminStrictServer) TestAdminAuthProvider(
	ctx context.Context, request admingen.TestAdminAuthProviderRequestObject,
) (admingen.TestAdminAuthProviderResponseObject, error) {
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}
	body := request.Body

	req := models.TestInstanceAuthProviderRequest{
		ClientID:     body.ClientId,
		ClientSecret: body.ClientSecret,
		IssuerURL:    body.IssuerUrl,
	}
	if body.Id != nil {
		id := body.Id.String()
		req.ID = &id
	}
	if body.Type != nil {
		providerType := models.InstanceAuthProviderType(*body.Type)
		req.Type = &providerType
	}

	result, err := a.s.container.InstanceAuthSettingsService().TestProvider(ctx, req)
	if err != nil {
		return nil, a.mapInstanceAuthError("TestAdminAuthProvider", adminMsgAuthProviderInvalid, err)
	}
	resp := admingen.AdminAuthProviderTestResult{IsValid: result.Valid}
	if !result.Valid {
		resp.Message = &result.Message
	}
	return admingen.TestAdminAuthProvider200JSONResponse(resp), nil
}

// lockoutContext gathers what the lockout guard needs from the request: the
// provider the caller's session was issued by and their confirmation.
func lockoutContext(ctx context.Context, confirmed *bool) models.InstanceAuthLockoutContext {
	return models.InstanceAuthLockoutContext{
		SessionProvider: sessionProviderFromContext(ctx),
		Confirmed:       confirmed != nil && *confirmed,
	}
}

// --- allowlist ----------------------------------------------------------------

// GetAdminAuthAllowlist returns the stored access allowlist. It is never a
// 404: with nothing stored, access is open and both lists are empty.
func (a *adminStrictServer) GetAdminAuthAllowlist(
	ctx context.Context, _ admingen.GetAdminAuthAllowlistRequestObject,
) (admingen.GetAdminAuthAllowlistResponseObject, error) {
	const handler = "GetAdminAuthAllowlist"

	allowlist, err := a.s.container.InstanceAuthSettingsService().GetAllowlist(ctx)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	resp, err := toGenAdminAuthAllowlist(allowlist)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.GetAdminAuthAllowlist200JSONResponse(resp), nil
}

// UpdateAdminAuthAllowlist replaces the access allowlist.
func (a *adminStrictServer) UpdateAdminAuthAllowlist(
	ctx context.Context, request admingen.UpdateAdminAuthAllowlistRequestObject,
) (admingen.UpdateAdminAuthAllowlistResponseObject, error) {
	const handler = "UpdateAdminAuthAllowlist"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	allowlist, err := a.s.container.InstanceAuthSettingsService().UpdateAllowlist(ctx, a.actingAdminID(ctx),
		request.Body.Domains, request.Body.Emails, request.Body.ExpectedVersion)
	if err != nil {
		return nil, a.mapInstanceAuthError(handler, adminMsgAuthAllowlistInvalid, err)
	}
	resp, err := toGenAdminAuthAllowlist(allowlist)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.UpdateAdminAuthAllowlist200JSONResponse(resp), nil
}

// ResetAdminAuthAllowlist removes the stored allowlist, reverting to open
// access. With nothing stored it is still a 204 and audits nothing.
func (a *adminStrictServer) ResetAdminAuthAllowlist(
	ctx context.Context, _ admingen.ResetAdminAuthAllowlistRequestObject,
) (admingen.ResetAdminAuthAllowlistResponseObject, error) {
	if err := a.s.container.InstanceAuthSettingsService().ResetAllowlist(ctx, a.actingAdminID(ctx)); err != nil {
		return nil, a.adminInternalError("ResetAdminAuthAllowlist", err)
	}
	return admingen.ResetAdminAuthAllowlist204Response{}, nil
}

// PreviewAdminAuthAllowlist reports who a candidate allowlist would shut out.
// Nothing is stored.
func (a *adminStrictServer) PreviewAdminAuthAllowlist(
	ctx context.Context, request admingen.PreviewAdminAuthAllowlistRequestObject,
) (admingen.PreviewAdminAuthAllowlistResponseObject, error) {
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	impact, err := a.s.container.InstanceAuthSettingsService().
		PreviewAllowlist(ctx, request.Body.Domains, request.Body.Emails)
	if err != nil {
		return nil, a.mapInstanceAuthError("PreviewAdminAuthAllowlist", adminMsgAuthAllowlistInvalid, err)
	}
	return admingen.PreviewAdminAuthAllowlist200JSONResponse{
		Count:           impact.Count,
		Sample:          nonNilStrings(impact.Sample),
		SampleTruncated: impact.SampleTruncated,
	}, nil
}

// --- instance admins ----------------------------------------------------------

// ListAdminInstanceAdmins returns the root admins' emails and the DB-granted
// admins.
func (a *adminStrictServer) ListAdminInstanceAdmins(
	ctx context.Context, _ admingen.ListAdminInstanceAdminsRequestObject,
) (admingen.ListAdminInstanceAdminsResponseObject, error) {
	const handler = "ListAdminInstanceAdmins"

	list, err := a.s.container.InstanceAuthSettingsService().ListAdmins(ctx)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	// make(...,0): `admins` is a required array on a generated type.
	admins := make([]admingen.AdminInstanceAdmin, 0, len(list.Admins))
	for i := range list.Admins {
		admin, err := toGenAdminInstanceAdmin(&list.Admins[i])
		if err != nil {
			return nil, a.adminInternalError(handler, err)
		}
		admins = append(admins, admin)
	}
	return admingen.ListAdminInstanceAdmins200JSONResponse{
		RootAdmins: nonNilStrings(list.RootAdmins),
		Admins:     admins,
	}, nil
}

// GrantAdminInstanceAdmin makes an existing user an instance admin. Root-only:
// the service refuses any other actor.
func (a *adminStrictServer) GrantAdminInstanceAdmin(
	ctx context.Context, request admingen.GrantAdminInstanceAdminRequestObject,
) (admingen.GrantAdminInstanceAdminResponseObject, error) {
	const handler = "GrantAdminInstanceAdmin"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	var target models.InstanceAdminGrantTarget
	if request.Body.UserId != nil {
		target.UserID = request.Body.UserId.String()
	}
	if request.Body.Email != nil {
		target.Email = string(*request.Body.Email)
	}

	view, err := a.s.container.InstanceAuthSettingsService().GrantAdmin(ctx, a.actingAdminID(ctx), target)
	if err != nil {
		return nil, a.mapInstanceAdminError(handler, err)
	}
	resp, err := toGenAdminInstanceAdmin(view)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.GrantAdminInstanceAdmin200JSONResponse(resp), nil
}

// RevokeAdminInstanceAdmin removes a DB grant. Root-only: the service refuses
// any other actor.
func (a *adminStrictServer) RevokeAdminInstanceAdmin(
	ctx context.Context, request admingen.RevokeAdminInstanceAdminRequestObject,
) (admingen.RevokeAdminInstanceAdminResponseObject, error) {
	err := a.s.container.InstanceAuthSettingsService().
		RevokeAdmin(ctx, a.actingAdminID(ctx), request.UserId.String())
	if err != nil {
		return nil, a.mapInstanceAdminError("RevokeAdminInstanceAdmin", err)
	}
	return admingen.RevokeAdminInstanceAdmin204Response{}, nil
}

// --- audit --------------------------------------------------------------------

// authAuditSnapshotFilters maps each authentication setting onto the filter
// that redacts its audit snapshots to an allowlist of keys.
var authAuditSnapshotFilters = map[admingen.AdminAuthSettingsAuditSetting]func(json.RawMessage) *map[string]interface{}{
	admingen.AdminAuthSettingsAuditSettingAuthProviders: func(raw json.RawMessage) *map[string]interface{} {
		return filterInstanceAuditSnapshotWithSecretMarker(raw, authProviderAuditSnapshotKeys, "client_secret")
	},
	admingen.AdminAuthSettingsAuditSettingAuthAllowlist: func(raw json.RawMessage) *map[string]interface{} {
		return filterInstanceAuditSnapshot(raw, authAllowlistAuditSnapshotKeys)
	},
	admingen.AdminAuthSettingsAuditSettingInstanceAdmins: func(raw json.RawMessage) *map[string]interface{} {
		return filterInstanceAuditSnapshot(raw, instanceAdminAuditSnapshotKeys)
	},
	admingen.AdminAuthSettingsAuditSettingAuthSetup: func(raw json.RawMessage) *map[string]interface{} {
		return filterInstanceAuditSnapshot(raw, authSetupAuditSnapshotKeys)
	},
}

// authAuditSourceKey is the snapshot key naming where a write came from when
// it was not the API: "cli" for the `vibexp admin auth` commands (#1237). Every
// authentication setting's snapshot may carry it.
const authAuditSourceKey = "source"

// The keys an authentication setting's audit snapshot may surface. The
// snapshots are already redacted at write time; filtering again here means a
// key added to a stored shape later cannot reach this surface unreviewed.
var (
	// authProviderAuditSnapshotKeys is models.InstanceAuthProvider's JSON shape
	// plus has_client_secret and the client_secret changed/unchanged marker.
	authProviderAuditSnapshotKeys = []string{
		"id", "type", "slug", "display_name", "enabled", "sort_order", "client_id", "issuer_url",
		"has_client_secret", "client_secret", "created_at", "updated_at", "updated_by", authAuditSourceKey,
	}
	// authAllowlistAuditSnapshotKeys is models.InstanceAuthAllowlist's JSON shape.
	authAllowlistAuditSnapshotKeys = []string{
		"domains", "emails", "created_at", "updated_at", "updated_by", "version", authAuditSourceKey,
	}
	// instanceAdminAuditSnapshotKeys is models.InstanceAdminGrant's JSON shape.
	instanceAdminAuditSnapshotKeys = []string{"user_id", "granted_by", "created_at", authAuditSourceKey}
	// authSetupAuditSnapshotKeys is models.InstanceAuthSetup's JSON shape (which
	// has no token hash) plus the event the entry records.
	authSetupAuditSnapshotKeys = []string{
		"event", "expires_at", "consumed_at", "consumed_by", "rearmed", "generation", "created_at", "updated_at",
		authAuditSourceKey,
	}
)

// ListAdminAuthSettingsAudit returns one page of one authentication setting's
// audit log, newest first.
func (a *adminStrictServer) ListAdminAuthSettingsAudit(
	ctx context.Context, request admingen.ListAdminAuthSettingsAuditRequestObject,
) (admingen.ListAdminAuthSettingsAuditResponseObject, error) {
	// The generated binder does not enforce a query parameter's enum.
	filter, ok := authAuditSnapshotFilters[request.Params.Setting]
	if !ok {
		return nil, apierrors.NewBadRequestError(fmt.Sprintf(
			"invalid setting %q: must be one of auth_providers, auth_allowlist, instance_admins, auth_setup",
			request.Params.Setting))
	}
	// The enum's values are the stored setting names (pinned by
	// TestInstanceSettingsAuditVocabularyMatchesSpec).
	page, err := a.listInstanceSettingsAudit(ctx, "ListAdminAuthSettingsAudit",
		string(request.Params.Setting), request.Params.Limit, request.Params.Cursor, filter)
	if err != nil {
		return nil, err
	}
	return admingen.ListAdminAuthSettingsAudit200JSONResponse(page), nil
}

// --- errors -------------------------------------------------------------------

// mapInstanceAuthError turns a provider or allowlist error into its HTTP
// shape: invalid values are a 400 naming the offending field, an unknown
// provider is a 404, a stale version, a colliding provider and a refused
// lockout risk are 409s, and anything else is a logged 500 (the error, never
// the request body).
func (a *adminStrictServer) mapInstanceAuthError(handler, invalidMsg string, err error) error {
	var lockout *services.ErrAuthLockoutRisk
	switch {
	case errors.Is(err, services.ErrInvalidInstanceAuthProvider),
		errors.Is(err, services.ErrInvalidInstanceAuthAllowlist):
		return apierrors.NewInstanceSettingsValidationError(invalidMsg, instanceSettingsValidationErrors(err))
	case errors.Is(err, repositories.ErrInstanceAuthProviderInvalid):
		// The database refused a row the validator accepted.
		return apierrors.NewInstanceSettingsValidationError(invalidMsg, nil)
	case errors.Is(err, repositories.ErrInstanceAuthProviderNotFound):
		return apierrors.NewResourceNotFoundError(authProviderResource, "")
	case errors.Is(err, repositories.ErrInstanceSettingsVersionConflict):
		return apierrors.NewInstanceSettingsVersionConflictError()
	case errors.Is(err, repositories.ErrInstanceAuthProviderConflict):
		return apierrors.NewInstanceAuthProviderConflictError()
	case errors.As(err, &lockout):
		return apierrors.NewLockoutRiskError(lockout.Reason, lockout.Error()+adminMsgLockoutConfirm)
	default:
		return a.adminInternalError(handler, err)
	}
}

// mapInstanceAdminError turns a grant or revoke error into its HTTP shape: a
// non-root actor is a 403, an unknown user or a missing grant a 404, a root
// target a 409, a malformed or suspended target a 400, and anything else a
// logged 500.
func (a *adminStrictServer) mapInstanceAdminError(handler string, err error) error {
	var (
		notRoot    *services.ErrInstanceAdminNotRoot
		targetRoot *services.ErrInstanceAdminTargetIsRoot
		invalid    *services.ErrInstanceAdminTargetInvalid
		notGranted *services.ErrInstanceAdminNotGranted
	)
	switch {
	case errors.Is(err, services.ErrInstanceAdminGrantTargetAmbiguous):
		return apierrors.NewBadRequestError(err.Error())
	case errors.As(err, &notRoot):
		return apierrors.NewForbiddenError(notRoot.Error())
	case errors.As(err, &targetRoot):
		return adminConflictError(targetRoot.Error())
	case errors.As(err, &invalid):
		if invalid.IsUnknownUser() {
			return apierrors.NewResourceNotFoundError("user", "")
		}
		return apierrors.NewBadRequestError(invalid.Error())
	case errors.As(err, &notGranted):
		return apierrors.NewResourceNotFoundError("instance admin grant", "")
	default:
		return a.adminInternalError(handler, err)
	}
}

// --- response mapping ---------------------------------------------------------

// toGenAdminAuthProvider maps a provider field by field, so a field added to
// the stored model cannot reach this surface unless it is deliberately added
// here and to the schema. The client secret has no field to travel in.
func toGenAdminAuthProvider(view models.InstanceAuthProviderView) (admingen.AdminAuthProvider, error) {
	p := view.Provider
	id, err := parseAdminUUID(authProviderResource, p.ID)
	if err != nil {
		return admingen.AdminAuthProvider{}, err
	}
	updatedBy, err := optionalGenUUID(p.UpdatedBy)
	if err != nil {
		return admingen.AdminAuthProvider{}, fmt.Errorf("updated_by: %w", err)
	}
	return admingen.AdminAuthProvider{
		Id:              id,
		Type:            admingen.AdminAuthProviderType(p.Type),
		Slug:            p.Slug,
		DisplayName:     p.DisplayName,
		Enabled:         p.Enabled,
		SortOrder:       p.SortOrder,
		ClientId:        p.ClientID,
		HasClientSecret: p.HasClientSecret(),
		IssuerUrl:       p.IssuerURL,
		RedirectUri:     view.RedirectURI,
		Health: admingen.AdminAuthProviderHealth{
			Status:    admingen.AdminAuthProviderHealthStatus(view.Health.Status),
			LastError: view.Health.LastError,
			CheckedAt: view.Health.CheckedAt,
		},
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
		UpdatedByUserId: updatedBy,
	}, nil
}

func toGenAdminAuthProviderSaved(saved *models.InstanceAuthProviderSaved) (admingen.AdminAuthProviderSaved, error) {
	provider, err := toGenAdminAuthProvider(saved.Provider)
	if err != nil {
		return admingen.AdminAuthProviderSaved{}, err
	}
	return admingen.AdminAuthProviderSaved{Provider: provider, Version: saved.Version}, nil
}

// toGenAdminAuthAllowlist maps the stored allowlist; nil (nothing stored) is
// open access with no version.
func toGenAdminAuthAllowlist(allowlist *models.InstanceAuthAllowlist) (admingen.AdminAuthAllowlist, error) {
	if allowlist == nil {
		return admingen.AdminAuthAllowlist{Domains: []string{}, Emails: []string{}}, nil
	}
	updatedBy, err := optionalGenUUID(allowlist.UpdatedBy)
	if err != nil {
		return admingen.AdminAuthAllowlist{}, fmt.Errorf("updated_by: %w", err)
	}
	version, updatedAt := allowlist.Version, allowlist.UpdatedAt
	return admingen.AdminAuthAllowlist{
		Domains:         nonNilStrings(allowlist.Domains),
		Emails:          nonNilStrings(allowlist.Emails),
		Active:          !allowlist.IsOpenAccess(),
		Version:         &version,
		UpdatedAt:       &updatedAt,
		UpdatedByUserId: updatedBy,
	}, nil
}

func toGenAdminInstanceAdmin(view *models.InstanceAdminView) (admingen.AdminInstanceAdmin, error) {
	userID, err := parseAdminUUID("user", view.UserID)
	if err != nil {
		return admingen.AdminInstanceAdmin{}, err
	}
	grantedBy, err := optionalGenUUID(view.GrantedBy)
	if err != nil {
		return admingen.AdminInstanceAdmin{}, fmt.Errorf("granted_by: %w", err)
	}
	return admingen.AdminInstanceAdmin{
		UserId:          userID,
		Email:           view.Email,
		Name:            optionalString(view.Name),
		GrantedByUserId: grantedBy,
		GrantedAt:       view.GrantedAt,
	}, nil
}

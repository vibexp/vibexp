package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
)

// Instance email settings for instance admins (#1189, epic #1185): thin
// adapters over InstanceEmailProviderService (#1188).
//
// Authorization is instanceAdminMiddleware, which 404s every non-admin and
// anonymous caller for the whole /api/v1/admin surface (epic decision 6); the
// team-scoped authz matrix does not apply to the instance's own settings.
//
// Secret safety holds by construction: no response schema has a field able to
// carry the credential, the audit snapshots are filtered to an allowlist, and
// no request body is ever logged.

const (
	// adminInstanceAuditDefaultLimit is the page size when none is given.
	adminInstanceAuditDefaultLimit = 20
	// adminInstanceAuditMaxLimit is the largest page a caller may request.
	adminInstanceAuditMaxLimit = 100

	adminMsgInstanceEmailInvalid = "The email provider configuration is invalid"
)

// errAdminInstanceAuditCursor marks a cursor that does not decode.
var errAdminInstanceAuditCursor = errors.New("invalid cursor")

// GetAdminInstanceEmailSettings returns the instance's stored email settings.
// It is never a 404: an instance with nothing stored reports configured false.
func (a *adminStrictServer) GetAdminInstanceEmailSettings(
	ctx context.Context, _ admingen.GetAdminInstanceEmailSettingsRequestObject,
) (admingen.GetAdminInstanceEmailSettingsResponseObject, error) {
	const handler = "GetAdminInstanceEmailSettings"

	eff, err := a.s.container.InstanceEmailProviderService().Get(ctx)
	if err != nil {
		return nil, a.mapInstanceEmailError(handler, err)
	}
	resp, err := toGenAdminInstanceEmailSettings(eff)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.GetAdminInstanceEmailSettings200JSONResponse(resp), nil
}

// UpsertAdminInstanceEmailSettings creates or replaces the instance's email
// settings. An omitted secret reaches the service as nil, which keeps the
// stored credential under the service's rules.
func (a *adminStrictServer) UpsertAdminInstanceEmailSettings(
	ctx context.Context, request admingen.UpsertAdminInstanceEmailSettingsRequestObject,
) (admingen.UpsertAdminInstanceEmailSettingsResponseObject, error) {
	const handler = "UpsertAdminInstanceEmailSettings"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON)
	}

	eff, err := a.s.container.InstanceEmailProviderService().
		Upsert(ctx, a.actingAdminID(ctx), toModelInstanceEmailUpsert(*request.Body))
	if err != nil {
		return nil, a.mapInstanceEmailError(handler, err)
	}
	resp, err := toGenAdminInstanceEmailSettings(eff)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.UpsertAdminInstanceEmailSettings200JSONResponse(resp), nil
}

// DeleteAdminInstanceEmailSettings removes the instance's email settings, or
// answers 409 when nothing is stored.
func (a *adminStrictServer) DeleteAdminInstanceEmailSettings(
	ctx context.Context, _ admingen.DeleteAdminInstanceEmailSettingsRequestObject,
) (admingen.DeleteAdminInstanceEmailSettingsResponseObject, error) {
	if err := a.s.container.InstanceEmailProviderService().Delete(ctx, a.actingAdminID(ctx)); err != nil {
		return nil, a.mapInstanceEmailError("DeleteAdminInstanceEmailSettings", err)
	}
	return admingen.DeleteAdminInstanceEmailSettings204Response{}, nil
}

// TestAdminInstanceEmailSettings sends a test message to the acting admin's own
// account email, through the stored configuration (empty body) or a candidate
// one. The service could take a recipient override, but this endpoint never
// passes one: the request schema has no recipient field, and the unknown-field
// guard rejects a body that sends one.
func (a *adminStrictServer) TestAdminInstanceEmailSettings(
	ctx context.Context, request admingen.TestAdminInstanceEmailSettingsRequestObject,
) (admingen.TestAdminInstanceEmailSettingsResponseObject, error) {
	req := models.TestInstanceEmailProviderRequest{Config: toModelInstanceEmailTestConfig(request.Body)}

	result, err := a.s.container.InstanceEmailProviderService().Test(ctx, a.actingAdminID(ctx), req)
	if err != nil {
		return nil, a.mapInstanceEmailError("TestAdminInstanceEmailSettings", err)
	}
	return admingen.TestAdminInstanceEmailSettings200JSONResponse(toGenAdminInstanceEmailTestResult(result)), nil
}

// ListAdminInstanceEmailSettingsAudit returns one page of the instance email
// provider's audit log, newest first. It reads the repository directly: the
// log is instance-scoped, so there is no team authorization for a service to
// add.
func (a *adminStrictServer) ListAdminInstanceEmailSettingsAudit(
	ctx context.Context, request admingen.ListAdminInstanceEmailSettingsAuditRequestObject,
) (admingen.ListAdminInstanceEmailSettingsAuditResponseObject, error) {
	page, err := a.listInstanceSettingsAudit(ctx, "ListAdminInstanceEmailSettingsAudit",
		models.InstanceSettingEmailProvider, request.Params.Limit, request.Params.Cursor,
		filterInstanceEmailAuditSnapshot)
	if err != nil {
		return nil, err
	}
	return admingen.ListAdminInstanceEmailSettingsAudit200JSONResponse(page), nil
}

// listInstanceSettingsAudit returns one page of one instance setting's audit
// log, newest first, shared by every /admin/settings/<setting>/audit route. It
// reads the repository directly: the log is instance-scoped, so there is no
// team authorization for a service to add. filter redacts each snapshot to the
// setting's allowlisted keys. The returned error is already an HTTP error.
func (a *adminStrictServer) listInstanceSettingsAudit(
	ctx context.Context, handler, setting string, limitParam *int, cursorParam *string,
	filter func(json.RawMessage) *map[string]interface{},
) (admingen.AdminInstanceSettingsAuditPage, error) {
	limit := adminInstanceAuditDefaultLimit
	if limitParam != nil {
		limit = *limitParam
		// The generated binder does not enforce minimum/maximum.
		if limit < 1 || limit > adminInstanceAuditMaxLimit {
			return admingen.AdminInstanceSettingsAuditPage{}, apierrors.NewBadRequestError(fmt.Sprintf(
				"invalid limit %d: must be between 1 and %d", limit, adminInstanceAuditMaxLimit))
		}
	}
	var cursor *models.InstanceSettingsAuditCursor
	if cursorParam != nil {
		decoded, err := decodeAdminInstanceAuditCursor(*cursorParam)
		if err != nil {
			return admingen.AdminInstanceSettingsAuditPage{}, apierrors.NewBadRequestError(err.Error())
		}
		cursor = &decoded
	}

	rows, next, err := a.s.container.InstanceSettingsAuditRepository().List(ctx, setting, limit, cursor)
	if err != nil {
		return admingen.AdminInstanceSettingsAuditPage{}, a.adminInternalError(handler, err)
	}

	page, err := a.toGenAdminInstanceSettingsAuditPage(ctx, rows, next, filter)
	if err != nil {
		return admingen.AdminInstanceSettingsAuditPage{}, a.adminInternalError(handler, err)
	}
	return page, nil
}

// mapInstanceEmailError turns the service's errors into their HTTP shapes:
// validation is a 400 with the offending fields, nothing stored is a 409, and
// anything else is a logged 500 (the error, never the request body).
func (a *adminStrictServer) mapInstanceEmailError(handler string, err error) error {
	switch {
	case errors.Is(err, services.ErrTeamEmailProviderValidation):
		return apierrors.NewInstanceEmailProviderValidationError(
			adminMsgInstanceEmailInvalid, teamEmailProviderValidationErrors(err))
	case errors.Is(err, repositories.ErrInstanceEmailProviderNotFound):
		return apierrors.NewInstanceEmailProviderNotConfiguredError()
	default:
		return a.adminInternalError(handler, err)
	}
}

// --- request mapping ----------------------------------------------------------

func toModelInstanceEmailUpsert(
	body admingen.AdminInstanceEmailSettingsRequest,
) models.UpsertInstanceEmailProviderRequest {
	return models.UpsertInstanceEmailProviderRequest{
		UpsertTeamEmailProviderRequest: models.UpsertTeamEmailProviderRequest{
			ProviderType: string(body.ProviderType),
			Settings:     toModelEmailProviderSettings(body.Settings),
			Secret:       body.Secret,
			FromAddress:  string(body.FromAddress),
			FromName:     body.FromName,
			ReplyTo:      emailPtrToString(body.ReplyTo),
		},
		ContactRecipientAddress: emailPtrToString(body.ContactRecipientAddress),
		PrivacyPolicyURL:        body.PrivacyPolicyUrl,
	}
}

// toModelInstanceEmailTestConfig maps a test body onto the candidate
// configuration. A missing body, or one with no field set, is nil: the service
// then tests the stored configuration.
func toModelInstanceEmailTestConfig(
	body *admingen.AdminInstanceEmailTestRequest,
) *models.UpsertInstanceEmailProviderRequest {
	if body == nil || *body == (admingen.AdminInstanceEmailTestRequest{}) {
		return nil
	}
	providerType := ""
	if body.ProviderType != nil {
		providerType = string(*body.ProviderType)
	}
	fromAddress := ""
	if body.FromAddress != nil {
		fromAddress = string(*body.FromAddress)
	}
	return &models.UpsertInstanceEmailProviderRequest{
		UpsertTeamEmailProviderRequest: models.UpsertTeamEmailProviderRequest{
			ProviderType: providerType,
			Settings:     toModelEmailProviderSettings(body.Settings),
			Secret:       body.Secret,
			FromAddress:  fromAddress,
			FromName:     body.FromName,
			ReplyTo:      emailPtrToString(body.ReplyTo),
		},
		ContactRecipientAddress: emailPtrToString(body.ContactRecipientAddress),
		PrivacyPolicyURL:        body.PrivacyPolicyUrl,
	}
}

func toModelEmailProviderSettings(s *admingen.TeamEmailProviderSettings) models.TeamEmailProviderSettings {
	var out models.TeamEmailProviderSettings
	if s == nil {
		return out
	}
	if s.Smtp != nil {
		out.SMTP = &models.SMTPProviderSettings{
			Host: s.Smtp.Host, Port: s.Smtp.Port, Username: optionalStringValue(s.Smtp.Username),
		}
	}
	if s.Mailgun != nil {
		out.Mailgun = &models.MailgunProviderSettings{
			Domain: s.Mailgun.Domain, BaseURL: optionalStringValue(s.Mailgun.BaseUrl),
		}
	}
	if s.Postmark != nil {
		out.Postmark = &models.PostmarkProviderSettings{MessageStream: optionalStringValue(s.Postmark.MessageStream)}
	}
	return out
}

func emailPtrToString(e *openapi_types.Email) *string {
	if e == nil {
		return nil
	}
	s := string(*e)
	return &s
}

// --- response mapping ---------------------------------------------------------

// toGenAdminInstanceEmailSettings maps the effective view field by field, so a
// field added to the service model cannot reach this surface unless it is
// deliberately added here and to the schema.
func toGenAdminInstanceEmailSettings(
	eff *models.InstanceEmailProviderEffective,
) (admingen.AdminInstanceEmailSettings, error) {
	updatedBy, err := optionalGenUUID(eff.UpdatedBy)
	if err != nil {
		return admingen.AdminInstanceEmailSettings{}, fmt.Errorf("updated_by: %w", err)
	}

	out := admingen.AdminInstanceEmailSettings{
		Configured:              eff.Configured,
		HasCredential:           eff.HasCredential,
		Settings:                toGenEmailProviderSettings(eff.Settings),
		FromName:                eff.FromName,
		ReplyTo:                 eff.ReplyTo,
		ContactRecipientAddress: eff.ContactRecipientAddress,
		PrivacyPolicyUrl:        eff.PrivacyPolicyURL,
		IsHealthy:               eff.IsHealthy,
		LastSuccessAt:           eff.LastSuccessAt,
		LastError:               eff.LastError,
		LastErrorAt:             eff.LastErrorAt,
		UpdatedAt:               eff.UpdatedAt,
		UpdatedBy:               updatedBy,
	}
	if eff.ProviderType != nil {
		providerType := admingen.AdminInstanceEmailSettingsProviderType(*eff.ProviderType)
		out.ProviderType = &providerType
	}
	if eff.Configured {
		fromAddress := eff.FromAddress
		out.FromAddress = &fromAddress
	}
	return out, nil
}

func toGenEmailProviderSettings(s *models.TeamEmailProviderSettings) *admingen.TeamEmailProviderSettings {
	if s == nil {
		return nil
	}
	out := &admingen.TeamEmailProviderSettings{}
	if s.SMTP != nil {
		out.Smtp = &admingen.SMTPProviderSettings{
			Host: s.SMTP.Host, Port: s.SMTP.Port, Username: optionalString(s.SMTP.Username),
		}
	}
	if s.Mailgun != nil {
		out.Mailgun = &admingen.MailgunProviderSettings{
			Domain: s.Mailgun.Domain, BaseUrl: optionalString(s.Mailgun.BaseURL),
		}
	}
	if s.Postmark != nil {
		out.Postmark = &admingen.PostmarkProviderSettings{MessageStream: optionalString(s.Postmark.MessageStream)}
	}
	return out
}

func toGenAdminInstanceEmailTestResult(r *models.TeamEmailProviderTestResult) admingen.TeamEmailProviderTestResponse {
	out := admingen.TeamEmailProviderTestResponse{
		IsValid:   r.Success,
		Message:   r.Message,
		Recipient: r.Recipient,
	}
	if r.ErrorDetails != "" {
		details := admingen.TeamEmailProviderTestDetailsErrorDetails(r.ErrorDetails)
		out.Details.ErrorDetails = &details
	}
	return out
}

// instanceEmailAuditSnapshotKeys is the allowlist of keys an email provider
// audit snapshot may surface (services.InstanceEmailAuditSnapshot's shape).
// The snapshot is already redacted at write time; filtering again here means a
// key added to the stored shape later cannot reach this surface unreviewed.
var instanceEmailAuditSnapshotKeys = []string{
	"provider_type", "settings", "from_address", "from_name", "reply_to",
	"contact_recipient_address", "privacy_policy_url", "has_credential", "secret",
	"last_success_at", "last_error", "last_error_at", "created_at", "updated_at",
	"updated_by", "version",
}

func (a *adminStrictServer) toGenAdminInstanceSettingsAuditPage(
	ctx context.Context, rows []*models.InstanceSettingsAuditEntry, next *models.InstanceSettingsAuditCursor,
	filter func(json.RawMessage) *map[string]interface{},
) (admingen.AdminInstanceSettingsAuditPage, error) {
	names := a.instanceAuditActorNames(ctx, rows)

	// make(...,0,...): `entries` is a required array on a generated type.
	entries := make([]admingen.AdminInstanceSettingsAuditEntry, 0, len(rows))
	for _, row := range rows {
		entry, err := toGenAdminInstanceSettingsAuditEntry(row, names, filter)
		if err != nil {
			return admingen.AdminInstanceSettingsAuditPage{}, err
		}
		entries = append(entries, entry)
	}

	page := admingen.AdminInstanceSettingsAuditPage{Entries: entries}
	if next != nil {
		encoded, err := encodeAdminInstanceAuditCursor(*next)
		if err != nil {
			return admingen.AdminInstanceSettingsAuditPage{}, err
		}
		page.NextCursor = &encoded
	}
	return page, nil
}

// instanceAuditActorNames resolves the page's actors to display names. A
// lookup failure degrades to null names rather than failing the read.
func (a *adminStrictServer) instanceAuditActorNames(
	ctx context.Context, rows []*models.InstanceSettingsAuditEntry,
) map[string]string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.ActorUserID != nil && *row.ActorUserID != "" && !slices.Contains(ids, *row.ActorUserID) {
			ids = append(ids, *row.ActorUserID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	names, err := a.s.container.UserRepository().GetNamesByIDs(ctx, ids)
	if err != nil {
		a.s.logger.Warn("Failed to resolve instance settings audit actor names",
			"service", serverLogServiceName, "error", err)
		return nil
	}
	return names
}

func toGenAdminInstanceSettingsAuditEntry(
	row *models.InstanceSettingsAuditEntry, names map[string]string,
	filter func(json.RawMessage) *map[string]interface{},
) (admingen.AdminInstanceSettingsAuditEntry, error) {
	id, err := parseAdminUUID("instance settings audit entry", row.ID)
	if err != nil {
		return admingen.AdminInstanceSettingsAuditEntry{}, err
	}
	actorID, err := optionalGenUUID(row.ActorUserID)
	if err != nil {
		return admingen.AdminInstanceSettingsAuditEntry{}, fmt.Errorf("actor_user_id: %w", err)
	}
	var actorName *string
	if row.ActorUserID != nil {
		if name, ok := names[*row.ActorUserID]; ok {
			actorName = &name
		}
	}

	return admingen.AdminInstanceSettingsAuditEntry{
		Id:          id,
		Setting:     admingen.AdminInstanceSettingsAuditEntrySetting(row.Setting),
		Action:      admingen.AdminInstanceSettingsAuditEntryAction(row.Action),
		ActorUserId: actorID,
		ActorName:   actorName,
		Before:      filter(row.Before),
		After:       filter(row.After),
		CreatedAt:   row.CreatedAt,
	}, nil
}

// filterInstanceEmailAuditSnapshot keeps only the email provider snapshot's
// allowlisted keys. The `secret` key is kept only when it holds one of the two
// redaction markers.
func filterInstanceEmailAuditSnapshot(raw json.RawMessage) *map[string]interface{} {
	filtered := filterInstanceAuditSnapshot(raw, instanceEmailAuditSnapshotKeys)
	if filtered == nil {
		return nil
	}
	if marker, _ := (*filtered)["secret"].(string); marker != models.InstanceSettingsAuditSecretChanged &&
		marker != models.InstanceSettingsAuditSecretUnchanged {
		delete(*filtered, "secret")
	}
	return filtered
}

// filterInstanceAuditSnapshot decodes a stored snapshot and keeps only the
// allowlisted keys. A missing snapshot (the create side of an upsert, the after
// side of a delete) is null.
func filterInstanceAuditSnapshot(raw json.RawMessage, keys []string) *map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded == nil {
		return nil
	}
	filtered := make(map[string]interface{}, len(keys))
	for key, value := range decoded {
		if slices.Contains(keys, key) {
			filtered[key] = value
		}
	}
	return &filtered
}

// --- cursor -------------------------------------------------------------------

// adminInstanceAuditCursorWire is the JSON inside an opaque audit cursor.
type adminInstanceAuditCursorWire struct {
	CreatedAt string `json:"t"`
	ID        string `json:"id"`
}

// encodeAdminInstanceAuditCursor serializes a keyset position as unpadded
// base64url JSON, so it needs no escaping in a query string.
func encodeAdminInstanceAuditCursor(c models.InstanceSettingsAuditCursor) (string, error) {
	raw, err := json.Marshal(adminInstanceAuditCursorWire{
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339Nano),
		ID:        c.ID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to encode audit cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// decodeAdminInstanceAuditCursor parses and validates an opaque cursor; every
// field is checked, so a tampered cursor is a 400 rather than a query error.
func decodeAdminInstanceAuditCursor(cursor string) (models.InstanceSettingsAuditCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return models.InstanceSettingsAuditCursor{}, errAdminInstanceAuditCursor
	}
	var wire adminInstanceAuditCursorWire
	if json.Unmarshal(raw, &wire) != nil {
		return models.InstanceSettingsAuditCursor{}, errAdminInstanceAuditCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, wire.CreatedAt)
	if err != nil {
		return models.InstanceSettingsAuditCursor{}, errAdminInstanceAuditCursor
	}
	if _, idErr := uuid.Parse(wire.ID); idErr != nil {
		return models.InstanceSettingsAuditCursor{}, errAdminInstanceAuditCursor
	}
	return models.InstanceSettingsAuditCursor{CreatedAt: createdAt, ID: wire.ID}, nil
}

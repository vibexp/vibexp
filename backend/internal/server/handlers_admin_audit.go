package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// The instance audit log for instance admins (#1516, epic #1510): a keyset
// listing and a CSV export of the same filter. Both bind their params through
// toAuditEventFilter, so an export can never disagree with the list.
//
// Authorization is instanceAdminMiddleware, which 404s every non-admin and
// anonymous caller on these routes.

const (
	// adminAuditDefaultLimit and adminAuditMaxLimit mirror the spec's limit
	// default and maximum; the generated binder enforces neither.
	adminAuditDefaultLimit = 50
	adminAuditMaxLimit     = 200
	// adminAuditMaxQueryLength mirrors the spec's maxLength on q, counted in
	// characters.
	adminAuditMaxQueryLength = 200
	// adminAuditMaxResourceTypeLength mirrors the spec's maxLength on
	// resource_type.
	adminAuditMaxResourceTypeLength = 64
)

// errAdminAuditCursor is the 400 detail for a cursor this server did not issue.
var errAdminAuditCursor = errors.New("invalid cursor")

// ListAdminAuditEvents returns one page of the audit log, newest first.
func (a *adminStrictServer) ListAdminAuditEvents(
	ctx context.Context, request admingen.ListAdminAuditEventsRequestObject,
) (admingen.ListAdminAuditEventsResponseObject, error) {
	const handler = "ListAdminAuditEvents"
	filter, err := toAuditEventFilter(request.Params)
	if err != nil {
		return nil, err
	}

	events, next, err := a.s.container.AdminAuditService().List(ctx, filter)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	page, err := toGenAdminAuditEventPage(events, next)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.ListAdminAuditEvents200JSONResponse(page), nil
}

// ExportAdminAuditEvents streams the filtered audit log as CSV.
func (a *adminStrictServer) ExportAdminAuditEvents(
	ctx context.Context, request admingen.ExportAdminAuditEventsRequestObject,
) (admingen.ExportAdminAuditEventsResponseObject, error) {
	const handler = "ExportAdminAuditEvents"
	params, err := adminExportParamsToList[admingen.ExportAdminAuditEventsParams, admingen.ListAdminAuditEventsParams](
		request.Params)
	if err != nil {
		return nil, a.logExportFailure(handler, err)
	}
	filter, err := toAuditEventFilter(params)
	if err != nil {
		return nil, err
	}

	exp, err := a.s.container.AdminAuditService().Export(ctx, filter)
	if err != nil {
		return nil, a.logExportFailure(handler, err)
	}

	body := a.startAdminExport(ctx, adminExportRequest{
		list: "audit", activityType: activities.ActivityTypeAdminAuditExported, params: request.Params,
	}, exp)
	return admingen.ExportAdminAuditEvents200TextcsvResponse{
		Body: body,
		Headers: admingen.ExportAdminAuditEvents200ResponseHeaders{
			ContentDisposition: adminExportFilename("audit", time.Now()),
			XExportTotalCount:  int64(exp.TotalCount),
			XExportTruncated:   exp.Truncated,
		},
	}, nil
}

// toAuditEventFilter validates the generated params and maps them onto the
// repository filter. The binder already rejects malformed ids and instants;
// this checks what it does not: the enums, the bounds and the cursor.
func toAuditEventFilter(p admingen.ListAdminAuditEventsParams) (models.AuditEventFilter, error) {
	f := models.AuditEventFilter{
		ActorUserID: uuidParamString(p.ActorUserId),
		TeamID:      uuidParamString(p.TeamId),
		ProjectID:   uuidParamString(p.ProjectId),
		From:        p.From,
		To:          p.To,
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return models.AuditEventFilter{}, apierrors.NewBadRequestError("invalid range: from must be before to")
	}
	steps := []func(*models.AuditEventFilter, admingen.ListAdminAuditEventsParams) error{
		applyAuditPaging, applyAuditText, applyAuditEnums,
	}
	for _, step := range steps {
		if err := step(&f, p); err != nil {
			return models.AuditEventFilter{}, err
		}
	}
	return f, nil
}

// applyAuditPaging sets the page size (default adminAuditDefaultLimit) and the
// keyset cursor.
func applyAuditPaging(f *models.AuditEventFilter, p admingen.ListAdminAuditEventsParams) error {
	f.Limit = adminAuditDefaultLimit
	if p.Limit != nil {
		if *p.Limit < 1 || *p.Limit > adminAuditMaxLimit {
			return apierrors.NewBadRequestError(fmt.Sprintf(
				"invalid limit %d: must be between 1 and %d", *p.Limit, adminAuditMaxLimit))
		}
		f.Limit = *p.Limit
	}
	if p.Cursor != nil {
		cursor, err := decodeAdminAuditCursor(*p.Cursor)
		if err != nil {
			return apierrors.NewBadRequestError(err.Error())
		}
		f.After = &cursor
	}
	return nil
}

// applyAuditText sets the length-bounded text filters. A blank q filters
// nothing.
func applyAuditText(f *models.AuditEventFilter, p admingen.ListAdminAuditEventsParams) error {
	if p.ResourceType != nil {
		if utf8.RuneCountInString(*p.ResourceType) > adminAuditMaxResourceTypeLength {
			return apierrors.NewBadRequestError(fmt.Sprintf(
				"invalid resource_type: must be at most %d characters", adminAuditMaxResourceTypeLength))
		}
		f.ResourceType = p.ResourceType
	}
	if p.Q != nil {
		if utf8.RuneCountInString(*p.Q) > adminAuditMaxQueryLength {
			return apierrors.NewBadRequestError(fmt.Sprintf(
				"invalid q: must be at most %d characters", adminAuditMaxQueryLength))
		}
		if strings.TrimSpace(*p.Q) != "" {
			f.Query = p.Q
		}
	}
	return nil
}

// applyAuditEnums sets the repeatable enum filters.
func applyAuditEnums(f *models.AuditEventFilter, p admingen.ListAdminAuditEventsParams) error {
	var err error
	if f.Operations, err = auditEnumFilter("operation", p.Operation,
		func(v admingen.AdminAuditOperation) (models.AuditOperation, bool) {
			return models.AuditOperation(v), v.Valid()
		}); err != nil {
		return err
	}
	if f.Sources, err = auditEnumFilter("source", p.Source,
		func(v admingen.AdminAuditSource) (models.AuditSource, bool) {
			return models.AuditSource(v), v.Valid()
		}); err != nil {
		return err
	}
	f.Outcomes, err = auditEnumFilter("outcome", p.Outcome,
		func(v admingen.AdminAuditOutcome) (models.AuditOutcome, bool) {
			return models.AuditOutcome(v), v.Valid()
		})
	return err
}

// auditEnumFilter converts a repeatable enum param, rejecting a value outside
// the enum (the generated binder does not check enums). Nil stays nil: no
// filter.
func auditEnumFilter[G ~string, M any](name string, values *[]G, convert func(G) (M, bool)) ([]M, error) {
	if values == nil || len(*values) == 0 {
		return nil, nil
	}
	out := make([]M, 0, len(*values))
	for _, v := range *values {
		m, ok := convert(v)
		if err := validateAdminSortEnum(name, string(v), ok); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// uuidParamString renders an optional UUID query param as the filter's string.
func uuidParamString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

// encodeAdminAuditCursor renders the keyset position as an opaque cursor:
// unpadded URL-safe base64 of "<created_at RFC 3339 nano>|<id>", so it never
// needs URL-encoding.
func encodeAdminAuditCursor(c models.AuditCursor) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID))
}

// decodeAdminAuditCursor parses and validates a cursor; every part is checked,
// so a tampered cursor is a 400 rather than a query error.
func decodeAdminAuditCursor(cursor string) (models.AuditCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return models.AuditCursor{}, errAdminAuditCursor
	}
	createdAt, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return models.AuditCursor{}, errAdminAuditCursor
	}
	at, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return models.AuditCursor{}, errAdminAuditCursor
	}
	if _, err := uuid.Parse(id); err != nil {
		return models.AuditCursor{}, errAdminAuditCursor
	}
	return models.AuditCursor{CreatedAt: at, ID: id}, nil
}

func toGenAdminAuditEventPage(
	events []models.AuditEvent, next *models.AuditCursor,
) (admingen.AdminAuditEventPage, error) {
	items := make([]admingen.AdminAuditEvent, 0, len(events))
	for i := range events {
		item, err := toGenAdminAuditEvent(&events[i])
		if err != nil {
			return admingen.AdminAuditEventPage{}, err
		}
		items = append(items, item)
	}
	page := admingen.AdminAuditEventPage{Items: items}
	if next != nil {
		cursor := encodeAdminAuditCursor(*next)
		page.NextCursor = &cursor
	}
	return page, nil
}

func toGenAdminAuditEvent(e *models.AuditEvent) (admingen.AdminAuditEvent, error) {
	id, err := parseAdminUUID("audit event", e.ID)
	if err != nil {
		return admingen.AdminAuditEvent{}, err
	}
	metadata := map[string]interface{}{}
	if len(e.Metadata) > 0 {
		if err := json.Unmarshal(e.Metadata, &metadata); err != nil {
			return admingen.AdminAuditEvent{}, fmt.Errorf("decode audit event %s metadata: %w", e.ID, err)
		}
		if metadata == nil { // a stored JSON null
			metadata = map[string]interface{}{}
		}
	}
	return admingen.AdminAuditEvent{
		Id:            id,
		CreatedAt:     e.CreatedAt,
		ActorUserId:   e.ActorUserID,
		ActorEmail:    e.ActorEmail,
		ActorName:     e.ActorName,
		ActorType:     admingen.AdminAuditActorType(e.ActorType),
		TeamId:        e.TeamID,
		TeamName:      e.TeamName,
		ProjectId:     e.ProjectID,
		ProjectName:   e.ProjectName,
		Operation:     admingen.AdminAuditOperation(e.Operation),
		ResourceType:  e.ResourceType,
		ResourceId:    e.ResourceID,
		ResourceName:  e.ResourceName,
		Source:        admingen.AdminAuditSource(e.Source),
		AssistantName: e.AssistantName,
		ApiKeyId:      e.APIKeyID,
		ApiKeyName:    e.APIKeyName,
		Outcome:       admingen.AdminAuditOutcome(e.Outcome),
		Ip:            e.IP,
		UserAgent:     e.UserAgent,
		RequestId:     e.RequestID,
		Metadata:      metadata,
	}, nil
}

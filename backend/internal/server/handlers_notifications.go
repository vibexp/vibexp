package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/server/gen"
	"github.com/vibexp/vibexp/internal/services/notifications"
)

// notificationsStrictServer implements gen.StrictServerInterface (#1713 PoC):
// the four REST notification operations are served through oapi-codegen
// strict-server bindings generated from openapi.yaml, so a spec/handler
// payload mismatch is a compile error for this domain. The two
// /internal/jobs/notifications/* handlers below are spec-exempt and stay
// hand-written.
type notificationsStrictServer struct {
	s *Server
}

var _ gen.StrictServerInterface = (*notificationsStrictServer)(nil)

// Strict handler implementations return *apierrors.APIError directly (it
// implements error). notificationsResponseErrorHandler renders it as RFC 9457
// application/problem+json — the typed gen.*JSONResponse error bodies would
// write application/json, so they are deliberately bypassed (see #1768).

// ListNotifications handles GET /api/v1/notifications
func (n *notificationsStrictServer) ListNotifications(
	ctx context.Context, request gen.ListNotificationsRequestObject,
) (gen.ListNotificationsResponseObject, error) {
	userID := ctx.Value(contextKeyUserID).(string)

	filters, err := validateNotificationListParams(request.Params)
	if err != nil {
		return nil, err
	}

	items, listErr := n.s.container.NotificationService().ListForUser(ctx, userID, filters)
	if listErr != nil {
		n.s.logger.With(
			"handler", "ListNotifications",
			"user_id", userID,
			"error", listErr.Error(),
		).Error("Failed to list notifications")

		return nil, apierrors.NewInternalError("Failed to list notifications")
	}

	genItems, convErr := toGenNotifications(items)
	if convErr != nil {
		n.s.logger.With(
			"handler", "ListNotifications",
			"user_id", userID,
			"error", convErr.Error(),
		).Error("Failed to convert notifications to spec types")

		return nil, apierrors.NewInternalError("Failed to list notifications")
	}

	return gen.ListNotifications200JSONResponse(gen.NotificationListResponse{
		Notifications: genItems,
		Count:         len(items),
		Limit:         filters.Limit,
		Offset:        filters.Offset,
	}), nil
}

// GetUnreadNotificationCount handles GET /api/v1/notifications/unread-count
func (n *notificationsStrictServer) GetUnreadNotificationCount(
	ctx context.Context, _ gen.GetUnreadNotificationCountRequestObject,
) (gen.GetUnreadNotificationCountResponseObject, error) {
	userID := ctx.Value(contextKeyUserID).(string)

	count, err := n.s.container.NotificationService().GetUnreadCount(ctx, userID)
	if err != nil {
		n.s.logger.With(
			"handler", "GetUnreadNotificationCount",
			"user_id", userID,
			"error", err.Error(),
		).Error("Failed to get unread notification count")

		return nil, apierrors.NewInternalError("Failed to get unread notification count")
	}

	return gen.GetUnreadNotificationCount200JSONResponse(gen.UnreadCountResponse{UnreadCount: count}), nil
}

// MarkNotificationRead handles PATCH /api/v1/notifications/{id}/read.
// A non-UUID id never reaches this method — the generated layer rejects it
// during binding (see notificationsBindErrorHandler).
func (n *notificationsStrictServer) MarkNotificationRead(
	ctx context.Context, request gen.MarkNotificationReadRequestObject,
) (gen.MarkNotificationReadResponseObject, error) {
	userID := ctx.Value(contextKeyUserID).(string)
	notifID := request.Id.String()

	if err := n.s.container.NotificationService().MarkRead(ctx, userID, notifID); err != nil {
		n.s.logger.With(
			"handler", "MarkNotificationRead",
			"user_id", userID,
			"notification_id", notifID,
			"error", err.Error(),
		).Error("Failed to mark notification as read")

		return nil, apierrors.NewInternalError("Failed to mark notification as read")
	}

	return gen.MarkNotificationRead204Response{}, nil
}

// MarkAllNotificationsRead handles PATCH /api/v1/notifications/read-all
func (n *notificationsStrictServer) MarkAllNotificationsRead(
	ctx context.Context, _ gen.MarkAllNotificationsReadRequestObject,
) (gen.MarkAllNotificationsReadResponseObject, error) {
	userID := ctx.Value(contextKeyUserID).(string)

	if err := n.s.container.NotificationService().MarkAllRead(ctx, userID); err != nil {
		n.s.logger.With(
			"handler", "MarkAllNotificationsRead",
			"user_id", userID,
			"error", err.Error(),
		).Error("Failed to mark all notifications as read")

		return nil, apierrors.NewInternalError("Failed to mark all notifications as read")
	}

	return gen.MarkAllNotificationsRead204Response{}, nil
}

// validateNotificationListParams applies defaults and bounds to the generated
// query params. The generated layer binds types only — spec minimum/maximum
// constraints are NOT enforced by oapi-codegen — so the bounds keep their
// historical hand-rolled checks and 400 messages here.
func validateNotificationListParams(params gen.ListNotificationsParams) (notifications.ListFilters, error) {
	const defaultLimit = 20
	const maxAllowedLimit = 100

	f := notifications.ListFilters{Limit: defaultLimit}

	if params.Limit != nil {
		if *params.Limit <= 0 || *params.Limit > maxAllowedLimit {
			return f, apierrors.NewBadRequestError("limit must be an integer between 1 and 100")
		}
		f.Limit = *params.Limit
	}

	if params.Offset != nil {
		if *params.Offset < 0 {
			return f, apierrors.NewBadRequestError("offset must be a non-negative integer")
		}
		f.Offset = *params.Offset
	}

	if params.Unread != nil {
		f.UnreadOnly = *params.Unread
	}

	return f, nil
}

// toGenNotifications converts service notifications to the generated spec
// types. Per-field copying is the adapter cost of the strict-server layer;
// in exchange the response shape can no longer drift from openapi.yaml.
func toGenNotifications(items []*notifications.Notification) ([]gen.Notification, error) {
	out := make([]gen.Notification, 0, len(items))
	for _, item := range items {
		g, err := toGenNotification(item)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func toGenNotification(item *notifications.Notification) (gen.Notification, error) {
	id, err := uuid.Parse(item.ID)
	if err != nil {
		// Cannot happen with database-generated IDs; failing loudly beats
		// serving a zero UUID that silently violates the documented format.
		return gen.Notification{}, fmt.Errorf("notification id %q is not a UUID: %w", item.ID, err)
	}

	g := gen.Notification{
		Id:          id,
		Type:        string(item.Type),
		Category:    gen.NotificationCategory(item.Category),
		Title:       item.Title,
		ReadAt:      item.ReadAt,
		DismissedAt: item.DismissedAt,
		CreatedAt:   item.CreatedAt,
	}

	// omitempty parity with the previous hand-written wire shape: empty
	// strings/maps are omitted, not serialized as "" / {}.
	if item.TeamID != "" {
		g.TeamId = &item.TeamID
	}
	if item.Body != "" {
		g.Body = &item.Body
	}
	if item.ActionURL != "" {
		g.ActionUrl = &item.ActionURL
	}
	if len(item.EntityRef) > 0 {
		g.EntityRef = &item.EntityRef
	}

	return g, nil
}

// notificationsBindErrorHandler translates parameter-binding failures from
// the generated layer into this domain's historical RFC 9457 400 responses
// (the generated default would write a plain-text 400).
func (s *Server) notificationsBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()

	var invalidParam *gen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		switch invalidParam.ParamName {
		case "limit":
			msg = "limit must be an integer between 1 and 100"
		case "offset":
			msg = "offset must be a non-negative integer"
		case "unread":
			msg = "unread must be a boolean"
		case "id":
			msg = "notification id must be a valid UUID"
		}
	}

	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// notificationsResponseErrorHandler writes errors returned by the strict
// handler implementations. *apierrors.APIError carries the intended RFC 9457
// error; anything else is defensive and maps to a generic 500.
func (s *Server) notificationsResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Notifications strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}

// handleNotificationRetentionJob handles POST /internal/jobs/notifications/retention
// Protected by pubSubOIDCMiddleware (Cloud Scheduler → OIDC-authenticated HTTP).
func (s *Server) handleNotificationRetentionJob(w http.ResponseWriter, r *http.Request) {
	if err := s.container.NotificationService().RunRetentionJob(r.Context()); err != nil {
		s.logger.With(
			"handler", "handleNotificationRetentionJob",
			"error", err.Error(),
		).Error("Notification retention job failed")

		apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Retention job failed"))

		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		s.logger.With("error", err).Error("Failed to write retention job response")
	}
}

// handleNotificationDigestJob handles POST /internal/jobs/notifications/digest
// Protected by pubSubOIDCMiddleware (Cloud Scheduler → OIDC-authenticated HTTP).
func (s *Server) handleNotificationDigestJob(w http.ResponseWriter, r *http.Request) {
	if err := s.container.DigestRunner().Run(r.Context(), time.Now().UTC()); err != nil {
		s.logger.With(
			"handler", "handleNotificationDigestJob",
			"error", err.Error(),
		).Error("Notification digest job failed")

		apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Digest job failed"))

		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		s.logger.With("error", err).Error("Failed to write digest job response")
	}
}

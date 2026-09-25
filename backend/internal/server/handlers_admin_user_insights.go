package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
)

// User detail insights handlers (#1135). All four are read-only and mounted
// through setupAdminRoutes, so instanceAdminMiddleware has already 404'd
// non-admins. None of their responses carries a resource title, slug or body.

// adminResourceShortIDLen is how much of a resource id the timeline exposes,
// following the activities/freshness precedent of an 8-character prefix.
const adminResourceShortIDLen = 8

// adminMsgUserNotFound is the 404 detail for an unknown user id.
const adminMsgUserNotFound = "User not found"

// GetAdminUserInsights returns per-type counts of the resources a user authored.
func (a *adminStrictServer) GetAdminUserInsights(
	ctx context.Context, request admingen.GetAdminUserInsightsRequestObject,
) (admingen.GetAdminUserInsightsResponseObject, error) {
	insights, err := a.s.container.AdminService().GetUserInsights(ctx, request.Id.String())
	if err != nil {
		return nil, a.adminInternalError("GetAdminUserInsights", err)
	}
	if insights == nil {
		return nil, apierrors.NewResourceNotFoundError("user", adminMsgUserNotFound)
	}

	resp, err := toGenAdminUserInsights(insights)
	if err != nil {
		return nil, a.adminInternalError("GetAdminUserInsights", err)
	}
	return admingen.GetAdminUserInsights200JSONResponse(resp), nil
}

// GetAdminUserResourceCreationMetrics returns the user's gap-filled creation
// series, stacked by type.
func (a *adminStrictServer) GetAdminUserResourceCreationMetrics(
	ctx context.Context, request admingen.GetAdminUserResourceCreationMetricsRequestObject,
) (admingen.GetAdminUserResourceCreationMetricsResponseObject, error) {
	granularity, err := adminGranularityParam(request.Params.Granularity)
	if err != nil {
		return nil, err
	}

	metrics, err := a.s.container.AdminService().GetUserCreationMetrics(ctx, request.Id.String(),
		services.AdminTimeseriesQuery{
			From:        request.Params.From,
			To:          request.Params.To,
			Granularity: granularity,
		})
	if err != nil {
		var rangeErr *services.ErrAdminTimeseriesRange
		if errors.As(err, &rangeErr) {
			return nil, apierrors.NewBadRequestError(rangeErr.Detail)
		}
		return nil, a.adminInternalError("GetAdminUserResourceCreationMetrics", err)
	}
	if metrics == nil {
		return nil, apierrors.NewResourceNotFoundError("user", adminMsgUserNotFound)
	}

	return admingen.GetAdminUserResourceCreationMetrics200JSONResponse(toGenAdminUserCreationMetrics(metrics)), nil
}

// GetAdminUserTimeline returns one page of the user's opaque resource timeline.
func (a *adminStrictServer) GetAdminUserTimeline(
	ctx context.Context, request admingen.GetAdminUserTimelineRequestObject,
) (admingen.GetAdminUserTimelineResponseObject, error) {
	limit := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
		// The generated binder does not enforce minimum/maximum.
		if limit < 1 || limit > services.AdminUserTimelineMaxLimit {
			return nil, apierrors.NewBadRequestError(
				fmt.Sprintf("invalid limit %d: must be between 1 and %d", limit, services.AdminUserTimelineMaxLimit))
		}
	}
	cursor := ""
	if request.Params.Cursor != nil {
		cursor = *request.Params.Cursor
	}

	page, err := a.s.container.AdminService().GetUserTimeline(ctx, request.Id.String(), cursor, limit)
	if err != nil {
		var cursorErr *services.ErrAdminInvalidCursor
		if errors.As(err, &cursorErr) {
			return nil, apierrors.NewBadRequestError(cursorErr.Detail)
		}
		return nil, a.adminInternalError("GetAdminUserTimeline", err)
	}
	if page == nil {
		return nil, apierrors.NewResourceNotFoundError("user", adminMsgUserNotFound)
	}

	resp, err := toGenAdminUserTimelinePage(page)
	if err != nil {
		return nil, a.adminInternalError("GetAdminUserTimeline", err)
	}
	return admingen.GetAdminUserTimeline200JSONResponse(resp), nil
}

// GetAdminUserNotificationPreferences returns the user's notification
// preferences, read-only. GetPreferences returns defaults for ANY id, so the
// 404 for an unknown user comes from a separate existence check.
func (a *adminStrictServer) GetAdminUserNotificationPreferences(
	ctx context.Context, request admingen.GetAdminUserNotificationPreferencesRequestObject,
) (admingen.GetAdminUserNotificationPreferencesResponseObject, error) {
	id := request.Id.String()
	exists, err := a.s.container.AdminService().UserExists(ctx, id)
	if err != nil {
		return nil, a.adminInternalError("GetAdminUserNotificationPreferences", err)
	}
	if !exists {
		return nil, apierrors.NewResourceNotFoundError("user", adminMsgUserNotFound)
	}

	prefs, err := a.s.container.UserPreferencesService().GetPreferences(ctx, id)
	if err != nil {
		return nil, a.adminInternalError("GetAdminUserNotificationPreferences", err)
	}
	return admingen.GetAdminUserNotificationPreferences200JSONResponse(
		toGenAdminUserNotificationPreferences(prefs)), nil
}

// adminInternalError logs an unexpected failure and returns the generic 500.
func (a *adminStrictServer) adminInternalError(handler string, err error) error {
	a.s.logger.With(
		"service", serverLogServiceName, "handler", handler, "error", err,
	).Error("Admin user insights request failed")
	return apierrors.NewInternalError(adminMsgInternalError)
}

// parseAdminUUID parses an id the database handed back.
func parseAdminUUID(what, id string) (openapi_types.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return openapi_types.UUID{}, fmt.Errorf("%s id %q is not a UUID: %w", what, id, err)
	}
	return parsed, nil
}

// toGenAdminUserInsights converts the domain insights, with every required
// array built non-nil (#125).
func toGenAdminUserInsights(in *models.AdminUserInsights) (admingen.AdminUserInsights, error) {
	userID, err := parseAdminUUID("user", in.UserID)
	if err != nil {
		return admingen.AdminUserInsights{}, err
	}

	teams := make([]admingen.AdminUserTeamResourceCounts, 0, len(in.Teams))
	for _, t := range in.Teams {
		teamID, err := parseAdminUUID("team", t.TeamID)
		if err != nil {
			return admingen.AdminUserInsights{}, err
		}
		projects := make([]admingen.AdminUserProjectResourceCounts, 0, len(t.Projects))
		for _, p := range t.Projects {
			projectID, err := parseAdminUUID("project", p.ProjectID)
			if err != nil {
				return admingen.AdminUserInsights{}, err
			}
			projects = append(projects, admingen.AdminUserProjectResourceCounts{
				ProjectId:   projectID,
				ProjectName: p.ProjectName,
				Counts: admingen.AdminProjectResourceCounts{
					Prompts:    p.Counts.Prompts,
					Memories:   p.Counts.Memories,
					Artifacts:  p.Counts.Artifacts,
					Blueprints: p.Counts.Blueprints,
				},
			})
		}
		teams = append(teams, admingen.AdminUserTeamResourceCounts{
			TeamId:   teamID,
			TeamName: t.TeamName,
			IsMember: t.IsMember,
			Counts:   toGenAdminResourceCounts(t.Counts),
			Projects: projects,
		})
	}

	return admingen.AdminUserInsights{
		UserId: userID,
		Totals: toGenAdminResourceCounts(in.Totals),
		Teams:  teams,
	}, nil
}

// toGenAdminUserCreationMetrics converts the domain series.
func toGenAdminUserCreationMetrics(m *models.AdminUserCreationMetrics) admingen.AdminUserResourceCreationMetrics {
	series := make([]admingen.AdminUserCreationPoint, 0, len(m.Series))
	for _, p := range m.Series {
		series = append(series, admingen.AdminUserCreationPoint{
			Bucket:      p.Bucket,
			Prompts:     p.Prompts,
			Memories:    p.Memories,
			Artifacts:   p.Artifacts,
			Blueprints:  p.Blueprints,
			Agents:      p.Agents,
			Feeds:       p.Feeds,
			FeedItems:   p.FeedItems,
			Comments:    p.Comments,
			Attachments: p.Attachments,
		})
	}
	return admingen.AdminUserResourceCreationMetrics{
		From:        m.From,
		To:          m.To,
		Granularity: admingen.AdminUserResourceCreationMetricsGranularity(m.Granularity),
		Series:      series,
	}
}

// toGenAdminUserTimelinePage converts a timeline page. The full resource id
// never leaves the server except inside the opaque cursor.
func toGenAdminUserTimelinePage(p *models.AdminUserTimelinePage) (admingen.AdminUserTimelinePage, error) {
	items := make([]admingen.AdminUserTimelineEvent, 0, len(p.Items))
	for _, ev := range p.Items {
		teamID, err := parseAdminUUID("team", ev.TeamID)
		if err != nil {
			return admingen.AdminUserTimelinePage{}, err
		}
		var projectID *openapi_types.UUID
		if ev.ProjectID != nil {
			parsed, err := parseAdminUUID("project", *ev.ProjectID)
			if err != nil {
				return admingen.AdminUserTimelinePage{}, err
			}
			projectID = &parsed
		}
		shortID := ev.ResourceID
		if len(shortID) > adminResourceShortIDLen {
			shortID = shortID[:adminResourceShortIDLen]
		}
		items = append(items, admingen.AdminUserTimelineEvent{
			ResourceType:    admingen.AdminUserTimelineEventResourceType(ev.ResourceType),
			Action:          admingen.AdminUserTimelineEventAction(ev.Action),
			TeamId:          teamID,
			TeamName:        ev.TeamName,
			ProjectId:       projectID,
			ProjectName:     ev.ProjectName,
			ResourceShortId: shortID,
			OccurredAt:      ev.OccurredAt,
		})
	}
	return admingen.AdminUserTimelinePage{Items: items, NextCursor: p.NextCursor}, nil
}

// toGenAdminUserNotificationPreferences converts the stored (or default)
// preferences. A zero UpdatedAt is how GetPreferences marks the defaults.
func toGenAdminUserNotificationPreferences(p *models.PreferencesResponse) admingen.AdminUserNotificationPreferences {
	email := p.Preferences.EmailNotification
	notif := p.Preferences.Notifications

	types := make(map[string]admingen.NotificationTypePreference, len(notif.Types))
	for name, t := range notif.Types {
		types[name] = admingen.NotificationTypePreference{
			InApp: t.InApp,
			Email: admingen.NotificationTypePreferenceEmail(t.Email),
		}
	}

	resp := admingen.AdminUserNotificationPreferences{
		EmailNotification: admingen.EmailNotificationPreferences{
			PlatformAnnouncement: email.PlatformAnnouncement,
			AccountSecurity:      email.AccountSecurity,
			NewFeature:           email.NewFeature,
			MarketingPromotional: email.MarketingPromotional,
		},
		Notifications: admingen.NotificationPreferences{
			Channels: admingen.NotificationChannelPreferences{
				InApp: notif.Channels.InApp,
				Email: notif.Channels.Email,
			},
			Types: types,
		},
		IsDefault: p.UpdatedAt.IsZero(),
	}
	if !p.UpdatedAt.IsZero() {
		updatedAt := p.UpdatedAt
		resp.UpdatedAt = &updatedAt
	}
	return resp
}

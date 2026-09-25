package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
)

// Per-user resource access handlers (#1136). Both are read-only and mounted
// through setupAdminRoutes, so instanceAdminMiddleware has already 404'd
// non-admins. Neither response carries a resource title, slug or body.

// GetAdminUserResourceAccessMetrics returns the user's gap-filled access series
// per source.
func (a *adminStrictServer) GetAdminUserResourceAccessMetrics(
	ctx context.Context, request admingen.GetAdminUserResourceAccessMetricsRequestObject,
) (admingen.GetAdminUserResourceAccessMetricsResponseObject, error) {
	granularity, err := adminGranularityParam(request.Params.Granularity)
	if err != nil {
		return nil, err
	}

	metrics, err := a.s.container.AdminService().GetUserAccessMetrics(ctx, request.Id.String(),
		services.AdminTimeseriesQuery{
			From:        request.Params.From,
			To:          request.Params.To,
			Granularity: granularity,
		})
	if err != nil {
		return nil, a.adminRangeOrInternalError("GetAdminUserResourceAccessMetrics", err)
	}
	if metrics == nil {
		return nil, apierrors.NewResourceNotFoundError("user", adminMsgUserNotFound)
	}

	access := make([]admingen.AdminSourcePoint, 0, len(metrics.AccessBySource))
	for _, p := range metrics.AccessBySource {
		access = append(access, admingen.AdminSourcePoint{Bucket: p.Bucket, Source: p.Source, Count: p.Count})
	}
	return admingen.GetAdminUserResourceAccessMetrics200JSONResponse(admingen.AdminUserAccessMetrics{
		From:               metrics.From,
		To:                 metrics.To,
		Granularity:        admingen.AdminUserAccessMetricsGranularity(metrics.Granularity),
		AccessBySource:     access,
		EarliestRetainedAt: a.accessEventsEarliestRetainedAt(),
	}), nil
}

// GetAdminUserTopAccessedResources returns the user's most-accessed resources
// as opaque rows.
func (a *adminStrictServer) GetAdminUserTopAccessedResources(
	ctx context.Context, request admingen.GetAdminUserTopAccessedResourcesRequestObject,
) (admingen.GetAdminUserTopAccessedResourcesResponseObject, error) {
	limit := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
		// The generated binder does not enforce minimum/maximum.
		if limit < 1 || limit > services.AdminTopResourcesMaxLimit {
			return nil, apierrors.NewBadRequestError(
				fmt.Sprintf("invalid limit %d: must be between 1 and %d", limit, services.AdminTopResourcesMaxLimit))
		}
	}

	top, err := a.s.container.AdminService().GetUserTopAccessedResources(ctx, request.Id.String(),
		services.AdminTopResourcesQuery{From: request.Params.From, To: request.Params.To, Limit: limit})
	if err != nil {
		return nil, a.adminRangeOrInternalError("GetAdminUserTopAccessedResources", err)
	}
	if top == nil {
		return nil, apierrors.NewResourceNotFoundError("user", adminMsgUserNotFound)
	}

	items, err := toGenAdminTopAccessedResources(top.Items)
	if err != nil {
		return nil, a.adminInternalError("GetAdminUserTopAccessedResources", err)
	}
	return admingen.GetAdminUserTopAccessedResources200JSONResponse(admingen.AdminTopAccessedResourcesResponse{
		From:               top.From,
		To:                 top.To,
		Items:              items,
		EarliestRetainedAt: a.accessEventsEarliestRetainedAt(),
	}), nil
}

// adminRangeOrInternalError maps an invalid range to 400 and anything else to
// the logged generic 500.
func (a *adminStrictServer) adminRangeOrInternalError(handler string, err error) error {
	var rangeErr *services.ErrAdminTimeseriesRange
	if errors.As(err, &rangeErr) {
		return apierrors.NewBadRequestError(rangeErr.Detail)
	}
	return a.adminInternalError(handler, err)
}

// accessEventsEarliestRetainedAt is the oldest access event retention keeps,
// computed as the dashboard's data window computes it.
func (a *adminStrictServer) accessEventsEarliestRetainedAt() time.Time {
	retention := a.s.config.Retention
	return services.AdminDataWindowFor(time.Now(), retention.ActivityDays, retention.AccessEventDays).
		AccessBySourceEarliestRetainedAt
}

// toGenAdminTopAccessedResources converts the opaque rows. The full resource id
// never leaves the server; only its first adminResourceShortIDLen characters.
func toGenAdminTopAccessedResources(
	rows []models.AdminTopAccessedResource,
) ([]admingen.AdminTopAccessedResource, error) {
	items := make([]admingen.AdminTopAccessedResource, 0, len(rows))
	for _, r := range rows {
		teamID, err := parseAdminUUID("team", r.TeamID)
		if err != nil {
			return nil, err
		}
		var projectID *openapi_types.UUID
		if r.ProjectID != nil {
			parsed, err := parseAdminUUID("project", *r.ProjectID)
			if err != nil {
				return nil, err
			}
			projectID = &parsed
		}
		shortID := r.ResourceID
		if len(shortID) > adminResourceShortIDLen {
			shortID = shortID[:adminResourceShortIDLen]
		}
		items = append(items, admingen.AdminTopAccessedResource{
			ResourceType:    admingen.AdminTopAccessedResourceResourceType(r.ResourceType),
			ResourceShortId: shortID,
			TeamId:          teamID,
			TeamName:        r.TeamName,
			ProjectId:       projectID,
			ProjectName:     r.ProjectName,
			ResourceDeleted: r.ResourceDeleted,
			AccessCount:     r.AccessCount,
		})
	}
	return items, nil
}

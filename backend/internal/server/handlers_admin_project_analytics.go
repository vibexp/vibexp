package server

import (
	"context"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
)

// Project detail analytics and configuration handlers (#1145). All are
// read-only and mounted through setupAdminRoutes, so instanceAdminMiddleware
// has already 404'd non-admins. No response carries a resource title, slug or
// body.

// GetAdminProjectResourceCreationMetrics returns the project's gap-filled
// creation series per type.
func (a *adminStrictServer) GetAdminProjectResourceCreationMetrics(
	ctx context.Context, request admingen.GetAdminProjectResourceCreationMetricsRequestObject,
) (admingen.GetAdminProjectResourceCreationMetricsResponseObject, error) {
	granularity, err := adminGranularityParam(request.Params.Granularity)
	if err != nil {
		return nil, err
	}

	metrics, err := a.s.container.AdminService().GetProjectCreationMetrics(ctx, request.Id.String(),
		services.AdminTimeseriesQuery{
			From:        request.Params.From,
			To:          request.Params.To,
			Granularity: granularity,
		})
	if err != nil {
		return nil, a.adminRangeOrInternalError("GetAdminProjectResourceCreationMetrics", err)
	}
	if metrics == nil {
		return nil, apierrors.NewResourceNotFoundError("project", projectMsgNotFound)
	}

	series := make([]admingen.AdminProjectCreationPoint, 0, len(metrics.Series))
	for _, p := range metrics.Series {
		series = append(series, admingen.AdminProjectCreationPoint{
			Bucket:     p.Bucket,
			Prompts:    p.Prompts,
			Memories:   p.Memories,
			Artifacts:  p.Artifacts,
			Blueprints: p.Blueprints,
			FeedItems:  p.FeedItems,
		})
	}
	return admingen.GetAdminProjectResourceCreationMetrics200JSONResponse(
		admingen.AdminProjectResourceCreationMetrics{
			From:        metrics.From,
			To:          metrics.To,
			Granularity: admingen.AdminProjectResourceCreationMetricsGranularity(metrics.Granularity),
			Series:      series,
		}), nil
}

// GetAdminProjectResourceAccessMetrics returns the project's gap-filled access
// series per source.
func (a *adminStrictServer) GetAdminProjectResourceAccessMetrics(
	ctx context.Context, request admingen.GetAdminProjectResourceAccessMetricsRequestObject,
) (admingen.GetAdminProjectResourceAccessMetricsResponseObject, error) {
	granularity, err := adminGranularityParam(request.Params.Granularity)
	if err != nil {
		return nil, err
	}

	metrics, err := a.s.container.AdminService().GetProjectAccessMetrics(ctx, request.Id.String(),
		services.AdminTimeseriesQuery{
			From:        request.Params.From,
			To:          request.Params.To,
			Granularity: granularity,
		})
	if err != nil {
		return nil, a.adminRangeOrInternalError("GetAdminProjectResourceAccessMetrics", err)
	}
	if metrics == nil {
		return nil, apierrors.NewResourceNotFoundError("project", projectMsgNotFound)
	}

	return admingen.GetAdminProjectResourceAccessMetrics200JSONResponse(admingen.AdminProjectAccessMetrics{
		From:               metrics.From,
		To:                 metrics.To,
		Granularity:        admingen.AdminProjectAccessMetricsGranularity(metrics.Granularity),
		AccessBySource:     toGenAdminSourcePoints(metrics.AccessBySource),
		EarliestRetainedAt: a.accessEventsEarliestRetainedAt(),
	}), nil
}

// GetAdminProjectTopAccessedResources returns the project's most-accessed
// resources as opaque rows.
func (a *adminStrictServer) GetAdminProjectTopAccessedResources(
	ctx context.Context, request admingen.GetAdminProjectTopAccessedResourcesRequestObject,
) (admingen.GetAdminProjectTopAccessedResourcesResponseObject, error) {
	limit, err := adminTopResourcesLimitParam(request.Params.Limit)
	if err != nil {
		return nil, err
	}

	top, err := a.s.container.AdminService().GetProjectTopAccessedResources(ctx, request.Id.String(),
		services.AdminTopResourcesQuery{From: request.Params.From, To: request.Params.To, Limit: limit})
	if err != nil {
		return nil, a.adminRangeOrInternalError("GetAdminProjectTopAccessedResources", err)
	}
	if top == nil {
		return nil, apierrors.NewResourceNotFoundError("project", projectMsgNotFound)
	}

	items, err := toGenAdminTopAccessedResources(top.Items)
	if err != nil {
		return nil, a.adminInternalError("GetAdminProjectTopAccessedResources", err)
	}
	return admingen.GetAdminProjectTopAccessedResources200JSONResponse(admingen.AdminTopAccessedResourcesResponse{
		From:               top.From,
		To:                 top.To,
		Items:              items,
		EarliestRetainedAt: a.accessEventsEarliestRetainedAt(),
	}), nil
}

// GetAdminProjectConfig returns the freshness rules that apply to the project,
// its own separately from the team-wide ones.
func (a *adminStrictServer) GetAdminProjectConfig(
	ctx context.Context, request admingen.GetAdminProjectConfigRequestObject,
) (admingen.GetAdminProjectConfigResponseObject, error) {
	const handler = "GetAdminProjectConfig"
	cfg, err := a.s.container.AdminService().GetProjectConfig(ctx, request.Id.String())
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	if cfg == nil {
		return nil, apierrors.NewResourceNotFoundError("project", projectMsgNotFound)
	}

	projectRules, err := toGenAdminFreshnessRules(cfg.ProjectRules)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	teamWideRules, err := toGenAdminFreshnessRules(cfg.TeamWideRules)
	if err != nil {
		return nil, a.adminInternalError(handler, err)
	}
	return admingen.GetAdminProjectConfig200JSONResponse(admingen.AdminProjectConfig{
		ProjectRules:  projectRules,
		TeamWideRules: teamWideRules,
	}), nil
}

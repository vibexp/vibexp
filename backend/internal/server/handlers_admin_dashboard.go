package server

import (
	"context"
	"errors"
	"time"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
)

// Admin dashboard metrics handlers (#451). Both operations are mounted through
// setupAdminRoutes, so instanceAdminMiddleware has already 404'd non-admins by
// the time these run.

// GetAdminDashboardOverview returns instance totals, breakdowns and system health.
func (a *adminStrictServer) GetAdminDashboardOverview(
	ctx context.Context, _ admingen.GetAdminDashboardOverviewRequestObject,
) (admingen.GetAdminDashboardOverviewResponseObject, error) {
	overview, err := a.s.container.AdminService().GetDashboardOverview(ctx, a.appVersion())
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminDashboardOverview", "error", err,
		).Error("Failed to get admin dashboard overview")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	return admingen.GetAdminDashboardOverview200JSONResponse(toGenAdminOverview(overview)), nil
}

// GetAdminDashboardTimeseries returns gap-filled growth, sign-in and
// access-by-source series for the requested range.
func (a *adminStrictServer) GetAdminDashboardTimeseries(
	ctx context.Context, request admingen.GetAdminDashboardTimeseriesRequestObject,
) (admingen.GetAdminDashboardTimeseriesResponseObject, error) {
	granularity, err := adminGranularityParam(request.Params.Granularity)
	if err != nil {
		return nil, err
	}

	retention := a.s.config.Retention
	window := services.AdminDataWindowFor(time.Now(), retention.ActivityDays, retention.AccessEventDays)

	series, err := a.s.container.AdminService().GetDashboardTimeseries(ctx, services.AdminTimeseriesQuery{
		From:        request.Params.From,
		To:          request.Params.To,
		Granularity: granularity,
	}, window)
	if err != nil {
		// An invalid range/granularity is the caller's fault, not a 500.
		var rangeErr *services.ErrAdminTimeseriesRange
		if errors.As(err, &rangeErr) {
			return nil, apierrors.NewBadRequestError(rangeErr.Detail)
		}
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminDashboardTimeseries", "error", err,
		).Error("Failed to get admin dashboard timeseries")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	return admingen.GetAdminDashboardTimeseries200JSONResponse(toGenAdminTimeseries(series)), nil
}

// adminGranularityParam validates the granularity enum the generated binder
// accepted verbatim — see validateAdminSortEnum in handlers_admin.go for why
// this cannot be left to oapi-codegen. An absent parameter is not an error; the
// service applies the default.
func adminGranularityParam(g *admingen.GetAdminDashboardTimeseriesParamsGranularity) (string, error) {
	if g == nil {
		return "", nil
	}
	if err := validateAdminSortEnum("granularity", string(*g), g.Valid()); err != nil {
		return "", err
	}
	return string(*g), nil
}

// appVersion returns the configured service version, falling back to "dev" —
// the same rule GetAdminStats applies.
func (a *adminStrictServer) appVersion() string {
	if v := a.s.config.Server.ServiceVersion; v != "" {
		return v
	}
	return "dev"
}

// toGenAdminOverview converts the domain overview to the generated response.
// Every required array is built with make(..., 0) so it serializes as [], never
// null (#125); the generated types cannot use models.JSONArray[T].
func toGenAdminOverview(o models.AdminDashboardOverview) admingen.AdminDashboardOverview {
	breakdowns := make([]admingen.AdminEntityBreakdown, 0, len(o.Breakdowns))
	for _, b := range o.Breakdowns {
		buckets := make([]admingen.AdminBreakdownBucket, 0, len(b.Buckets))
		for _, bucket := range b.Buckets {
			buckets = append(buckets, admingen.AdminBreakdownBucket{
				Value: bucket.Value,
				Count: bucket.Count,
			})
		}
		breakdowns = append(breakdowns, admingen.AdminEntityBreakdown{
			Entity:  b.Entity,
			Field:   b.Field,
			Buckets: buckets,
		})
	}

	tables := make([]admingen.AdminTableStat, 0, len(o.SystemHealth.Tables))
	for _, t := range o.SystemHealth.Tables {
		tables = append(tables, admingen.AdminTableStat{
			Table:         t.Table,
			EstimatedRows: t.EstimatedRows,
		})
	}

	return admingen.AdminDashboardOverview{
		Counts: admingen.AdminExtendedCounts{
			Users:      o.Counts.Users,
			Teams:      o.Counts.Teams,
			Projects:   o.Counts.Projects,
			Prompts:    o.Counts.Prompts,
			Artifacts:  o.Counts.Artifacts,
			Memories:   o.Counts.Memories,
			Blueprints: o.Counts.Blueprints,
			Agents:     o.Counts.Agents,
			Feeds:      o.Counts.Feeds,
			ApiKeys:    o.Counts.APIKeys,
		},
		Breakdowns: breakdowns,
		SystemHealth: admingen.AdminSystemHealth{
			DatabaseSizeBytes: o.SystemHealth.DatabaseSizeBytes,
			Tables:            tables,
		},
		Version: o.Version,
	}
}

// toGenAdminTimeseries converts the domain series to the generated response,
// with every required array non-nil by construction.
func toGenAdminTimeseries(s models.AdminTimeseries) admingen.AdminTimeseriesResponse {
	growth := make([]admingen.AdminGrowthPoint, 0, len(s.Growth))
	for _, p := range s.Growth {
		growth = append(growth, admingen.AdminGrowthPoint{
			Bucket:    p.Bucket,
			Users:     p.Users,
			Teams:     p.Teams,
			Projects:  p.Projects,
			Prompts:   p.Prompts,
			Artifacts: p.Artifacts,
			Memories:  p.Memories,
		})
	}

	signIns := make([]admingen.AdminCountPoint, 0, len(s.SignIns))
	for _, p := range s.SignIns {
		signIns = append(signIns, admingen.AdminCountPoint{Bucket: p.Bucket, Count: p.Count})
	}

	access := make([]admingen.AdminSourcePoint, 0, len(s.AccessBySource))
	for _, p := range s.AccessBySource {
		access = append(access, admingen.AdminSourcePoint{
			Bucket: p.Bucket, Source: p.Source, Count: p.Count,
		})
	}

	return admingen.AdminTimeseriesResponse{
		From:           s.From,
		To:             s.To,
		Granularity:    admingen.AdminTimeseriesResponseGranularity(s.Granularity),
		Growth:         growth,
		SignIns:        signIns,
		AccessBySource: access,
		DataWindow: admingen.AdminDataWindow{
			SignInsEarliestRetainedAt:        s.DataWindow.SignInsEarliestRetainedAt,
			AccessBySourceEarliestRetainedAt: s.DataWindow.AccessBySourceEarliestRetainedAt,
		},
	}
}

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	creationmetricsgen "github.com/vibexp/vibexp/internal/server/gen/creationmetrics"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// The two resource-creation-metrics reads (team and project), spec-first
// strict-server bound from their own package (#1392). They share one payload
// shape: a zero-filled daily series plus the per-channel breakdown of the same
// window.
//
// Each response builds its required `counts` array with make(..., 0, n), so it
// serializes as `[]` and never `null`: generated strict-server types cannot use
// the models.JSONArray shim. In practice the array is never empty, because the
// smallest window still zero-fills eight days.

// Response messages. They are part of the body, so they are constants rather
// than inline literals that could drift from the spec's examples.
const (
	creationMetricsStatusSuccess    = "success"
	teamCreationMetricsMsgOK        = "Team resource creation metrics retrieved successfully"
	projectCreationMetricsMsgOK     = "Resource creation metrics retrieved successfully"
	teamCreationMetricsMsgFailed    = "Failed to get team resource creation metrics"
	projectCreationMetricsMsgFailed = "Failed to get project resource creation metrics"
	creationMetricsMsgAccessDenied  = "Access denied"
	creationMetricsCodeAccessDenied = "access_denied"
)

// creationMetricsStrictServer adapts the Server to the generated
// creationmetricsgen.StrictServerInterface.
type creationMetricsStrictServer struct {
	s *Server
}

var _ creationmetricsgen.StrictServerInterface = (*creationMetricsStrictServer)(nil)

// creationDay is one zero-filled day of a creation series, before it is shaped
// into the team or the project payload.
type creationDay struct {
	date       time.Time
	prompts    int
	artifacts  int
	blueprints int
	memories   int
	projects   int
}

// creationRow is one sparse (day, resource type) count, the common shape of
// models.TeamResourceCreationCount and models.ProjectResourceCreationCount.
type creationRow struct {
	date         string
	resourceType string
	count        int
}

// GetTeamResourceCreationMetrics returns a team's zero-filled daily creation
// series (prompts, artifacts, blueprints, memories, projects) with its
// per-channel breakdown.
func (cs *creationMetricsStrictServer) GetTeamResourceCreationMetrics(
	ctx context.Context, request creationmetricsgen.GetTeamResourceCreationMetricsRequestObject,
) (creationmetricsgen.GetTeamResourceCreationMetricsResponseObject, error) {
	teamID := request.Id.String()
	rangeStr := ""
	if request.Params.Range != nil {
		rangeStr = string(*request.Params.Range)
	}
	window, err := cs.openWindow(ctx, "GetTeamResourceCreationMetrics", teamID, rangeStr)
	if err != nil {
		return nil, err
	}

	counts, err := cs.s.container.TeamService().GetTeamResourceCreationMetrics(ctx, teamID, window.since)
	if err != nil {
		return nil, cs.internalError("GetTeamResourceCreationMetrics", teamCreationMetricsMsgFailed, err)
	}
	sources, err := cs.s.container.ResourceWriteService().GetTeamCreationSources(ctx, teamID, window.since)
	if err != nil {
		return nil, cs.internalError("GetTeamResourceCreationMetrics", teamCreationMetricsMsgFailed, err)
	}

	rows := make([]creationRow, 0, len(counts))
	for _, c := range counts {
		rows = append(rows, creationRow{date: c.Date, resourceType: c.ResourceType, count: c.Count})
	}
	series, totalCreated := teamCreationSeries(zeroFillCreationDays(rows, window))

	return creationmetricsgen.GetTeamResourceCreationMetrics200JSONResponse{
		Status:  creationMetricsStatusSuccess,
		Message: teamCreationMetricsMsgOK,
		Data: creationmetricsgen.TeamResourceCreationMetricsData{
			TotalCreated:   totalCreated,
			Range:          creationmetricsgen.TeamResourceCreationMetricsDataRange(window.rangeStr),
			Counts:         series,
			BySource:       toGenCreationBySource(sources),
			WritesBySource: toGenWritesBySource(sources),
		},
	}, nil
}

// GetProjectResourceCreationMetrics returns one project's zero-filled daily
// creation series (prompts, artifacts, blueprints, memories) with its
// per-channel breakdown.
func (cs *creationMetricsStrictServer) GetProjectResourceCreationMetrics(
	ctx context.Context, request creationmetricsgen.GetProjectResourceCreationMetricsRequestObject,
) (creationmetricsgen.GetProjectResourceCreationMetricsResponseObject, error) {
	teamID := request.TeamId.String()
	rangeStr := ""
	if request.Params.Range != nil {
		rangeStr = string(*request.Params.Range)
	}
	window, err := cs.openWindow(ctx, "GetProjectResourceCreationMetrics", teamID, rangeStr)
	if err != nil {
		return nil, err
	}

	// The project is resolved and authorized by this call (owner or member of
	// its team), so an unknown or inaccessible slug is a 404 before the
	// per-channel queries run. They trust the slug for that reason.
	userID, _ := ctx.Value(contextKeyUserID).(string)
	counts, err := cs.s.container.ProjectService().
		GetProjectResourceCreationMetrics(teamID, userID, request.Slug, window.since)
	if err != nil {
		// Matched on the message, as every project handler does: the service
		// wraps the repository's not-found sentinel in more than one way.
		if strings.Contains(err.Error(), "not found") {
			return nil, apierrors.NewResourceNotFoundError("resource", "Project not found")
		}
		return nil, cs.internalError("GetProjectResourceCreationMetrics", projectCreationMetricsMsgFailed, err)
	}
	sources, err := cs.s.container.ResourceWriteService().
		GetProjectCreationSources(ctx, teamID, request.Slug, window.since)
	if err != nil {
		return nil, cs.internalError("GetProjectResourceCreationMetrics", projectCreationMetricsMsgFailed, err)
	}

	rows := make([]creationRow, 0, len(counts))
	for _, c := range counts {
		rows = append(rows, creationRow{date: c.Date, resourceType: c.ResourceType, count: c.Count})
	}
	series, totalCreated := projectCreationSeries(zeroFillCreationDays(rows, window))

	return creationmetricsgen.GetProjectResourceCreationMetrics200JSONResponse{
		Status:  creationMetricsStatusSuccess,
		Message: projectCreationMetricsMsgOK,
		Data: creationmetricsgen.ProjectResourceCreationMetricsData{
			TotalCreated:   totalCreated,
			Range:          creationmetricsgen.ProjectResourceCreationMetricsDataRange(window.rangeStr),
			Counts:         series,
			BySource:       toGenCreationBySource(sources),
			WritesBySource: toGenWritesBySource(sources),
		},
	}, nil
}

// openWindow is the shared opening of both reads: the caller must be a member
// of the team, and the range must be one the API offers. Membership is checked
// first, so a non-member learns nothing about which ranges are valid.
func (cs *creationMetricsStrictServer) openWindow(
	ctx context.Context, op, teamID, rangeStr string,
) (creationMetricsWindow, error) {
	if err := cs.authorizeTeam(ctx, op, teamID); err != nil {
		return creationMetricsWindow{}, err
	}
	return resolveCreationMetricsWindow(rangeStr)
}

// teamCreationSeries shapes the zero-filled days into the team payload and
// returns the grand total alongside. Projects count toward a team's totals.
func teamCreationSeries(days []creationDay) ([]creationmetricsgen.TeamResourceCreationDailyCount, int) {
	series := make([]creationmetricsgen.TeamResourceCreationDailyCount, 0, len(days))
	totalCreated := 0
	for _, day := range days {
		total := day.prompts + day.artifacts + day.blueprints + day.memories + day.projects
		totalCreated += total
		series = append(series, creationmetricsgen.TeamResourceCreationDailyCount{
			Date:       openapi_types.Date{Time: day.date},
			Prompts:    day.prompts,
			Artifacts:  day.artifacts,
			Blueprints: day.blueprints,
			Memories:   day.memories,
			Projects:   day.projects,
			Total:      total,
		})
	}
	return series, totalCreated
}

// projectCreationSeries shapes the zero-filled days into the project payload
// and returns the grand total alongside. A project series has no projects.
func projectCreationSeries(days []creationDay) ([]creationmetricsgen.ProjectResourceCreationDailyCount, int) {
	series := make([]creationmetricsgen.ProjectResourceCreationDailyCount, 0, len(days))
	totalCreated := 0
	for _, day := range days {
		total := day.prompts + day.artifacts + day.blueprints + day.memories
		totalCreated += total
		series = append(series, creationmetricsgen.ProjectResourceCreationDailyCount{
			Date:       openapi_types.Date{Time: day.date},
			Prompts:    day.prompts,
			Artifacts:  day.artifacts,
			Blueprints: day.blueprints,
			Memories:   day.memories,
			Total:      total,
		})
	}
	return series, totalCreated
}

// authorizeTeam requires the caller to be a member of the team. Both routes
// authorize here rather than through teamValidationMiddleware: the team route
// names its path parameter {id} where the middleware reads {team_id}, and one
// generated handler serves both. It is the same membership check the middleware
// wraps, and it answers a non-member and an unknown team alike.
func (cs *creationMetricsStrictServer) authorizeTeam(ctx context.Context, op, teamID string) error {
	userID, _ := ctx.Value(contextKeyUserID).(string)
	if err := cs.s.validateTeamAccess(ctx, userID, teamID); err != nil {
		cs.s.logger.With(
			"service", serverLogServiceName,
			"handler", op,
			"user_id", userID,
			"team_id", teamID,
		).Warn("Team access denied for creation metrics request")
		return apierrors.NewAPIError(
			creationMetricsCodeAccessDenied, creationMetricsCodeAccessDenied,
			creationMetricsMsgAccessDenied, http.StatusForbidden,
		)
	}
	return nil
}

// internalError logs the cause and returns an opaque 500.
func (cs *creationMetricsStrictServer) internalError(op, message string, err error) error {
	cs.s.logger.With(
		"service", serverLogServiceName,
		"handler", op,
		"error", fmt.Sprintf("%+v", err),
	).Error(message)
	return apierrors.NewInternalError(message)
}

// creationMetricsWindow is a resolved reporting window.
type creationMetricsWindow struct {
	rangeStr  string
	rangeDays int
	// since is UTC midnight of the first day of the window.
	since time.Time
}

// resolveCreationMetricsWindow turns the optional ?range= value into a window.
//
// This check is the ONLY enforcement of the range enum: oapi-codegen binds a
// query parameter as a plain string and never validates its enum, so without it
// an unknown range would silently become a zero-day window.
//
// since is truncated to UTC midnight so the SQL window starts where the
// zero-filled series starts. AddDate carries the current time of day; without
// the truncation the oldest day would undercount rows created earlier in it.
func resolveCreationMetricsWindow(rangeStr string) (creationMetricsWindow, error) {
	if rangeStr == "" {
		rangeStr = defaultResourceAccessRange
	}
	rangeDays, ok := resourceAccessRanges[rangeStr]
	if !ok {
		invalid := apierrors.NewBadRequestError(teamAnalyticsMsgInvalidRange)
		invalid.Code = apierrors.CodeValidationFailed
		return creationMetricsWindow{}, invalid
	}

	start := time.Now().UTC().AddDate(0, 0, -rangeDays)
	return creationMetricsWindow{
		rangeStr:  rangeStr,
		rangeDays: rangeDays,
		since:     time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC),
	}, nil
}

// zeroFillCreationDays turns the sparse per-day, per-type counts into a
// contiguous daily series: every day from the window's start through today,
// each with every resource type (zero when absent).
func zeroFillCreationDays(rows []creationRow, window creationMetricsWindow) []creationDay {
	// date -> resource type -> count
	byDay := make(map[string]map[string]int, len(rows))
	for _, row := range rows {
		if byDay[row.date] == nil {
			byDay[row.date] = make(map[string]int, 5)
		}
		byDay[row.date][row.resourceType] += row.count
	}

	days := make([]creationDay, 0, window.rangeDays+1)
	for offset := 0; offset <= window.rangeDays; offset++ {
		date := window.since.AddDate(0, 0, offset)
		day := byDay[date.Format("2006-01-02")]
		days = append(days, creationDay{
			date:       date,
			prompts:    day["prompts"],
			artifacts:  day["artifacts"],
			blueprints: day["blueprints"],
			memories:   day["memories"],
			projects:   day["projects"],
		})
	}
	return days
}

// toGenCreationBySource shapes the per-channel creation counts. Every key is
// read from the map explicitly, so a channel the service did not report is a
// zero rather than a missing field.
func toGenCreationBySource(sources *resourceaccess.CreationSources) creationmetricsgen.ResourceCreationBySource {
	counts := func(source string) creationmetricsgen.ResourceCreationSourceCounts {
		c := sources.Created[source]
		return creationmetricsgen.ResourceCreationSourceCounts{
			Prompts:    c.Prompts,
			Artifacts:  c.Artifacts,
			Blueprints: c.Blueprints,
			Memories:   c.Memories,
			Total:      c.Total,
		}
	}
	return creationmetricsgen.ResourceCreationBySource{
		Web:     counts(resourceaccess.SourceWeb),
		Cli:     counts(resourceaccess.SourceCLI),
		Mcp:     counts(resourceaccess.SourceMCP),
		Api:     counts(resourceaccess.SourceAPI),
		Unknown: counts(resourceaccess.SourceUnknown),
	}
}

func toGenWritesBySource(sources *resourceaccess.CreationSources) creationmetricsgen.ResourceWritesBySource {
	return creationmetricsgen.ResourceWritesBySource{
		Web: sources.Writes[resourceaccess.SourceWeb],
		Cli: sources.Writes[resourceaccess.SourceCLI],
		Mcp: sources.Writes[resourceaccess.SourceMCP],
		Api: sources.Writes[resourceaccess.SourceAPI],
		// Unknown is the writes of the window with no recorded channel (#1418).
		Unknown: sources.Writes[resourceaccess.SourceUnknown],
	}
}

// creationMetricsUUIDParams are the two routes' uuid-formatted path parameters,
// mapped to the name each legacy handler used in its 400.
var creationMetricsUUIDParams = map[string]string{
	"id":      "team id must be a valid UUID",
	"team_id": "team_id must be a valid UUID",
}

// creationMetricsBindErrorHandler turns a parameter-binding failure into a 400.
func (s *Server) creationMetricsBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()
	var invalidParam *creationmetricsgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		msg = fmt.Sprintf("%s is not in the expected format", invalidParam.ParamName)
		if uuidMsg, ok := creationMetricsUUIDParams[invalidParam.ParamName]; ok {
			msg = uuidMsg
		}
	}
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// creationMetricsResponseErrorHandler writes the API error a handler returned.
func (s *Server) creationMetricsResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Unhandled creation metrics handler error")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(""))
}

// setupCreationMetricsRoutes mounts the generated handler for the two
// resource-creation-metrics reads.
//
// Both routes are registered at FULL LENGTH on the root router. They sit
// beneath the r.Route("/api/v1/teams", ...) and
// r.Route("/api/v1/{team_id}/projects", ...) subtrees, and chi resolves the
// longer absolute pattern first, exactly as it does for setupTeamRolesRoutes.
// No teamValidationMiddleware is mounted: see authorizeTeam.
func (s *Server) setupCreationMetricsRoutes(r chi.Router) {
	strict := creationmetricsgen.NewStrictHandlerWithOptions(
		&creationMetricsStrictServer{s: s},
		nil,
		creationmetricsgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  s.creationMetricsBindErrorHandler,
			ResponseErrorHandlerFunc: s.creationMetricsResponseErrorHandler,
		},
	)
	creationmetricsgen.HandlerWithOptions(strict, creationmetricsgen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: s.creationMetricsBindErrorHandler,
	})
}

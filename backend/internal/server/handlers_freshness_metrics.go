package server

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	freshnessgen "github.com/vibexp/vibexp/internal/server/gen/freshness"
	"github.com/vibexp/vibexp/internal/services"
)

// The analytics + audit reads (issue #734). They live beside the rule and
// settings handlers and share their error mapping, but in a separate file
// because they are a distinct surface: five reads, no writes, no authz call.
//
// Every response is built with `make(..., 0, len(...))` so its required array
// serializes as `[]` and never `null`. Generated strict-server types cannot use
// the models.JSONArray shim, so that guarantee has to be constructed here —
// which is why each of these schemas appears in adHocRequiredArrayAllowlist.

// Success messages. They are part of the response body, so they are constants
// rather than inline literals that could drift from the spec's examples.
const (
	freshnessMsgOverTimeOK   = "Freshness over-time metrics retrieved successfully"
	freshnessMsgByTypeOK     = "Freshness by-type metrics retrieved successfully"
	freshnessMsgByProjectOK  = "Freshness by-project metrics retrieved successfully"
	freshnessMsgByRuleOK     = "Freshness by-rule metrics retrieved successfully"
	freshnessMsgInvalidPage  = "page must be a positive integer"
	freshnessMsgInvalidLimit = "limit must be between 1 and 100"
)

// GetFreshnessOverTimeMetrics returns the daily freshness activity series.
func (fs *freshnessStrictServer) GetFreshnessOverTimeMetrics(
	ctx context.Context, request freshnessgen.GetFreshnessOverTimeMetricsRequestObject,
) (freshnessgen.GetFreshnessOverTimeMetricsResponseObject, error) {
	rangeStr := ""
	if request.Params.Range != nil {
		rangeStr = string(*request.Params.Range)
	}
	// This check is the ONLY enforcement of the range enum. oapi-codegen binds
	// a query param as a plain string and never validates its enum (the same
	// gap as minimum/maximum below), so deleting this silently turns every
	// unknown range into the default 30-day window.
	rangeDays, ok := services.FreshnessMetricsRangeDays(rangeStr)
	if !ok {
		return nil, apierrors.NewBadRequestError(fmt.Sprintf("range %q is not a supported window", rangeStr))
	}

	metrics, err := fs.s.container.FreshnessService().
		GetOverTimeMetrics(ctx, request.TeamId.String(), rangeDays)
	if err != nil {
		return nil, fs.freshnessError("GetFreshnessOverTimeMetrics", err)
	}

	counts := make([]freshnessgen.FreshnessDailyStaleCount, 0, len(metrics.Days))
	for _, day := range metrics.Days {
		parsed, perr := time.Parse(freshnessSeriesDateLayout, day.Date)
		if perr != nil {
			return nil, fs.freshnessError("GetFreshnessOverTimeMetrics",
				fmt.Errorf("series date %q is not a date: %w", day.Date, perr))
		}
		counts = append(counts, freshnessgen.FreshnessDailyStaleCount{
			Date:       openapi_types.Date{Time: parsed},
			Marked:     int32Bounded(day.Marked),
			Cleared:    int32Bounded(day.Cleared),
			StaleTotal: int32Bounded(day.StaleTotal),
			// The chart's per-day total is the ACTIVITY, not the level: it
			// sums the rendered series (marked + cleared), matching every
			// other daily-count payload the shared component consumes.
			Total: int32Bounded(day.Marked + day.Cleared),
		})
	}

	return freshnessgen.GetFreshnessOverTimeMetrics200JSONResponse{
		Status:  freshnessStatusSuccess,
		Message: freshnessMsgOverTimeOK,
		Data: freshnessgen.FreshnessOverTimeMetricsData{
			Range:        freshnessgen.FreshnessMetricsRange(rangeOrDefault(rangeStr)),
			TotalMarked:  int32Bounded(metrics.TotalMarked),
			TotalCleared: int32Bounded(metrics.TotalCleared),
			Counts:       counts,
		},
	}, nil
}

// GetFreshnessByTypeMetrics returns current stale counts per resource type.
func (fs *freshnessStrictServer) GetFreshnessByTypeMetrics(
	ctx context.Context, request freshnessgen.GetFreshnessByTypeMetricsRequestObject,
) (freshnessgen.GetFreshnessByTypeMetricsResponseObject, error) {
	metrics, err := fs.s.container.FreshnessService().GetByTypeMetrics(ctx, request.TeamId.String())
	if err != nil {
		return nil, fs.freshnessError("GetFreshnessByTypeMetrics", err)
	}

	counts := make([]freshnessgen.FreshnessTypeCount, 0, len(metrics.Counts))
	for _, bucket := range metrics.Counts {
		counts = append(counts, freshnessgen.FreshnessTypeCount{
			ResourceType: freshnessgen.FreshnessRuleResourceType(bucket.Key),
			Count:        int32Bounded(bucket.Count),
		})
	}

	return freshnessgen.GetFreshnessByTypeMetrics200JSONResponse{
		Status:  freshnessStatusSuccess,
		Message: freshnessMsgByTypeOK,
		Data: freshnessgen.FreshnessByTypeMetricsData{
			TotalStale: int32Bounded(metrics.TotalStale),
			Counts:     counts,
		},
	}, nil
}

// GetFreshnessByProjectMetrics returns current stale counts per project.
func (fs *freshnessStrictServer) GetFreshnessByProjectMetrics(
	ctx context.Context, request freshnessgen.GetFreshnessByProjectMetricsRequestObject,
) (freshnessgen.GetFreshnessByProjectMetricsResponseObject, error) {
	metrics, err := fs.s.container.FreshnessService().GetByProjectMetrics(ctx, request.TeamId.String())
	if err != nil {
		return nil, fs.freshnessError("GetFreshnessByProjectMetrics", err)
	}

	counts := make([]freshnessgen.FreshnessProjectCount, 0, len(metrics.Counts))
	for _, project := range metrics.Counts {
		projectID, perr := uuid.Parse(project.ProjectID)
		if perr != nil {
			return nil, fs.freshnessError("GetFreshnessByProjectMetrics",
				fmt.Errorf("project id %q is not a UUID: %w", project.ProjectID, perr))
		}
		counts = append(counts, freshnessgen.FreshnessProjectCount{
			ProjectId: projectID,
			Name:      project.Name,
			Slug:      project.Slug,
			Count:     int32Bounded(project.Count),
		})
	}

	return freshnessgen.GetFreshnessByProjectMetrics200JSONResponse{
		Status:  freshnessStatusSuccess,
		Message: freshnessMsgByProjectOK,
		Data: freshnessgen.FreshnessByProjectMetricsData{
			TotalStale: int32Bounded(metrics.TotalStale),
			Counts:     counts,
		},
	}, nil
}

// GetFreshnessByRuleMetrics returns how many resources each rule marks.
func (fs *freshnessStrictServer) GetFreshnessByRuleMetrics(
	ctx context.Context, request freshnessgen.GetFreshnessByRuleMetricsRequestObject,
) (freshnessgen.GetFreshnessByRuleMetricsResponseObject, error) {
	metrics, err := fs.s.container.FreshnessService().GetByRuleMetrics(ctx, request.TeamId.String())
	if err != nil {
		return nil, fs.freshnessError("GetFreshnessByRuleMetrics", err)
	}

	counts := make([]freshnessgen.FreshnessRuleImpact, 0, len(metrics.Counts))
	for _, impact := range metrics.Counts {
		converted, cerr := toGenFreshnessRuleImpact(impact)
		if cerr != nil {
			return nil, fs.freshnessError("GetFreshnessByRuleMetrics", cerr)
		}
		counts = append(counts, converted)
	}

	return freshnessgen.GetFreshnessByRuleMetrics200JSONResponse{
		Status:  freshnessStatusSuccess,
		Message: freshnessMsgByRuleOK,
		Data: freshnessgen.FreshnessByRuleMetricsData{
			TotalStale: int32Bounded(metrics.TotalStale),
			Counts:     counts,
		},
	}, nil
}

// ListFreshnessAudit returns one page of the team's audit log.
func (fs *freshnessStrictServer) ListFreshnessAudit(
	ctx context.Context, request freshnessgen.ListFreshnessAuditRequestObject,
) (freshnessgen.ListFreshnessAuditResponseObject, error) {
	page, limit, err := freshnessAuditPaging(request.Params)
	if err != nil {
		return nil, err
	}

	result, err := fs.s.container.FreshnessService().ListAudit(ctx, request.TeamId.String(), page, limit)
	if err != nil {
		return nil, fs.freshnessError("ListFreshnessAudit", err)
	}

	entries := make([]freshnessgen.FreshnessAuditEntry, 0, len(result.Entries))
	for _, entry := range result.Entries {
		converted, cerr := toGenFreshnessAuditEntry(entry)
		if cerr != nil {
			return nil, fs.freshnessError("ListFreshnessAudit", cerr)
		}
		entries = append(entries, converted)
	}

	return freshnessgen.ListFreshnessAudit200JSONResponse{
		Entries:    entries,
		TotalCount: result.TotalCount,
		Page:       result.Page,
		PerPage:    result.PerPage,
		TotalPages: totalPagesFor(result.TotalCount, result.PerPage),
	}, nil
}

// freshnessSeriesDateLayout is the layout the service renders series buckets
// in; the handler parses them back into the spec's `date` type.
const freshnessSeriesDateLayout = "2006-01-02"

// freshnessStatusSuccess is the envelope's status value, matching the other
// analytics endpoints.
const freshnessStatusSuccess = "success"

// rangeOrDefault echoes back the window actually used, so a client that omitted
// the parameter still learns which range the numbers cover.
func rangeOrDefault(rangeStr string) string {
	if rangeStr == "" {
		return services.DefaultFreshnessMetricsRange
	}
	return rangeStr
}

// freshnessAuditPaging resolves the optional paging params, applying the
// spec's defaults and enforcing its bounds.
//
// BOTH halves are this function's job because oapi-codegen does neither:
// an omitted optional query param arrives as nil rather than as the schema
// default, and `minimum`/`maximum` on a query param are not enforced at all
// (the same gap as `enum` on query params). Without the bounds check a caller
// could ask for any page size they liked — the spec would say 100 and the
// server would hand over the whole log.
func freshnessAuditPaging(params freshnessgen.ListFreshnessAuditParams) (page, limit int, err error) {
	page, limit = 1, defaultFreshnessAuditLimit
	if params.Page != nil {
		page = *params.Page
	}
	if params.Limit != nil {
		limit = *params.Limit
	}

	if page < 1 || page > maxFreshnessAuditPage {
		return 0, 0, apierrors.NewBadRequestError(freshnessMsgInvalidPage)
	}
	if limit < 1 || limit > maxFreshnessAuditLimit {
		return 0, 0, apierrors.NewBadRequestError(freshnessMsgInvalidLimit)
	}
	return page, limit, nil
}

const (
	// defaultFreshnessAuditLimit and maxFreshnessAuditLimit mirror the `limit`
	// query parameter's default and maximum in the spec.
	defaultFreshnessAuditLimit = 20
	maxFreshnessAuditLimit     = 100

	// maxFreshnessAuditPage caps the page number. Without it `(page-1)*limit`
	// overflows int for a large enough page, and the repository clamps the
	// resulting NEGATIVE offset to zero — so an absurd page number silently
	// returns the FIRST page while echoing back the number that was asked for.
	// Rejecting is the only honest answer; no real log is a million pages long.
	maxFreshnessAuditPage = 1_000_000
)

// totalPagesFor reports how many pages the total spans. An empty log is one
// (empty) page rather than zero, so a client's "page 1 of N" never reads
// "page 1 of 0".
func totalPagesFor(totalCount, perPage int) int {
	if perPage < 1 || totalCount <= 0 {
		return 1
	}
	return (totalCount + perPage - 1) / perPage
}

// toGenFreshnessRuleImpact converts one rule's impact, building its
// resource-type array with make(...,0) so it never serializes as null.
func toGenFreshnessRuleImpact(impact models.FreshnessRuleImpact) (freshnessgen.FreshnessRuleImpact, error) {
	ruleID, err := uuid.Parse(impact.RuleID)
	if err != nil {
		return freshnessgen.FreshnessRuleImpact{}, fmt.Errorf("rule id %q is not a UUID: %w", impact.RuleID, err)
	}

	var projectID *openapi_types.UUID
	if impact.ProjectID != nil {
		parsed, perr := uuid.Parse(*impact.ProjectID)
		if perr != nil {
			return freshnessgen.FreshnessRuleImpact{}, fmt.Errorf(
				"rule project_id %q is not a UUID: %w", *impact.ProjectID, perr)
		}
		projectID = &parsed
	}

	resourceTypes := make([]freshnessgen.FreshnessRuleResourceType, 0, len(impact.ResourceTypes))
	for _, resourceType := range impact.ResourceTypes {
		resourceTypes = append(resourceTypes, freshnessgen.FreshnessRuleResourceType(resourceType))
	}

	return freshnessgen.FreshnessRuleImpact{
		RuleId:        ruleID,
		ProjectId:     projectID,
		ResourceTypes: resourceTypes,
		ThresholdDays: int32Bounded(impact.ThresholdDays),
		Enabled:       impact.Enabled,
		Count:         int32Bounded(impact.Count),
	}, nil
}

// toGenFreshnessAuditEntry converts one audit row.
func toGenFreshnessAuditEntry(entry *models.ResourceFreshnessAudit) (freshnessgen.FreshnessAuditEntry, error) {
	id, err := uuid.Parse(entry.ID)
	if err != nil {
		return freshnessgen.FreshnessAuditEntry{}, fmt.Errorf("audit id %q is not a UUID: %w", entry.ID, err)
	}
	resourceID, err := uuid.Parse(entry.ResourceID)
	if err != nil {
		return freshnessgen.FreshnessAuditEntry{}, fmt.Errorf(
			"audit resource_id %q is not a UUID: %w", entry.ResourceID, err)
	}

	var ruleID *openapi_types.UUID
	if entry.RuleID != nil {
		parsed, perr := uuid.Parse(*entry.RuleID)
		if perr != nil {
			return freshnessgen.FreshnessAuditEntry{}, fmt.Errorf(
				"audit rule_id %q is not a UUID: %w", *entry.RuleID, perr)
		}
		ruleID = &parsed
	}

	return freshnessgen.FreshnessAuditEntry{
		Id:           id,
		ResourceType: freshnessgen.FreshnessRuleResourceType(entry.ResourceType),
		ResourceId:   resourceID,
		RuleId:       ruleID,
		Action:       freshnessgen.FreshnessAuditEntryAction(entry.Action),
		Reason:       freshnessgen.FreshnessAuditEntryReason(entry.Reason),
		CreatedAt:    entry.CreatedAt,
	}, nil
}

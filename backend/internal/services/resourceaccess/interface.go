package resourceaccess

import (
	"context"

	"github.com/vibexp/vibexp/internal/models"
)

// ResourceAccessService records resource detail-access events, exposes access
// metrics, and runs the retention job that prunes old events.
type ResourceAccessService interface {
	// RecordAccess records a resource detail-access event asynchronously.
	// It is fire-and-forget: it never blocks the caller, never returns an error,
	// and never panics the caller even if persistence fails.
	//
	// The event is captured and persisted on another goroutine, so callers must
	// pass a freshly-allocated event and must not mutate or reuse it after the
	// call returns, to avoid a data race.
	RecordAccess(event *models.ResourceAccessEvent)

	// GetMetrics returns the per-source daily access counts for a resource over the
	// last rangeDays days, zero-filled so every day in the range has a row per source.
	GetMetrics(
		ctx context.Context,
		teamID, resourceType, resourceID string,
		rangeDays int,
	) (*MetricsResult, error)

	// GetTeamMetrics returns the per-source daily access counts across the whole
	// team (every resource) over the last rangeDays days, zero-filled so every day
	// in the range has a row per source. Team membership is authorized by the caller.
	GetTeamMetrics(
		ctx context.Context,
		teamID string,
		rangeDays int,
	) (*MetricsResult, error)

	// GetTopAccessedResources returns the team's most-accessed resources over the
	// last rangeDays days, ranked by access count descending and capped at `limit`,
	// with each resource's display name resolved. An empty or "all" source
	// aggregates across access channels; a concrete source (web/cli/mcp/api)
	// restricts the ranking to that channel. Team membership is authorized by
	// the caller.
	GetTopAccessedResources(
		ctx context.Context,
		teamID string,
		rangeDays int,
		source string,
		limit int,
	) ([]models.TopAccessedResource, error)

	// RunRetentionJob deletes access events older than the configured retention
	// window. Called via HTTP from Cloud Scheduler.
	RunRetentionJob(ctx context.Context) error
}

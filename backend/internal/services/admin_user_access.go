package services

import (
	"context"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// Per-user resource access analytics (#1136): the range handling and gap-fill
// behind /admin/users/{id}/resource-access-metrics and
// /admin/users/{id}/top-accessed-resources. The repository returns sparse rows.

const (
	// AdminTopResourcesDefaultLimit is the top-resources row count when none is given.
	AdminTopResourcesDefaultLimit = 10
	// AdminTopResourcesMaxLimit caps the top-resources row count.
	AdminTopResourcesMaxLimit = 50
)

// AdminTopResourcesQuery is the caller's requested top-resources window. Nil
// pointers mean "use the default"; Limit 0 means the default.
type AdminTopResourcesQuery struct {
	From  *time.Time
	To    *time.Time
	Limit int
}

// GetUserAccessMetrics returns the gap-filled per-source access series for a
// user. The range rules are the dashboard's (resolveAdminRange), so an invalid
// range yields *ErrAdminTimeseriesRange. (nil, nil) for an unknown user.
func (s *AdminService) GetUserAccessMetrics(
	ctx context.Context, id string, q AdminTimeseriesQuery,
) (*models.AdminUserAccessMetrics, error) {
	resolved, err := resolveAdminRange(q, time.Now())
	if err != nil {
		return nil, err
	}

	exists, err := s.adminRepo.UserExists(ctx, id)
	if err != nil || !exists {
		return nil, err
	}

	rows, err := s.adminRepo.GetUserAccessBySourceSeries(ctx, id, resolved.from, resolved.to, resolved.granularity)
	if err != nil {
		return nil, err
	}

	return &models.AdminUserAccessMetrics{
		From:           resolved.from,
		To:             resolved.to,
		Granularity:    resolved.granularity,
		AccessBySource: fillSources(adminBuckets(resolved), rows),
	}, nil
}

// GetUserTopAccessedResources returns a user's most-accessed resources in a
// window. The window is not bucket-snapped; it follows resolveAdminRange's
// defaults and span cap. An invalid window or a limit outside
// 1..AdminTopResourcesMaxLimit yields *ErrAdminTimeseriesRange. (nil, nil) for
// an unknown user.
func (s *AdminService) GetUserTopAccessedResources(
	ctx context.Context, id string, q AdminTopResourcesQuery,
) (*models.AdminTopAccessedResources, error) {
	from, to, err := resolveAdminWindow(q.From, q.To, time.Now())
	if err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit == 0 {
		limit = AdminTopResourcesDefaultLimit
	}
	if limit < 1 || limit > AdminTopResourcesMaxLimit {
		return nil, &ErrAdminTimeseriesRange{
			Detail: fmt.Sprintf("invalid limit %d: must be between 1 and %d", limit, AdminTopResourcesMaxLimit),
		}
	}

	exists, err := s.adminRepo.UserExists(ctx, id)
	if err != nil || !exists {
		return nil, err
	}

	items, err := s.adminRepo.GetUserTopAccessedResources(ctx, id, from, to, limit)
	if err != nil {
		return nil, err
	}
	return &models.AdminTopAccessedResources{From: from, To: to, Items: items}, nil
}

// resolveAdminWindow applies resolveAdminRange's defaults and validation to an
// un-bucketed window: to defaults to now, from to 30 days before to, to must be
// after from, and the span is capped at adminTimeseriesMaxDays.
func resolveAdminWindow(fromQ, toQ *time.Time, now time.Time) (from, to time.Time, err error) {
	to = now.UTC()
	if toQ != nil {
		to = toQ.UTC()
	}
	from = to.AddDate(0, 0, -adminTimeseriesDefaultDays)
	if fromQ != nil {
		from = fromQ.UTC()
	}

	if !to.After(from) {
		return time.Time{}, time.Time{}, &ErrAdminTimeseriesRange{
			Detail: fmt.Sprintf("invalid range: to (%s) must be after from (%s)",
				to.Format(time.RFC3339), from.Format(time.RFC3339)),
		}
	}
	if to.Sub(from) > adminTimeseriesMaxDays*24*time.Hour {
		return time.Time{}, time.Time{}, &ErrAdminTimeseriesRange{
			Detail: fmt.Sprintf("range too wide: at most %d days", adminTimeseriesMaxDays),
		}
	}
	return from, to, nil
}

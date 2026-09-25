package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// Project detail analytics and configuration (#1145): the range handling,
// gap-fill and rule partitioning behind /admin/projects/{id}/*. The ranges are
// resolved before the existence lookup, as for the per-user ops, so a bad range
// is a 400 even for an unknown project.

// GetProjectCreationMetrics returns the gap-filled per-type creation series for
// a project. The range rules are the dashboard's (resolveAdminRange), so an
// invalid range yields *ErrAdminTimeseriesRange. (nil, nil) for an unknown
// project.
func (s *AdminService) GetProjectCreationMetrics(
	ctx context.Context, id string, q AdminTimeseriesQuery,
) (*models.AdminProjectCreationMetrics, error) {
	resolved, err := resolveAdminRange(q, time.Now())
	if err != nil {
		return nil, err
	}

	_, found, err := s.adminRepo.ProjectTeamID(ctx, id)
	if err != nil || !found {
		return nil, err
	}

	rows, err := s.adminRepo.GetProjectCreationSeries(ctx, id, resolved.from, resolved.to, resolved.granularity)
	if err != nil {
		return nil, err
	}

	return &models.AdminProjectCreationMetrics{
		From:        resolved.from,
		To:          resolved.to,
		Granularity: resolved.granularity,
		Series:      fillProjectCreation(adminBuckets(resolved), rows),
	}, nil
}

// fillProjectCreation pivots sparse (type, bucket, count) rows into one point
// per bucket, every type present as an explicit 0 when it had no rows.
func fillProjectCreation(buckets []time.Time, rows []models.AdminGrowthCount) []models.AdminProjectCreationPoint {
	byBucket := make(map[time.Time]*models.AdminProjectCreationPoint, len(buckets))
	points := make([]models.AdminProjectCreationPoint, len(buckets))
	for i, b := range buckets {
		points[i] = models.AdminProjectCreationPoint{Bucket: b}
		byBucket[b] = &points[i]
	}

	for _, row := range rows {
		point, ok := byBucket[row.Bucket.UTC()]
		if !ok {
			// Only possible if the SQL and Go bucketing disagreed; see fillGrowth.
			continue
		}
		if field := projectCreationField(point, row.Entity); field != nil {
			*field = row.Count
		}
	}
	return points
}

// projectCreationField returns the per-type counter of p for resourceType, or
// nil for a type that is not project-scoped.
func projectCreationField(p *models.AdminProjectCreationPoint, resourceType string) *int64 {
	switch resourceType {
	case models.AdminResourceTypePrompt:
		return &p.Prompts
	case models.AdminResourceTypeMemory:
		return &p.Memories
	case models.AdminResourceTypeArtifact:
		return &p.Artifacts
	case models.AdminResourceTypeBlueprint:
		return &p.Blueprints
	case models.AdminResourceTypeFeedItem:
		return &p.FeedItems
	default:
		return nil
	}
}

// GetProjectAccessMetrics returns the gap-filled per-source access series for a
// project, attributed by each resource's current project. An invalid range
// yields *ErrAdminTimeseriesRange. (nil, nil) for an unknown project.
func (s *AdminService) GetProjectAccessMetrics(
	ctx context.Context, id string, q AdminTimeseriesQuery,
) (*models.AdminProjectAccessMetrics, error) {
	resolved, err := resolveAdminRange(q, time.Now())
	if err != nil {
		return nil, err
	}

	teamID, found, err := s.adminRepo.ProjectTeamID(ctx, id)
	if err != nil || !found {
		return nil, err
	}

	rows, err := s.adminRepo.GetProjectAccessBySourceSeries(
		ctx, id, teamID, resolved.from, resolved.to, resolved.granularity)
	if err != nil {
		return nil, err
	}

	return &models.AdminProjectAccessMetrics{
		From:           resolved.from,
		To:             resolved.to,
		Granularity:    resolved.granularity,
		AccessBySource: fillSources(adminBuckets(resolved), rows),
	}, nil
}

// GetProjectTopAccessedResources returns a project's most-accessed resources in
// a window, with GetUserTopAccessedResources' window and limit rules. (nil, nil)
// for an unknown project.
func (s *AdminService) GetProjectTopAccessedResources(
	ctx context.Context, id string, q AdminTopResourcesQuery,
) (*models.AdminTopAccessedResources, error) {
	from, to, limit, err := resolveAdminTopResourcesQuery(q, time.Now())
	if err != nil {
		return nil, err
	}

	teamID, found, err := s.adminRepo.ProjectTeamID(ctx, id)
	if err != nil || !found {
		return nil, err
	}

	items, err := s.adminRepo.GetProjectTopAccessedResources(ctx, id, teamID, from, to, limit)
	if err != nil {
		return nil, err
	}
	return &models.AdminTopAccessedResources{From: from, To: to, Items: items}, nil
}

// resolveAdminTopResourcesQuery applies resolveAdminWindow and the limit
// default and bounds shared by the top-resources ops.
func resolveAdminTopResourcesQuery(
	q AdminTopResourcesQuery, now time.Time,
) (from, to time.Time, limit int, err error) {
	from, to, err = resolveAdminWindow(q.From, q.To, now)
	if err != nil {
		return time.Time{}, time.Time{}, 0, err
	}
	limit = q.Limit
	if limit == 0 {
		limit = AdminTopResourcesDefaultLimit
	}
	if limit < 1 || limit > AdminTopResourcesMaxLimit {
		return time.Time{}, time.Time{}, 0, &ErrAdminTimeseriesRange{
			Detail: fmt.Sprintf("invalid limit %d: must be between 1 and %d", limit, AdminTopResourcesMaxLimit),
		}
	}
	return from, to, limit, nil
}

// errAdminFreshnessUnwired reports a wiring without the freshness service.
var errAdminFreshnessUnwired = errors.New("admin service: freshness service not configured")

// GetProjectConfig returns the freshness rules that apply to a project: its own
// rules and its team's team-wide (project-less) rules, each oldest first as
// ListRules returns them. Rules scoped to another project are dropped. (nil,
// nil) for an unknown project.
func (s *AdminService) GetProjectConfig(ctx context.Context, id string) (*models.AdminProjectConfig, error) {
	teamID, found, err := s.adminRepo.ProjectTeamID(ctx, id)
	if err != nil || !found {
		return nil, err
	}
	if s.freshness == nil {
		return nil, errAdminFreshnessUnwired
	}

	rules, err := s.freshness.ListRules(ctx, teamID)
	if err != nil {
		return nil, err
	}
	return partitionProjectFreshnessRules(id, rules), nil
}

// partitionProjectFreshnessRules splits a team's rules into the project's own
// and the team-wide ones, preserving order. Both slices are non-nil.
func partitionProjectFreshnessRules(projectID string, rules []*models.FreshnessRule) *models.AdminProjectConfig {
	cfg := &models.AdminProjectConfig{
		ProjectRules:  make([]*models.FreshnessRule, 0),
		TeamWideRules: make([]*models.FreshnessRule, 0),
	}
	for _, rule := range rules {
		switch {
		case rule.ProjectID == nil:
			cfg.TeamWideRules = append(cfg.TeamWideRules, rule)
		case *rule.ProjectID == projectID:
			cfg.ProjectRules = append(cfg.ProjectRules, rule)
		}
	}
	return cfg
}

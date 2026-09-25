package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/models"
)

// User detail insights (#1135): the pivoting, gap-filling and cursor handling
// behind the four read-only /admin/users/{id}/... operations. The repository
// returns sparse rows; everything shaped for the API happens here.

const (
	// AdminUserTimelineDefaultLimit is the timeline page size when none is given.
	AdminUserTimelineDefaultLimit = 50
	// AdminUserTimelineMaxLimit caps the timeline page size.
	AdminUserTimelineMaxLimit = 100
)

// ErrAdminInvalidCursor is returned for a timeline cursor that does not decode
// to a valid position. The handler maps it to 400.
type ErrAdminInvalidCursor struct {
	Detail string
}

func (e *ErrAdminInvalidCursor) Error() string { return e.Detail }

// UserExists reports whether a user with that id exists.
func (s *AdminService) UserExists(ctx context.Context, id string) (bool, error) {
	return s.adminRepo.UserExists(ctx, id)
}

// GetUserInsights returns the per-type counts of the resources a user authored,
// in total, per team and per project. (nil, nil) for an unknown user.
func (s *AdminService) GetUserInsights(ctx context.Context, id string) (*models.AdminUserInsights, error) {
	exists, err := s.adminRepo.UserExists(ctx, id)
	if err != nil || !exists {
		return nil, err
	}

	rows, err := s.adminRepo.GetUserResourceCounts(ctx, id)
	if err != nil {
		return nil, err
	}
	insights := pivotUserResourceCounts(rows)
	insights.UserID = id
	return &insights, nil
}

// pivotUserResourceCounts folds the sparse (type, team, project) rows into
// totals and a team → project tree. Rows arrive ordered by team then project,
// so appending in arrival order preserves that order.
func pivotUserResourceCounts(rows []models.AdminUserResourceCountRow) models.AdminUserInsights {
	insights := models.AdminUserInsights{Teams: make([]models.AdminUserTeamResourceCounts, 0)}
	teamIdx := make(map[string]int)
	projectIdx := make(map[string]int)

	for _, row := range rows {
		ti, ok := teamIdx[row.TeamID]
		if !ok {
			insights.Teams = append(insights.Teams, models.AdminUserTeamResourceCounts{
				TeamID:   row.TeamID,
				TeamName: row.TeamName,
				IsMember: row.IsMember,
				Projects: make([]models.AdminUserProjectResourceCounts, 0),
			})
			ti = len(insights.Teams) - 1
			teamIdx[row.TeamID] = ti
		}
		team := &insights.Teams[ti]

		addResourceCount(&insights.Totals, row.ResourceType, row.Count)
		addResourceCount(&team.Counts, row.ResourceType, row.Count)

		if row.ProjectID == nil {
			continue
		}
		pi, ok := projectIdx[*row.ProjectID]
		if !ok {
			name := ""
			if row.ProjectName != nil {
				name = *row.ProjectName
			}
			team.Projects = append(team.Projects, models.AdminUserProjectResourceCounts{
				ProjectID:   *row.ProjectID,
				ProjectName: name,
			})
			pi = len(team.Projects) - 1
			projectIdx[*row.ProjectID] = pi
		}
		addProjectResourceCount(&team.Projects[pi].Counts, row.ResourceType, row.Count)
	}
	return insights
}

// addResourceCount adds n to the per-type field for resourceType and to Total.
// An unknown type is ignored rather than counted in Total alone, so Total
// always equals the sum of the nine fields.
func addResourceCount(c *models.AdminResourceCounts, resourceType string, n int64) {
	var field *int64
	switch resourceType {
	case models.AdminResourceTypePrompt:
		field = &c.Prompts
	case models.AdminResourceTypeMemory:
		field = &c.Memories
	case models.AdminResourceTypeArtifact:
		field = &c.Artifacts
	case models.AdminResourceTypeBlueprint:
		field = &c.Blueprints
	case models.AdminResourceTypeAgent:
		field = &c.Agents
	case models.AdminResourceTypeFeed:
		field = &c.Feeds
	case models.AdminResourceTypeFeedItem:
		field = &c.FeedItems
	case models.AdminResourceTypeComment:
		field = &c.Comments
	case models.AdminResourceTypeAttachment:
		field = &c.Attachments
	default:
		return
	}
	*field += n
	c.Total += n
}

// addProjectResourceCount adds n to the project-scoped field for resourceType.
func addProjectResourceCount(c *models.AdminProjectResourceCounts, resourceType string, n int64) {
	switch resourceType {
	case models.AdminResourceTypePrompt:
		c.Prompts += n
	case models.AdminResourceTypeMemory:
		c.Memories += n
	case models.AdminResourceTypeArtifact:
		c.Artifacts += n
	case models.AdminResourceTypeBlueprint:
		c.Blueprints += n
	}
}

// GetUserCreationMetrics returns the gap-filled per-type creation series for a
// user. The range rules are the dashboard's (resolveAdminRange), so an invalid
// range yields *ErrAdminTimeseriesRange. (nil, nil) for an unknown user.
func (s *AdminService) GetUserCreationMetrics(
	ctx context.Context, id string, q AdminTimeseriesQuery,
) (*models.AdminUserCreationMetrics, error) {
	resolved, err := resolveAdminRange(q, time.Now())
	if err != nil {
		return nil, err
	}

	exists, err := s.adminRepo.UserExists(ctx, id)
	if err != nil || !exists {
		return nil, err
	}

	rows, err := s.adminRepo.GetUserCreationSeries(ctx, id, resolved.from, resolved.to, resolved.granularity)
	if err != nil {
		return nil, err
	}

	return &models.AdminUserCreationMetrics{
		From:        resolved.from,
		To:          resolved.to,
		Granularity: resolved.granularity,
		Series:      fillUserCreation(adminBuckets(resolved), rows),
	}, nil
}

// fillUserCreation pivots sparse (type, bucket, count) rows into one point per
// bucket, every type present as an explicit 0 when it had no rows.
func fillUserCreation(buckets []time.Time, rows []models.AdminGrowthCount) []models.AdminUserCreationPoint {
	byBucket := make(map[time.Time]*models.AdminUserCreationPoint, len(buckets))
	points := make([]models.AdminUserCreationPoint, len(buckets))
	for i, b := range buckets {
		points[i] = models.AdminUserCreationPoint{Bucket: b}
		byBucket[b] = &points[i]
	}

	for _, row := range rows {
		point, ok := byBucket[row.Bucket.UTC()]
		if !ok {
			// Only possible if the SQL and Go bucketing disagreed; see fillGrowth.
			continue
		}
		if field := userCreationField(point, row.Entity); field != nil {
			*field = row.Count
		}
	}
	return points
}

// userCreationField returns the per-type counter of p for resourceType, or nil
// for an unknown type.
func userCreationField(p *models.AdminUserCreationPoint, resourceType string) *int64 {
	switch resourceType {
	case models.AdminResourceTypePrompt:
		return &p.Prompts
	case models.AdminResourceTypeMemory:
		return &p.Memories
	case models.AdminResourceTypeArtifact:
		return &p.Artifacts
	case models.AdminResourceTypeBlueprint:
		return &p.Blueprints
	case models.AdminResourceTypeAgent:
		return &p.Agents
	case models.AdminResourceTypeFeed:
		return &p.Feeds
	case models.AdminResourceTypeFeedItem:
		return &p.FeedItems
	case models.AdminResourceTypeComment:
		return &p.Comments
	case models.AdminResourceTypeAttachment:
		return &p.Attachments
	default:
		return nil
	}
}

// GetUserTimeline returns one page of a user's opaque resource timeline, newest
// first. cursor is the previous page's NextCursor ("" for the first page); a
// malformed one yields *ErrAdminInvalidCursor. limit <= 0 means the default,
// and it is capped at AdminUserTimelineMaxLimit. (nil, nil) for an unknown user.
func (s *AdminService) GetUserTimeline(
	ctx context.Context, id, cursor string, limit int,
) (*models.AdminUserTimelinePage, error) {
	var pos *models.AdminTimelineCursor
	if cursor != "" {
		decoded, err := decodeAdminTimelineCursor(cursor)
		if err != nil {
			return nil, err
		}
		pos = &decoded
	}
	if limit <= 0 {
		limit = AdminUserTimelineDefaultLimit
	}
	if limit > AdminUserTimelineMaxLimit {
		limit = AdminUserTimelineMaxLimit
	}

	exists, err := s.adminRepo.UserExists(ctx, id)
	if err != nil || !exists {
		return nil, err
	}

	// Fetch one extra row: its presence is what says another page exists.
	events, err := s.adminRepo.ListUserTimeline(ctx, id, pos, limit+1)
	if err != nil {
		return nil, err
	}

	page := &models.AdminUserTimelinePage{Items: events}
	if len(events) > limit {
		page.Items = events[:limit]
		last := page.Items[limit-1]
		next, encErr := encodeAdminTimelineCursor(models.AdminTimelineCursor{
			OccurredAt:   last.OccurredAt,
			ResourceType: last.ResourceType,
			Action:       last.Action,
			ResourceID:   last.ResourceID,
		})
		if encErr != nil {
			return nil, encErr
		}
		page.NextCursor = &next
	}
	return page, nil
}

// adminTimelineCursorWire is the JSON inside the opaque cursor. The time is
// RFC3339Nano so Postgres' microsecond precision survives the round-trip —
// truncating it would make the keyset predicate skip or repeat rows.
type adminTimelineCursorWire struct {
	OccurredAt   string `json:"t"`
	ResourceType string `json:"r"`
	Action       string `json:"a"`
	ResourceID   string `json:"i"`
}

// adminTimelineResourceTypes is the allowlist a decoded cursor's type must be in.
var adminTimelineResourceTypes = map[string]struct{}{
	models.AdminResourceTypePrompt:     {},
	models.AdminResourceTypeMemory:     {},
	models.AdminResourceTypeArtifact:   {},
	models.AdminResourceTypeBlueprint:  {},
	models.AdminResourceTypeAgent:      {},
	models.AdminResourceTypeFeed:       {},
	models.AdminResourceTypeFeedItem:   {},
	models.AdminResourceTypeComment:    {},
	models.AdminResourceTypeAttachment: {},
}

// encodeAdminTimelineCursor serializes a position as unpadded base64url JSON.
func encodeAdminTimelineCursor(c models.AdminTimelineCursor) (string, error) {
	raw, err := json.Marshal(adminTimelineCursorWire{
		OccurredAt:   c.OccurredAt.UTC().Format(time.RFC3339Nano),
		ResourceType: c.ResourceType,
		Action:       c.Action,
		ResourceID:   c.ResourceID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to encode timeline cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// decodeAdminTimelineCursor parses and validates an opaque cursor. Every field
// is checked, so a tampered cursor is a 400 rather than a query error.
func decodeAdminTimelineCursor(cursor string) (models.AdminTimelineCursor, error) {
	invalid := &ErrAdminInvalidCursor{Detail: "invalid cursor"}

	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return models.AdminTimelineCursor{}, invalid
	}
	var wire adminTimelineCursorWire
	if jsonErr := json.Unmarshal(raw, &wire); jsonErr != nil {
		return models.AdminTimelineCursor{}, invalid
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, wire.OccurredAt)
	if err != nil {
		return models.AdminTimelineCursor{}, invalid
	}
	if _, ok := adminTimelineResourceTypes[wire.ResourceType]; !ok {
		return models.AdminTimelineCursor{}, invalid
	}
	if wire.Action != models.AdminTimelineActionCreated && wire.Action != models.AdminTimelineActionUpdated {
		return models.AdminTimelineCursor{}, invalid
	}
	if _, idErr := uuid.Parse(wire.ResourceID); idErr != nil {
		return models.AdminTimelineCursor{}, invalid
	}

	return models.AdminTimelineCursor{
		OccurredAt:   occurredAt,
		ResourceType: wire.ResourceType,
		Action:       wire.Action,
		ResourceID:   wire.ResourceID,
	}, nil
}

package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
)

// dailyTeamCreationCount is the flattened per-day creation count returned to the
// client for the team analytics page. It mirrors dailyCreationCount but adds a
// projects series, since projects belong to a team (not a project).
type dailyTeamCreationCount struct {
	Date       string `json:"date"`
	Prompts    int    `json:"prompts"`
	Artifacts  int    `json:"artifacts"`
	Blueprints int    `json:"blueprints"`
	Memories   int    `json:"memories"`
	Projects   int    `json:"projects"`
	Total      int    `json:"total"`
}

// teamResourceCreationMetricsData is the data payload of the team
// resource-creation-metrics response.
type teamResourceCreationMetricsData struct {
	TotalCreated int                      `json:"total_created"`
	Range        string                   `json:"range"`
	Counts       []dailyTeamCreationCount `json:"counts"`
}

// validateTeamAnalyticsRequest extracts and authorizes the team for an analytics
// request. The team analytics routes live under /api/v1/teams/{id}, whose path
// param is {id}; teamValidationMiddleware reads {team_id} and cannot mount here
// (chi forbids two wildcard names at the same position), so authorization is
// performed in-handler via the same validateTeamAccess membership check the
// middleware wraps. Returns the team id and true on success; otherwise it has
// already written the error response and returns false.
func (s *Server) validateTeamAnalyticsRequest(
	w http.ResponseWriter, r *http.Request, handler string,
) (string, bool) {
	userID, _ := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "id")

	if _, err := uuid.Parse(teamID); err != nil {
		writeErrorResponse(w, r, "bad_request", "team id must be a valid UUID", http.StatusBadRequest)
		return "", false
	}

	if err := s.validateTeamAccess(r.Context(), userID, teamID); err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": handler,
			"user_id": userID,
			"team_id": teamID,
		}).Warn("Team access denied for analytics request")
		writeErrorResponse(w, r, "access_denied", "Access denied", http.StatusForbidden)
		return "", false
	}

	return teamID, true
}

// handleGetTeamStats returns team-wide resource counts (projects, prompts,
// artifacts, blueprints, memories, feed_items) for the team analytics page.
func (s *Server) handleGetTeamStats(w http.ResponseWriter, r *http.Request) {
	teamID, ok := s.validateTeamAnalyticsRequest(w, r, "handleGetTeamStats")
	if !ok {
		return
	}

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleGetTeamStats",
		"team_id": teamID,
	}).Info("Get team stats request received")

	stats, err := s.container.TeamService().GetTeamStats(r.Context(), teamID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleGetTeamStats",
			"team_id": teamID,
			"error":   err.Error(),
		}).Error("Failed to get team stats")
		writeErrorResponse(w, r, "internal_error", "Failed to get team stats", http.StatusInternalServerError)
		return
	}

	writeOK(w, stats, s.logger)
}

// handleGetTeamResourceCreationMetrics returns a zero-filled daily creation
// timeseries (prompts, artifacts, blueprints, memories, projects) scoped to a
// team. Reuses the shared range definitions (resourceAccessRanges /
// defaultResourceAccessRange).
func (s *Server) handleGetTeamResourceCreationMetrics(w http.ResponseWriter, r *http.Request) {
	teamID, ok := s.validateTeamAnalyticsRequest(w, r, "handleGetTeamResourceCreationMetrics")
	if !ok {
		return
	}

	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = defaultResourceAccessRange
	}
	rangeDays, ok := resourceAccessRanges[rangeStr]
	if !ok {
		writeErrorResponse(w, r, "validation_error", "range is invalid", http.StatusBadRequest)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleGetTeamResourceCreationMetrics",
		"team_id": teamID,
		"range":   rangeStr,
	}).Info("Get team resource creation metrics request received")

	// Align the SQL window start with the zero-filled series start by truncating to
	// UTC midnight (see handleGetProjectResourceCreationMetrics for why).
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -rangeDays)
	since := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	counts, err := s.container.TeamService().GetTeamResourceCreationMetrics(r.Context(), teamID, since)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleGetTeamResourceCreationMetrics",
			"team_id": teamID,
			"error":   err.Error(),
		}).Error("Failed to get team resource creation metrics")
		writeErrorResponse(
			w, r, "internal_error",
			"Failed to get team resource creation metrics", http.StatusInternalServerError,
		)
		return
	}

	data := buildTeamResourceCreationMetricsData(counts, rangeStr, since, rangeDays)

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Team resource creation metrics retrieved successfully",
		"data":    data,
	}, s.logger)
}

// dailyTeamFeedCreationCount is the flattened per-day feed-creation count returned
// to the client. feed_items (AI updates) is the primary series the dashboard chart
// renders; feeds (channels) is included for completeness.
type dailyTeamFeedCreationCount struct {
	Date      string `json:"date"`
	Feeds     int    `json:"feeds"`
	FeedItems int    `json:"feed_items"`
	Total     int    `json:"total"`
}

// teamFeedCreationMetricsData is the data payload of the team
// feed-creation-metrics response.
type teamFeedCreationMetricsData struct {
	TotalCreated int                          `json:"total_created"`
	Range        string                       `json:"range"`
	Counts       []dailyTeamFeedCreationCount `json:"counts"`
}

// handleGetTeamFeedCreationMetrics returns a zero-filled daily feed-creation
// timeseries (feeds + feed_items) scoped to a team, powering the "AI feeds
// created" dashboard chart. Reuses the shared range definitions
// (resourceAccessRanges / defaultResourceAccessRange).
func (s *Server) handleGetTeamFeedCreationMetrics(w http.ResponseWriter, r *http.Request) {
	teamID, ok := s.validateTeamAnalyticsRequest(w, r, "handleGetTeamFeedCreationMetrics")
	if !ok {
		return
	}

	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = defaultResourceAccessRange
	}
	rangeDays, ok := resourceAccessRanges[rangeStr]
	if !ok {
		writeErrorResponse(w, r, "validation_error", "range is invalid", http.StatusBadRequest)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleGetTeamFeedCreationMetrics",
		"team_id": teamID,
		"range":   rangeStr,
	}).Info("Get team feed creation metrics request received")

	// Align the SQL window start with the zero-filled series start by truncating to
	// UTC midnight (see handleGetTeamResourceCreationMetrics for why).
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -rangeDays)
	since := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	counts, err := s.container.TeamService().GetTeamFeedCreationMetrics(r.Context(), teamID, since)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleGetTeamFeedCreationMetrics",
			"team_id": teamID,
			"error":   err.Error(),
		}).Error("Failed to get team feed creation metrics")
		writeErrorResponse(
			w, r, "internal_error",
			"Failed to get team feed creation metrics", http.StatusInternalServerError,
		)
		return
	}

	data := buildTeamFeedCreationMetricsData(counts, rangeStr, since, rangeDays)

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Team feed creation metrics retrieved successfully",
		"data":    data,
	}, s.logger)
}

// handleGetTeamResourceAccessMetrics returns a zero-filled daily access
// timeseries, grouped by source (web/cli/mcp/api), aggregated across the whole
// team for the team analytics page.
func (s *Server) handleGetTeamResourceAccessMetrics(w http.ResponseWriter, r *http.Request) {
	teamID, ok := s.validateTeamAnalyticsRequest(w, r, "handleGetTeamResourceAccessMetrics")
	if !ok {
		return
	}

	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = defaultResourceAccessRange
	}
	rangeDays, ok := resourceAccessRanges[rangeStr]
	if !ok {
		writeErrorResponse(w, r, "validation_error", "range is invalid", http.StatusBadRequest)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleGetTeamResourceAccessMetrics",
		"team_id": teamID,
		"range":   rangeStr,
	}).Info("Get team resource access metrics request received")

	result, err := s.container.ResourceAccessService().GetTeamMetrics(r.Context(), teamID, rangeDays)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleGetTeamResourceAccessMetrics",
			"team_id": teamID,
			"error":   err.Error(),
		}).Error("Failed to get team resource access metrics")
		writeErrorResponse(
			w, r, "internal_error",
			"Failed to get team resource access metrics", http.StatusInternalServerError,
		)
		return
	}

	// The team-wide result reuses the per-resource MetricsResult shape, so the
	// existing pivot/zero-fill builder applies unchanged.
	data := buildResourceAccessMetricsData(result, rangeStr)

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Team resource access metrics retrieved successfully",
		"data":    data,
	}, s.logger)
}

// buildTeamResourceCreationMetricsData zero-fills the sparse per-day/per-type
// counts into a contiguous daily series — every day from `since` through today,
// each carrying all five resource types (zero when absent) — and computes per-day
// and grand totals.
func buildTeamResourceCreationMetricsData(
	counts []models.TeamResourceCreationCount,
	rangeStr string,
	since time.Time,
	rangeDays int,
) teamResourceCreationMetricsData {
	// date -> resourceType -> count
	byDay := make(map[string]map[string]int, len(counts))
	for _, c := range counts {
		if byDay[c.Date] == nil {
			byDay[c.Date] = make(map[string]int, 5)
		}
		byDay[c.Date][c.ResourceType] += c.Count
	}

	series := make([]dailyTeamCreationCount, 0, rangeDays+1)
	totalCreated := 0
	for offset := 0; offset <= rangeDays; offset++ {
		date := since.AddDate(0, 0, offset).Format("2006-01-02")
		day := byDay[date]
		count := dailyTeamCreationCount{
			Date:       date,
			Prompts:    day["prompts"],
			Artifacts:  day["artifacts"],
			Blueprints: day["blueprints"],
			Memories:   day["memories"],
			Projects:   day["projects"],
		}
		count.Total = count.Prompts + count.Artifacts + count.Blueprints + count.Memories + count.Projects
		totalCreated += count.Total
		series = append(series, count)
	}

	return teamResourceCreationMetricsData{
		TotalCreated: totalCreated,
		Range:        rangeStr,
		Counts:       series,
	}
}

// buildTeamFeedCreationMetricsData zero-fills the sparse per-day feed/feed_item
// counts into a contiguous daily series — every day from `since` through today,
// each carrying both entity kinds (zero when absent) — and computes per-day and
// grand totals.
func buildTeamFeedCreationMetricsData(
	counts []models.TeamFeedCreationCount,
	rangeStr string,
	since time.Time,
	rangeDays int,
) teamFeedCreationMetricsData {
	// date -> entityType -> count
	byDay := make(map[string]map[string]int, len(counts))
	for _, c := range counts {
		if byDay[c.Date] == nil {
			byDay[c.Date] = make(map[string]int, 2)
		}
		byDay[c.Date][c.EntityType] += c.Count
	}

	series := make([]dailyTeamFeedCreationCount, 0, rangeDays+1)
	totalCreated := 0
	for offset := 0; offset <= rangeDays; offset++ {
		date := since.AddDate(0, 0, offset).Format("2006-01-02")
		day := byDay[date]
		count := dailyTeamFeedCreationCount{
			Date:      date,
			Feeds:     day["feeds"],
			FeedItems: day["feed_items"],
		}
		count.Total = count.Feeds + count.FeedItems
		totalCreated += count.Total
		series = append(series, count)
	}

	return teamFeedCreationMetricsData{
		TotalCreated: totalCreated,
		Range:        rangeStr,
		Counts:       series,
	}
}

// Top-accessed-resources defaults and bounds for the `limit` query param.
const (
	defaultTopAccessedLimit = 5
	maxTopAccessedLimit     = 50
)

// teamTopAccessedResourcesData is the data payload of the team
// top-accessed-resources response.
type teamTopAccessedResourcesData struct {
	Range string                       `json:"range"`
	Items []models.TopAccessedResource `json:"items"`
}

// handleGetTeamTopAccessedResources returns the team's most-accessed resources over
// a range, ranked by access count descending, capped by the `limit` query param
// (default 5, max 50). Each row carries the resolved display name so the frontend
// can render and deep-link without extra calls.
func (s *Server) handleGetTeamTopAccessedResources(w http.ResponseWriter, r *http.Request) {
	teamID, ok := s.validateTeamAnalyticsRequest(w, r, "handleGetTeamTopAccessedResources")
	if !ok {
		return
	}

	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = defaultResourceAccessRange
	}
	rangeDays, ok := resourceAccessRanges[rangeStr]
	if !ok {
		writeErrorResponse(w, r, "validation_error", "range is invalid", http.StatusBadRequest)
		return
	}

	limit, ok := parseTopAccessedLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeErrorResponse(w, r, "validation_error", "limit is invalid", http.StatusBadRequest)
		return
	}

	source := r.URL.Query().Get("source")
	if !isValidTopAccessedSource(source) {
		writeErrorResponse(w, r, "validation_error", "source is invalid", http.StatusBadRequest)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleGetTeamTopAccessedResources",
		"team_id": teamID,
		"range":   rangeStr,
		"limit":   limit,
		"source":  source,
	}).Info("Get team top accessed resources request received")

	items, err := s.container.ResourceAccessService().
		GetTopAccessedResources(r.Context(), teamID, rangeDays, source, limit)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleGetTeamTopAccessedResources",
			"team_id": teamID,
			"error":   err.Error(),
		}).Error("Failed to get team top accessed resources")
		writeErrorResponse(
			w, r, "internal_error",
			"Failed to get team top accessed resources", http.StatusInternalServerError,
		)
		return
	}

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Team top accessed resources retrieved successfully",
		"data": teamTopAccessedResourcesData{
			Range: rangeStr,
			Items: items,
		},
	}, s.logger)
}

// parseTopAccessedLimit validates the `limit` query param. Empty falls back to the
// default; otherwise it must be a positive integer no greater than the cap.
func parseTopAccessedLimit(raw string) (int, bool) {
	if raw == "" {
		return defaultTopAccessedLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxTopAccessedLimit {
		return 0, false
	}
	return n, true
}

// topAccessedSources is the set of accepted `source` query values. Empty and "all"
// both mean "aggregate across channels"; the rest mirror the access-event source
// dimension (web/cli/mcp/api) exposed by the resource-access-metrics endpoint.
var topAccessedSources = map[string]struct{}{
	"":    {},
	"all": {},
	"web": {},
	"cli": {},
	"mcp": {},
	"api": {},
}

// isValidTopAccessedSource reports whether the `source` query param is accepted.
func isValidTopAccessedSource(source string) bool {
	_, ok := topAccessedSources[source]
	return ok
}

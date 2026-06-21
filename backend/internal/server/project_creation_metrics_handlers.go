package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
)

// dailyCreationCount is the flattened per-day creation count returned to the
// client, with one field per resource type plus a per-day total. Mirrors the
// shape of dailyAccessCount so the frontend chart shell is shared.
type dailyCreationCount struct {
	Date       string `json:"date"`
	Prompts    int    `json:"prompts"`
	Artifacts  int    `json:"artifacts"`
	Blueprints int    `json:"blueprints"`
	Memories   int    `json:"memories"`
	Total      int    `json:"total"`
}

// projectResourceCreationMetricsData is the data payload of the
// resource-creation-metrics response.
type projectResourceCreationMetricsData struct {
	TotalCreated int                  `json:"total_created"`
	Range        string               `json:"range"`
	Counts       []dailyCreationCount `json:"counts"`
}

// handleGetProjectResourceCreationMetrics returns a zero-filled daily creation
// timeseries (prompts, artifacts, blueprints, memories) scoped to a single
// project, for the project detail-page chart. The team is validated by
// teamValidationMiddleware before this handler runs; project ownership/membership
// is enforced in the repository (same boundary as GetProjectStats). Reuses the
// shared range definitions (resourceAccessRanges / defaultResourceAccessRange).
func (s *Server) handleGetProjectResourceCreationMetrics(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	slug := chi.URLParam(r, "slug")

	decodedSlug, ok := s.decodeProjectSlug(w, userID, "handleGetProjectResourceCreationMetrics", slug)
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
		"handler": "handleGetProjectResourceCreationMetrics",
		"user_id": userID,
		"team_id": teamID,
		"slug":    decodedSlug,
		"range":   rangeStr,
	}).Info("Get project resource creation metrics request received")

	// Align the SQL window start with the zero-filled series start by truncating
	// to UTC midnight. AddDate carries the current time-of-day; without this the
	// oldest day would silently undercount rows created before now's time-of-day.
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -rangeDays)
	since := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)

	counts, err := s.container.ProjectService().GetProjectResourceCreationMetrics(teamID, userID, decodedSlug, since)
	if err != nil {
		s.handleGetProjectResourceCreationMetricsError(w, userID, decodedSlug, err)
		return
	}

	data := buildProjectResourceCreationMetricsData(counts, rangeStr, since, rangeDays)

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Resource creation metrics retrieved successfully",
		"data":    data,
	}, s.logger)
}

// handleGetProjectResourceCreationMetricsError maps repository errors to HTTP
// responses, mirroring handleGetProjectStatsError.
func (s *Server) handleGetProjectResourceCreationMetricsError(w http.ResponseWriter, userID, slug string, err error) {
	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleGetProjectResourceCreationMetrics",
		"user_id": userID,
		"slug":    slug,
		"error":   fmt.Sprintf("%+v", err),
	}).Error("Failed to get project resource creation metrics")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Project not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(
		w, nil, "internal_error",
		"Failed to get project resource creation metrics", http.StatusInternalServerError,
	)
}

// buildProjectResourceCreationMetricsData zero-fills the sparse per-day/per-type
// counts into a contiguous daily series — every day from `since` through today,
// each carrying all four resource types (zero when absent) — and computes per-day
// and grand totals.
func buildProjectResourceCreationMetricsData(
	counts []models.ProjectResourceCreationCount,
	rangeStr string,
	since time.Time,
	rangeDays int,
) projectResourceCreationMetricsData {
	// date -> resourceType -> count
	byDay := make(map[string]map[string]int, len(counts))
	for _, c := range counts {
		if byDay[c.Date] == nil {
			byDay[c.Date] = make(map[string]int, 4)
		}
		byDay[c.Date][c.ResourceType] += c.Count
	}

	series := make([]dailyCreationCount, 0, rangeDays+1)
	totalCreated := 0
	for offset := 0; offset <= rangeDays; offset++ {
		date := since.AddDate(0, 0, offset).Format("2006-01-02")
		day := byDay[date]
		count := dailyCreationCount{
			Date:       date,
			Prompts:    day["prompts"],
			Artifacts:  day["artifacts"],
			Blueprints: day["blueprints"],
			Memories:   day["memories"],
		}
		count.Total = count.Prompts + count.Artifacts + count.Blueprints + count.Memories
		totalCreated += count.Total
		series = append(series, count)
	}

	return projectResourceCreationMetricsData{
		TotalCreated: totalCreated,
		Range:        rangeStr,
		Counts:       series,
	}
}

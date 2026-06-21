package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// allowedResourceAccessTypes is the set of resource types the access-metrics
// endpoint accepts. These singular forms match what the recording middleware
// stores (see internal/services/resourceaccess).
var allowedResourceAccessTypes = map[string]struct{}{
	"prompt":    {},
	"artifact":  {},
	"blueprint": {},
	"memory":    {},
	"project":   {},
	"agent":     {},
}

// resourceAccessRanges maps the supported range query values to their day count.
// Shared by the per-resource, per-project, and team access/creation metrics
// handlers, so every one of those endpoints accepts the same set of ranges.
var resourceAccessRanges = map[string]int{
	"7d":   7,
	"14d":  14,
	"30d":  30,
	"60d":  60,  // 2 months
	"90d":  90,  // 3 months
	"180d": 180, // 6 months
}

// defaultResourceAccessRange is used when the range query param is omitted.
const defaultResourceAccessRange = "30d"

// dailyAccessCount is the flattened per-day access count returned to the client,
// with one field per known source plus a per-day total.
type dailyAccessCount struct {
	Date  string `json:"date"`
	Web   int    `json:"web"`
	CLI   int    `json:"cli"`
	MCP   int    `json:"mcp"`
	API   int    `json:"api"`
	Total int    `json:"total"`
}

// resourceAccessMetricsData is the data payload of the access-metrics response.
type resourceAccessMetricsData struct {
	TotalAccesses int                `json:"total_accesses"`
	Range         string             `json:"range"`
	Counts        []dailyAccessCount `json:"counts"`
}

// handleGetResourceAccessMetrics returns a per-resource daily access timeseries,
// grouped by source, for the resource detail-page chart. The team is validated
// by teamValidationMiddleware before this handler runs.
func (s *Server) handleGetResourceAccessMetrics(w http.ResponseWriter, r *http.Request) {
	// userID is observability-only here; team membership (not user identity) is the
	// authorization boundary, enforced by teamValidationMiddleware. Use the comma-ok
	// form so a missing context value logs an empty string instead of panicking.
	userID, _ := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	query := r.URL.Query()
	resourceType := query.Get("resource_type")
	resourceID := query.Get("resource_id")
	rangeStr := query.Get("range")
	if rangeStr == "" {
		rangeStr = defaultResourceAccessRange
	}

	if _, ok := allowedResourceAccessTypes[resourceType]; !ok {
		writeErrorResponse(w, r, "validation_error", "resource_type is invalid", http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(resourceID); err != nil {
		writeErrorResponse(w, r, "validation_error", "resource_id must be a valid UUID", http.StatusBadRequest)
		return
	}
	rangeDays, ok := resourceAccessRanges[rangeStr]
	if !ok {
		writeErrorResponse(w, r, "validation_error", "range is invalid", http.StatusBadRequest)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"service":       "vibexp-api",
		"handler":       "handleGetResourceAccessMetrics",
		"user_id":       userID,
		"team_id":       teamID,
		"resource_type": resourceType,
		"resource_id":   resourceID,
		"range":         rangeStr,
	}).Info("Resource access metrics request received")

	result, err := s.container.ResourceAccessService().GetMetrics(r.Context(), teamID, resourceType, resourceID, rangeDays)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service":       "vibexp-api",
			"handler":       "handleGetResourceAccessMetrics",
			"user_id":       userID,
			"team_id":       teamID,
			"resource_type": resourceType,
			"resource_id":   resourceID,
			"error":         err.Error(),
		}).Error("Failed to get resource access metrics")
		writeErrorResponse(w, r, "internal_error", "Failed to get resource access metrics", http.StatusInternalServerError)
		return
	}

	data := buildResourceAccessMetricsData(result, rangeStr)

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Resource access metrics retrieved successfully",
		"data":    data,
	}, s.logger)
}

// buildResourceAccessMetricsData pivots the service result's per-day source list
// into the flattened response shape, summing per-day and grand totals.
func buildResourceAccessMetricsData(
	result *resourceaccess.MetricsResult,
	rangeStr string,
) resourceAccessMetricsData {
	counts := make([]dailyAccessCount, 0, len(result.Days))
	totalAccesses := 0

	for _, day := range result.Days {
		count := dailyAccessCount{Date: day.Date}
		for _, point := range day.Sources {
			switch point.Source {
			case resourceaccess.SourceWeb:
				count.Web = point.Count
			case resourceaccess.SourceCLI:
				count.CLI = point.Count
			case resourceaccess.SourceMCP:
				count.MCP = point.Count
			case resourceaccess.SourceAPI:
				count.API = point.Count
			}
			count.Total += point.Count
		}
		totalAccesses += count.Total
		counts = append(counts, count)
	}

	return resourceAccessMetricsData{
		TotalAccesses: totalAccesses,
		Range:         rangeStr,
		Counts:        counts,
	}
}

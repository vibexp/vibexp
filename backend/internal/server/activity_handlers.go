package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/contextkeys"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// handleActivitiesGet handles GET /api/v1/activities
func (s *Server) handleActivitiesGet(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r)
	if userID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleActivitiesGet",
			"endpoint", "/api/v1/activities",
			"remote_ip", getClientIP(r),
			"user_agent", r.UserAgent(),
			"security_event", "unauthorized_access_attempt",
		).Warn("Unauthorized access attempt to activities list endpoint")
		writeErrorResponse(w, r, "unauthorized", "User not authenticated", http.StatusUnauthorized)
		return
	}

	filters := s.parseActivityFilters(r, userID)
	response, err := s.activityService.GetActivities(r.Context(), filters)
	if err != nil {
		s.logger.With("error", err).Error("Failed to get activities")
		writeErrorResponse(w, r, "server_error", "Failed to retrieve activities", http.StatusInternalServerError)
		return
	}

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Activities retrieved successfully",
		"data":    response,
	}, s.logger)
}

func (s *Server) parseActivityFilters(r *http.Request, userID string) activities.ActivityFilters {
	filters := activities.ActivityFilters{
		UserID: &userID,
		Limit:  25,
		Offset: 0,
	}

	s.parsePaginationParams(r, &filters)
	s.parseFilterParams(r, &filters)
	s.parseDateParams(r, &filters)

	return filters
}

func (s *Server) parsePaginationParams(r *http.Request, filters *activities.ActivityFilters) {
	// Parse limit first before calculating offset from page
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 100 {
			filters.Limit = limit
		}
	}
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			filters.Offset = (page - 1) * filters.Limit
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filters.Offset = offset
		}
	}
}

func (s *Server) parseFilterParams(r *http.Request, filters *activities.ActivityFilters) {
	if activityType := r.URL.Query().Get("activity_type"); activityType != "" {
		filters.ActivityType = &activityType
	}
	if entityType := r.URL.Query().Get("entity_type"); entityType != "" {
		filters.EntityType = &entityType
	}
	if entityID := r.URL.Query().Get("entity_id"); entityID != "" {
		filters.EntityID = &entityID
	}
	if sessionID := r.URL.Query().Get("session_id"); sessionID != "" {
		filters.SessionID = &sessionID
	}
	if search := r.URL.Query().Get("search"); search != "" {
		filters.Search = &search
	}
}

func (s *Server) parseDateParams(r *http.Request, filters *activities.ActivityFilters) {
	if dateFromStr := r.URL.Query().Get("date_from"); dateFromStr != "" {
		if dateFrom, err := time.Parse("2006-01-02", dateFromStr); err == nil {
			filters.DateFrom = &dateFrom
		}
	}
	if dateToStr := r.URL.Query().Get("date_to"); dateToStr != "" {
		if dateTo, err := time.Parse("2006-01-02", dateToStr); err == nil {
			dateTo = dateTo.Add(24*time.Hour - time.Nanosecond)
			filters.DateTo = &dateTo
		}
	}
}

// handleActivityGet handles GET /api/v1/activities/{id}
func (s *Server) handleActivityGet(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r)
	if userID == "" {
		// Log unauthorized access attempt for security monitoring
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleActivityGet",
			"endpoint", "/api/v1/activities/{id}",
			"remote_ip", getClientIP(r),
			"user_agent", r.UserAgent(),
			"security_event", "unauthorized_access_attempt",
		).Warn("Unauthorized access attempt to individual activity endpoint")

		writeErrorResponse(w, r, "unauthorized", "User not authenticated", http.StatusUnauthorized)
		return
	}

	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		writeErrorResponse(w, r, "bad_request", "Activity ID is required", http.StatusBadRequest)
		return
	}

	activity, err := s.activityService.GetActivityByID(r.Context(), userID, activityID)
	if err != nil {
		if errors.Is(err, repositories.ErrActivityNotFound) {
			writeErrorResponse(w, r, "not_found", "Activity not found", http.StatusNotFound)
			return
		}
		s.logger.With("error", err).Error("Failed to get activity")
		writeErrorResponse(w, r, "server_error", "Failed to retrieve activity", http.StatusInternalServerError)
		return
	}

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Activity retrieved successfully",
		"data":    activity,
	}, s.logger)
}

// handleActivitiesStatsGet handles GET /api/v1/activities/stats
func (s *Server) handleActivitiesStatsGet(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r)
	if userID == "" {
		// Log unauthorized access attempt for security monitoring
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleActivitiesStatsGet",
			"endpoint", "/api/v1/activities/stats",
			"remote_ip", getClientIP(r),
			"user_agent", r.UserAgent(),
			"security_event", "unauthorized_access_attempt",
		).Warn("Unauthorized access attempt to activity stats endpoint")

		writeErrorResponse(w, r, "unauthorized", "User not authenticated", http.StatusUnauthorized)
		return
	}

	stats, err := s.activityService.GetActivityStats(r.Context(), userID)
	if err != nil {
		s.logger.With("error", err).Error("Failed to get activity stats")
		writeErrorResponse(
			w,
			r,
			"server_error",
			"Failed to retrieve activity statistics",
			http.StatusInternalServerError,
		)
		return
	}

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Activity statistics retrieved successfully",
		"data":    stats,
	}, s.logger)
}

// handleActivitiesTypesGet handles GET /api/v1/activities/types
func (s *Server) handleActivitiesTypesGet(w http.ResponseWriter, r *http.Request) {
	// Validate user authentication - security fix for DL-207
	userID := s.getUserIDFromContext(r)
	if userID == "" {
		// Log unauthorized access attempt for security monitoring
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleActivitiesTypesGet",
			"endpoint", "/api/v1/activities/types",
			"remote_ip", getClientIP(r),
			"user_agent", r.UserAgent(),
			"security_event", "unauthorized_access_attempt",
		).Warn("Unauthorized access attempt to activity types endpoint")

		writeErrorResponse(w, r, "unauthorized", "User not authenticated", http.StatusUnauthorized)
		return
	}

	types := s.activityService.GetAllTypes()

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Activity and entity types retrieved successfully",
		"data":    types,
	}, s.logger)
}

// handleActivitiesEntityTypesGet handles GET /api/v1/activities/entity-types
func (s *Server) handleActivitiesEntityTypesGet(w http.ResponseWriter, r *http.Request) {
	// Validate user authentication - security fix for DL-207
	userID := s.getUserIDFromContext(r)
	if userID == "" {
		// Log unauthorized access attempt for security monitoring
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleActivitiesEntityTypesGet",
			"endpoint", "/api/v1/activities/entity-types",
			"remote_ip", getClientIP(r),
			"user_agent", r.UserAgent(),
			"security_event", "unauthorized_access_attempt",
		).Warn("Unauthorized access attempt to activity entity types endpoint")

		writeErrorResponse(w, r, "unauthorized", "User not authenticated", http.StatusUnauthorized)
		return
	}

	entityTypes := s.activityService.GetEntityTypes()

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Activity entity types retrieved successfully",
		"data":    entityTypes,
	}, s.logger)
}

// handleActivityPost handles POST /api/v1/activities (for manual activity creation, admin only)
func (s *Server) handleActivityPost(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r)
	if userID == "" {
		s.logUnauthorizedActivityAccess(r, "handleActivityPost")
		writeErrorResponse(w, r, "unauthorized", "User not authenticated", http.StatusUnauthorized)
		return
	}

	var req activities.CreateActivityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.With("error", err).Error("Failed to decode activity request")
		writeErrorResponse(w, r, "bad_request", "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if err := s.validateActivityRequest(&req); err != nil {
		writeErrorResponse(w, r, "bad_request", err.Error(), http.StatusBadRequest)
		return
	}

	s.enrichActivityRequest(&req, r)

	activity, err := s.activityService.RecordActivity(r.Context(), userID, req)
	if err != nil {
		s.logger.With("error", err).Error("Failed to create activity")
		writeErrorResponse(w, r, "server_error", "Failed to create activity", http.StatusInternalServerError)
		return
	}

	s.logActivityCreation(userID, activity, req)
	s.writeActivityResponse(w, activity, http.StatusCreated, "Activity created successfully")
}

func (s *Server) logUnauthorizedActivityAccess(r *http.Request, handler string) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", handler,
		"endpoint", "/api/v1/activities",
		"method", "POST",
		"remote_ip", getClientIP(r),
		"user_agent", r.UserAgent(),
		"security_event", "unauthorized_access_attempt",
	).Warn("Unauthorized access attempt to activity creation endpoint")
}

func (s *Server) validateActivityRequest(req *activities.CreateActivityRequest) error {
	if req.ActivityType == "" {
		return fmt.Errorf("activity type is required")
	}
	if req.EntityType == "" {
		return fmt.Errorf("entity type is required")
	}
	if req.Description == "" {
		return fmt.Errorf("description is required")
	}
	return nil
}

func (s *Server) enrichActivityRequest(req *activities.CreateActivityRequest, r *http.Request) {
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata["manual_creation"] = true
	req.Metadata["created_via"] = "api"

	clientIP := getClientIP(r)
	userAgent := r.UserAgent()
	req.SourceIP = &clientIP
	req.UserAgent = &userAgent
}

func (s *Server) logActivityCreation(
	userID string,
	activity *activities.Activity,
	req activities.CreateActivityRequest,
) {
	s.logger.With(
		"user_id", userID,
		"activity_id", activity.ID,
		"activity_type", req.ActivityType,
		"entity_type", req.EntityType,
	).Info("Activity created successfully")
}

func (s *Server) writeActivityResponse(
	w http.ResponseWriter,
	activity *activities.Activity,
	statusCode int,
	message string,
) {
	writeJSON(w, statusCode, map[string]interface{}{
		"status":  "success",
		"message": message,
		"data":    activity,
	}, s.logger)
}

// Helper function to get client IP address
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for load balancers/proxies)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		if idx := strings.Index(forwarded, ","); idx != -1 {
			return strings.TrimSpace(forwarded[:idx])
		}
		return strings.TrimSpace(forwarded)
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return strings.TrimSpace(realIP)
	}

	// Fall back to RemoteAddr
	// Handle both IPv4 (192.168.1.1:8080) and IPv6 ([::1]:8080) formats
	addr := r.RemoteAddr

	// Check if it's an IPv6 address (starts with '[')
	if strings.HasPrefix(addr, "[") {
		// IPv6 format: [::1]:8080
		if idx := strings.LastIndex(addr, "]:"); idx != -1 {
			// Remove brackets: [::1] -> ::1
			return addr[1:idx]
		}
		// If no port, just remove brackets
		return strings.Trim(addr, "[]")
	}

	// IPv4 format: 192.168.1.1:8080
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}

	return addr
}

// handleActivityRetentionJob handles POST /internal/jobs/activities/retention
// Protected by pubSubOIDCMiddleware (Cloud Scheduler → OIDC-authenticated HTTP).
func (s *Server) handleActivityRetentionJob(w http.ResponseWriter, r *http.Request) {
	logger := contextkeys.GetLoggerFromContext(r.Context())

	if err := s.activityService.RunRetentionJob(r.Context()); err != nil {
		logger.With("error", err).Error("Activity retention job failed")
		apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Retention job failed"))
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		logger.With("error", err).Error("Failed to write activity retention job response")
	}
}

// handleAccessEventsRetentionJob handles POST /internal/jobs/access-events/retention
// Protected by pubSubOIDCMiddleware (Cloud Scheduler → OIDC-authenticated HTTP).
func (s *Server) handleAccessEventsRetentionJob(w http.ResponseWriter, r *http.Request) {
	logger := contextkeys.GetLoggerFromContext(r.Context())

	if err := s.resourceAccessService.RunRetentionJob(r.Context()); err != nil {
		logger.With("error", err).Error("Access events retention job failed")
		apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Retention job failed"))
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		logger.With("error", err).Error("Failed to write access events retention job response")
	}
}

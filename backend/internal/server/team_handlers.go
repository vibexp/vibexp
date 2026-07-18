package server

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/pkg/events"
)

const (
	// serverLogServiceName is the service log-field value for the team handlers.

	// errNotFoundFragment is the service-error substring that maps to a
	// 404 with teamMsgNotFound.
	teamMsgNotFound       = "Team not found"
	teamMsgForbiddenWrite = "Forbidden team write attempt"
)

//nolint:funlen // Validation and limit checking adds necessary complexity
func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleCreateTeam",
		"user_id", userID,
	).Info("Create team request received")

	var req models.CreateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logTeamError("handleCreateTeam", userID, "", err, "Failed to decode request body")
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateCreateTeamRequest(w, &req) {
		return
	}

	// Check resource limit
	canCreate, err := s.container.ResourceUsageService().CheckResourceLimit(r.Context(), userID, events.ResourceTypeTeam)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleCreateTeam",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to check team creation limit")
		writeErrorResponse(w, nil, "internal_error", "Failed to check team creation limit", http.StatusInternalServerError)
		return
	}
	if !canCreate {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleCreateTeam",
			"user_id", userID,
		).Warn("Team creation limit exceeded")
		writeErrorResponse(
			w, nil, "resource_limit_exceeded",
			"You have reached your team creation limit for your current plan",
			http.StatusForbidden,
		)
		return
	}

	team, err := s.container.TeamService().CreateTeam(r.Context(), userID, &req)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleCreateTeam",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create team")
		writeErrorResponse(w, nil, "internal_error", "Failed to create team", http.StatusInternalServerError)
		return
	}

	s.bootstrapDefaultProject(userID, team)

	writeCreated(w, team, s.logger)
}

// bootstrapDefaultProject creates the default "Project 1" project for a newly
// created team so resources created immediately afterwards are correctly scoped.
// It mirrors the signup listener's non-blocking semantics: failures are logged
// and swallowed so team creation still succeeds.
//
// The call is deliberately synchronous (unlike the signup listener) so the
// project exists before the 201 response returns — that ordering is what fixes
// the mis-scoping race in #1579. CreateProject manages its own context
// internally, so the write completes even if the client disconnects.
func (s *Server) bootstrapDefaultProject(userID string, team *models.Team) {
	projectService := s.container.ProjectService()
	if projectService == nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleCreateTeam",
			"user_id", userID,
			"team_id", team.ID,
		).Warn("Project service not configured, skipping default project creation")
		return
	}

	if _, err := projectService.CreateProject(userID, team.ID, models.DefaultProjectRequest()); err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleCreateTeam",
			"user_id", userID,
			"team_id", team.ID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create default project for new team")
	}
}

func (s *Server) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "id")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleGetTeam",
		"user_id", userID,
		"team_id", teamID,
	).Info("Get team request received")

	team, err := s.container.TeamService().GetTeam(r.Context(), userID, teamID)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleGetTeam",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get team")

		if strings.Contains(err.Error(), errNotFoundFragment) {
			writeErrorResponse(w, nil, "not_found", teamMsgNotFound, http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to get team", http.StatusInternalServerError)
		return
	}

	writeOK(w, team, s.logger)
}

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleListTeams",
		"user_id", userID,
	).Info("List teams request received")

	// Parse query parameters
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 20
	if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	response, err := s.container.TeamService().ListTeams(r.Context(), userID, page, pageSize)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleListTeams",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list teams")
		writeErrorResponse(w, nil, "internal_error", "Failed to list teams", http.StatusInternalServerError)
		return
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "id")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleUpdateTeam",
		"user_id", userID,
		"team_id", teamID,
	).Info("Update team request received")

	var req models.UpdateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logTeamError("handleUpdateTeam", userID, teamID, err, "Failed to decode request body")
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateUpdateTeamRequest(w, &req) {
		return
	}

	team, err := s.container.TeamService().UpdateTeam(r.Context(), userID, teamID, &req)
	if err != nil {
		if stderrors.Is(err, services.ErrPermissionDenied) {
			s.logger.With(
				"service", serverLogServiceName,
				"handler", "handleUpdateTeam",
				"user_id", userID,
				"team_id", teamID,
			).Warn(teamMsgForbiddenWrite)
			writeErrorResponse(
				w, r, "forbidden",
				"Only team owners and admins can update a team", http.StatusForbidden,
			)
			return
		}

		if stderrors.Is(err, services.ErrTeamNotFound) || strings.Contains(err.Error(), errNotFoundFragment) {
			writeErrorResponse(w, nil, "not_found", teamMsgNotFound, http.StatusNotFound)
			return
		}

		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleUpdateTeam",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update team")
		writeErrorResponse(w, nil, "internal_error", "Failed to update team", http.StatusInternalServerError)
		return
	}

	writeOK(w, team, s.logger)
}

func (s *Server) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "id")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleDeleteTeam",
		"user_id", userID,
		"team_id", teamID,
	).Info("Delete team request received")

	err := s.container.TeamService().DeleteTeam(r.Context(), userID, teamID)
	if err != nil {
		// Handle forbidden before logging at ERROR to keep benign client errors at WARN
		if stderrors.Is(err, services.ErrPermissionDenied) {
			s.logger.With(
				"service", serverLogServiceName,
				"handler", "handleDeleteTeam",
				"user_id", userID,
				"team_id", teamID,
			).Warn(teamMsgForbiddenWrite)
			writeErrorResponse(w, r, "forbidden", "Only the team owner can delete a team", http.StatusForbidden)
			return
		}

		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleDeleteTeam",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete team")

		writeDeleteTeamError(w, r, err)
		return
	}

	writeNoContent(w)
}

// writeDeleteTeamError maps a DeleteTeam failure (other than permission
// denied, which the handler responds to before logging at ERROR) to its HTTP
// response.
func writeDeleteTeamError(w http.ResponseWriter, r *http.Request, err error) {
	// Handle custom error types with detailed RFC 9457 responses
	if activeSubErr, ok := err.(*services.ActiveSubscriptionError); ok {
		writeErrorResponseWithDetails(w, r, "ACTIVE_SUBSCRIPTION_EXISTS",
			"Active Subscription Exists",
			activeSubErr.Error(),
			http.StatusConflict,
			map[string]any{
				"subscription_id":    activeSubErr.SubscriptionID,
				"subscription_tier":  activeSubErr.SubscriptionTier,
				"billing_portal_url": activeSubErr.BillingPortalURL,
				"help_text":          activeSubErr.HelpText,
			})
		return
	}

	if cancelingErr, ok := err.(*services.SubscriptionCancelingError); ok {
		writeErrorResponseWithDetails(w, r, "SUBSCRIPTION_CANCELING",
			"Subscription Canceling",
			cancelingErr.Error(),
			http.StatusConflict,
			map[string]any{
				"cancel_at": cancelingErr.CancelAt,
			})
		return
	}

	if membersErr, ok := err.(*services.TeamHasMembersError); ok {
		writeErrorResponseWithDetails(w, r, "TEAM_HAS_MEMBERS",
			"Team Has Members",
			membersErr.Error(),
			http.StatusConflict,
			map[string]any{
				"member_count": strconv.Itoa(membersErr.MemberCount),
			})
		return
	}

	if _, ok := err.(*services.CannotDeletePersonalWorkspaceError); ok {
		writeErrorResponse(w, nil, "CANNOT_DELETE_PERSONAL_WORKSPACE",
			"Cannot delete personal workspace", http.StatusForbidden)
		return
	}

	// Handle generic error strings
	if stderrors.Is(err, services.ErrTeamNotFound) || strings.Contains(err.Error(), errNotFoundFragment) {
		writeErrorResponse(w, nil, "not_found", teamMsgNotFound, http.StatusNotFound)
		return
	}

	if strings.Contains(err.Error(), "cannot delete default team") {
		writeErrorResponse(w, nil, "forbidden", "Cannot delete default team", http.StatusForbidden)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to delete team", http.StatusInternalServerError)
}

// validateCreateTeamRequest validates the create team request
func (s *Server) validateCreateTeamRequest(w http.ResponseWriter, req *models.CreateTeamRequest) bool {
	if req.Name == "" {
		writeErrorResponse(w, nil, "validation_error", "Name is required", http.StatusBadRequest)
		return false
	}

	if len(req.Name) > 100 {
		writeErrorResponse(w, nil, "validation_error", "Name cannot be longer than 100 characters", http.StatusBadRequest)
		return false
	}

	if len(req.Description) > 500 {
		writeErrorResponse(
			w, nil, "validation_error",
			"Description cannot be longer than 500 characters",
			http.StatusBadRequest,
		)
		return false
	}

	return true
}

// validateUpdateTeamRequest validates the update team request
func (s *Server) validateUpdateTeamRequest(w http.ResponseWriter, req *models.UpdateTeamRequest) bool {
	if req.Name == nil && req.Description == nil {
		writeErrorResponse(
			w, nil, "validation_error",
			"At least one field (name or description) must be provided",
			http.StatusBadRequest,
		)
		return false
	}

	if req.Name != nil {
		if *req.Name == "" {
			writeErrorResponse(w, nil, "validation_error", "Name cannot be empty", http.StatusBadRequest)
			return false
		}
		if len(*req.Name) > 100 {
			writeErrorResponse(
				w, nil, "validation_error",
				"Name cannot be longer than 100 characters",
				http.StatusBadRequest,
			)
			return false
		}
	}

	if req.Description != nil && len(*req.Description) > 500 {
		writeErrorResponse(
			w, nil, "validation_error",
			"Description cannot be longer than 500 characters",
			http.StatusBadRequest,
		)
		return false
	}

	return true
}

// logTeamError logs a team-related error
func (s *Server) logTeamError(handler, userID, teamID string, err error, msg string) {
	fields := []any{
		"service", serverLogServiceName,
		"handler", handler,
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	}
	if teamID != "" {
		fields = append(fields, "team_id", teamID)
	}
	s.logger.With(fields...).Error(msg)
}

func (s *Server) handleGetTeamMembers(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "id")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleGetTeamMembers",
		"user_id", userID,
		"team_id", teamID,
	).Info("Get team members request received")

	// Parse pagination parameters
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 100 // Default page size for members list
	if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	response, err := s.container.TeamService().GetTeamMembers(r.Context(), userID, teamID, page, pageSize)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleGetTeamMembers",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get team members")

		if strings.Contains(err.Error(), errNotFoundFragment) {
			writeErrorResponse(w, nil, "not_found", teamMsgNotFound, http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to get team members", http.StatusInternalServerError)
		return
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "id")
	memberUserID := chi.URLParam(r, "userId")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleRemoveTeamMember",
		"user_id", userID,
		"team_id", teamID,
		"member_id", memberUserID,
	).Info("Remove team member request received")

	err := s.container.TeamService().RemoveTeamMember(r.Context(), userID, teamID, memberUserID)
	if err != nil {
		if stderrors.Is(err, services.ErrPermissionDenied) {
			s.logger.With(
				"service", serverLogServiceName,
				"handler", "handleRemoveTeamMember",
				"user_id", userID,
				"team_id", teamID,
				"member_id", memberUserID,
			).Warn(teamMsgForbiddenWrite)
			writeErrorResponse(
				w, r, "forbidden",
				"Only team owners and admins can remove members", http.StatusForbidden,
			)
			return
		}

		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleRemoveTeamMember",
			"user_id", userID,
			"team_id", teamID,
			"member_id", memberUserID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to remove team member")

		if stderrors.Is(err, services.ErrTeamNotFound) || strings.Contains(err.Error(), errNotFoundFragment) {
			writeErrorResponse(w, nil, "not_found", "Team or member not found", http.StatusNotFound)
			return
		}

		if stderrors.Is(err, services.ErrCannotRemoveTeamOwner) {
			writeErrorResponse(w, r, "forbidden", "Cannot remove team owner", http.StatusForbidden)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to remove team member", http.StatusInternalServerError)
		return
	}

	writeNoContent(w)
}

// writeErrorResponseWithDetails writes an RFC 9457 error response carrying the
// given metadata. The metadata values must be JSON-serializable; callers pass
// strings so the frontend's string-only metadata reader can consume them.
func writeErrorResponseWithDetails(
	w http.ResponseWriter,
	r *http.Request,
	code, title, detail string,
	statusCode int,
	metadata map[string]any,
) {
	apiErr := errors.NewAPIError(code, title, detail, statusCode)
	apiErr.Metadata = metadata
	errors.WriteJSONError(w, r, apiErr)
}

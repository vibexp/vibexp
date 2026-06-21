package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/sirupsen/logrus"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// parseTeamMemberRole converts string role to TeamMemberRole type
func parseTeamMemberRole(roleStr string) (models.TeamMemberRole, error) {
	switch roleStr {
	case "member":
		return models.TeamMemberRoleMember, nil
	case "admin":
		return models.TeamMemberRoleAdmin, nil
	default:
		return "", fmt.Errorf("invalid role: %s", roleStr)
	}
}

// validateInvitationRequest validates and parses the invitation request
func (s *Server) validateInvitationRequest(
	w http.ResponseWriter, r *http.Request, userID, teamID string,
) (*models.SendInvitationsRequest, models.TeamMemberRole, bool) {
	var req models.SendInvitationsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleSendTeamInvitations",
			"user_id": userID,
			"team_id": teamID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to decode request body")
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return nil, "", false
	}

	if err := validator.New().Struct(req); err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleSendTeamInvitations",
			"user_id": userID,
			"team_id": teamID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Request validation failed")
		writeErrorResponse(w, nil, "validation_error", "Invalid request data", http.StatusBadRequest)
		return nil, "", false
	}

	if len(req.Emails) > 50 {
		writeErrorResponse(w, nil, "validation_error", "Maximum 50 emails allowed per request", http.StatusBadRequest)
		return nil, "", false
	}

	role, err := parseTeamMemberRole(req.Role)
	if err != nil {
		writeErrorResponse(w, nil, "validation_error", "Invalid role. Must be 'member' or 'admin'", http.StatusBadRequest)
		return nil, "", false
	}

	return &req, role, true
}

// convertInvitationsToResponses converts invitation models to response format
func convertInvitationsToResponses(invitations []*models.TeamInvitation) []models.InvitationResponse {
	responses := make([]models.InvitationResponse, len(invitations))
	for i, inv := range invitations {
		responses[i] = models.InvitationResponse{
			ID:           inv.ID,
			TeamID:       inv.TeamID,
			InviteeEmail: inv.InviteeEmail,
			Role:         string(inv.Role),
			Status:       string(inv.Status),
			ExpiresAt:    inv.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
			CreatedAt:    inv.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}
	return responses
}

// handleSendTeamInvitations sends invitations to join a team
// POST /api/v1/teams/{id}/invitations
func (s *Server) handleSendTeamInvitations(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleSendTeamInvitations",
		"user_id": userID,
		"team_id": teamID,
	}).Info("Send team invitations request received")

	req, role, valid := s.validateInvitationRequest(w, r, userID, teamID)
	if !valid {
		return
	}

	invitations, err := s.container.TeamInvitationService().InviteMembers(r.Context(), userID, teamID, req.Emails, role)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleSendTeamInvitations",
			"user_id": userID,
			"team_id": teamID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to send invitations")

		s.handleInvitationError(w, r, err, teamID)
		return
	}

	responses := convertInvitationsToResponses(invitations)

	writeCreated(w, responses, s.logger)
}

// handleListTeamInvitations lists all invitations for a team
// GET /api/v1/teams/{id}/invitations
func (s *Server) handleListTeamInvitations(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleListTeamInvitations",
		"user_id": userID,
		"team_id": teamID,
	}).Info("List team invitations request received")

	invitations, err := s.container.TeamInvitationService().GetTeamInvitations(r.Context(), userID, teamID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleListTeamInvitations",
			"user_id": userID,
			"team_id": teamID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to list invitations")

		if strings.Contains(err.Error(), "permission") {
			writeErrorResponse(w, nil, "forbidden", "You don't have permission to view invitations", http.StatusForbidden)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to list invitations", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	responses := make([]models.InvitationResponse, len(invitations))
	for i, inv := range invitations {
		responses[i] = models.InvitationResponse{
			ID:           inv.ID,
			TeamID:       inv.TeamID,
			InviteeEmail: inv.InviteeEmail,
			Role:         string(inv.Role),
			Status:       string(inv.Status),
			ExpiresAt:    inv.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
			CreatedAt:    inv.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	writeOK(w, responses, s.logger)
}

// handlePersonalWorkspaceError handles personal workspace errors
func handlePersonalWorkspaceError(w http.ResponseWriter) {
	writeErrorResponse(
		w, nil,
		"upgrade_required",
		"This is a personal workspace. Upgrade to a team plan to invite members and enable collaboration.",
		http.StatusForbidden,
	)
}

// handleDuplicateMembersError handles duplicate members errors
func handleDuplicateMembersError(w http.ResponseWriter, r *http.Request, duplicateErr *services.DuplicateMembersError) {
	apiErr := apierrors.NewDuplicateMembersError(duplicateErr.DuplicateEmails)
	apierrors.WriteJSONError(w, r, apiErr)
}

// handleNoSubscriptionError handles no active subscription errors
func handleNoSubscriptionError(w http.ResponseWriter, r *http.Request, teamID string) {
	apiErr := apierrors.NewResourceLimitExceededErrorWithMetadata(
		"Team requires an active subscription to invite members. Please upgrade your plan to enable team collaboration.",
		map[string]any{
			"team_id":     teamID,
			"feature":     "team_invitations",
			"upgrade_url": "/subscription?type=team",
		},
	)
	apierrors.WriteJSONError(w, r, apiErr)
}

// handleSeatLimitError handles seat limit exceeded errors
func handleSeatLimitError(
	w http.ResponseWriter, r *http.Request, seatLimitErr *services.SeatLimitExceededError, teamID string,
) {
	totalOccupied := seatLimitErr.CurrentMembers + seatLimitErr.PendingInvites
	availableSeats := seatLimitErr.PaidSeats - totalOccupied

	// Ensure availableSeats doesn't go negative
	if availableSeats < 0 {
		availableSeats = 0
	}

	additionalSeatsNeeded := seatLimitErr.RequestedInvites - availableSeats
	if additionalSeatsNeeded < 0 {
		additionalSeatsNeeded = 0
	}

	apiErr := apierrors.NewResourceLimitExceededErrorWithMetadata(
		seatLimitErr.Error(),
		map[string]any{
			"team_id":                 teamID,
			"total_seats":             seatLimitErr.PaidSeats,
			"occupied_seats":          totalOccupied,
			"current_members":         seatLimitErr.CurrentMembers,
			"pending_invitations":     seatLimitErr.PendingInvites,
			"requested_invitations":   seatLimitErr.RequestedInvites,
			"available_seats":         availableSeats,
			"additional_seats_needed": additionalSeatsNeeded,
			"upgrade_url":             "/subscription?type=team",
		},
	)
	apierrors.WriteJSONError(w, r, apiErr)
}

// handleRevokeInvitation revokes a team invitation
// DELETE /api/v1/teams/{id}/invitations/{invitationId}
// handleInvitationError handles errors from invitation operations
func (s *Server) handleInvitationError(w http.ResponseWriter, r *http.Request, err error, teamID string) {
	var personalWorkspaceErr *services.PersonalWorkspaceError
	if errors.As(err, &personalWorkspaceErr) {
		handlePersonalWorkspaceError(w)
		return
	}

	var duplicateErr *services.DuplicateMembersError
	if errors.As(err, &duplicateErr) {
		handleDuplicateMembersError(w, r, duplicateErr)
		return
	}

	var noSubErr *services.NoActiveSubscriptionError
	if errors.As(err, &noSubErr) {
		handleNoSubscriptionError(w, r, teamID)
		return
	}

	var seatLimitErr *services.SeatLimitExceededError
	if errors.As(err, &seatLimitErr) {
		handleSeatLimitError(w, r, seatLimitErr, teamID)
		return
	}

	if strings.Contains(err.Error(), "permission") {
		writeErrorResponse(w, nil, "forbidden", "You don't have permission to invite members", http.StatusForbidden)
		return
	}

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Team not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to send invitations", http.StatusInternalServerError)
}

func (s *Server) handleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "id")
	invitationID := chi.URLParam(r, "invitationId")

	s.logger.WithFields(logrus.Fields{
		"service":       "vibexp-api",
		"handler":       "handleRevokeInvitation",
		"user_id":       userID,
		"team_id":       teamID,
		"invitation_id": invitationID,
	}).Info("Revoke invitation request received")

	err := s.container.TeamInvitationService().RevokeInvitation(r.Context(), userID, invitationID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service":       "vibexp-api",
			"handler":       "handleRevokeInvitation",
			"user_id":       userID,
			"team_id":       teamID,
			"invitation_id": invitationID,
			"error":         fmt.Sprintf("%+v", err),
		}).Error("Failed to revoke invitation")

		if strings.Contains(err.Error(), "permission") {
			writeErrorResponse(w, nil, "forbidden", "You don't have permission to revoke invitations", http.StatusForbidden)
			return
		}

		if strings.Contains(err.Error(), "not found") {
			writeErrorResponse(w, nil, "not_found", "Invitation not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to revoke invitation", http.StatusInternalServerError)
		return
	}

	writeNoContent(w)
}

// fetchTeamsForInvitations fetches team details for invitations
func (s *Server) fetchTeamsForInvitations(
	ctx context.Context,
	userID string,
	teamIDs map[string]bool,
) map[string]*models.Team {
	teams := make(map[string]*models.Team)
	for teamID := range teamIDs {
		team, teamErr := s.container.TeamService().GetTeam(ctx, userID, teamID)
		if teamErr == nil {
			teams[teamID] = team
		}
	}
	return teams
}

// fetchInvitersForInvitations fetches inviter details for invitations
func (s *Server) fetchInvitersForInvitations(
	ctx context.Context,
	inviterIDs map[string]bool,
) map[string]*models.User {
	inviters := make(map[string]*models.User)
	for inviterID := range inviterIDs {
		inviter, inviterErr := s.container.UserRepository().GetByID(ctx, inviterID)
		if inviterErr == nil {
			inviters[inviterID] = inviter
		}
	}
	return inviters
}

// buildInvitationResponses builds invitation responses with enriched data
func (s *Server) buildInvitationResponses(
	invitations []*models.TeamInvitation,
	teams map[string]*models.Team,
	inviters map[string]*models.User,
) []models.InvitationResponse {
	responses := make([]models.InvitationResponse, len(invitations))
	for i, inv := range invitations {
		responses[i] = models.InvitationResponse{
			ID:           inv.ID,
			Token:        inv.Token,
			TeamID:       inv.TeamID,
			TeamName:     "",
			InviteeEmail: inv.InviteeEmail,
			Role:         string(inv.Role),
			Status:       string(inv.Status),
			ExpiresAt:    inv.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
			CreatedAt:    inv.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		if team, exists := teams[inv.TeamID]; exists {
			responses[i].TeamName = team.Name
		}

		if inviter, exists := inviters[inv.InviterID]; exists {
			responses[i].InvitedBy = &models.InviterInfo{
				ID:    inviter.ID,
				Name:  inviter.Name,
				Email: inviter.Email,
			}
		}
	}
	return responses
}

// handleGetPendingInvitations gets pending invitations for the current user
// GET /api/v1/invitations/pending
func (s *Server) handleGetPendingInvitations(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	user, err := s.container.UserRepository().GetByID(r.Context(), userID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleGetPendingInvitations",
			"user_id": userID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to get user")
		writeErrorResponse(w, nil, "internal_error", "Failed to get user information", http.StatusInternalServerError)
		return
	}

	invitations, err := s.container.TeamInvitationService().GetPendingInvitations(r.Context(), user.Email)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleGetPendingInvitations",
			"user_id": userID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to get pending invitations")
		writeErrorResponse(w, nil, "internal_error", "Failed to get pending invitations", http.StatusInternalServerError)
		return
	}

	// Collect unique team IDs and inviter IDs
	teamIDs := make(map[string]bool)
	inviterIDs := make(map[string]bool)
	for _, inv := range invitations {
		teamIDs[inv.TeamID] = true
		inviterIDs[inv.InviterID] = true
	}

	teams := s.fetchTeamsForInvitations(r.Context(), userID, teamIDs)
	inviters := s.fetchInvitersForInvitations(r.Context(), inviterIDs)
	responses := s.buildInvitationResponses(invitations, teams, inviters)

	response := models.PendingInvitationsListResponse{
		Invitations: responses,
		TotalCount:  len(responses),
		Page:        1,
		PageSize:    20,
	}

	writeOK(w, response, s.logger)
}

// handleGetInvitationByTokenError maps service-layer errors from GetInvitationByToken
// to RFC 9457 API errors with appropriate HTTP statuses.
//
// 404 → unknown / not found
// 410 → expired (pending and past expires_at)
// 409 → revoked, accepted, or rejected (state error)
// 500 → anything else (generic message; details only in logs)
func (s *Server) handleGetInvitationByTokenError(w http.ResponseWriter, r *http.Request, err error) {
	var notFoundErr *services.InvitationNotFoundError
	if errors.As(err, &notFoundErr) {
		apiErr := apierrors.NewResourceNotFoundError("invitation", "Invitation not found")
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	var expiredErr *services.InvitationExpiredError
	if errors.As(err, &expiredErr) {
		apiErr := apierrors.NewAPIError(
			apierrors.CodeResourceConflict,
			"Invitation Expired",
			"Invitation has expired",
			http.StatusGone,
		)
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	var stateErr *services.InvitationStateError
	if errors.As(err, &stateErr) {
		var detail string
		switch stateErr.Status {
		case models.InvitationStatusAccepted:
			detail = "Invitation has already been accepted"
		case models.InvitationStatusRejected:
			detail = "Invitation has been rejected"
		case models.InvitationStatusRevoked:
			detail = "Invitation has been revoked"
		default:
			detail = "Invitation is no longer pending"
		}
		apiErr := apierrors.NewAPIError(
			apierrors.CodeResourceConflict,
			apierrors.GetErrorTitle(apierrors.CodeResourceConflict),
			detail,
			http.StatusConflict,
		)
		apiErr.Metadata = map[string]any{"status": string(stateErr.Status)}
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	// Unmapped error → genuine 500. Log the underlying error here (not in the
	// caller) so 4xx-class typed errors don't add log noise. Token is redacted.
	s.logger.WithError(err).WithFields(logrus.Fields{
		"service":      "vibexp-api",
		"handler":      "handleGetInvitationByToken",
		"token_digest": redactToken(chi.URLParam(r, "token")),
	}).Error("Failed to load invitation by token")
	apiErr := apierrors.NewInternalError("Failed to load invitation")
	apierrors.WriteJSONError(w, r, apiErr)
}

// redactToken produces a short, irreversible fingerprint of an invitation token
// suitable for log correlation without exposing the credential.
//
// Format: "<sha256[:8]>" (16 hex chars). Empty inputs render as "(empty)" so we
// never emit a misleading partial token for absent values.
func redactToken(token string) string {
	if token == "" {
		return "(empty)"
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:8])
}

// handleGetInvitationByToken returns the details of an invitation by token so the
// email-link landing page can display the team name, inviter, role, and expiry
// before the user accepts or rejects.
// GET /api/v1/invitations/{token}
func (s *Server) handleGetInvitationByToken(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	// Token is the access credential for the invitation — never log it raw.
	// A short, irreversible fingerprint is enough for correlation in logs.
	s.logger.WithFields(logrus.Fields{
		"service":      "vibexp-api",
		"handler":      "handleGetInvitationByToken",
		"token_digest": redactToken(token),
	}).Info("Get invitation by token request received")

	details, err := s.container.TeamInvitationService().GetInvitationByToken(r.Context(), token)
	if err != nil {
		s.handleGetInvitationByTokenError(w, r, err)
		return
	}

	inv := details.Invitation
	response := models.InvitationDetailsResponse{
		Invitation: models.InvitationResponse{
			ID:           inv.ID,
			Token:        inv.Token,
			TeamID:       inv.TeamID,
			TeamName:     details.TeamName,
			InviteeEmail: inv.InviteeEmail,
			Role:         string(inv.Role),
			Status:       string(inv.Status),
			ExpiresAt:    inv.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
			CreatedAt:    inv.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			InvitedBy:    details.InvitedBy,
		},
	}

	writeOK(w, response, s.logger)
}

// handleAcceptInvitationError handles errors during invitation acceptance
func (s *Server) handleAcceptInvitationError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Invalid invitation token", http.StatusNotFound)
		return
	}

	if strings.Contains(err.Error(), "expired") {
		writeErrorResponse(w, nil, "expired", "Invitation has expired", http.StatusGone)
		return
	}

	if strings.Contains(err.Error(), "already a member") {
		writeErrorResponse(w, nil, "conflict", "You are already a member of this team", http.StatusConflict)
		return
	}

	if strings.Contains(err.Error(), "not pending") {
		writeErrorResponse(w, nil, "invalid_state", "Invitation is not pending", http.StatusBadRequest)
		return
	}

	if strings.Contains(err.Error(), "different email") {
		writeErrorResponse(
			w, nil, "forbidden",
			"This invitation was sent to a different email address",
			http.StatusForbidden,
		)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to accept invitation", http.StatusInternalServerError)
}

// handleAcceptInvitation accepts a team invitation
// POST /api/v1/invitations/{token}/accept
func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	token := chi.URLParam(r, "token")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleAcceptInvitation",
		"user_id": userID,
		"token":   token,
	}).Info("Accept invitation request received")

	teamID, err := s.container.TeamInvitationService().AcceptInvitation(r.Context(), token, userID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleAcceptInvitation",
			"user_id": userID,
			"token":   token,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to accept invitation")

		s.handleAcceptInvitationError(w, err)
		return
	}

	team, err := s.container.TeamService().GetTeam(r.Context(), userID, teamID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleAcceptInvitation",
			"user_id": userID,
			"team_id": teamID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to fetch team details after accepting invitation")
		writeErrorResponse(
			w, nil, "internal_error",
			"Invitation accepted but failed to fetch team details",
			http.StatusInternalServerError,
		)
		return
	}

	response := models.AcceptInvitationResponse{
		TeamID:   team.ID,
		TeamName: team.Name,
		Message:  fmt.Sprintf("Successfully joined team %s", team.Name),
	}

	writeOK(w, response, s.logger)
}

// handleRejectInvitation rejects a team invitation
// POST /api/v1/invitations/{token}/reject
func (s *Server) handleRejectInvitation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	token := chi.URLParam(r, "token")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleRejectInvitation",
		"user_id": userID,
		"token":   token,
	}).Info("Reject invitation request received")

	err := s.container.TeamInvitationService().RejectInvitation(r.Context(), token, userID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleRejectInvitation",
			"user_id": userID,
			"token":   token,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to reject invitation")

		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "not found") {
			writeErrorResponse(w, nil, "not_found", "Invalid invitation token", http.StatusNotFound)
			return
		}

		if strings.Contains(err.Error(), "not pending") {
			writeErrorResponse(w, nil, "invalid_state", "Invitation is not pending", http.StatusBadRequest)
			return
		}

		if strings.Contains(err.Error(), "not authorized") {
			writeErrorResponse(w, nil, "forbidden", "You are not authorized to reject this invitation", http.StatusForbidden)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to reject invitation", http.StatusInternalServerError)
		return
	}

	writeNoContent(w)
}

package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// githubMsgAppNotInstalled is the error message returned when a team has no
// GitHub App installation.
const githubMsgAppNotInstalled = "GitHub App not installed for this team"

// githubStateMACDomain domain-separates the install-state HMAC key from the
// webhook secret it is derived from, so the two signing purposes never share a
// key even though they share a configured secret (#463). Mirrors the
// DeriveStateMACKey / stateMACDomain pattern in internal/auth/session.
const githubStateMACDomain = "vx-github-install-state-mac-v1"

// githubStateTTL bounds how long a minted install state stays usable.
const githubStateTTL = time.Hour

// githubStateMACKey derives the install-state signing key from the configured
// webhook secret. The webhook secret itself is never used as the HMAC key.
func (s *Server) githubStateMACKey() []byte {
	mac := hmac.New(sha256.New, []byte(s.config.GitHub.WebhookSecret))
	mac.Write([]byte(githubStateMACDomain))
	return mac.Sum(nil)
}

// signGitHubState creates an HMAC-signed state parameter for CSRF protection.
// Format: teamID:installationID:timestamp:signature
//
// installationID is 0 at mint time, because the install URL is generated before
// GitHub has created the installation. A non-zero value binds the state to one
// installation and is rejected on mismatch; the actual authority guarantee is
// the user-token check in the service (#463), this is defence in depth.
func (s *Server) signGitHubState(teamID string, installationID int64) string {
	timestamp := time.Now().Unix()
	message := fmt.Sprintf("%s:%d:%d", teamID, installationID, timestamp)

	mac := hmac.New(sha256.New, s.githubStateMACKey())
	mac.Write([]byte(message))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("%s:%d:%d:%s", teamID, installationID, timestamp, signature)
}

// verifyGitHubState validates the HMAC-signed state parameter and extracts the
// team ID and the installation id it is bound to (0 = unbound).
// Returns (teamID, installationID, valid).
func (s *Server) verifyGitHubState(state string) (string, int64, bool) {
	parts := strings.Split(state, ":")
	if len(parts) != 4 {
		return "", 0, false
	}

	teamID := parts[0]
	installationIDStr := parts[1]
	timestampStr := parts[2]
	providedSignature := parts[3]

	installationID, err := strconv.ParseInt(installationIDStr, 10, 64)
	if err != nil {
		return "", 0, false
	}

	// Parse timestamp
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return "", 0, false
	}

	// Check if state is not expired (valid for 1 hour)
	if time.Since(time.Unix(timestamp, 0)) > githubStateTTL {
		return "", 0, false
	}

	// Verify signature
	message := fmt.Sprintf("%s:%d:%d", teamID, installationID, timestamp)
	mac := hmac.New(sha256.New, s.githubStateMACKey())
	mac.Write([]byte(message))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedSignature), []byte(providedSignature)) {
		return "", 0, false
	}

	return teamID, installationID, true
}

// handleGitHubStatus returns the GitHub App installation status for a team
func (s *Server) handleGitHubStatus(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r)
	teamID := chi.URLParam(r, "team_id")

	status, err := s.container.GitHubAppService().GetInstallationStatus(r.Context(), teamID)
	if err != nil {
		s.logger.Error("Failed to get GitHub installation status", "error", err)
		writeErrorResponse(w, r, "internal_error", "Failed to get installation status", http.StatusInternalServerError)
		return
	}

	s.logger.With("user_id", userID).With("team_id", teamID).Info("GitHub installation status retrieved")

	writeOK(w, status, s.logger)
}

// handleGitHubInstallURL returns the GitHub App installation URL
func (s *Server) handleGitHubInstallURL(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "team_id")

	// Generate HMAC-signed state to prevent CSRF. The installation does not
	// exist yet, so the state is minted unbound (installation id 0).
	state := s.signGitHubState(teamID, 0)

	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s",
		s.config.GitHub.AppSlug, url.QueryEscape(state))

	response := map[string]string{
		"install_url": installURL,
	}

	writeOK(w, response, s.logger)
}

// handleGitHubCallback processes the GitHub App installation callback
func (s *Server) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r)
	teamID := chi.URLParam(r, "team_id")

	// Limit request body size to prevent denial of service
	const MaxBodyBytes = int64(65536) // 64KB limit for callback payloads
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	var req models.GitHubInstallCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, "invalid_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateGitHubCallbackRequest(w, r, &req, teamID) {
		return
	}

	reconnected, err := s.container.GitHubAppService().HandleInstallationCallback(
		r.Context(), userID, teamID, req.InstallationID, req.Code,
	)
	if err != nil {
		s.handleGitHubCallbackError(w, r, err)
		return
	}

	writeCreated(w, map[string]interface{}{
		"reconnected": reconnected,
	}, s.logger)
}

// validateGitHubCallbackRequest checks the callback's required fields and the
// signed state. It writes the error response and returns false on rejection.
//
// None of this establishes who the caller is on GitHub — that is the service's
// user-token check (#463). These are the cheap, local preconditions.
func (s *Server) validateGitHubCallbackRequest(
	w http.ResponseWriter, r *http.Request,
	req *models.GitHubInstallCallbackRequest, teamID string,
) bool {
	if req.InstallationID == 0 {
		writeErrorResponse(w, r, "invalid_request", "installation_id is required", http.StatusBadRequest)
		return false
	}

	// Verify HMAC-signed state parameter for CSRF protection
	if req.State == "" {
		writeErrorResponse(w, r, "invalid_request", "state parameter is required", http.StatusBadRequest)
		return false
	}

	// The authorization code proves who is calling; without it the caller's
	// authority over the installation cannot be established.
	if req.Code == "" {
		writeErrorResponse(w, r, "invalid_request", "code is required", http.StatusBadRequest)
		return false
	}

	stateTeamID, stateInstallationID, valid := s.verifyGitHubState(req.State)
	if !valid {
		s.logger.With("state", req.State).Warn("Invalid or expired state parameter")
		writeErrorResponse(w, r, "invalid_request", "Invalid or expired state parameter", http.StatusBadRequest)
		return false
	}

	// Verify state matches the team ID from URL
	if stateTeamID != teamID {
		s.logger.With(
			"state_team_id", stateTeamID,
			"url_team_id", teamID,
		).Warn("State team ID mismatch")
		writeErrorResponse(w, r, "forbidden", "State parameter does not match team", http.StatusForbidden)
		return false
	}

	// A state bound to an installation may only be replayed against that one.
	if stateInstallationID != 0 && stateInstallationID != req.InstallationID {
		s.logger.With(
			"state_installation_id", stateInstallationID,
			"request_installation_id", req.InstallationID,
		).Warn("State installation ID mismatch")
		writeErrorResponse(w, r, "invalid_request",
			"State parameter does not match installation", http.StatusBadRequest)
		return false
	}

	return true
}

// handleGitHubCallbackError maps installation-callback service errors to responses.
func (s *Server) handleGitHubCallbackError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrInstallationAlreadyConnected):
		const conflictMsg = "This GitHub organization is already connected to another team." +
			" Each GitHub org/account can only be connected to one team."
		writeErrorResponse(w, r, "installation_already_connected", conflictMsg, http.StatusConflict)
	case errors.Is(err, services.ErrInstallationNotAuthorized):
		writeErrorResponse(w, r, "installation_not_authorized",
			"You are not authorized to connect this GitHub installation.", http.StatusForbidden)
	case errors.Is(err, services.ErrPermissionDenied):
		writeErrorResponse(w, r, "forbidden",
			"You do not have permission to manage this team's GitHub integration.", http.StatusForbidden)
	case errors.Is(err, services.ErrGitHubUserAuthUnavailable):
		s.logger.Error("GitHub App user authorization is not configured; install callback rejected")
		writeErrorResponse(w, r, "github_user_auth_not_configured",
			"GitHub App user authorization is not configured on this instance.",
			http.StatusServiceUnavailable)
	default:
		s.logger.Error("Failed to handle installation callback", "error", err)
		writeErrorResponse(w, r, "internal_error", "Failed to complete installation", http.StatusInternalServerError)
	}
}

// handleGitHubRepositories lists repositories accessible by the installation
func (s *Server) handleGitHubRepositories(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r)
	teamID := chi.URLParam(r, "team_id")

	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	repos, err := s.container.GitHubAppService().GetRepositories(r.Context(), teamID, userID, page)
	if err != nil {
		if errors.Is(err, external.ErrGitHubInstallationGone) ||
			errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
			writeErrorResponse(w, r, "github_not_installed", githubMsgAppNotInstalled, http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to get GitHub repositories", "error", err)
		writeErrorResponse(w, r, "internal_error", "Failed to get repositories", http.StatusInternalServerError)
		return
	}

	writeOK(w, repos, s.logger)
}

// handleGitHubDisconnect disconnects the GitHub App installation
func (s *Server) handleGitHubDisconnect(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r)
	teamID := chi.URLParam(r, "team_id")

	if err := s.container.GitHubAppService().DisconnectInstallation(r.Context(), userID, teamID); err != nil {
		if errors.Is(err, services.ErrPermissionDenied) {
			writeErrorResponse(w, r, "forbidden",
				"You do not have permission to manage this team's GitHub integration.", http.StatusForbidden)
			return
		}
		s.logger.Error("Failed to disconnect GitHub installation", "error", err)
		writeErrorResponse(w, r, "internal_error", "Failed to disconnect installation", http.StatusInternalServerError)
		return
	}

	writeNoContent(w)
}

// handleGitHubImportProject imports a GitHub repository as a VibeXP project
func (s *Server) handleGitHubImportProject(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r)
	teamID := chi.URLParam(r, "team_id")
	repoIDStr := chi.URLParam(r, "repo_id")

	repoID, err := strconv.ParseInt(repoIDStr, 10, 64)
	if err != nil {
		writeErrorResponse(w, r, "invalid_request", "Invalid repository ID", http.StatusBadRequest)
		return
	}

	project, created, err := s.container.GitHubAppService().ImportProjectFromRepository(
		r.Context(), userID, teamID, repoID,
	)
	if err != nil {
		s.logger.Error("Failed to import project from repository", "error", err)
		s.handleImportProjectError(w, r, err)
		return
	}

	if created {
		s.recordGitHubImportActivity(r.Context(), resourceActivityParams{
			userID:       userID,
			activityType: activities.ActivityTypeGitHubProjectImported,
			entityType:   activities.EntityTypeProject,
			entityID:     &project.ID,
			description:  fmt.Sprintf("Imported project from GitHub repository %s", project.Name),
			metadata: map[string]interface{}{
				"repo_id":      repoID,
				"repo_name":    project.Name,
				"repo_git_url": project.GitURL,
				"team_id":      teamID,
			},
		}, r)
	}

	s.writeImportProjectResponse(w, project, created, userID, teamID, repoID)
}

// handleImportProjectError handles errors from the import project service call
func (s *Server) handleImportProjectError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
		writeErrorResponse(w, r, "github_not_installed", githubMsgAppNotInstalled, http.StatusNotFound)
		return
	}
	if errors.Is(err, repositories.ErrGitHubRepositoryNotFound) {
		writeErrorResponse(w, r, "repository_not_found", "Repository not found or not accessible", http.StatusNotFound)
		return
	}
	writeErrorResponse(w, r, "internal_error", "Failed to import project", http.StatusInternalServerError)
}

// writeImportProjectResponse writes the successful import project response
func (s *Server) writeImportProjectResponse(
	w http.ResponseWriter,
	project *models.Project, created bool,
	userID, teamID string, repoID int64,
) {
	response := map[string]interface{}{
		"project": project,
		"created": created,
	}
	if !created {
		response["message"] = "Project already exists for this repository"
	}

	statusCode := http.StatusOK
	if created {
		statusCode = http.StatusCreated
	}

	writeJSON(w, statusCode, response, s.logger)

	s.logger.With(
		"user_id", userID,
		"team_id", teamID,
		"repo_id", repoID,
		"project_id", project.ID,
		"created", created,
	).Info("GitHub repository import completed")
}

// handleGitHubImportBlueprints imports AI assistant configurations from a repository as blueprints
func (s *Server) handleGitHubImportBlueprints(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r)
	teamID := chi.URLParam(r, "team_id")

	// Limit request body size to prevent denial of service
	const MaxBodyBytes = int64(65536) // 64KB limit for import payloads
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	var req models.BlueprintImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, "invalid_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.RepositoryID == 0 {
		writeErrorResponse(w, r, "invalid_request", "repository_id is required", http.StatusBadRequest)
		return
	}

	// Project is automatically discovered by matching repository URL
	report, err := s.container.GitHubAppService().ImportBlueprintsFromRepository(
		r.Context(), userID, teamID, req.RepositoryID,
	)
	if err != nil {
		s.logger.Error("Failed to import blueprints from repository", "error", err)
		s.handleImportBlueprintsError(w, r, err)
		return
	}

	if report.TotalSuccessful > 0 {
		s.recordGitHubImportActivity(r.Context(), resourceActivityParams{
			userID:       userID,
			activityType: activities.ActivityTypeGitHubBlueprintsImported,
			entityType:   activities.EntityTypeBlueprint,
			// entityID stays nil: the import has no single blueprint entity.
			description: fmt.Sprintf(
				"Imported %d blueprints from GitHub repository (id: %d)", report.TotalSuccessful, req.RepositoryID,
			),
			metadata: map[string]interface{}{
				"repo_id":        req.RepositoryID,
				"team_id":        teamID,
				"total_scanned":  report.TotalScanned,
				"total_imported": report.TotalSuccessful,
				"total_skipped":  report.TotalSkipped,
				"total_failed":   report.TotalFailed,
			},
		}, r)
	}

	writeOK(w, report, s.logger)

	s.logger.With(
		"user_id", userID,
		"team_id", teamID,
		"repo_id", req.RepositoryID,
		"total_scanned", report.TotalScanned,
		"total_successful", report.TotalSuccessful,
		"total_failed", report.TotalFailed,
		"total_skipped", report.TotalSkipped,
	).Info("GitHub blueprints import completed")
}

// handleImportBlueprintsError handles errors from the import blueprints service call
func (s *Server) handleImportBlueprintsError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
		writeErrorResponse(w, r, "github_not_installed", githubMsgAppNotInstalled, http.StatusNotFound)
		return
	}
	if errors.Is(err, repositories.ErrGitHubRepositoryNotFound) {
		writeErrorResponse(w, r, "repository_not_found", "Repository not found or not accessible", http.StatusNotFound)
		return
	}
	if errors.Is(err, repositories.ErrProjectNotFoundForRepo) {
		writeErrorResponse(w, r, "project_not_found",
			"Project not found for this repository. Please import this repository as a project first "+
				"using the 'Import as Project' button, then try importing blueprints again.",
			http.StatusPreconditionFailed)
		return
	}
	writeErrorResponse(w, r, "internal_error", "Failed to import blueprints", http.StatusInternalServerError)
}

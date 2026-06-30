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

// signGitHubState creates an HMAC-signed state parameter for CSRF protection
// Format: teamID:timestamp:signature
func (s *Server) signGitHubState(teamID string) string {
	timestamp := time.Now().Unix()
	message := fmt.Sprintf("%s:%d", teamID, timestamp)

	mac := hmac.New(sha256.New, []byte(s.config.GitHub.WebhookSecret))
	mac.Write([]byte(message))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("%s:%d:%s", teamID, timestamp, signature)
}

// verifyGitHubState validates the HMAC-signed state parameter and extracts the team ID
// Returns (teamID, valid)
func (s *Server) verifyGitHubState(state string) (string, bool) {
	parts := strings.Split(state, ":")
	if len(parts) != 3 {
		return "", false
	}

	teamID := parts[0]
	timestampStr := parts[1]
	providedSignature := parts[2]

	// Parse timestamp
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return "", false
	}

	// Check if state is not expired (valid for 1 hour)
	if time.Since(time.Unix(timestamp, 0)) > time.Hour {
		return "", false
	}

	// Verify signature
	message := fmt.Sprintf("%s:%d", teamID, timestamp)
	mac := hmac.New(sha256.New, []byte(s.config.GitHub.WebhookSecret))
	mac.Write([]byte(message))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedSignature), []byte(providedSignature)) {
		return "", false
	}

	return teamID, true
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

	// Generate HMAC-signed state to prevent CSRF
	state := s.signGitHubState(teamID)

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

	if req.InstallationID == 0 {
		writeErrorResponse(w, r, "invalid_request", "installation_id is required", http.StatusBadRequest)
		return
	}

	// Verify HMAC-signed state parameter for CSRF protection
	if req.State == "" {
		writeErrorResponse(w, r, "invalid_request", "state parameter is required", http.StatusBadRequest)
		return
	}

	stateTeamID, valid := s.verifyGitHubState(req.State)
	if !valid {
		s.logger.With("state", req.State).Warn("Invalid or expired state parameter")
		writeErrorResponse(w, r, "invalid_request", "Invalid or expired state parameter", http.StatusBadRequest)
		return
	}

	// Verify state matches the team ID from URL
	if stateTeamID != teamID {
		s.logger.With(
			"state_team_id", stateTeamID,
			"url_team_id", teamID,
		).Warn("State team ID mismatch")
		writeErrorResponse(w, r, "forbidden", "State parameter does not match team", http.StatusForbidden)
		return
	}

	reconnected, err := s.container.GitHubAppService().HandleInstallationCallback(
		r.Context(), userID, teamID, req.InstallationID,
	)
	if err != nil {
		if errors.Is(err, services.ErrInstallationAlreadyConnected) {
			const conflictMsg = "This GitHub organization is already connected to another team." +
				" Each GitHub org/account can only be connected to one team."
			writeErrorResponse(w, r, "installation_already_connected", conflictMsg, http.StatusConflict)
			return
		}
		s.logger.Error("Failed to handle installation callback", "error", err)
		writeErrorResponse(w, r, "internal_error", "Failed to complete installation", http.StatusInternalServerError)
		return
	}

	writeCreated(w, map[string]interface{}{
		"reconnected": reconnected,
	}, s.logger)
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
			writeErrorResponse(w, r, "github_not_installed", "GitHub App not installed for this team", http.StatusNotFound)
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
		s.recordGitHubImportActivity(
			r.Context(), userID,
			activities.ActivityTypeGitHubProjectImported,
			activities.EntityTypeProject,
			project.ID,
			fmt.Sprintf("Imported project from GitHub repository %s", project.Name),
			map[string]interface{}{
				"repo_id":      repoID,
				"repo_name":    project.Name,
				"repo_git_url": project.GitURL,
				"team_id":      teamID,
			},
			r,
		)
	}

	s.writeImportProjectResponse(w, project, created, userID, teamID, repoID)
}

// handleImportProjectError handles errors from the import project service call
func (s *Server) handleImportProjectError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
		writeErrorResponse(w, r, "github_not_installed", "GitHub App not installed for this team", http.StatusNotFound)
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
		s.recordGitHubImportActivity(
			r.Context(), userID,
			activities.ActivityTypeGitHubBlueprintsImported,
			activities.EntityTypeBlueprint,
			"",
			fmt.Sprintf("Imported %d blueprints from GitHub repository (id: %d)", report.TotalSuccessful, req.RepositoryID),
			map[string]interface{}{
				"repo_id":        req.RepositoryID,
				"team_id":        teamID,
				"total_scanned":  report.TotalScanned,
				"total_imported": report.TotalSuccessful,
				"total_skipped":  report.TotalSkipped,
				"total_failed":   report.TotalFailed,
			},
			r,
		)
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
		writeErrorResponse(w, r, "github_not_installed", "GitHub App not installed for this team", http.StatusNotFound)
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

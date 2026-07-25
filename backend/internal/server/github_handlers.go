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

	"github.com/vibexp/vibexp/internal/authz"
	apierrors "github.com/vibexp/vibexp/internal/errors"
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
// instance encryption key it is derived from, so that key never signs anything
// under its raw form. Mirrors the DeriveStateMACKey / stateMACDomain pattern in
// internal/auth/session.
//
// The "-v2" is not decoration: #482 moved the key material off the (now
// deleted) instance-wide GitHub webhook secret onto security.encryption_key.
// Changing the domain string with the key material makes states minted under
// the old scheme cleanly unverifiable rather than merely improbable.
const githubStateMACDomain = "vx-github-install-state-mac-v2"

// githubStateTTL bounds how long a minted install state stays usable.
const githubStateTTL = time.Hour

// githubStateParts is the number of colon-separated fields in a state:
// teamID:appConfigID:installationID:timestamp:signature.
const githubStateParts = 5

// githubStateMACKey derives the install-state signing key from the instance
// encryption key. That key is mandatory and startup-validated (see
// config.validateEncryptionKey), and the install state is instance-scoped CSRF
// protection — tying it to any one team's App secret would be a category error.
// The key itself is never used raw as the HMAC key.
//
// Rotating security.encryption_key invalidates in-flight states. That is
// bounded by githubStateTTL and the flow is interactive, so the worst case is
// an admin restarting an install — not a bug to hunt later.
func (s *Server) githubStateMACKey() []byte {
	mac := hmac.New(sha256.New, []byte(s.config.Security.EncryptionKey))
	mac.Write([]byte(githubStateMACDomain))
	return mac.Sum(nil)
}

// githubStateMessage is the signed portion of a state, shared by the signer and
// the verifier so the two layouts cannot drift apart.
func githubStateMessage(teamID, appConfigID string, installationID, timestamp int64) string {
	return fmt.Sprintf("%s:%s:%d:%d", teamID, appConfigID, installationID, timestamp)
}

// signGitHubState creates an HMAC-signed state parameter for CSRF protection.
// Format: teamID:appConfigID:installationID:timestamp:signature
//
// appConfigID binds the state to the App config it was minted for, so a state
// cannot be redeemed after the team replaces or rotates its App (#482).
//
// installationID is 0 at mint time, because the install URL is generated before
// GitHub has created the installation. A non-zero value binds the state to one
// installation and is rejected on mismatch; the actual authority guarantee is
// the user-token check in the service (#463), this is defence in depth.
func (s *Server) signGitHubState(teamID, appConfigID string, installationID int64) string {
	timestamp := time.Now().Unix()
	message := githubStateMessage(teamID, appConfigID, installationID, timestamp)

	mac := hmac.New(sha256.New, s.githubStateMACKey())
	mac.Write([]byte(message))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	return message + ":" + signature
}

// githubState is what a verified state carries: the team it was minted for, the
// App config it is bound to, and the installation it is bound to (0 = unbound).
type githubState struct {
	teamID         string
	appConfigID    string
	installationID int64
}

// verifyGitHubState validates the HMAC-signed state parameter and extracts its
// fields. The boolean is the only success signal — on false the struct is zero.
func (s *Server) verifyGitHubState(state string) (githubState, bool) {
	parts := strings.Split(state, ":")
	if len(parts) != githubStateParts {
		return githubState{}, false
	}

	teamID := parts[0]
	appConfigID := parts[1]
	installationIDStr := parts[2]
	timestampStr := parts[3]
	providedSignature := parts[4]

	installationID, err := strconv.ParseInt(installationIDStr, 10, 64)
	if err != nil {
		return githubState{}, false
	}

	// Parse timestamp
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return githubState{}, false
	}

	// Check if state is not expired (valid for 1 hour)
	if time.Since(time.Unix(timestamp, 0)) > githubStateTTL {
		return githubState{}, false
	}

	// Verify signature
	message := githubStateMessage(teamID, appConfigID, installationID, timestamp)
	mac := hmac.New(sha256.New, s.githubStateMACKey())
	mac.Write([]byte(message))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedSignature), []byte(providedSignature)) {
		return githubState{}, false
	}

	return githubState{teamID: teamID, appConfigID: appConfigID, installationID: installationID}, true
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
	userID := s.getUserIDFromContext(r)
	teamID := chi.URLParam(r, "team_id")

	// Gated on the same permission as the callback: only someone who could
	// actually complete the connect may start it. Without this a member can
	// install the App on a GitHub org and then be refused at the callback,
	// leaving the org with the App installed and no team connected (#463).
	if authzErr := s.container.AuthorizationService().Can(
		r.Context(), userID, teamID, authz.TeamUpdate,
	); authzErr != nil {
		writeErrorResponse(w, r, "forbidden",
			"You do not have permission to manage this team's GitHub integration.", http.StatusForbidden)
		return
	}

	// The install link must point at the team's OWN App: with per-team Apps an
	// instance-wide slug would send every team to an App they do not own (#482).
	appConfig, err := s.container.GitHubAppConfigService().GetAppConfig(r.Context(), teamID)
	if err != nil {
		s.writeGitHubInstallAppConfigError(w, r, "handleGitHubInstallURL", teamID, err)
		return
	}

	// Generate HMAC-signed state to prevent CSRF. The installation does not
	// exist yet, so the state is minted unbound (installation id 0), but it is
	// bound to the App config so it cannot survive that App being replaced.
	state := s.signGitHubState(teamID, appConfig.ID, 0)

	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s",
		appConfig.AppSlug, url.QueryEscape(state))

	response := map[string]string{
		"install_url": installURL,
	}

	writeOK(w, response, s.logger)
}

// writeGitHubInstallAppConfigError maps a failed App-config lookup in the
// install flow. A team with no App gets 409 github_app_not_configured — never a
// 500, and never a URL pointing at an App that does not exist.
func (s *Server) writeGitHubInstallAppConfigError(
	w http.ResponseWriter, r *http.Request, handler, teamID string, err error,
) {
	if errors.Is(err, services.ErrGitHubAppNotConfigured) {
		apierrors.WriteJSONError(w, r, apierrors.NewGitHubAppNotConfiguredError())
		return
	}

	s.logger.With("handler", handler, "team_id", teamID).
		Error("Failed to load GitHub App config", "error", err)
	writeErrorResponse(w, r, "internal_error", "Failed to load GitHub App configuration",
		http.StatusInternalServerError)
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

	state, valid := s.verifyGitHubState(req.State)
	if !valid {
		s.logger.With("state", req.State).Warn("Invalid or expired state parameter")
		writeErrorResponse(w, r, "invalid_request", "Invalid or expired state parameter", http.StatusBadRequest)
		return false
	}

	// Verify state matches the team ID from URL
	if state.teamID != teamID {
		s.logger.With(
			"state_team_id", state.teamID,
			"url_team_id", teamID,
		).Warn("State team ID mismatch")
		writeErrorResponse(w, r, "forbidden", "State parameter does not match team", http.StatusForbidden)
		return false
	}

	// A state bound to an installation may only be replayed against that one.
	if state.installationID != 0 && state.installationID != req.InstallationID {
		s.logger.With(
			"state_installation_id", state.installationID,
			"request_installation_id", req.InstallationID,
		).Warn("State installation ID mismatch")
		writeErrorResponse(w, r, "invalid_request",
			"State parameter does not match installation", http.StatusBadRequest)
		return false
	}

	return s.validateGitHubCallbackAppConfig(w, r, teamID, state.appConfigID)
}

// validateGitHubCallbackAppConfig rejects a state that was minted for a
// different App config than the team currently has.
//
// The signature already proves nobody forged the id, so this is a freshness
// check rather than an authenticity one: replacing or rotating a team's App
// must retire the install states minted against the previous one, instead of
// letting a stale state complete an install against credentials that have since
// changed hands (#482).
//
// It is the last check because it is the only one that touches the database —
// everything cheap and local has already had its chance to reject.
func (s *Server) validateGitHubCallbackAppConfig(
	w http.ResponseWriter, r *http.Request, teamID, stateAppConfigID string,
) bool {
	appConfig, err := s.container.GitHubAppConfigService().GetAppConfig(r.Context(), teamID)
	if err != nil {
		s.writeGitHubInstallAppConfigError(w, r, "handleGitHubCallback", teamID, err)
		return false
	}

	if stateAppConfigID != appConfig.ID {
		s.logger.With(
			"state_app_config_id", stateAppConfigID,
			"team_app_config_id", appConfig.ID,
		).Warn("State App config mismatch")
		writeErrorResponse(w, r, "invalid_request",
			"State parameter does not match the team's GitHub App", http.StatusBadRequest)
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

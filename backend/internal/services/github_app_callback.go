package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// ErrInstallationNotAuthorized is returned when the caller cannot be shown to
// have access to the GitHub App installation they are trying to bind to their
// team.
var ErrInstallationNotAuthorized = errors.New("you are not authorized to connect this GitHub installation")

// ErrGitHubUserAuthUnavailable is returned when the GitHub App's OAuth
// credentials are not configured, so caller authority cannot be established.
// The callback fails closed rather than falling back to the app-JWT-only path.
var ErrGitHubUserAuthUnavailable = errors.New("github app user authorization is not configured")

// HandleInstallationCallback processes the installation callback from GitHub.
// Returns (reconnected, error) where reconnected=true means the same team reconnected an existing installation.
func (s *GitHubAppService) HandleInstallationCallback(
	ctx context.Context,
	userID, teamID string,
	installationID int64,
	code string,
) (bool, error) {
	// Connecting a GitHub org grants the whole team read access to its private
	// source — an owner/admin decision, not a member one (#463).
	if authzErr := s.authz.Can(ctx, userID, teamID, authz.TeamUpdate); authzErr != nil {
		return false, authzErr
	}

	// Establish that the caller is an insider of this installation BEFORE
	// touching it. The signed state only proves a VibeXP user started an install
	// flow for a team they can access — it says nothing about who owns
	// installationID, and GetInstallation below authenticates as the App, so it
	// resolves every installation regardless of who is asking. The check lives
	// here rather than in the handler so no future caller can reach the store
	// path without it (#463).
	if err := s.verifyCallerCanAccessInstallation(ctx, code, installationID); err != nil {
		return false, err
	}

	// Get installation details from GitHub
	installationInfo, err := s.githubClient.GetInstallation(ctx, installationID)
	if err != nil {
		return false, fmt.Errorf("failed to get installation info: %w", err)
	}

	// Cross-team conflict detection: check if this installationID is already connected to a different team
	reconnected, err := s.detectInstallationReconnection(ctx, installationID, teamID)
	if err != nil {
		return false, err
	}

	// NOTE: We don't fetch/store tokens here because ghinstallation library
	// handles token generation and management automatically when making API calls.
	// Store placeholder values to satisfy database schema (can be migrated away later).
	placeholderToken := fmt.Sprintf("ghinstallation-managed-%d", installationID)
	encryptedToken, err := s.encryptionSvc.Encrypt(placeholderToken)
	if err != nil {
		return false, fmt.Errorf("failed to encrypt placeholder token: %w", err)
	}

	// Set expiry far in the future since ghinstallation manages actual tokens
	expiresAt := time.Now().Add(365 * 24 * time.Hour)

	installation := newInstallationRecord(teamID, installationID, installationInfo, encryptedToken, expiresAt)

	updated, err := s.upsertInstallation(ctx, teamID, installation)
	if err != nil {
		return false, err
	}
	if updated {
		reconnected = true
	}

	// NOTE: Token caching removed since ghinstallation manages tokens automatically

	// Emit event
	payload := map[string]interface{}{
		"team_id":         teamID,
		"installation_id": installationID,
		"account_login":   installationInfo.AccountLogin,
	}
	event := events.NewBaseEvent("github.installation.created", payload, userID)
	if err := s.eventManager.Publish(ctx, event); err != nil {
		s.logger.Warn("Failed to publish installation created event", "error", err)
	}

	return reconnected, nil
}

// verifyCallerCanAccessInstallation exchanges GitHub's post-install code for a
// user access token and requires installationID to be one that user can reach.
// Any failure is a denial: an unusable code, an installation the user has no
// access to, and absent OAuth credentials all stop the bind.
func (s *GitHubAppService) verifyCallerCanAccessInstallation(
	ctx context.Context, code string, installationID int64,
) error {
	if code == "" {
		return ErrInstallationNotAuthorized
	}

	userToken, err := s.githubClient.ExchangeUserCode(ctx, code)
	if err != nil {
		if errors.Is(err, external.ErrGitHubUserAuthNotConfigured) {
			return ErrGitHubUserAuthUnavailable
		}
		if errors.Is(err, external.ErrGitHubUserCodeInvalid) {
			return ErrInstallationNotAuthorized
		}
		return fmt.Errorf("failed to exchange installation authorization code: %w", err)
	}

	accessible, err := s.githubClient.UserCanAccessInstallation(ctx, userToken, installationID)
	if err != nil {
		return fmt.Errorf("failed to verify installation access: %w", err)
	}
	if !accessible {
		s.logger.With("installation_id", installationID).
			Warn("Rejected install callback for an installation the caller cannot access")
		return ErrInstallationNotAuthorized
	}

	return nil
}

// detectInstallationReconnection checks whether installationID is already connected.
// It returns true when the same team is reconnecting an existing installation, and
// ErrInstallationAlreadyConnected when the installation belongs to a different team.
func (s *GitHubAppService) detectInstallationReconnection(
	ctx context.Context, installationID int64, teamID string,
) (bool, error) {
	existingByInstallID, err := s.installationRepo.GetByInstallationID(ctx, installationID)
	if err != nil && !errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
		return false, fmt.Errorf("failed to check installation by installation ID: %w", err)
	}
	if existingByInstallID == nil {
		return false, nil
	}
	if existingByInstallID.TeamID != teamID {
		// This GitHub org/account is already connected to a different team
		return false, ErrInstallationAlreadyConnected
	}
	// Same team reconnecting an existing installation
	return true, nil
}

// upsertInstallation stores the installation for the team, updating the existing
// record when one exists (returning updated=true) and creating it otherwise.
func (s *GitHubAppService) upsertInstallation(
	ctx context.Context, teamID string, installation *models.GitHubInstallation,
) (bool, error) {
	// Check if installation already exists for this team (upsert logic)
	existing, err := s.installationRepo.GetByTeamID(ctx, teamID)
	if err != nil && !errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
		return false, fmt.Errorf("failed to check existing installation: %w", err)
	}

	if existing != nil {
		// Update existing installation
		installation.ID = existing.ID
		if err := s.installationRepo.Update(ctx, installation); err != nil {
			return false, fmt.Errorf("failed to update installation: %w", err)
		}
		return true, nil
	}

	// Create new installation — also handle race condition: pq unique constraint on installation_id
	if err := s.installationRepo.Create(ctx, installation); err != nil {
		if isUniqueViolationOnInstallationID(err) {
			// Race condition: another team connected this installation between our check and create
			return false, ErrInstallationAlreadyConnected
		}
		return false, fmt.Errorf("failed to create installation: %w", err)
	}
	return false, nil
}

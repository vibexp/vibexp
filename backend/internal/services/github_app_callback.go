package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// HandleInstallationCallback processes the installation callback from GitHub.
// Returns (reconnected, error) where reconnected=true means the same team reconnected an existing installation.
//
//nolint:funlen,gocognit,gocyclo // Multi-step: GitHub API fetch, conflict detection, encrypt, upsert, event emit
func (s *GitHubAppService) HandleInstallationCallback(
	ctx context.Context,
	userID, teamID string,
	installationID int64,
) (bool, error) {
	// Get installation details from GitHub
	installationInfo, err := s.githubClient.GetInstallation(ctx, installationID)
	if err != nil {
		return false, fmt.Errorf("failed to get installation info: %w", err)
	}

	// Cross-team conflict detection: check if this installationID is already connected to a different team
	reconnected := false
	existingByInstallID, err := s.installationRepo.GetByInstallationID(ctx, installationID)
	if err != nil && !errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
		return false, fmt.Errorf("failed to check installation by installation ID: %w", err)
	}
	if existingByInstallID != nil {
		if existingByInstallID.TeamID != teamID {
			// This GitHub org/account is already connected to a different team
			return false, ErrInstallationAlreadyConnected
		}
		// Same team reconnecting an existing installation
		reconnected = true
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
		reconnected = true
	} else {
		// Create new installation — also handle race condition: pq unique constraint on installation_id
		if err := s.installationRepo.Create(ctx, installation); err != nil {
			if isUniqueViolationOnInstallationID(err) {
				// Race condition: another team connected this installation between our check and create
				return false, ErrInstallationAlreadyConnected
			}
			return false, fmt.Errorf("failed to create installation: %w", err)
		}
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
		s.logger.WithError(err).Warn("Failed to publish installation created event")
	}

	return reconnected, nil
}

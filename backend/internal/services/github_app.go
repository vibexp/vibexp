package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// Compile-time assertion: GitHubAppService must implement GitHubAppServiceInterface.
var _ GitHubAppServiceInterface = (*GitHubAppService)(nil)

// ErrInstallationAlreadyConnected is returned when a GitHub installation is already connected to a different team
var ErrInstallationAlreadyConnected = errors.New("this GitHub organization is already connected to another team")

// GitHubAppService implements GitHubAppServiceInterface
type GitHubAppService struct {
	installationRepo repositories.GitHubInstallationRepository
	projectRepo      repositories.ProjectRepository
	blueprintRepo    repositories.BlueprintRepository
	githubClient     external.GitHubAppClient
	encryptionSvc    EncryptionServiceInterface
	eventManager     events.EventPublisher
	logger           *slog.Logger
}

// NewGitHubAppService creates a new GitHub App service
func NewGitHubAppService(
	installationRepo repositories.GitHubInstallationRepository,
	projectRepo repositories.ProjectRepository,
	blueprintRepo repositories.BlueprintRepository,
	githubClient external.GitHubAppClient,
	encryptionSvc EncryptionServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
) GitHubAppServiceInterface {
	return &GitHubAppService{
		installationRepo: installationRepo,
		projectRepo:      projectRepo,
		blueprintRepo:    blueprintRepo,
		githubClient:     githubClient,
		encryptionSvc:    encryptionSvc,
		eventManager:     eventManager,
		logger:           logger,
	}
}

// GetInstallationStatus retrieves the installation status for a team
func (s *GitHubAppService) GetInstallationStatus(
	ctx context.Context,
	teamID string,
) (*models.GitHubInstallationStatus, error) {
	installation, err := s.installationRepo.GetByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
			return &models.GitHubInstallationStatus{Installed: false}, nil
		}
		return nil, fmt.Errorf("failed to get installation: %w", err)
	}

	return &models.GitHubInstallationStatus{
		Installed:      true,
		AccountLogin:   installation.AccountLogin,
		InstallationID: installation.InstallationID,
		Suspended:      installation.SuspendedAt != nil,
		InstalledAt:    installation.CreatedAt,
	}, nil
}

// DisconnectInstallation removes the GitHub App installation for a team
func (s *GitHubAppService) DisconnectInstallation(ctx context.Context, userID, teamID string) error {
	installation, err := s.installationRepo.GetByTeamID(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to get installation: %w", err)
	}

	if err := s.installationRepo.Delete(ctx, teamID); err != nil {
		return fmt.Errorf("failed to delete installation: %w", err)
	}

	// Evict the cached GitHub client so stale credentials are not served
	// after the installation is removed.
	s.githubClient.EvictCachedClient(installation.InstallationID)

	// Emit event
	payload := map[string]interface{}{
		"team_id":         teamID,
		"installation_id": installation.InstallationID,
	}
	event := events.NewBaseEvent("github.installation.deleted", payload, userID)
	if err := s.eventManager.Publish(ctx, event); err != nil {
		s.logger.With("error", err).Warn("Failed to publish installation deleted event")
	}

	return nil
}

// removeGoneInstallation deletes an installation record after GitHub reported
// it no longer exists (token refresh 404), mirroring the installation.deleted
// webhook path. This reconciles installations whose deletion webhook was never
// processed, so the integration stops polling a dead installation. Failures are
// logged, not returned: the caller's GitHub API error is the primary outcome.
func (s *GitHubAppService) removeGoneInstallation(ctx context.Context, teamID string, installationID int64) {
	logger := s.logger.With(
		"team_id", teamID,
		"installation_id", installationID,
	)

	// A concurrent caller may have removed the record already — that is
	// success for our purposes, and the cache eviction below must still run.
	err := s.installationRepo.Delete(ctx, teamID)
	if err != nil && !errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
		logger.With("error", err).Error("Failed to remove gone GitHub installation")
		return
	}
	s.githubClient.EvictCachedClient(installationID)
	logger.Warn("Removed GitHub installation that no longer exists on GitHub; team must re-install the app")
}

// RefreshInstallationToken refreshes the installation access token
// NOTE: This is a no-op since ghinstallation library manages tokens automatically.
// Kept for interface compatibility but does not perform actual token refresh.
func (s *GitHubAppService) RefreshInstallationToken(ctx context.Context, teamID string) error {
	installation, err := s.installationRepo.GetByTeamID(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to get installation: %w", err)
	}

	// Store placeholder token to maintain database consistency
	placeholderToken := fmt.Sprintf("ghinstallation-managed-%d", installation.InstallationID)
	encryptedToken, err := s.encryptionSvc.Encrypt(placeholderToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt placeholder token: %w", err)
	}

	// Update installation with far-future expiry since ghinstallation manages actual tokens
	installation.EncryptedAccessToken = encryptedToken
	installation.TokenExpiresAt = time.Now().Add(365 * 24 * time.Hour)

	if err := s.installationRepo.Update(ctx, installation); err != nil {
		return fmt.Errorf("failed to update installation: %w", err)
	}

	s.logger.With("team_id", teamID).Debug("Token refresh requested but tokens are managed by ghinstallation library")

	return nil
}

// HandleWebhookEvent processes GitHub webhook events
func (s *GitHubAppService) HandleWebhookEvent(
	ctx context.Context,
	eventType string,
	installationID int64,
	action string,
) error {
	installation, err := s.installationRepo.GetByInstallationID(ctx, installationID)
	if err != nil {
		if errors.Is(err, repositories.ErrGitHubInstallationNotFound) {
			s.logger.With("installation_id", installationID).Warn("Installation not found for webhook event")
			return nil
		}
		return fmt.Errorf("failed to get installation: %w", err)
	}

	switch eventType {
	case "installation":
		return s.handleInstallationEvent(ctx, installation, action)
	case "installation_repositories":
		s.logger.With(
			"installation_id", installationID,
			"action", action,
		).Info("Installation repositories event received")
		return nil
	default:
		s.logger.With("event_type", eventType).Info("Unhandled webhook event type")
		return nil
	}
}

func (s *GitHubAppService) handleInstallationEvent(
	ctx context.Context,
	installation *models.GitHubInstallation,
	action string,
) error {
	switch action {
	case "deleted":
		if err := s.installationRepo.Delete(ctx, installation.TeamID); err != nil {
			return fmt.Errorf("failed to delete installation: %w", err)
		}

	case "suspend":
		now := time.Now()
		installation.SuspendedAt = &now
		if err := s.installationRepo.Update(ctx, installation); err != nil {
			return fmt.Errorf("failed to suspend installation: %w", err)
		}

	case "unsuspend":
		installation.SuspendedAt = nil
		if err := s.installationRepo.Update(ctx, installation); err != nil {
			return fmt.Errorf("failed to unsuspend installation: %w", err)
		}
	}

	return nil
}

// newInstallationRecord builds a GitHubInstallation model from GitHub API info.
// Shared by HandleInstallationCallback.
func newInstallationRecord(
	teamID string,
	installationID int64,
	installationInfo *external.GitHubInstallationInfo,
	encryptedToken string,
	expiresAt time.Time,
) *models.GitHubInstallation {
	permissions := make(map[string]interface{})
	for k, v := range installationInfo.Permissions {
		permissions[k] = v
	}

	return &models.GitHubInstallation{
		ID:                   uuid.New().String(),
		TeamID:               teamID,
		InstallationID:       installationID,
		AccountLogin:         installationInfo.AccountLogin,
		AccountType:          installationInfo.AccountType,
		TargetType:           installationInfo.TargetType,
		EncryptedAccessToken: encryptedToken,
		TokenExpiresAt:       expiresAt,
		Permissions:          permissions,
		Events:               installationInfo.Events,
		SuspendedAt:          installationInfo.SuspendedAt,
	}
}

// isUniqueViolationOnInstallationID returns true when the postgres error is a
// unique-constraint violation on the installation_id column.
func isUniqueViolationOnInstallationID(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) &&
		pqErr.Code == "23505" &&
		strings.Contains(pqErr.Constraint, "installation_id")
}

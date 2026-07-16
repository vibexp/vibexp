package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

type PromptShareService struct {
	shareRepo     repositories.PromptShareRepository
	promptRepo    repositories.PromptRepository
	promptService *PromptService
	logger        *slog.Logger
}

func NewPromptShareService(
	shareRepo repositories.PromptShareRepository,
	promptRepo repositories.PromptRepository,
	promptService *PromptService,
	logger *slog.Logger,
) *PromptShareService {
	return &PromptShareService{
		shareRepo:     shareRepo,
		promptRepo:    promptRepo,
		promptService: promptService,
		logger:        logger,
	}
}

// generateShareToken generates a cryptographically secure share token
func (s *PromptShareService) generateShareToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	// RawURLEncoding is unpadded by construction: 32 bytes (256 bits) encode to
	// exactly 43 URL-safe characters ([A-Za-z0-9_-]) with no '=' padding. That
	// matters because the token is a path parameter — chi routes on the encoded
	// RawPath, so any '=' a client percent-encodes to %3D would arrive still
	// encoded and miss the exact-match lookup (the #251 failure mode). The old
	// scheme padded (base64.URLEncoding) then truncated to 43 chars, which was
	// URL-safe only by the accident of chopping the '=' padding. Truncating the
	// padding loses no entropy (it is not data), and RawURLEncoding produces the
	// identical 43 characters, so already-issued tokens keep resolving — this
	// just makes the URL-safety intentional instead of incidental.
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, nil
}

// CreateShare creates or updates a share for a prompt
//
//nolint:gocognit,gocyclo,funlen // Complex business logic: create/update with access control
func (s *PromptShareService) CreateShare(
	userID, promptSlug string,
	req *models.CreateShareRequest,
) (*models.ShareResponse, error) {
	ctx := context.Background()

	// Verify prompt exists and user owns it across all user's teams
	prompt, err := s.promptRepo.GetBySlugCrossTeam(ctx, userID, promptSlug)
	if err != nil {
		return nil, fmt.Errorf("prompt not found")
	}

	// Validate restricted share has emails
	if req.ShareType == "restricted" && len(req.Emails) == 0 {
		return nil, fmt.Errorf("restricted shares must specify at least one email")
	}

	// Check if share already exists
	existingShare, err := s.shareRepo.GetByPromptID(ctx, prompt.ID)
	if err == nil {
		// Share exists, update it
		existingShare.ShareType = req.ShareType
		if updateErr := s.shareRepo.Update(ctx, existingShare); updateErr != nil {
			s.logger.With("error", updateErr).Error("Failed to update prompt share")
			return nil, fmt.Errorf("failed to update share: %w", updateErr)
		}

		// Update access emails if restricted
		if req.ShareType == "restricted" {
			if emailErr := s.shareRepo.AddAccessEmails(ctx, existingShare.ID, req.Emails); emailErr != nil {
				s.logger.With("error", emailErr).Error("Failed to update access emails")
				return nil, fmt.Errorf("failed to update access emails: %w", emailErr)
			}
		} else {
			// Clear access emails for public shares
			if clearErr := s.shareRepo.AddAccessEmails(ctx, existingShare.ID, []string{}); clearErr != nil {
				s.logger.With("error", clearErr).Error("Failed to clear access emails")
			}
		}

		// Get emails for response
		var emails []string
		if req.ShareType == "restricted" {
			if fetchedEmails, fetchErr := s.shareRepo.GetAccessEmails(ctx, existingShare.ID); fetchErr == nil {
				emails = fetchedEmails
			}
		}

		return &models.ShareResponse{
			ShareToken: existingShare.ShareToken,
			ShareURL:   fmt.Sprintf("/shared/prompts/%s", existingShare.ShareToken),
			ShareType:  existingShare.ShareType,
			Emails:     emails,
			CreatedAt:  existingShare.CreatedAt,
		}, nil
	}

	// Create new share
	token, err := s.generateShareToken()
	if err != nil {
		s.logger.With("error", err).Error("Failed to generate share token")
		return nil, fmt.Errorf("failed to generate share token: %w", err)
	}

	share := &models.PromptShare{
		PromptID:    prompt.ID,
		ShareToken:  token,
		ShareType:   req.ShareType,
		CreatedBy:   userID,
		CreatedAt:   time.Now(),
		IsActive:    true,
		AccessCount: 0,
	}

	if err := s.shareRepo.Create(ctx, share); err != nil {
		s.logger.With("error", err).Error("Failed to create prompt share")
		return nil, fmt.Errorf("failed to create share: %w", err)
	}

	// Add access emails for restricted shares
	if req.ShareType == "restricted" {
		if err := s.shareRepo.AddAccessEmails(ctx, share.ID, req.Emails); err != nil {
			s.logger.With("error", err).Error("Failed to add access emails")
			return nil, fmt.Errorf("failed to add access emails: %w", err)
		}
	}

	return &models.ShareResponse{
		ShareToken: share.ShareToken,
		ShareURL:   fmt.Sprintf("/shared/prompts/%s", share.ShareToken),
		ShareType:  share.ShareType,
		Emails:     req.Emails,
		CreatedAt:  share.CreatedAt,
	}, nil
}

// GetShare retrieves share details for a prompt
func (s *PromptShareService) GetShare(userID, promptSlug string) (*models.ShareResponse, error) {
	ctx := context.Background()

	// Verify prompt exists and user owns it across all user's teams
	prompt, err := s.promptRepo.GetBySlugCrossTeam(ctx, userID, promptSlug)
	if err != nil {
		return nil, fmt.Errorf("prompt not found")
	}

	// Get share
	share, err := s.shareRepo.GetByPromptID(ctx, prompt.ID)
	if err != nil {
		return nil, fmt.Errorf("share not found")
	}

	// Get emails for restricted shares
	var emails []string
	if share.ShareType == "restricted" {
		if fetchedEmails, err := s.shareRepo.GetAccessEmails(ctx, share.ID); err == nil {
			emails = fetchedEmails
		}
	}

	return &models.ShareResponse{
		ShareToken: share.ShareToken,
		ShareURL:   fmt.Sprintf("/shared/prompts/%s", share.ShareToken),
		ShareType:  share.ShareType,
		Emails:     emails,
		CreatedAt:  share.CreatedAt,
	}, nil
}

// DeleteShare deletes a share for a prompt
func (s *PromptShareService) DeleteShare(userID, promptSlug string) error {
	ctx := context.Background()

	// Verify prompt exists and user owns it across all user's teams
	prompt, err := s.promptRepo.GetBySlugCrossTeam(ctx, userID, promptSlug)
	if err != nil {
		return fmt.Errorf("prompt not found")
	}

	// Get share
	share, err := s.shareRepo.GetByPromptID(ctx, prompt.ID)
	if err != nil {
		return fmt.Errorf("share not found")
	}

	// Delete share
	if err := s.shareRepo.Delete(ctx, share.ID); err != nil {
		s.logger.With("error", err).Error("Failed to delete prompt share")
		return fmt.Errorf("failed to delete share: %w", err)
	}

	return nil
}

// GetSharedPrompt retrieves a shared prompt by token with access control
//
//nolint:gocognit,gocyclo // Complex business logic: handles auth, access control, expiration, and prompt rendering
func (s *PromptShareService) GetSharedPrompt(
	token string,
	userEmail *string,
) (*models.SharedPromptResponse, error) {
	ctx := context.Background()

	// Get share by token
	share, err := s.shareRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("shared prompt not found")
	}

	// Check if share is active
	if !share.IsActive {
		return nil, fmt.Errorf("share has been disabled")
	}

	// Check expiration
	if share.ExpiresAt != nil && time.Now().After(*share.ExpiresAt) {
		return nil, fmt.Errorf("share has expired")
	}

	// Access control for restricted shares
	if share.ShareType == "restricted" {
		if userEmail == nil || *userEmail == "" {
			return nil, fmt.Errorf("authentication required")
		}

		hasAccess, accessErr := s.shareRepo.HasAccess(ctx, share.ID, *userEmail)
		if accessErr != nil || !hasAccess {
			return nil, fmt.Errorf("access denied")
		}
	}

	// Increment access count asynchronously (don't block on errors)
	go func() {
		if incErr := s.shareRepo.IncrementAccessCount(context.Background(), share.ID); incErr != nil {
			s.logger.With("error", incErr).Warn("Failed to increment access count")
		}
	}()

	// Get prompt (for shared prompts, we need to bypass user/team checks)
	// Using empty userID triggers the shared prompt logic in GetByID
	prompt, err := s.promptRepo.GetByID(ctx, "", "", share.PromptID)
	if err != nil {
		return nil, fmt.Errorf("prompt not found")
	}

	// Render prompt body (resolve @references but leave {{placeholders}})
	// The client will handle placeholder substitution
	renderedBody := prompt.Body
	if s.promptService != nil {
		// Use the prompt service's render method to resolve @references
		// We'll pass empty placeholders map so they remain in the output
		// Pass empty teamID for shared prompts
		renderResp, renderErr := s.promptService.RenderPrompt(prompt.UserID, "", prompt.Slug, make(map[string]string))
		if renderErr != nil {
			s.logger.With("error", renderErr).Warn("Failed to render prompt, using raw body")
		} else {
			renderedBody = renderResp.RenderedBody
		}
	}

	return &models.SharedPromptResponse{
		Prompt:       *prompt,
		ShareType:    share.ShareType,
		RenderedBody: renderedBody,
	}, nil
}

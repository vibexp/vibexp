package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// escapedAtSentinel temporarily replaces escaped "@@" sequences during prompt
// rendering so they survive reference expansion before being restored as "@".
const escapedAtSentinel = "\x00ESCAPED_AT\x00"

type PromptService struct {
	repo              repositories.PromptRepository
	refRepo           repositories.PromptReferenceRepository
	userRepo          repositories.UserRepository
	projectRepo       repositories.ProjectRepository
	teamService       TeamServiceInterface
	authz             AuthorizationServiceInterface
	eventManager      events.EventPublisher
	contentVersionSvc ContentVersionServiceInterface
	commentRepo       repositories.CommentRepository
	logger            *slog.Logger
}

// Ensure PromptService implements PromptServiceInterface
var _ PromptServiceInterface = (*PromptService)(nil)

func NewPromptService(
	repo repositories.PromptRepository,
	refRepo repositories.PromptReferenceRepository,
	userRepo repositories.UserRepository,
	projectRepo repositories.ProjectRepository,
	teamService TeamServiceInterface,
	authzService AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
	contentVersionSvc ContentVersionServiceInterface,
	commentRepo repositories.CommentRepository,
) *PromptService {
	return &PromptService{
		repo:              repo,
		refRepo:           refRepo,
		userRepo:          userRepo,
		projectRepo:       projectRepo,
		teamService:       teamService,
		authz:             authzService,
		eventManager:      eventManager,
		contentVersionSvc: contentVersionSvc,
		commentRepo:       commentRepo,
		logger:            logger,
	}
}

type PromptFilters struct {
	Status    string
	Search    string
	UserID    string
	TeamID    string
	MCPExpose *bool
	IsShared  *bool
	Labels    []string
	ProjectID *string
	SortBy    string
	SortOrder string
	Page      int
	Limit     int
}

// publishPromptCreatedEvent publishes a prompt.created event with rendered body
func (s *PromptService) publishPromptCreatedEvent(ctx context.Context, prompt *models.Prompt) {
	if s.eventManager == nil {
		return
	}

	// Fetch user email for the event payload
	userEmail := ""
	if s.userRepo != nil {
		user, err := s.userRepo.GetByID(ctx, prompt.UserID)
		if err != nil {
			s.logger.With("error", err).Warn("Failed to fetch user email for prompt created event")
		} else {
			userEmail = user.Email
		}
	}

	// Render the prompt body to resolve all @references and {{placeholders}}
	renderedBody, err := s.RenderPromptBody(prompt.UserID, prompt.Body)
	if err != nil {
		s.logger.With(
			"prompt_id", prompt.ID,
			"error", fmt.Sprintf("%+v", err),
		).
			Warn("Failed to render prompt for event, sending raw body instead")
		renderedBody = prompt.Body
	}

	event := events.NewPromptCreatedEvent(
		prompt.ID, prompt.UserID, userEmail, "default", prompt.Slug, prompt.Name,
		prompt.Description, renderedBody, prompt.CreatedAt,
	)
	if err := s.eventManager.Publish(ctx, event); err != nil {
		s.logger.With("error", err).Warn("Failed to publish prompt created event")
	}
}

func (s *PromptService) CreatePrompt(userID, teamID string, req *models.CreatePromptRequest) (*models.Prompt, error) {
	ctx := context.Background()

	// Team ID comes from URL path and is already validated by middleware
	finalTeamID := teamID

	// Creating a resource is open to any team member (epic #220), but the caller
	// must still BE one — the middleware proves tenancy, this proves the role
	// grants the action. Checked before the project validation below so an
	// unauthorized caller cannot probe which projects exist.
	if authzErr := s.authz.Can(ctx, userID, finalTeamID, authz.ResourceCreate); authzErr != nil {
		return nil, authzErr
	}

	// Validate that the project exists and belongs to the user
	if err := s.validateProjectOwnership(ctx, userID, req.ProjectID); err != nil {
		return nil, err
	}

	if req.Status == "" {
		req.Status = "draft"
	}

	// Default mcp_expose to true if not specified
	mcpExpose := true
	if req.MCPExpose != nil {
		mcpExpose = *req.MCPExpose
	}

	// Initialize labels, default to empty array if not provided
	labels := req.Labels
	if labels == nil {
		labels = []string{}
	}

	prompt := &models.Prompt{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Body:        req.Body,
		UserID:      userID,
		TeamID:      finalTeamID,
		ProjectID:   req.ProjectID,
		Status:      req.Status,
		MCPExpose:   mcpExpose,
		Labels:      labels,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.repo.Create(ctx, prompt); err != nil {
		s.logger.With("error", err).Error("Failed to create prompt")
		return nil, err
	}

	// Extract and store references from the prompt body
	if err := s.updatePromptReferences(ctx, userID, prompt.ID, prompt.Body); err != nil {
		s.logger.With("error", err).Warn("Failed to update prompt references")
		// Don't fail the creation, just log the warning
	}

	// Publish prompt created event with rendered body
	s.publishPromptCreatedEvent(ctx, prompt)

	s.logger.With(
		"prompt_id", prompt.ID,
		"user_id", userID,
		"slug", prompt.Slug,
	).Info("Prompt created successfully")

	return prompt, nil
}

func (s *PromptService) GetPrompt(userID, teamID, promptID string) (*models.Prompt, error) {
	ctx := context.Background()

	prompt, err := s.repo.GetByID(ctx, userID, teamID, promptID)
	if err != nil {
		s.logger.With("error", err).Error("Failed to get prompt")
		return nil, err
	}

	return prompt, nil
}

func (s *PromptService) GetPromptBySlug(userID, teamID, slug string) (*models.Prompt, error) {
	ctx := context.Background()

	prompt, err := s.repo.GetBySlug(ctx, userID, teamID, slug)
	if err != nil {
		// Check if this is a "not found" error
		if errors.Is(err, repositories.ErrPromptNotFound) {
			s.logger.With(
				"user_id", userID,
				"team_id", teamID,
				"slug", slug,
			).Warn("Prompt not found by slug")
			// Propagate the sentinel so handlers can errors.Is it to a 404.
			return nil, repositories.ErrPromptNotFound
		}

		// For other errors (database errors, etc.), log as ERROR
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to get prompt by slug")
		return nil, err
	}

	return prompt, nil
}

func (s *PromptService) ListPrompts(userID string, filters PromptFilters) (*models.PromptListResponse, error) {
	ctx := context.Background()

	if filters.Page < 1 {
		filters.Page = 1
	}
	if filters.Limit < 1 || filters.Limit > 100 {
		filters.Limit = 20
	}

	// Convert service filters to repository filters
	repoFilters := repositories.PromptFilters{
		Status:    filters.Status,
		Search:    filters.Search,
		TeamID:    filters.TeamID,
		MCPExpose: filters.MCPExpose,
		IsShared:  filters.IsShared,
		Labels:    filters.Labels,
		ProjectID: filters.ProjectID,
		SortBy:    filters.SortBy,
		SortOrder: filters.SortOrder,
		Page:      filters.Page,
		Limit:     filters.Limit,
	}

	prompts, totalCount, err := s.repo.List(ctx, userID, repoFilters)
	if err != nil {
		s.logger.With("error", err).Error("Failed to list prompts")
		return nil, fmt.Errorf("failed to list prompts: %w", err)
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(filters.Limit)))

	return &models.PromptListResponse{
		Prompts:    prompts,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: totalPages,
	}, nil
}

func (s *PromptService) UpdatePromptBySlug(
	userID, teamID, slug string, req *models.UpdatePromptRequest,
) (*models.Prompt, error) {
	existingPrompt, err := s.GetPromptBySlug(userID, teamID, slug)
	if err != nil {
		return nil, err
	}

	return s.UpdatePrompt(userID, teamID, existingPrompt.ID, req)
}

func (s *PromptService) UpdatePrompt(
	userID, teamID, promptID string, req *models.UpdatePromptRequest,
) (*models.Prompt, error) {
	return s.updatePromptInternal(userID, teamID, promptID, req, models.ActorTypeHuman, nil)
}

// updatePromptInternal applies an update request to a prompt, snapshotting the prior raw Body
// template as a content version when the body changed, then persisting and publishing the
// updated event. actorType and changeSummary describe the snapshot: human edits pass
// (ActorTypeHuman, nil); a restore passes (ActorTypeSystem, "Restored Version N").
// changeSummary is an internal snapshot attribute only — it is never read from
// UpdatePromptRequest, so the prompt update API exposes no user-facing change-summary field
// (parity with artifacts/blueprints/memory). The versioned content is the raw Body template
// (placeholders and @slug references), not any rendered output.
func (s *PromptService) updatePromptInternal(
	userID, teamID, promptID string, req *models.UpdatePromptRequest,
	actorType string, changeSummary *string,
) (*models.Prompt, error) {
	ctx := context.Background()

	// First get the existing prompt
	existingPrompt, err := s.repo.GetByID(ctx, userID, teamID, promptID)
	if err != nil {
		return nil, err
	}

	// Any member may update any prompt, including another member's (epic #220
	// decision D1 — uniform update). This is not a relaxation in practice: the
	// repository's `user_id` check compared the stored owner against itself, so
	// it always passed; the rule now simply says what it does.
	if authzErr := s.authz.Can(ctx, userID, teamID, authz.ResourceUpdateAny); authzErr != nil {
		return nil, authzErr
	}

	// Check if there are any updates to apply
	if !hasPromptUpdates(req) {
		return existingPrompt, nil
	}

	// Note: team_id cannot be changed via update (removed from UpdatePromptRequest)
	// Team reassignment is forbidden to prevent cross-team resource moves

	// Validate project ownership if project_id is being updated
	if req.ProjectID != nil {
		if validationErr := s.validateProjectOwnership(ctx, userID, *req.ProjectID); validationErr != nil {
			return nil, validationErr
		}
	}

	updatedPrompt := buildUpdatedPrompt(existingPrompt, req)

	s.snapshotPromptBody(ctx, userID, existingPrompt, updatedPrompt, actorType, changeSummary)

	err = s.repo.Update(ctx, updatedPrompt)
	if err != nil {
		s.logger.With("error", err).Error("Failed to update prompt")
		return nil, err
	}

	// Update references if body changed
	if req.Body != nil {
		if err := s.updatePromptReferences(ctx, userID, updatedPrompt.ID, updatedPrompt.Body); err != nil {
			s.logger.With("error", err).Warn("Failed to update prompt references")
			// Don't fail the update, just log the warning
		}
	}

	s.publishPromptUpdatedEvent(ctx, userID, updatedPrompt)

	s.logger.With(
		"prompt_id", updatedPrompt.ID,
		"user_id", userID,
	).Info("Prompt updated successfully")

	return updatedPrompt, nil
}

// hasPromptUpdates reports whether the update request carries at least one field to apply.
func hasPromptUpdates(req *models.UpdatePromptRequest) bool {
	return req.Name != nil || req.Slug != nil || req.Description != nil ||
		req.Body != nil || req.ProjectID != nil || req.Status != nil || req.MCPExpose != nil || req.Labels != nil
}

// buildUpdatedPrompt copies the existing prompt and applies the requested field
// updates. TeamID is preserved (immutable after creation).
func buildUpdatedPrompt(existingPrompt *models.Prompt, req *models.UpdatePromptRequest) *models.Prompt {
	updatedPrompt := &models.Prompt{
		ID:          existingPrompt.ID,
		Name:        existingPrompt.Name,
		Slug:        existingPrompt.Slug,
		Description: existingPrompt.Description,
		Body:        existingPrompt.Body,
		ProjectID:   existingPrompt.ProjectID,
		Status:      existingPrompt.Status,
		MCPExpose:   existingPrompt.MCPExpose,
		Labels:      existingPrompt.Labels,
		UserID:      existingPrompt.UserID,
		TeamID:      existingPrompt.TeamID, // Preserve team_id (immutable after creation)
		CreatedAt:   existingPrompt.CreatedAt,
		UpdatedAt:   time.Now(),
		Version:     existingPrompt.Version,
	}

	// Apply updates
	if req.Name != nil {
		updatedPrompt.Name = *req.Name
	}
	if req.Slug != nil {
		updatedPrompt.Slug = *req.Slug
	}
	if req.Description != nil {
		updatedPrompt.Description = *req.Description
	}
	if req.Body != nil {
		updatedPrompt.Body = *req.Body
	}
	if req.ProjectID != nil {
		updatedPrompt.ProjectID = *req.ProjectID
	}
	if req.Status != nil {
		updatedPrompt.Status = *req.Status
	}
	if req.MCPExpose != nil {
		updatedPrompt.MCPExpose = *req.MCPExpose
	}
	if req.Labels != nil {
		updatedPrompt.Labels = req.Labels
	}

	return updatedPrompt
}

// snapshotPromptBody records a best-effort content-version snapshot of the prior raw
// Body template when it changed. A snapshot failure must not fail the update (mirrors
// event publishing), so it is logged and swallowed.
func (s *PromptService) snapshotPromptBody(
	ctx context.Context, userID string,
	existingPrompt, updatedPrompt *models.Prompt,
	actorType string, changeSummary *string,
) {
	if s.contentVersionSvc == nil || existingPrompt.Body == updatedPrompt.Body {
		return
	}
	if snapErr := s.contentVersionSvc.SnapshotIfChanged(ctx, SnapshotRequest{
		ResourceType:  "prompt",
		ResourceID:    updatedPrompt.ID,
		TeamID:        updatedPrompt.TeamID,
		UserID:        userID,
		OldContent:    existingPrompt.Body,
		NewContent:    updatedPrompt.Body,
		ChangeSummary: changeSummary,
		ActorType:     actorType,
	}); snapErr != nil {
		s.logger.With("error", snapErr).Warn("Failed to snapshot prompt content version")
	}
}

// publishPromptUpdatedEvent publishes the prompt updated event with the rendered body
// (all @references and {{placeholders}} resolved, for embedding generation). Rendering
// or publish failures are logged and swallowed — the update already succeeded.
func (s *PromptService) publishPromptUpdatedEvent(
	ctx context.Context, userID string, updatedPrompt *models.Prompt,
) {
	if s.eventManager == nil {
		return
	}

	// Render the prompt body to resolve all @references and {{placeholders}}
	// For embedding generation, we want the fully resolved content
	renderedBody := updatedPrompt.Body
	renderResponse, err := s.renderPromptRecursive(
		userID, updatedPrompt.Body, make(map[string]string), make(map[string]bool),
	)
	if err != nil {
		// If rendering fails (e.g., missing placeholders or circular refs), log warning but continue
		// We'll send the raw body instead
		s.logger.With(
			"prompt_id", updatedPrompt.ID,
			"error", fmt.Sprintf("%+v", err),
		).
			Warn("Failed to render prompt for event, sending raw body instead")
	} else {
		renderedBody = renderResponse.RenderedBody
	}

	event := events.NewPromptUpdatedEvent(
		updatedPrompt.ID, updatedPrompt.UserID, "default", updatedPrompt.Slug,
		updatedPrompt.Name, updatedPrompt.Description, renderedBody, updatedPrompt.UpdatedAt,
	)
	if err := s.eventManager.Publish(ctx, event); err != nil {
		s.logger.With("error", err).Warn("Failed to publish prompt updated event")
	}
}

// ListPromptVersions returns the content-version history for a prompt, newest-first.
// The prompt is loaded through the authorization-enforcing team-scoped lookup before its
// versions are read; the resolved prompt's TeamID scopes the version query.
func (s *PromptService) ListPromptVersions(
	userID, teamID, slug string,
) ([]*models.ContentVersion, error) {
	prompt, err := s.GetPromptBySlug(userID, teamID, slug)
	if err != nil {
		return nil, err
	}
	return s.contentVersionSvc.ListVersions(context.Background(), prompt.TeamID, "prompt", prompt.ID)
}

// GetPromptVersion returns a single content version of a prompt by version number.
func (s *PromptService) GetPromptVersion(
	userID, teamID, slug string, versionNumber int,
) (*models.ContentVersion, error) {
	prompt, err := s.GetPromptBySlug(userID, teamID, slug)
	if err != nil {
		return nil, err
	}
	return s.contentVersionSvc.GetVersion(context.Background(), prompt.TeamID, "prompt", prompt.ID, versionNumber)
}

// RestorePromptVersion restores a prompt's raw Body template to the given version by applying
// it through the shared update path, which snapshots the pre-restore body as a new version.
// The restore is non-destructive (the current body is preserved as a snapshot) and re-applies
// the raw template verbatim, so placeholders and @slug references survive the round-trip.
func (s *PromptService) RestorePromptVersion(
	userID, teamID, slug string, versionNumber int,
) (*models.Prompt, error) {
	prompt, err := s.GetPromptBySlug(userID, teamID, slug)
	if err != nil {
		return nil, err
	}

	target, err := s.contentVersionSvc.Restore(context.Background(), prompt.TeamID, "prompt", prompt.ID, versionNumber)
	if err != nil {
		return nil, err
	}

	// A restore is a system-authored edit: the pre-restore body is snapshotted with a
	// default "Restored Version N" summary so the timeline reads clearly.
	restoreSummary := fmt.Sprintf("Restored Version %d", versionNumber)
	return s.updatePromptInternal(
		userID, teamID, prompt.ID, &models.UpdatePromptRequest{Body: &target},
		models.ActorTypeSystem, &restoreSummary,
	)
}

func (s *PromptService) DeletePromptBySlug(userID, teamID, slug string) error {
	existingPrompt, err := s.GetPromptBySlug(userID, teamID, slug)
	if err != nil {
		return err
	}

	// Hand the already-loaded prompt straight through: the own-vs-any check needs
	// its owner, and re-fetching it by ID would be a second identical read.
	return s.deleteFetchedPrompt(context.Background(), userID, teamID, existingPrompt)
}

func (s *PromptService) DeletePrompt(userID, teamID, promptID string) error {
	ctx := context.Background()

	// Fetch first to learn who created the prompt: delete is own-vs-any (members
	// delete only what they authored; Admin+ delete anyone's), and the
	// repository no longer carries that decision in its SQL.
	existingPrompt, err := s.repo.GetByID(ctx, userID, teamID, promptID)
	if err != nil {
		return err
	}

	return s.deleteFetchedPrompt(ctx, userID, teamID, existingPrompt)
}

// deleteFetchedPrompt authorizes and deletes a prompt the caller has already
// loaded, so the slug path does not pay for a second identical read.
func (s *PromptService) deleteFetchedPrompt(
	ctx context.Context, userID, teamID string, existingPrompt *models.Prompt,
) error {
	if authzErr := s.authz.CanActOnResource(
		ctx, userID, teamID, existingPrompt.UserID,
		authz.ResourceDeleteOwn, authz.ResourceDeleteAny,
	); authzErr != nil {
		return authzErr
	}

	promptID := existingPrompt.ID

	// Check if the prompt has dependents (is being used by other prompts)
	hasDependents, err := s.refRepo.HasDependents(ctx, promptID)
	if err != nil {
		s.logger.With("error", err).Error("Failed to check prompt dependencies")
		return fmt.Errorf("failed to check prompt dependencies: %w", err)
	}

	if hasDependents {
		// Get the list of dependent prompts for the error message
		dependents, depErr := s.refRepo.GetPromptsUsingPrompt(ctx, userID, promptID)
		if depErr != nil {
			s.logger.With("error", depErr).Error("Failed to get dependent prompts")
			return fmt.Errorf("cannot delete prompt: it is being used by other prompts")
		}

		var dependentNames []string
		for _, dep := range dependents {
			dependentNames = append(dependentNames, dep.Name)
		}

		return fmt.Errorf(
			"cannot delete prompt: it is being used by %d other prompt(s): %s",
			len(dependents),
			strings.Join(dependentNames, ", "),
		)
	}

	err = s.repo.Delete(ctx, userID, teamID, promptID)
	if err != nil {
		s.logger.With("error", err).Error("Failed to delete prompt")
		return err
	}

	s.deletePromptComments(ctx, teamID, promptID)

	s.logger.With(
		"prompt_id", promptID,
		"user_id", userID,
		"team_id", teamID,
	).Info("Prompt deleted successfully")

	return nil
}

// deletePromptComments removes a prompt's comments after it is deleted.
// Best-effort: a failure is logged but does not fail the completed delete.
func (s *PromptService) deletePromptComments(ctx context.Context, teamID, promptID string) {
	if s.commentRepo == nil {
		return
	}
	if _, err := s.commentRepo.DeleteByResource(
		ctx, teamID, models.CommentResourceTypePrompt, promptID,
	); err != nil {
		s.logger.With(
			"service", "prompt",
			"team_id", teamID,
			"prompt_id", promptID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to delete comments for deleted prompt")
	}
}

func (s *PromptService) RenderPrompt(
	userID, teamID, slug string, placeholders map[string]string,
) (*models.RenderPromptResponse, error) {
	prompt, err := s.GetPromptBySlug(userID, teamID, slug)
	if err != nil {
		return nil, err
	}

	return s.renderPromptRecursive(userID, prompt.Body, placeholders, make(map[string]bool))
}

// RenderPromptBody resolves all @references and {{placeholders}} in an
// already-loaded prompt body for the given user, returning the fully rendered
// text. It exists so callers that hold the body (e.g. the create-event path and
// the embedding backfill) embed the same reference-resolved content the live
// pipeline produces, without re-fetching the prompt by slug.
func (s *PromptService) RenderPromptBody(userID, body string) (string, error) {
	rendered, err := s.renderPromptRecursive(userID, body, make(map[string]string), make(map[string]bool))
	if err != nil {
		return "", err
	}
	return rendered.RenderedBody, nil
}

func (s *PromptService) renderPromptRecursive(
	userID, body string, placeholders map[string]string, visitedRefs map[string]bool,
) (*models.RenderPromptResponse, error) {
	warnings := make([]string, 0)
	referencesUsed := make([]string, 0)

	// Handle escaped @@ sequences first
	renderedBody := strings.ReplaceAll(body, "@@", escapedAtSentinel)

	// Substitute {{placeholder_key}} patterns with provided values
	renderedBody = substitutePlaceholders(renderedBody, placeholders)

	// Parse reference patterns @prompt_slug
	referenceRegex := regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)
	referenceMatches := referenceRegex.FindAllStringSubmatch(renderedBody, -1)

	for _, match := range referenceMatches {
		if len(match) < 2 {
			continue
		}

		reference := match[0]                  // Full match like @slug
		refSlug := strings.TrimSpace(match[1]) // Just the slug

		// Check for circular references
		if visitedRefs[refSlug] {
			return nil, fmt.Errorf("circular reference detected for prompt: %s", refSlug)
		}

		// Get the referenced prompt across all user's teams
		refPrompt, err := s.repo.GetBySlugCrossTeam(context.Background(), userID, refSlug)
		if err != nil {
			if errors.Is(err, repositories.ErrPromptNotFound) {
				// Log warning but don't fail - keep the reference as-is
				s.logger.With("referenced_slug", refSlug).Warn("Referenced prompt not found, keeping reference as-is")

				// Add to warnings list
				warnings = append(warnings, fmt.Sprintf("Reference not found: @%s", refSlug))
				continue // Skip this reference, keep it in the rendered body
			}
			return nil, fmt.Errorf("failed to get referenced prompt %s: %w", refSlug, err)
		}

		// Recursively render the referenced prompt, marking this reference as visited
		refResponse, err := s.renderPromptRecursive(userID, refPrompt.Body, placeholders, visitedWith(visitedRefs, refSlug))
		if err != nil {
			return nil, fmt.Errorf("failed to render referenced prompt %s: %w", refSlug, err)
		}

		// Replace the reference with the rendered content
		renderedBody = strings.ReplaceAll(renderedBody, reference, refResponse.RenderedBody)

		// Collect references used from nested prompt
		referencesUsed = append(referencesUsed, refSlug)
		referencesUsed = append(referencesUsed, refResponse.ReferencesUsed...)

		// Collect warnings from nested prompt
		warnings = append(warnings, refResponse.Warnings...)
	}

	// Remove duplicates from references
	referencesUsed = lo.Uniq(referencesUsed)

	// Replace escaped sequences back
	renderedBody = strings.ReplaceAll(renderedBody, escapedAtSentinel, "@")

	return &models.RenderPromptResponse{
		RenderedBody:   renderedBody,
		ReferencesUsed: referencesUsed,
		Warnings:       warnings,
	}, nil
}

func (s *PromptService) GetPromptPlaceholders(userID, teamID, slug string) ([]string, error) {
	prompt, err := s.GetPromptBySlug(userID, teamID, slug)
	if err != nil {
		return nil, err
	}

	return s.ExtractAllPlaceholders(userID, prompt.Body, make(map[string]bool))
}

func (s *PromptService) ExtractAllPlaceholders(userID, body string, visitedRefs map[string]bool) ([]string, error) {
	// Handle escaped @@ sequences first (replace temporarily to avoid matching them)
	bodyForExtraction := strings.ReplaceAll(body, "@@", escapedAtSentinel)

	// Extract placeholders from current body
	allPlaceholders := appendUniquePlaceholders(nil, extractPlaceholderKeys(bodyForExtraction))

	// Extract references and get their placeholders recursively
	referenceRegex := regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)
	referenceMatches := referenceRegex.FindAllStringSubmatch(bodyForExtraction, -1)

	for _, match := range referenceMatches {
		if len(match) < 2 {
			continue
		}

		refSlug := strings.TrimSpace(match[1])

		// Check for circular references
		if visitedRefs[refSlug] {
			continue // Skip circular references
		}

		// Get the referenced prompt across all user's teams
		refPrompt, err := s.repo.GetBySlugCrossTeam(context.Background(), userID, refSlug)
		if err != nil {
			if errors.Is(err, repositories.ErrPromptNotFound) {
				continue // Skip missing references
			}
			return nil, fmt.Errorf("failed to get referenced prompt %s: %w", refSlug, err)
		}

		// Recursively get placeholders from the referenced prompt, marking this
		// reference as visited
		refPlaceholders, err := s.ExtractAllPlaceholders(userID, refPrompt.Body, visitedWith(visitedRefs, refSlug))
		if err != nil {
			return nil, fmt.Errorf("failed to extract placeholders from referenced prompt %s: %w", refSlug, err)
		}

		// Add unique placeholders
		allPlaceholders = appendUniquePlaceholders(allPlaceholders, refPlaceholders)
	}

	return allPlaceholders, nil
}

// substitutePlaceholders replaces every {{placeholder_key}} pattern that has a value
// in placeholders; patterns without a value remain in the body as-is.
func substitutePlaceholders(body string, placeholders map[string]string) string {
	placeholderRegex := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	for _, match := range placeholderRegex.FindAllStringSubmatch(body, -1) {
		if len(match) < 2 {
			continue
		}

		placeholder := match[0]            // Full match like {{key}}
		key := strings.TrimSpace(match[1]) // Just the key

		// Only replace if value exists, otherwise keep placeholder as-is
		if value, exists := placeholders[key]; exists {
			body = strings.ReplaceAll(body, placeholder, value)
		}
		// If value doesn't exist, placeholder remains in the rendered body
	}
	return body
}

// extractPlaceholderKeys returns the trimmed {{placeholder}} keys found in body, in order.
func extractPlaceholderKeys(body string) []string {
	placeholderRegex := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	matches := placeholderRegex.FindAllStringSubmatch(body, -1)
	keys := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		keys = append(keys, strings.TrimSpace(match[1]))
	}
	return keys
}

// appendUniquePlaceholders appends each value not already present in dst, preserving order.
func appendUniquePlaceholders(dst, values []string) []string {
	for _, v := range values {
		if !slices.Contains(dst, v) {
			dst = append(dst, v)
		}
	}
	return dst
}

// visitedWith returns a copy of visited with slug marked as visited, leaving the
// original map untouched so sibling references do not see each other's paths.
func visitedWith(visited map[string]bool, slug string) map[string]bool {
	next := make(map[string]bool, len(visited)+1)
	for k, v := range visited {
		next[k] = v
	}
	next[slug] = true
	return next
}

// updatePromptReferences extracts references from prompt body and updates the database
func (s *PromptService) updatePromptReferences(ctx context.Context, userID, promptID, body string) error {
	// First, delete existing references for this prompt
	if err := s.refRepo.DeleteByPromptID(ctx, promptID); err != nil {
		return fmt.Errorf("failed to delete existing references: %w", err)
	}

	// Handle escaped @@ sequences first (replace temporarily to avoid matching them)
	bodyForExtraction := strings.ReplaceAll(body, "@@", escapedAtSentinel)

	// Extract reference patterns @prompt_slug
	referenceRegex := regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)
	referenceMatches := referenceRegex.FindAllStringSubmatch(bodyForExtraction, -1)

	if len(referenceMatches) == 0 {
		return nil // No references to store
	}

	// Collect unique referenced prompt slugs
	uniqueSlugs := make(map[string]bool)
	for _, match := range referenceMatches {
		if len(match) < 2 {
			continue
		}
		slug := strings.TrimSpace(match[1])
		uniqueSlugs[slug] = true
	}

	// Build references list
	references := make([]models.PromptReference, 0, len(uniqueSlugs))
	for slug := range uniqueSlugs {
		// Get the referenced prompt to get its ID across all user's teams
		refPrompt, err := s.repo.GetBySlugCrossTeam(ctx, userID, slug)
		if err != nil {
			// If referenced prompt doesn't exist, log warning but continue
			s.logger.With(
				"prompt_id", promptID,
				"referenced_slug", slug,
				"error", fmt.Sprintf("%+v", err),
			).
				Warn("Referenced prompt not found, skipping reference")
			continue
		}

		references = append(references, models.PromptReference{
			PromptID:           promptID,
			ReferencedPromptID: refPrompt.ID,
			CreatedAt:          time.Now(),
		})
	}

	// Create the references in batch
	if len(references) > 0 {
		if err := s.refRepo.CreateBatch(ctx, references); err != nil {
			return fmt.Errorf("failed to create references: %w", err)
		}
	}

	return nil
}

// GetPromptDependencies returns the dependency information for a prompt
func (s *PromptService) GetPromptDependencies(
	userID, teamID, promptID string,
) (*models.PromptDependenciesResponse, error) {
	ctx := context.Background()

	// Get prompts that use this prompt (used by)
	usedBy, err := s.refRepo.GetPromptsUsingPrompt(ctx, userID, promptID)
	if err != nil {
		s.logger.With("error", err).Error("Failed to get prompts using this prompt")
		return nil, fmt.Errorf("failed to get prompt dependencies: %w", err)
	}

	// Get prompts that this prompt uses (uses)
	uses, err := s.refRepo.GetPromptsUsedByPrompt(ctx, userID, promptID)
	if err != nil {
		s.logger.With("error", err).Error("Failed to get prompts used by this prompt")
		return nil, fmt.Errorf("failed to get prompt dependencies: %w", err)
	}

	return &models.PromptDependenciesResponse{
		UsedBy: usedBy,
		Uses:   uses,
	}, nil
}

// GetPromptDependenciesBySlug returns the dependency information for a prompt by slug
func (s *PromptService) GetPromptDependenciesBySlug(
	userID, teamID, slug string,
) (*models.PromptDependenciesResponse, error) {
	prompt, err := s.GetPromptBySlug(userID, teamID, slug)
	if err != nil {
		return nil, err
	}

	return s.GetPromptDependencies(userID, teamID, prompt.ID)
}

// GetUserLabels retrieves all distinct labels used by a user's prompts
func (s *PromptService) GetUserLabels(userID string) ([]string, error) {
	ctx := context.Background()

	labels, err := s.repo.GetUserLabels(ctx, userID)
	if err != nil {
		s.logger.With("error", err).Error("Failed to get user labels")
		return nil, fmt.Errorf("failed to get user labels: %w", err)
	}

	return labels, nil
}

// validateProjectOwnership validates that the project exists and belongs to the user
func (s *PromptService) validateProjectOwnership(ctx context.Context, userID, projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project_id is required")
	}

	project, err := s.projectRepo.GetByID(ctx, userID, projectID)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"project_id", projectID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to get project for validation")
		return fmt.Errorf("project not found or does not belong to user")
	}

	if project.UserID != userID {
		s.logger.With(
			"user_id", userID,
			"project_id", projectID,
			"project_owner_id", project.UserID,
		).
			Warn("User attempted to use project owned by another user")
		return fmt.Errorf("project does not belong to user")
	}

	return nil
}

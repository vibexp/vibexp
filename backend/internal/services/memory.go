package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

type MemoryService struct {
	repo              repositories.MemoryRepository
	teamService       TeamServiceInterface
	authz             AuthorizationServiceInterface
	eventManager      events.EventPublisher
	contentVersionSvc ContentVersionServiceInterface
	commentRepo       repositories.CommentRepository
	relationRepo      repositories.RelationRepository
	logger            *slog.Logger
}

// Ensure MemoryService implements MemoryServiceInterface
var _ MemoryServiceInterface = (*MemoryService)(nil)

func NewMemoryService(
	repo repositories.MemoryRepository,
	teamService TeamServiceInterface,
	authzService AuthorizationServiceInterface,
	eventManager events.EventPublisher,
	logger *slog.Logger,
	contentVersionSvc ContentVersionServiceInterface,
	commentRepo repositories.CommentRepository,
	relationRepo repositories.RelationRepository,
) *MemoryService {
	return &MemoryService{
		repo:              repo,
		teamService:       teamService,
		authz:             authzService,
		eventManager:      eventManager,
		contentVersionSvc: contentVersionSvc,
		commentRepo:       commentRepo,
		relationRepo:      relationRepo,
		logger:            logger,
	}
}

type MemoryFilters struct {
	Search        string
	MetadataKey   *string
	MetadataValue *string
	Status        *string
	UserID        string
	TeamID        string
	ProjectID     *string
	SortBy        string
	SortOrder     string
	Page          int
	Limit         int
}

func (s *MemoryService) CreateMemory(userID, teamID string, req *models.CreateMemoryRequest) (*models.Memory, error) {
	ctx := context.Background()

	// Creating a resource is open to any team member (epic #220), but the caller
	// must still BE one — the middleware proves tenancy, this proves the role.
	if authzErr := s.authz.Can(ctx, userID, teamID, authz.ResourceCreate); authzErr != nil {
		return nil, authzErr
	}

	// Default to active when no status is supplied (mirrors artifact create).
	status := models.MemoryStatusActive
	if req.Status != nil && *req.Status != "" {
		status = *req.Status
	}

	now := time.Now()
	memory := &models.Memory{
		UserID:    userID,
		TeamID:    teamID,
		ProjectID: req.ProjectID,
		Text:      req.Text,
		Status:    status,
		Metadata:  req.Metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if memory.Metadata == nil {
		memory.Metadata = make(map[string]interface{})
	}

	err := s.repo.Create(ctx, memory)
	if err != nil {
		return nil, err
	}

	// Publish memory created event using project_id as project identifier
	if s.eventManager != nil {
		event := events.NewMemoryCreatedEvent(memory.ID, memory.UserID, memory.ProjectID, memory.Text, memory.CreatedAt)
		if err := s.eventManager.Publish(ctx, event); err != nil {
			s.logger.With("error", err).Warn("Failed to publish memory created event")
		}
	}

	return memory, nil
}

func (s *MemoryService) GetMemory(userID, teamID, memoryID string) (*models.Memory, error) {
	ctx := context.Background()
	return s.repo.GetByID(ctx, userID, teamID, memoryID)
}

func (s *MemoryService) ListMemories(userID string, filters MemoryFilters) (*models.MemoryListResponse, error) {
	ctx := context.Background()

	if filters.Page == 0 {
		filters.Page = 1
	}
	if filters.Limit == 0 {
		filters.Limit = 50
	}

	repoFilters := repositories.MemoryFilters{
		Search:        filters.Search,
		MetadataKey:   filters.MetadataKey,
		MetadataValue: filters.MetadataValue,
		Status:        filters.Status,
		TeamID:        filters.TeamID,
		ProjectID:     filters.ProjectID,
		SortBy:        filters.SortBy,
		SortOrder:     filters.SortOrder,
		Page:          filters.Page,
		Limit:         filters.Limit,
	}

	memories, totalCount, err := s.repo.List(ctx, userID, repoFilters)
	if err != nil {
		return nil, err
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(filters.Limit)))

	return &models.MemoryListResponse{
		Memories:   memories,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: totalPages,
	}, nil
}

func (s *MemoryService) UpdateMemory(
	userID, teamID, memoryID string, req *models.UpdateMemoryRequest,
) (*models.Memory, error) {
	ctx := context.Background()

	// Get existing memory (team-scoped — enforces tenancy)
	memory, err := s.repo.GetByID(ctx, userID, teamID, memoryID)
	if err != nil {
		return nil, err
	}

	return s.applyAndPersistMemoryUpdate(ctx, userID, memory, req, models.ActorTypeHuman, nil)
}

// applyAndPersistMemoryUpdate applies the update request to an already-loaded memory,
// snapshots the pre-update text when it changed, persists the memory, and publishes the
// updated event. The memory must already have been loaded through an authorization-enforcing
// team-scoped lookup. actorType and changeSummary describe the content-version snapshot:
// human edits pass (ActorTypeHuman, nil); a restore passes (ActorTypeSystem, "Restored Version N").
// changeSummary is an internal snapshot attribute only — it is never read from
// UpdateMemoryRequest, so the memory update API exposes no user-facing change-summary field
// (parity with artifacts and blueprints). The memory's Text field is the versioned content.
// applyMemoryUpdates copies the provided fields onto the memory. TeamID is
// deliberately absent: it is immutable, so team reassignment cannot happen
// through an update.
func applyMemoryUpdates(memory *models.Memory, req *models.UpdateMemoryRequest) {
	if req.Text != nil {
		memory.Text = *req.Text
	}
	if req.Metadata != nil {
		memory.Metadata = req.Metadata
	}
	if req.ProjectID != nil {
		memory.ProjectID = *req.ProjectID
	}
	// An empty status is treated as "unchanged" (omitempty parity), so a partial
	// update never clears an existing status back to the default.
	if req.Status != nil && *req.Status != "" {
		memory.Status = *req.Status
	}

	memory.UpdatedAt = time.Now()
}

func (s *MemoryService) applyAndPersistMemoryUpdate(
	ctx context.Context, userID string, memory *models.Memory, req *models.UpdateMemoryRequest,
	actorType string, changeSummary *string,
) (*models.Memory, error) {
	// Any member may update any memory, including another member's (epic #220
	// decision D1 — uniform update). No behavior change: this domain already
	// allowed it; the rule now simply says so.
	//
	// Gated HERE rather than in UpdateMemory so RestoreMemoryVersion — which
	// reaches this helper directly and is just as much a write — cannot slip past
	// it. Same placement as the artifact/blueprint helpers.
	if authzErr := s.authz.Can(ctx, userID, memory.TeamID, authz.ResourceUpdateAny); authzErr != nil {
		return nil, authzErr
	}

	// Note: team_id cannot be changed via update (removed from UpdateMemoryRequest)
	// Team reassignment is forbidden to prevent cross-team resource moves

	// Snapshot the prior text before the update mutates it, so a version history is
	// built whenever the text actually changes.
	oldContent := memory.Text

	applyMemoryUpdates(memory, req)

	// Best-effort content-version snapshot: record the pre-update text when it changed.
	// A snapshot failure must not fail the update (mirrors event publishing).
	if s.contentVersionSvc != nil && oldContent != memory.Text {
		if err := s.contentVersionSvc.SnapshotIfChanged(ctx, SnapshotRequest{
			ResourceType:  "memory",
			ResourceID:    memory.ID,
			TeamID:        memory.TeamID,
			UserID:        userID,
			OldContent:    oldContent,
			NewContent:    memory.Text,
			ChangeSummary: changeSummary,
			ActorType:     actorType,
		}); err != nil {
			s.logger.With("error", err).Warn("Failed to snapshot memory content version")
		}
	}

	if err := s.repo.Update(ctx, memory); err != nil {
		return nil, err
	}

	// Publish memory updated event using project_id as project identifier
	if s.eventManager != nil {
		event := events.NewMemoryUpdatedEvent(memory.ID, memory.UserID, memory.ProjectID, memory.Text, memory.UpdatedAt)
		if err := s.eventManager.Publish(ctx, event); err != nil {
			s.logger.With("error", err).Warn("Failed to publish memory updated event")
		}
	}

	return memory, nil
}

// ListMemoryVersions returns the content-version history for a memory, newest-first.
// The memory is loaded through the authorization-enforcing team-scoped lookup before its
// versions are read; the resolved memory's TeamID scopes the version query.
func (s *MemoryService) ListMemoryVersions(
	userID, teamID, memoryID string,
) ([]*models.ContentVersion, error) {
	ctx := context.Background()
	memory, err := s.repo.GetByID(ctx, userID, teamID, memoryID)
	if err != nil {
		return nil, err
	}
	return s.contentVersionSvc.ListVersions(ctx, memory.TeamID, "memory", memory.ID)
}

// GetMemoryVersion returns a single content version of a memory by version number.
func (s *MemoryService) GetMemoryVersion(
	userID, teamID, memoryID string, versionNumber int,
) (*models.ContentVersion, error) {
	ctx := context.Background()
	memory, err := s.repo.GetByID(ctx, userID, teamID, memoryID)
	if err != nil {
		return nil, err
	}
	return s.contentVersionSvc.GetVersion(ctx, memory.TeamID, "memory", memory.ID, versionNumber)
}

// RestoreMemoryVersion restores a memory's text to the given version by applying it through
// the shared update path, which snapshots the pre-restore text as a new version.
func (s *MemoryService) RestoreMemoryVersion(
	userID, teamID, memoryID string, versionNumber int,
) (*models.Memory, error) {
	ctx := context.Background()
	memory, err := s.repo.GetByID(ctx, userID, teamID, memoryID)
	if err != nil {
		return nil, err
	}

	target, err := s.contentVersionSvc.Restore(ctx, memory.TeamID, "memory", memory.ID, versionNumber)
	if err != nil {
		return nil, err
	}

	// A restore is a system-authored edit: the pre-restore text is snapshotted with a
	// default "Restored Version N" summary so the timeline reads clearly.
	restoreSummary := fmt.Sprintf("Restored Version %d", versionNumber)
	return s.applyAndPersistMemoryUpdate(
		ctx, userID, memory, &models.UpdateMemoryRequest{Text: &target},
		models.ActorTypeSystem, &restoreSummary,
	)
}

func (s *MemoryService) DeleteMemory(userID, teamID, memoryID string) error {
	ctx := context.Background()

	// Fetch first to learn who created the memory: delete is own-vs-any (members
	// delete only what they authored; Admin+ delete anyone's), and the repository
	// no longer carries that decision in its SQL. This fetch is also what now
	// surfaces a missing memory.
	memory, err := s.repo.GetByID(ctx, userID, teamID, memoryID)
	if err != nil {
		return err
	}

	if authzErr := s.authz.CanActOnResource(
		ctx, userID, teamID, memory.UserID,
		authz.ResourceDeleteOwn, authz.ResourceDeleteAny,
	); authzErr != nil {
		return authzErr
	}

	if err := s.repo.Delete(ctx, userID, teamID, memoryID); err != nil {
		return err
	}

	s.deleteMemoryComments(ctx, teamID, memoryID)
	s.deleteMemoryRelations(ctx, teamID, memoryID)

	return nil
}

// deleteMemoryComments removes a memory's comments after it is deleted.
// Best-effort: a failure is logged but does not fail the completed delete.
func (s *MemoryService) deleteMemoryComments(ctx context.Context, teamID, memoryID string) {
	if s.commentRepo == nil {
		return
	}
	if _, err := s.commentRepo.DeleteByResource(
		ctx, teamID, models.CommentResourceTypeMemory, memoryID,
	); err != nil {
		s.logger.With(
			"service", "memory",
			"team_id", teamID,
			"memory_id", memoryID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to delete comments for deleted memory")
	}
}

// deleteMemoryRelations removes every relation the memory appears on (either
// endpoint) after it is deleted. Best-effort, like the comment cascade.
func (s *MemoryService) deleteMemoryRelations(ctx context.Context, teamID, memoryID string) {
	if s.relationRepo == nil {
		return
	}
	if _, err := s.relationRepo.DeleteByResource(
		ctx, teamID, models.RelationResourceTypeMemory, memoryID,
	); err != nil {
		s.logger.With(
			"service", "memory",
			"team_id", teamID,
			"memory_id", memoryID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to delete relations for deleted memory")
	}
}

func (s *MemoryService) SearchMemoriesByMetadata(
	userID, metadataKey, metadataValue string, filters MemoryFilters,
) (*models.MemoryListResponse, error) {
	ctx := context.Background()

	if filters.Page == 0 {
		filters.Page = 1
	}
	if filters.Limit == 0 {
		filters.Limit = 50
	}

	repoFilters := repositories.MemoryFilters{
		Search:    filters.Search,
		Status:    filters.Status,
		TeamID:    filters.TeamID,
		ProjectID: filters.ProjectID,
		SortBy:    filters.SortBy,
		SortOrder: filters.SortOrder,
		Page:      filters.Page,
		Limit:     filters.Limit,
	}

	memories, totalCount, err := s.repo.SearchByMetadata(ctx, userID, metadataKey, metadataValue, repoFilters)
	if err != nil {
		return nil, err
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(filters.Limit)))

	return &models.MemoryListResponse{
		Memories:   memories,
		TotalCount: totalCount,
		Page:       filters.Page,
		PerPage:    filters.Limit,
		TotalPages: totalPages,
	}, nil
}

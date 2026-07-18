package activities

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Service implements the ActivityService interface
type Service struct {
	repo          repositories.ActivityRepository
	projectRepo   repositories.ProjectRepository
	promptRepo    repositories.PromptRepository
	artifactRepo  repositories.ArtifactRepository
	userRepo      repositories.UserRepository
	agentRepo     repositories.AgentRepository
	blueprintRepo repositories.BlueprintRepository
	apiKeyRepo    repositories.APIKeyRepository
	memoryRepo    repositories.MemoryRepository
	retentionDays int
}

// ServiceDeps groups the dependencies injected into the activity Service.
type ServiceDeps struct {
	Repo          repositories.ActivityRepository
	ProjectRepo   repositories.ProjectRepository
	PromptRepo    repositories.PromptRepository
	ArtifactRepo  repositories.ArtifactRepository
	UserRepo      repositories.UserRepository
	AgentRepo     repositories.AgentRepository
	BlueprintRepo repositories.BlueprintRepository
	APIKeyRepo    repositories.APIKeyRepository
	MemoryRepo    repositories.MemoryRepository
	RetentionDays int
}

// NewService creates a new activity service
func NewService(deps ServiceDeps) *Service {
	return &Service{
		repo:          deps.Repo,
		projectRepo:   deps.ProjectRepo,
		promptRepo:    deps.PromptRepo,
		artifactRepo:  deps.ArtifactRepo,
		userRepo:      deps.UserRepo,
		agentRepo:     deps.AgentRepo,
		blueprintRepo: deps.BlueprintRepo,
		apiKeyRepo:    deps.APIKeyRepo,
		memoryRepo:    deps.MemoryRepo,
		retentionDays: deps.RetentionDays,
	}
}

// RecordActivity records a new activity
func (s *Service) RecordActivity(ctx context.Context, userID string, req CreateActivityRequest) (*Activity, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)

	// Skip activity recording if repository is not available (e.g., during tests)
	if s.repo == nil {
		logger.Debug("Repository not available, skipping activity recording")
		return nil, fmt.Errorf("repository not available")
	}

	activityID := uuid.New().String()

	activity := &models.Activity{
		ID:           activityID,
		UserID:       userID,
		ActivityType: req.ActivityType,
		EntityType:   req.EntityType,
		EntityID:     req.EntityID,
		SessionID:    req.SessionID,
		Description:  req.Description,
		Metadata:     req.Metadata,
		SourceIP:     req.SourceIP,
		UserAgent:    req.UserAgent,
	}

	err := s.repo.Create(ctx, activity)
	if err != nil {
		logger.With("error", err).Error("Failed to create activity")
		return nil, fmt.Errorf("failed to create activity: %w", err)
	}

	// Convert back to service activity type
	converted := convertModelActivity(*activity)
	serviceActivity := &converted

	logger.With(
		"activity_id", activityID,
		"user_id", userID,
		"activity_type", req.ActivityType,
		"entity_type", req.EntityType,
	).Info("Activity recorded successfully")

	return serviceActivity, nil
}

// RecordAuthActivity records authentication-related activities
func (s *Service) RecordAuthActivity(
	ctx context.Context, userID string, activityType string, sessionID *string,
	metadata map[string]interface{}, sourceIP *string, userAgent *string,
) error {
	description := s.generateAuthActivityDescription(activityType, metadata)

	req := CreateActivityRequest{
		ActivityType: activityType,
		EntityType:   EntityTypeUser,
		EntityID:     &userID,
		SessionID:    sessionID,
		Description:  description,
		Metadata:     metadata,
		SourceIP:     sourceIP,
		UserAgent:    userAgent,
	}

	_, err := s.RecordActivity(ctx, userID, req)
	return err
}

// RecordResourceActivity records resource management activities
func (s *Service) RecordResourceActivity(
	ctx context.Context, userID string, activityType string, entityType string,
	entityID *string, description string, metadata map[string]interface{},
) error {
	req := CreateActivityRequest{
		ActivityType: activityType,
		EntityType:   entityType,
		EntityID:     entityID,
		Description:  description,
		Metadata:     metadata,
	}

	_, err := s.RecordActivity(ctx, userID, req)
	return err
}

// RecordClaudeCodeActivity records Claude Code session activities
func (s *Service) RecordClaudeCodeActivity(
	ctx context.Context, userID string, sessionID string, toolName *string,
	hookEventName string, metadata map[string]interface{},
) error {
	description := s.generateClaudeCodeActivityDescription(hookEventName, toolName, metadata)

	// Determine activity type based on hook event
	activityType := ActivityTypeClaudeCodeSession
	if toolName != nil {
		activityType = ActivityTypeClaudeCodeTool
	}
	if hookEventName == "UserPromptSubmit" {
		activityType = ActivityTypeClaudeCodePrompt
	}

	req := CreateActivityRequest{
		ActivityType: activityType,
		EntityType:   EntityTypeSession,
		EntityID:     &sessionID,
		SessionID:    &sessionID,
		Description:  description,
		Metadata:     metadata,
	}

	_, err := s.RecordActivity(ctx, userID, req)
	return err
}

// GetActivities retrieves activities with filtering and pagination
func (s *Service) GetActivities(ctx context.Context, filters ActivityFilters) (*ActivityListResponse, error) {
	// Convert service filters to repository filters
	repoFilters := repositories.ActivityFilters{
		UserID:       filters.UserID,
		ActivityType: filters.ActivityType,
		EntityType:   filters.EntityType,
		EntityID:     filters.EntityID,
		SessionID:    filters.SessionID,
		Search:       filters.Search,
		DateFrom:     convertTimeToString(filters.DateFrom),
		DateTo:       convertTimeToString(filters.DateTo),
		Limit:        filters.Limit,
		Offset:       filters.Offset,
	}

	response, err := s.repo.List(ctx, repoFilters)
	if err != nil {
		return nil, err
	}

	// Convert models to service types
	serviceActivities := make([]Activity, len(response.Activities))
	for i, modelActivity := range response.Activities {
		serviceActivities[i] = convertModelActivity(modelActivity)
	}

	// Resolve human-readable names (best-effort; a failure must not fail the request).
	userID := ""
	if filters.UserID != nil {
		userID = *filters.UserID
	}
	s.resolveNames(ctx, serviceActivities, userID)

	serviceResponse := &ActivityListResponse{
		Activities: serviceActivities,
		TotalCount: response.TotalCount,
		Page:       response.Page,
		PerPage:    response.PerPage,
		TotalPages: response.TotalPages,
	}

	contextkeys.GetLoggerFromContext(ctx).With(
		"total_count", response.TotalCount,
		"page", response.Page,
		"per_page", response.PerPage,
		"total_pages", response.TotalPages,
		"activities", len(serviceActivities),
	).Debug("Activities retrieved successfully")

	return serviceResponse, nil
}

// GetActivityByID retrieves a specific activity by ID
func (s *Service) GetActivityByID(ctx context.Context, userID string, activityID string) (*Activity, error) {
	modelActivity, err := s.repo.GetByID(ctx, userID, activityID)
	if err != nil {
		return nil, err
	}

	activity := convertModelActivity(*modelActivity)

	// Resolve names for this single activity (best-effort).
	activities := []Activity{activity}
	s.resolveNames(ctx, activities, userID)

	return &activities[0], nil
}

// GetActivityStats retrieves activity statistics
func (s *Service) GetActivityStats(ctx context.Context, userID string) (*ActivityStatsResponse, error) {
	modelStats, err := s.repo.GetStats(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Convert models to service types
	serviceActivities := make([]Activity, len(modelStats.RecentActivities))
	for i, modelActivity := range modelStats.RecentActivities {
		serviceActivities[i] = convertModelActivity(modelActivity)
	}

	// Resolve names for recent activities (best-effort).
	s.resolveNames(ctx, serviceActivities, userID)

	// Convert ActivityTypeCount
	serviceTopActivityTypes := make([]ActivityTypeCount, len(modelStats.TopActivityTypes))
	for i, modelItem := range modelStats.TopActivityTypes {
		serviceTopActivityTypes[i] = ActivityTypeCount{
			ActivityType: modelItem.ActivityType,
			Count:        modelItem.Count,
		}
	}

	// Convert EntityTypeCount
	serviceTopEntityTypes := make([]EntityTypeCount, len(modelStats.TopEntityTypes))
	for i, modelItem := range modelStats.TopEntityTypes {
		serviceTopEntityTypes[i] = EntityTypeCount{
			EntityType: modelItem.EntityType,
			Count:      modelItem.Count,
		}
	}

	// Convert ActivityCountByDate
	serviceActivitiesByDate := make([]ActivityCountByDate, len(modelStats.ActivitiesByDateWeek))
	for i, modelItem := range modelStats.ActivitiesByDateWeek {
		serviceActivitiesByDate[i] = ActivityCountByDate{
			Date:  modelItem.Date,
			Count: modelItem.Count,
		}
	}

	stats := &ActivityStatsResponse{
		TotalActivities:      modelStats.TotalActivities,
		ActivitiesToday:      modelStats.ActivitiesToday,
		ActivitiesThisWeek:   modelStats.ActivitiesThisWeek,
		TopActivityTypes:     serviceTopActivityTypes,
		TopEntityTypes:       serviceTopEntityTypes,
		RecentActivities:     serviceActivities,
		ActivitiesByDateWeek: serviceActivitiesByDate,
	}

	return stats, nil
}

// DeleteActivity deletes an activity (admin only)
func (s *Service) DeleteActivity(ctx context.Context, activityID string) error {
	err := s.repo.Delete(ctx, activityID)
	if err != nil {
		return err
	}

	contextkeys.GetLoggerFromContext(ctx).With("activity_id", activityID).Info("Activity deleted successfully")
	return nil
}

// GetActivityTypes returns all available activity types
func (s *Service) GetActivityTypes() []string {
	return []string{
		ActivityTypeAuthLogin,
		ActivityTypeAuthLogout,
		ActivityTypeAuthFailure,
		ActivityTypeTokenRefresh,
		ActivityTypeAPIKeyCreated,
		ActivityTypeAPIKeyDeleted,
		ActivityTypeAPIKeyUsed,
		ActivityTypePromptCreated,
		ActivityTypePromptUpdated,
		ActivityTypePromptDeleted,
		ActivityTypeClaudeCodeSession,
		ActivityTypeClaudeCodeTool,
		ActivityTypeClaudeCodePrompt,
		ActivityTypeSystemError,
		ActivityTypeSystemWarning,
		ActivityTypeSystemInfo,
	}
}

// GetEntityTypes returns all available entity types
func (s *Service) GetEntityTypes() []string {
	return []string{
		EntityTypeUser,
		EntityTypeAPIKey,
		EntityTypePrompt,
		EntityTypeSession,
		EntityTypeSystem,
	}
}

// GetAllTypes returns both activity types and entity types
func (s *Service) GetAllTypes() *ActivityTypesResponse {
	return &ActivityTypesResponse{
		ActivityTypes: s.GetActivityTypes(),
		EntityTypes:   s.GetEntityTypes(),
	}
}

// RunRetentionJob deletes activities older than the configured retention window.
// Called via HTTP from Cloud Scheduler.
func (s *Service) RunRetentionJob(ctx context.Context) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -s.retentionDays)

	count, err := s.repo.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("run activity retention job: %w", err)
	}

	contextkeys.GetLoggerFromContext(ctx).With(
		"deleted_count", count,
		"older_than", cutoff.Format(time.RFC3339),
		"retention_days", s.retentionDays,
	).Info("Activity retention job completed")

	return nil
}

// Helper functions

// convertModelActivity converts a models.Activity (DB layer) to an activities.Activity (service layer).
func convertModelActivity(m models.Activity) Activity {
	return Activity{
		ID:           m.ID,
		UserID:       m.UserID,
		ActivityType: m.ActivityType,
		EntityType:   m.EntityType,
		EntityID:     m.EntityID,
		SessionID:    m.SessionID,
		Description:  m.Description,
		Metadata:     m.Metadata,
		SourceIP:     m.SourceIP,
		UserAgent:    m.UserAgent,
		CreatedAt:    m.CreatedAt,
	}
}

// resolveNames populates EntityName and ActorName on each activity in-place.
// It performs one batch query per entity type that appears in the slice, plus one
// batch query for actor names — zero N+1 queries.
// Failures are logged and silently swallowed so that a name-lookup error never
// prevents the caller from returning activity data.
func (s *Service) resolveNames(ctx context.Context, activities []Activity, userID string) {
	if len(activities) == 0 {
		return
	}

	entityNames := s.fetchEntityNames(ctx, activities, userID)
	actorNames := s.fetchActorNames(ctx, activities)
	applyNames(activities, entityNames, actorNames)
}

// fetchEntityNames groups entity IDs by type, makes one batch call per type,
// and returns a merged id→name map. Lookup errors are logged and skipped.
func (s *Service) fetchEntityNames(
	ctx context.Context, activities []Activity, userID string,
) map[string]string {
	logger := contextkeys.GetLoggerFromContext(ctx)

	byType := make(map[string][]string)
	for i := range activities {
		a := &activities[i]
		if a.EntityID == nil || *a.EntityID == "" {
			continue
		}
		// Skip non-UUID entity IDs (e.g. legacy slug or composite "<projectID>/<slug>"
		// values recorded before the recording-site fix). These would cause a Postgres
		// "invalid input syntax for type uuid" error that silences the entire batch.
		if _, err := uuid.Parse(*a.EntityID); err != nil {
			continue
		}
		switch a.EntityType {
		case EntityTypeProject, EntityTypePrompt, EntityTypeArtifact,
			EntityTypeUser, EntityTypeAPIKey, EntityTypeAgent,
			EntityTypeMemory, EntityTypeBlueprint:
			byType[a.EntityType] = append(byType[a.EntityType], *a.EntityID)
		}
	}

	result := make(map[string]string)
	mergeInto(result, s.lookupProjectNames(ctx, logger, userID, byType[EntityTypeProject]))
	mergeInto(result, s.lookupPromptNames(ctx, logger, userID, byType[EntityTypePrompt]))
	mergeInto(result, s.lookupArtifactNames(ctx, logger, userID, byType[EntityTypeArtifact]))
	mergeInto(result, s.lookupUserEntityNames(ctx, logger, byType[EntityTypeUser]))
	mergeInto(result, s.lookupAPIKeyNames(ctx, logger, userID, byType[EntityTypeAPIKey]))
	mergeInto(result, s.lookupAgentNames(ctx, logger, userID, byType[EntityTypeAgent]))
	mergeInto(result, s.lookupMemoryNames(ctx, logger, userID, byType[EntityTypeMemory]))
	mergeInto(result, s.lookupBlueprintNames(ctx, logger, userID, byType[EntityTypeBlueprint]))
	return result
}

func (s *Service) lookupProjectNames(
	ctx context.Context, logger *slog.Logger, userID string, ids []string,
) map[string]string {
	if len(ids) == 0 || s.projectRepo == nil {
		return nil
	}
	names, err := s.projectRepo.GetNamesByIDs(ctx, userID, lo.Uniq(ids))
	if err != nil {
		logger.With("error", err).Warn("failed to resolve project names for activities")
		return nil
	}
	return names
}

func (s *Service) lookupPromptNames(
	ctx context.Context, logger *slog.Logger, userID string, ids []string,
) map[string]string {
	if len(ids) == 0 || s.promptRepo == nil {
		return nil
	}
	names, err := s.promptRepo.GetNamesByIDsCrossTeam(ctx, userID, lo.Uniq(ids))
	if err != nil {
		logger.With("error", err).Warn("failed to resolve prompt names for activities")
		return nil
	}
	return names
}

func (s *Service) lookupArtifactNames(
	ctx context.Context, logger *slog.Logger, userID string, ids []string,
) map[string]string {
	if len(ids) == 0 || s.artifactRepo == nil {
		return nil
	}
	names, err := s.artifactRepo.GetNamesByIDsCrossTeam(ctx, userID, lo.Uniq(ids))
	if err != nil {
		logger.With("error", err).Warn("failed to resolve artifact names for activities")
		return nil
	}
	return names
}

// lookupUserEntityNames resolves user display names when the entity type is "user"
// (e.g. auth_login / auth_logout rows). This is distinct from actor name resolution:
// here the entity_id IS a user ID and we want to show that user's display name as the
// EntityName field, not the ActorName field.
func (s *Service) lookupUserEntityNames(
	ctx context.Context, logger *slog.Logger, ids []string,
) map[string]string {
	if len(ids) == 0 || s.userRepo == nil {
		return nil
	}
	names, err := s.userRepo.GetNamesByIDs(ctx, lo.Uniq(ids))
	if err != nil {
		logger.With("error", err).Warn("failed to resolve user entity names for activities")
		return nil
	}
	return names
}

func (s *Service) lookupAPIKeyNames(
	ctx context.Context, logger *slog.Logger, userID string, ids []string,
) map[string]string {
	if len(ids) == 0 || s.apiKeyRepo == nil {
		return nil
	}
	names, err := s.apiKeyRepo.GetNamesByIDs(ctx, userID, lo.Uniq(ids))
	if err != nil {
		logger.With("error", err).Warn("failed to resolve api key names for activities")
		return nil
	}
	return names
}

func (s *Service) lookupAgentNames(
	ctx context.Context, logger *slog.Logger, userID string, ids []string,
) map[string]string {
	if len(ids) == 0 || s.agentRepo == nil {
		return nil
	}
	names, err := s.agentRepo.GetNamesByIDsCrossTeam(ctx, userID, lo.Uniq(ids))
	if err != nil {
		logger.With("error", err).Warn("failed to resolve agent names for activities")
		return nil
	}
	return names
}

func (s *Service) lookupMemoryNames(
	ctx context.Context, logger *slog.Logger, userID string, ids []string,
) map[string]string {
	if len(ids) == 0 || s.memoryRepo == nil {
		return nil
	}
	names, err := s.memoryRepo.GetNamesByIDsCrossTeam(ctx, userID, lo.Uniq(ids))
	if err != nil {
		logger.With("error", err).Warn("failed to resolve memory names for activities")
		return nil
	}
	return names
}

func (s *Service) lookupBlueprintNames(
	ctx context.Context, logger *slog.Logger, userID string, ids []string,
) map[string]string {
	if len(ids) == 0 || s.blueprintRepo == nil {
		return nil
	}
	names, err := s.blueprintRepo.GetNamesByIDsCrossTeam(ctx, userID, lo.Uniq(ids))
	if err != nil {
		logger.With("error", err).Warn("failed to resolve blueprint names for activities")
		return nil
	}
	return names
}

// fetchActorNames collects unique actor IDs across all activities and makes a
// single batch call to the user repository. Errors are logged and skipped.
func (s *Service) fetchActorNames(ctx context.Context, activities []Activity) map[string]string {
	if s.userRepo == nil {
		return nil
	}

	seen := make(map[string]struct{})
	for _, a := range activities {
		if a.UserID != "" {
			seen[a.UserID] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}

	actorIDs := make([]string, 0, len(seen))
	for id := range seen {
		actorIDs = append(actorIDs, id)
	}

	names, err := s.userRepo.GetNamesByIDs(ctx, actorIDs)
	if err != nil {
		contextkeys.GetLoggerFromContext(ctx).With("error", err).Warn("failed to resolve actor names for activities")
		return nil
	}
	return names
}

// applyNames writes entity and actor names onto each activity record.
func applyNames(activities []Activity, entityNames, actorNames map[string]string) {
	for i := range activities {
		a := &activities[i]
		setEntityName(a, entityNames)
		if actorNames != nil {
			if name, ok := actorNames[a.UserID]; ok {
				a.ActorName = &name
			}
		}
	}
}

// setEntityName resolves and assigns EntityName for a single activity.
// When the entity ID is present but not found in the name map (deleted entity),
// a short fallback label is used for the resolvable entity types.
func setEntityName(a *Activity, entityNames map[string]string) {
	if a.EntityID == nil || *a.EntityID == "" {
		return
	}
	if name, ok := entityNames[*a.EntityID]; ok {
		a.EntityName = &name
		return
	}
	if isResolvableEntityType(a.EntityType) {
		fallback := truncateEntityID(*a.EntityID) + "… (deleted)"
		a.EntityName = &fallback
	}
}

// isResolvableEntityType reports whether the given entity type supports name lookup.
func isResolvableEntityType(entityType string) bool {
	switch entityType {
	case EntityTypeProject, EntityTypePrompt, EntityTypeArtifact,
		EntityTypeUser, EntityTypeAPIKey, EntityTypeAgent,
		EntityTypeMemory, EntityTypeBlueprint:
		return true
	default:
		return false
	}
}

// mergeInto copies all entries from src into dst, ignoring nil src.
func mergeInto(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

// truncateEntityID returns up to the first 8 characters of an entity ID.
func truncateEntityID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// convertTimeToString converts *time.Time to *string for repository use
func convertTimeToString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	str := t.Format(time.RFC3339)
	return &str
}

func (s *Service) generateAuthActivityDescription(activityType string, metadata map[string]interface{}) string {
	switch activityType {
	case ActivityTypeAuthLogin:
		return "User successfully logged in"
	case ActivityTypeAuthLogout:
		return "User logged out"
	case ActivityTypeAuthFailure:
		if reason, ok := metadata["reason"].(string); ok {
			return fmt.Sprintf("Authentication failed: %s", reason)
		}
		return "Authentication failed"
	case ActivityTypeTokenRefresh:
		return "Authentication token refreshed"
	default:
		return fmt.Sprintf("Authentication activity: %s", activityType)
	}
}

func (s *Service) generateClaudeCodeActivityDescription(
	hookEventName string, toolName *string, _ map[string]interface{},
) string {
	switch hookEventName {
	case "UserPromptSubmit":
		return "User submitted a prompt to Claude Code"
	case "ToolUse":
		if toolName != nil {
			return fmt.Sprintf("Used Claude Code tool: %s", *toolName)
		}
		return "Used Claude Code tool"
	case "SessionStart":
		return "Started Claude Code session"
	case "SessionEnd":
		return "Ended Claude Code session"
	default:
		if toolName != nil {
			return fmt.Sprintf("Claude Code %s with tool: %s", hookEventName, *toolName)
		}
		return fmt.Sprintf("Claude Code %s", hookEventName)
	}
}

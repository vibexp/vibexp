package events

import "time"

// Event type constants
const (
	// User events
	EventTypeUserCreated = "user.created"
	EventTypeUserUpdated = "user.updated"

	// Prompt events
	EventTypePromptCreated = "prompt.created"
	EventTypePromptUpdated = "prompt.updated"

	// Artifact events
	EventTypeArtifactCreated = "artifact.created"
	EventTypeArtifactUpdated = "artifact.updated"

	// Memory events
	EventTypeMemoryCreated = "memory.created"
	EventTypeMemoryUpdated = "memory.updated"

	// Blueprint events
	EventTypeBlueprintCreated = "blueprint.created"
	EventTypeBlueprintUpdated = "blueprint.updated"

	// Project events
	EventTypeProjectCreated = "project.created"
	EventTypeProjectUpdated = "project.updated"
	EventTypeProjectDeleted = "project.deleted"

	// Resource usage events
	EventTypeResourceCreated = "resource.created"
	EventTypeResourceDeleted = "resource.deleted"

	// AI Tool Session events
	EventTypeAIToolSessionCreated = "ai_tool_session.created"

	// Feed events
	EventTypeFeedItemCreated      = "feed_item.created"
	EventTypeFeedItemReplyCreated = "feed_item_reply.created"
)

// Resource type constants
const (
	ResourceTypeAITool    = "ai_tool"
	ResourceTypeAISession = "ai_session"
	ResourceTypePrompt    = "prompt"
	ResourceTypeArtifact  = "artifact"
	ResourceTypeMemory    = "memory"
	ResourceTypeAgent     = "agent"
	ResourceTypeAgentConv = "agent_conversation"
	ResourceTypeBlueprint = "blueprint"
	ResourceTypeProject   = "project"
	ResourceTypeTeam      = "team"
	ResourceTypeFeed      = "feed"
	ResourceTypeFeedItem  = "feed_item"
)

// UserCreatedPayload represents the payload for user.created event
type UserCreatedPayload struct {
	UserID    string
	Email     string
	Name      string
	CreatedAt time.Time
}

// UserCreatedEvent represents a user creation event
type UserCreatedEvent struct {
	*BaseEvent
}

// NewUserCreatedEvent creates a new user created event
func NewUserCreatedEvent(userID, email, name string, createdAt time.Time) Event {
	payload := &UserCreatedPayload{
		UserID:    userID,
		Email:     email,
		Name:      name,
		CreatedAt: createdAt,
	}
	return &UserCreatedEvent{
		BaseEvent: NewBaseEvent(EventTypeUserCreated, payload, userID),
	}
}

// UserUpdatedPayload represents the payload for user.updated event
type UserUpdatedPayload struct {
	UserID    string
	Email     string
	Name      string
	UpdatedAt time.Time
}

// UserUpdatedEvent represents a user update event
type UserUpdatedEvent struct {
	*BaseEvent
}

// NewUserUpdatedEvent creates a new user updated event
func NewUserUpdatedEvent(userID, email, name string, updatedAt time.Time) Event {
	payload := &UserUpdatedPayload{
		UserID:    userID,
		Email:     email,
		Name:      name,
		UpdatedAt: updatedAt,
	}
	return &UserUpdatedEvent{
		BaseEvent: NewBaseEvent(EventTypeUserUpdated, payload, userID),
	}
}

// PromptCreatedPayload represents the payload for prompt.created event
type PromptCreatedPayload struct {
	PromptID    string
	UserID      string
	Email       string // User email for the event payload
	ProjectName string
	Slug        string
	Title       string
	Description string // Short summary embedded as part of the per-chunk context header
	Body        string // Rendered body with all references resolved
	CreatedAt   time.Time
}

// PromptCreatedEvent represents a prompt creation event
type PromptCreatedEvent struct {
	*BaseEvent
}

// NewPromptCreatedEvent creates a new prompt created event
func NewPromptCreatedEvent(
	promptID, userID, email, projectName, slug, title, description, body string, createdAt time.Time,
) Event {
	payload := &PromptCreatedPayload{
		PromptID:    promptID,
		UserID:      userID,
		Email:       email,
		ProjectName: projectName,
		Slug:        slug,
		Title:       title,
		Description: description,
		Body:        body,
		CreatedAt:   createdAt,
	}
	return &PromptCreatedEvent{
		BaseEvent: NewBaseEvent(EventTypePromptCreated, payload, userID),
	}
}

// PromptUpdatedPayload represents the payload for prompt.updated event
type PromptUpdatedPayload struct {
	PromptID    string
	UserID      string
	ProjectName string
	Slug        string
	Title       string
	Description string // Short summary embedded as part of the per-chunk context header
	Body        string // Rendered body with all references resolved
	UpdatedAt   time.Time
}

// PromptUpdatedEvent represents a prompt update event
type PromptUpdatedEvent struct {
	*BaseEvent
}

// NewPromptUpdatedEvent creates a new prompt updated event
func NewPromptUpdatedEvent(
	promptID, userID, projectName, slug, title, description, body string, updatedAt time.Time,
) Event {
	payload := &PromptUpdatedPayload{
		PromptID:    promptID,
		UserID:      userID,
		ProjectName: projectName,
		Slug:        slug,
		Title:       title,
		Description: description,
		Body:        body,
		UpdatedAt:   updatedAt,
	}
	return &PromptUpdatedEvent{
		BaseEvent: NewBaseEvent(EventTypePromptUpdated, payload, userID),
	}
}

// ArtifactCreatedPayload represents the payload for artifact.created event
type ArtifactCreatedPayload struct {
	ArtifactID  string
	UserID      string
	ProjectName string
	Slug        string
	Title       string
	Description string // Short summary embedded as part of the per-chunk context header
	Type        string
	Body        string // Artifact content used by the embedding pipeline
	CreatedAt   time.Time
}

// ArtifactCreatedEvent represents an artifact creation event
type ArtifactCreatedEvent struct {
	*BaseEvent
}

// NewArtifactCreatedEvent creates a new artifact created event
func NewArtifactCreatedEvent(
	artifactID, userID, projectName, slug, title, description, artifactType, body string, createdAt time.Time,
) Event {
	payload := &ArtifactCreatedPayload{
		ArtifactID:  artifactID,
		UserID:      userID,
		ProjectName: projectName,
		Slug:        slug,
		Title:       title,
		Description: description,
		Type:        artifactType,
		Body:        body,
		CreatedAt:   createdAt,
	}
	return &ArtifactCreatedEvent{
		BaseEvent: NewBaseEvent(EventTypeArtifactCreated, payload, userID),
	}
}

// ArtifactUpdatedPayload represents the payload for artifact.updated event
type ArtifactUpdatedPayload struct {
	ArtifactID  string
	UserID      string
	ProjectName string
	Slug        string
	Title       string
	Description string // Short summary embedded as part of the per-chunk context header
	Type        string
	Body        string // Artifact content used by the embedding pipeline
	UpdatedAt   time.Time
}

// ArtifactUpdatedEvent represents an artifact update event
type ArtifactUpdatedEvent struct {
	*BaseEvent
}

// NewArtifactUpdatedEvent creates a new artifact updated event
func NewArtifactUpdatedEvent(
	artifactID, userID, projectName, slug, title, description, artifactType, body string, updatedAt time.Time,
) Event {
	payload := &ArtifactUpdatedPayload{
		ArtifactID:  artifactID,
		UserID:      userID,
		ProjectName: projectName,
		Slug:        slug,
		Title:       title,
		Description: description,
		Type:        artifactType,
		Body:        body,
		UpdatedAt:   updatedAt,
	}
	return &ArtifactUpdatedEvent{
		BaseEvent: NewBaseEvent(EventTypeArtifactUpdated, payload, userID),
	}
}

// MemoryCreatedPayload represents the payload for memory.created event
type MemoryCreatedPayload struct {
	MemoryID    string
	UserID      string
	ProjectName string
	Text        string
	CreatedAt   time.Time
}

// MemoryCreatedEvent represents a memory creation event
type MemoryCreatedEvent struct {
	*BaseEvent
}

// NewMemoryCreatedEvent creates a new memory created event
func NewMemoryCreatedEvent(memoryID, userID, projectName, text string, createdAt time.Time) Event {
	payload := &MemoryCreatedPayload{
		MemoryID:    memoryID,
		UserID:      userID,
		ProjectName: projectName,
		Text:        text,
		CreatedAt:   createdAt,
	}
	return &MemoryCreatedEvent{
		BaseEvent: NewBaseEvent(EventTypeMemoryCreated, payload, userID),
	}
}

// MemoryUpdatedPayload represents the payload for memory.updated event
type MemoryUpdatedPayload struct {
	MemoryID    string
	UserID      string
	ProjectName string
	Text        string
	UpdatedAt   time.Time
}

// MemoryUpdatedEvent represents a memory update event
type MemoryUpdatedEvent struct {
	*BaseEvent
}

// NewMemoryUpdatedEvent creates a new memory updated event
func NewMemoryUpdatedEvent(memoryID, userID, projectName, text string, updatedAt time.Time) Event {
	payload := &MemoryUpdatedPayload{
		MemoryID:    memoryID,
		UserID:      userID,
		ProjectName: projectName,
		Text:        text,
		UpdatedAt:   updatedAt,
	}
	return &MemoryUpdatedEvent{
		BaseEvent: NewBaseEvent(EventTypeMemoryUpdated, payload, userID),
	}
}

// ResourceUsagePayload represents payload for resource.created/deleted events
type ResourceUsagePayload struct {
	UserID       string    `json:"user_id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Timestamp    time.Time `json:"timestamp"`
}

// ResourceCreatedEvent represents a resource creation event
type ResourceCreatedEvent struct {
	*BaseEvent
}

// NewResourceCreatedEvent creates a new resource created event
func NewResourceCreatedEvent(userID, resourceType, resourceID string) Event {
	payload := ResourceUsagePayload{
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Timestamp:    time.Now(),
	}
	// Using a concrete value instead of a pointer to ensure better serialization/deserialization
	return &ResourceCreatedEvent{
		BaseEvent: NewBaseEvent(EventTypeResourceCreated, payload, userID),
	}
}

// ResourceDeletedEvent represents a resource deletion event
type ResourceDeletedEvent struct {
	*BaseEvent
}

// NewResourceDeletedEvent creates a new resource deleted event
func NewResourceDeletedEvent(userID, resourceType, resourceID string) Event {
	payload := ResourceUsagePayload{
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Timestamp:    time.Now(),
	}
	// Using a concrete value instead of a pointer to ensure better serialization/deserialization
	return &ResourceDeletedEvent{
		BaseEvent: NewBaseEvent(EventTypeResourceDeleted, payload, userID),
	}
}

// AIToolSessionCreatedPayload represents payload for ai_tool_session.created events
type AIToolSessionCreatedPayload struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"` // User email for the event payload
	SessionID string    `json:"session_id"`
	ToolType  string    `json:"tool_type"` // "claude_code_cli" or "cursor_ide"
	IsNewTool bool      `json:"is_new_tool"`
	CreatedAt time.Time `json:"created_at"`
}

// AIToolSessionCreatedEvent represents an AI tool session creation event
type AIToolSessionCreatedEvent struct {
	*BaseEvent
}

// NewAIToolSessionCreatedEvent creates a new AI tool session created event
func NewAIToolSessionCreatedEvent(userID, email, sessionID, toolType string, isNewTool bool) Event {
	payload := &AIToolSessionCreatedPayload{
		UserID:    userID,
		Email:     email,
		SessionID: sessionID,
		ToolType:  toolType,
		IsNewTool: isNewTool,
		CreatedAt: time.Now(),
	}
	return &AIToolSessionCreatedEvent{
		BaseEvent: NewBaseEvent(EventTypeAIToolSessionCreated, payload, userID),
	}
}

// BlueprintCreatedPayload represents the payload for blueprint.created event
type BlueprintCreatedPayload struct {
	BlueprintID string
	UserID      string
	ProjectName string
	Slug        string
	Title       string
	Description string // Short summary embedded as part of the per-chunk context header
	Type        string
	Body        string // Blueprint content used by the embedding pipeline
	CreatedAt   time.Time
}

// BlueprintCreatedEvent represents a spec library creation event
type BlueprintCreatedEvent struct {
	*BaseEvent
}

// NewBlueprintCreatedEvent creates a new spec library created event
func NewBlueprintCreatedEvent(
	blueprintID, userID, projectName, slug, title, description, blueprintType, body string, createdAt time.Time,
) Event {
	payload := &BlueprintCreatedPayload{
		BlueprintID: blueprintID,
		UserID:      userID,
		ProjectName: projectName,
		Slug:        slug,
		Title:       title,
		Description: description,
		Type:        blueprintType,
		Body:        body,
		CreatedAt:   createdAt,
	}
	return &BlueprintCreatedEvent{
		BaseEvent: NewBaseEvent(EventTypeBlueprintCreated, payload, userID),
	}
}

// BlueprintUpdatedPayload represents the payload for blueprint.updated event
type BlueprintUpdatedPayload struct {
	BlueprintID string
	UserID      string
	ProjectName string
	Slug        string
	Title       string
	Description string // Short summary embedded as part of the per-chunk context header
	Type        string
	Body        string // Blueprint content used by the embedding pipeline
	UpdatedAt   time.Time
}

// BlueprintUpdatedEvent represents a spec library update event
type BlueprintUpdatedEvent struct {
	*BaseEvent
}

// NewBlueprintUpdatedEvent creates a new spec library updated event
func NewBlueprintUpdatedEvent(
	blueprintID, userID, projectName, slug, title, description, blueprintType, body string, updatedAt time.Time,
) Event {
	payload := &BlueprintUpdatedPayload{
		BlueprintID: blueprintID,
		UserID:      userID,
		ProjectName: projectName,
		Slug:        slug,
		Title:       title,
		Description: description,
		Type:        blueprintType,
		Body:        body,
		UpdatedAt:   updatedAt,
	}
	return &BlueprintUpdatedEvent{
		BaseEvent: NewBaseEvent(EventTypeBlueprintUpdated, payload, userID),
	}
}

// ProjectCreatedPayload represents the payload for project.created event
type ProjectCreatedPayload struct {
	ProjectID   string
	UserID      string
	Name        string
	Slug        string
	Description string
	GitURL      string
	Homepage    string
	CreatedAt   time.Time
}

// ProjectCreatedEvent represents a project creation event
type ProjectCreatedEvent struct {
	*BaseEvent
}

// NewProjectCreatedEvent creates a new project created event
func NewProjectCreatedEvent(
	projectID, userID, name, slug, description, gitURL, homepage string, createdAt time.Time,
) Event {
	payload := &ProjectCreatedPayload{
		ProjectID:   projectID,
		UserID:      userID,
		Name:        name,
		Slug:        slug,
		Description: description,
		GitURL:      gitURL,
		Homepage:    homepage,
		CreatedAt:   createdAt,
	}
	return &ProjectCreatedEvent{
		BaseEvent: NewBaseEvent(EventTypeProjectCreated, payload, userID),
	}
}

// ProjectUpdatedPayload represents the payload for project.updated event
type ProjectUpdatedPayload struct {
	ProjectID   string
	UserID      string
	Name        string
	Slug        string
	Description string
	GitURL      string
	Homepage    string
	UpdatedAt   time.Time
}

// ProjectUpdatedEvent represents a project update event
type ProjectUpdatedEvent struct {
	*BaseEvent
}

// NewProjectUpdatedEvent creates a new project updated event
func NewProjectUpdatedEvent(
	projectID, userID, name, slug, description, gitURL, homepage string, updatedAt time.Time,
) Event {
	payload := &ProjectUpdatedPayload{
		ProjectID:   projectID,
		UserID:      userID,
		Name:        name,
		Slug:        slug,
		Description: description,
		GitURL:      gitURL,
		Homepage:    homepage,
		UpdatedAt:   updatedAt,
	}
	return &ProjectUpdatedEvent{
		BaseEvent: NewBaseEvent(EventTypeProjectUpdated, payload, userID),
	}
}

// ProjectDeletedPayload represents the payload for project.deleted event
type ProjectDeletedPayload struct {
	ProjectID string
	UserID    string
	Slug      string
	DeletedAt time.Time
}

// ProjectDeletedEvent represents a project deletion event
type ProjectDeletedEvent struct {
	*BaseEvent
}

// NewProjectDeletedEvent creates a new project deleted event
func NewProjectDeletedEvent(projectID, userID, slug string, deletedAt time.Time) Event {
	payload := &ProjectDeletedPayload{
		ProjectID: projectID,
		UserID:    userID,
		Slug:      slug,
		DeletedAt: deletedAt,
	}
	return &ProjectDeletedEvent{
		BaseEvent: NewBaseEvent(EventTypeProjectDeleted, payload, userID),
	}
}

// FeedItemCreatedPayload represents the payload for feed_item.created event.
// UserID holds the feed item's PostedByUserID and is the key the embedding
// pipeline uses to write the entity's embedding row.
type FeedItemCreatedPayload struct {
	ItemID   string    `json:"item_id"`
	UserID   string    `json:"user_id"`
	TeamID   string    `json:"team_id"`
	FeedID   string    `json:"feed_id"`
	Title    string    `json:"title"`
	Content  string    `json:"content"` // Feed item content used by the embedding pipeline
	Excerpt  string    `json:"excerpt"`
	PostedAt time.Time `json:"posted_at"`
}

// FeedItemCreatedEvent represents a feed item creation event
type FeedItemCreatedEvent struct {
	*BaseEvent
}

// NewFeedItemCreatedEvent creates a new feed item created event. userID is the
// item's PostedByUserID; it keys both the event and the resulting embedding row.
func NewFeedItemCreatedEvent(
	itemID, userID, teamID, feedID, title, content, excerpt string, postedAt time.Time,
) Event {
	payload := &FeedItemCreatedPayload{
		ItemID:   itemID,
		UserID:   userID,
		TeamID:   teamID,
		FeedID:   feedID,
		Title:    title,
		Content:  content,
		Excerpt:  excerpt,
		PostedAt: postedAt,
	}
	return &FeedItemCreatedEvent{
		BaseEvent: NewBaseEvent(EventTypeFeedItemCreated, payload, userID),
	}
}

// FeedItemReplyCreatedPayload represents the payload for feed_item_reply.created event.
// UserID holds the reply's PostedByUserID and is the key the embedding pipeline uses
// to write the reply's embedding row. TeamID is carried for future team-keying
// (#1363) but is NOT persisted to the embedding row today.
type FeedItemReplyCreatedPayload struct {
	ReplyID    string    `json:"reply_id"`
	UserID     string    `json:"user_id"`
	TeamID     string    `json:"team_id"`
	FeedItemID string    `json:"feed_item_id"`
	Content    string    `json:"content"` // Reply content used by the embedding pipeline
	PostedAt   time.Time `json:"posted_at"`
}

// FeedItemReplyCreatedEvent represents a feed item reply creation event
type FeedItemReplyCreatedEvent struct {
	*BaseEvent
}

// NewFeedItemReplyCreatedEvent creates a new feed item reply created event. userID is
// the reply's PostedByUserID; it keys both the event and the resulting embedding row.
func NewFeedItemReplyCreatedEvent(
	replyID, userID, teamID, feedItemID, content string, postedAt time.Time,
) Event {
	payload := &FeedItemReplyCreatedPayload{
		ReplyID:    replyID,
		UserID:     userID,
		TeamID:     teamID,
		FeedItemID: feedItemID,
		Content:    content,
		PostedAt:   postedAt,
	}
	return &FeedItemReplyCreatedEvent{
		BaseEvent: NewBaseEvent(EventTypeFeedItemReplyCreated, payload, userID),
	}
}

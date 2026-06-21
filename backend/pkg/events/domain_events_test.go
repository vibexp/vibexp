package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUserCreatedEvent(t *testing.T) {
	userID := "user123"
	email := "test@example.com"
	name := "Test User"
	createdAt := time.Now()

	event := NewUserCreatedEvent(userID, email, name, createdAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypeUserCreated, event.Type())
	assert.Equal(t, userID, event.UserID())

	payload, ok := event.Payload().(*UserCreatedPayload)
	assert.True(t, ok)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, email, payload.Email)
	assert.Equal(t, name, payload.Name)
	assert.Equal(t, createdAt, payload.CreatedAt)
}

func TestUserUpdatedEvent(t *testing.T) {
	userID := "user123"
	email := "updated@example.com"
	name := "Updated User"
	updatedAt := time.Now()

	event := NewUserUpdatedEvent(userID, email, name, updatedAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypeUserUpdated, event.Type())
	assert.Equal(t, userID, event.UserID())

	payload, ok := event.Payload().(*UserUpdatedPayload)
	assert.True(t, ok)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, email, payload.Email)
	assert.Equal(t, name, payload.Name)
	assert.Equal(t, updatedAt, payload.UpdatedAt)
}

func TestPromptCreatedEvent(t *testing.T) {
	promptID := "prompt123"
	userID := "user123"
	projectName := "test-project"
	slug := "test-slug"
	email := "test@example.com"
	title := "Test Prompt"
	body := "This is the rendered prompt body"
	createdAt := time.Now()

	event := NewPromptCreatedEvent(promptID, userID, email, projectName, slug, title, body, createdAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypePromptCreated, event.Type())
	assert.Equal(t, userID, event.UserID())

	payload, ok := event.Payload().(*PromptCreatedPayload)
	assert.True(t, ok)
	assert.Equal(t, promptID, payload.PromptID)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, projectName, payload.ProjectName)
	assert.Equal(t, slug, payload.Slug)
	assert.Equal(t, title, payload.Title)
	assert.Equal(t, body, payload.Body)
	assert.Equal(t, createdAt, payload.CreatedAt)
}

func TestPromptUpdatedEvent(t *testing.T) {
	promptID := "prompt123"
	userID := "user123"
	projectName := "test-project"
	slug := "test-slug"
	title := "Updated Prompt"
	body := "This is the updated rendered prompt body"
	updatedAt := time.Now()

	event := NewPromptUpdatedEvent(promptID, userID, projectName, slug, title, body, updatedAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypePromptUpdated, event.Type())
	assert.Equal(t, userID, event.UserID())

	payload, ok := event.Payload().(*PromptUpdatedPayload)
	assert.True(t, ok)
	assert.Equal(t, promptID, payload.PromptID)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, projectName, payload.ProjectName)
	assert.Equal(t, slug, payload.Slug)
	assert.Equal(t, title, payload.Title)
	assert.Equal(t, body, payload.Body)
	assert.Equal(t, updatedAt, payload.UpdatedAt)
}

func TestArtifactCreatedEvent(t *testing.T) {
	artifactID := "artifact123"
	userID := "user123"
	projectName := "test-project"
	slug := "test-slug"
	title := "Test Artifact"
	artifactType := "general"
	body := "This is the artifact content used for embeddings"
	createdAt := time.Now()

	event := NewArtifactCreatedEvent(artifactID, userID, projectName, slug, title, artifactType, body, createdAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypeArtifactCreated, event.Type())
	assert.Equal(t, userID, event.UserID())

	payload, ok := event.Payload().(*ArtifactCreatedPayload)
	assert.True(t, ok)
	assert.Equal(t, artifactID, payload.ArtifactID)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, projectName, payload.ProjectName)
	assert.Equal(t, slug, payload.Slug)
	assert.Equal(t, title, payload.Title)
	assert.Equal(t, artifactType, payload.Type)
	assert.Equal(t, body, payload.Body)
	assert.Equal(t, createdAt, payload.CreatedAt)
}

func TestArtifactUpdatedEvent(t *testing.T) {
	artifactID := "artifact123"
	userID := "user123"
	projectName := "test-project"
	slug := "test-slug"
	title := "Updated Artifact"
	artifactType := "general"
	body := "Updated artifact content"
	updatedAt := time.Now()

	event := NewArtifactUpdatedEvent(artifactID, userID, projectName, slug, title, artifactType, body, updatedAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypeArtifactUpdated, event.Type())
	assert.Equal(t, userID, event.UserID())

	payload, ok := event.Payload().(*ArtifactUpdatedPayload)
	assert.True(t, ok)
	assert.Equal(t, artifactID, payload.ArtifactID)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, projectName, payload.ProjectName)
	assert.Equal(t, slug, payload.Slug)
	assert.Equal(t, title, payload.Title)
	assert.Equal(t, artifactType, payload.Type)
	assert.Equal(t, body, payload.Body)
	assert.Equal(t, updatedAt, payload.UpdatedAt)
}

func TestArtifactCreatedPayload_IncludesBody(t *testing.T) {
	body := "Substantive artifact content for embedding"
	event := NewArtifactCreatedEvent("a1", "u1", "p1", "s1", "t1", "general", body, time.Now())

	payload, ok := event.Payload().(*ArtifactCreatedPayload)
	assert.True(t, ok)
	assert.Equal(t, body, payload.Body, "Body must be propagated so the AI service can embed real content")
}

func TestArtifactUpdatedPayload_IncludesBody(t *testing.T) {
	body := "Updated artifact body for re-embedding"
	event := NewArtifactUpdatedEvent("a1", "u1", "p1", "s1", "t1", "general", body, time.Now())

	payload, ok := event.Payload().(*ArtifactUpdatedPayload)
	assert.True(t, ok)
	assert.Equal(t, body, payload.Body, "Body must be propagated so the AI service can re-embed real content")
}

func TestBlueprintCreatedEvent(t *testing.T) {
	blueprintID := "bp123"
	userID := "user123"
	projectName := "test-project"
	slug := "test-spec"
	title := "Test Blueprint"
	blueprintType := "spec"
	body := "Blueprint content for embedding"
	createdAt := time.Now()

	event := NewBlueprintCreatedEvent(blueprintID, userID, projectName, slug, title, blueprintType, body, createdAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypeBlueprintCreated, event.Type())
	assert.Equal(t, userID, event.UserID())

	payload, ok := event.Payload().(*BlueprintCreatedPayload)
	assert.True(t, ok)
	assert.Equal(t, blueprintID, payload.BlueprintID)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, projectName, payload.ProjectName)
	assert.Equal(t, slug, payload.Slug)
	assert.Equal(t, title, payload.Title)
	assert.Equal(t, blueprintType, payload.Type)
	assert.Equal(t, body, payload.Body)
	assert.Equal(t, createdAt, payload.CreatedAt)
}

func TestBlueprintUpdatedEvent(t *testing.T) {
	blueprintID := "bp123"
	userID := "user123"
	projectName := "test-project"
	slug := "test-spec"
	title := "Updated Blueprint"
	blueprintType := "spec"
	body := "Updated blueprint content"
	updatedAt := time.Now()

	event := NewBlueprintUpdatedEvent(blueprintID, userID, projectName, slug, title, blueprintType, body, updatedAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypeBlueprintUpdated, event.Type())
	assert.Equal(t, userID, event.UserID())

	payload, ok := event.Payload().(*BlueprintUpdatedPayload)
	assert.True(t, ok)
	assert.Equal(t, blueprintID, payload.BlueprintID)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, projectName, payload.ProjectName)
	assert.Equal(t, slug, payload.Slug)
	assert.Equal(t, title, payload.Title)
	assert.Equal(t, blueprintType, payload.Type)
	assert.Equal(t, body, payload.Body)
	assert.Equal(t, updatedAt, payload.UpdatedAt)
}

func TestBlueprintCreatedPayload_IncludesBody(t *testing.T) {
	body := "Substantive blueprint content for embedding"
	event := NewBlueprintCreatedEvent("b1", "u1", "p1", "s1", "t1", "spec", body, time.Now())

	payload, ok := event.Payload().(*BlueprintCreatedPayload)
	assert.True(t, ok)
	assert.Equal(t, body, payload.Body, "Body must be propagated so the AI service can embed real content")
}

func TestBlueprintUpdatedPayload_IncludesBody(t *testing.T) {
	body := "Updated blueprint body for re-embedding"
	event := NewBlueprintUpdatedEvent("b1", "u1", "p1", "s1", "t1", "spec", body, time.Now())

	payload, ok := event.Payload().(*BlueprintUpdatedPayload)
	assert.True(t, ok)
	assert.Equal(t, body, payload.Body, "Body must be propagated so the AI service can re-embed real content")
}

func TestMemoryCreatedEvent(t *testing.T) {
	memoryID := "memory123"
	userID := "user123"
	projectName := "test-project"
	text := "Test memory"
	createdAt := time.Now()

	event := NewMemoryCreatedEvent(memoryID, userID, projectName, text, createdAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypeMemoryCreated, event.Type())
	assert.Equal(t, userID, event.UserID())

	payload, ok := event.Payload().(*MemoryCreatedPayload)
	assert.True(t, ok)
	assert.Equal(t, memoryID, payload.MemoryID)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, projectName, payload.ProjectName)
	assert.Equal(t, text, payload.Text)
	assert.Equal(t, createdAt, payload.CreatedAt)
}

func TestMemoryUpdatedEvent(t *testing.T) {
	memoryID := "memory123"
	userID := "user123"
	projectName := "test-project"
	text := "Updated memory"
	updatedAt := time.Now()

	event := NewMemoryUpdatedEvent(memoryID, userID, projectName, text, updatedAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypeMemoryUpdated, event.Type())
	assert.Equal(t, userID, event.UserID())

	payload, ok := event.Payload().(*MemoryUpdatedPayload)
	assert.True(t, ok)
	assert.Equal(t, memoryID, payload.MemoryID)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, projectName, payload.ProjectName)
	assert.Equal(t, text, payload.Text)
	assert.Equal(t, updatedAt, payload.UpdatedAt)
}

func TestFeedItemCreatedEvent_IncludesBody(t *testing.T) {
	itemID := "item-1"
	userID := "poster-1"
	teamID := "team-1"
	feedID := "feed-1"
	title := "Refactored the auth flow"
	content := "Substantive feed item content for embedding"
	excerpt := "Substantive feed item content"
	postedAt := time.Now()

	event := NewFeedItemCreatedEvent(itemID, userID, teamID, feedID, title, content, excerpt, postedAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypeFeedItemCreated, event.Type())
	assert.Equal(t, userID, event.UserID(), "event must be keyed by the posting user")

	payload, ok := event.Payload().(*FeedItemCreatedPayload)
	assert.True(t, ok)
	assert.Equal(t, itemID, payload.ItemID)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, teamID, payload.TeamID)
	assert.Equal(t, feedID, payload.FeedID)
	assert.Equal(t, title, payload.Title)
	assert.Equal(t, content, payload.Content, "Content must be propagated so the AI service can embed real content")
	assert.Equal(t, excerpt, payload.Excerpt)
	assert.Equal(t, postedAt, payload.PostedAt)
}

func TestFeedItemReplyCreatedEvent(t *testing.T) {
	replyID := "reply-1"
	userID := "poster-2"
	teamID := "team-1"
	feedItemID := "item-1"
	content := "A substantive reply that should be embedded"
	postedAt := time.Now()

	event := NewFeedItemReplyCreatedEvent(replyID, userID, teamID, feedItemID, content, postedAt)

	assert.NotNil(t, event)
	assert.Equal(t, EventTypeFeedItemReplyCreated, event.Type())
	assert.Equal(t, userID, event.UserID(), "event must be keyed by the posting user")

	payload, ok := event.Payload().(*FeedItemReplyCreatedPayload)
	assert.True(t, ok)
	assert.Equal(t, replyID, payload.ReplyID)
	assert.Equal(t, userID, payload.UserID)
	assert.Equal(t, teamID, payload.TeamID)
	assert.Equal(t, feedItemID, payload.FeedItemID)
	assert.Equal(t, content, payload.Content, "Content must be propagated so the AI service can embed real content")
	assert.Equal(t, postedAt, payload.PostedAt)
}

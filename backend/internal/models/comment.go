package models

import "time"

// Comment resource types. A comment belongs to exactly one resource of one of
// these polymorphic types (issue #273); nothing else is commentable in v1.
const (
	CommentResourceTypeArtifact  = "artifact"
	CommentResourceTypeMemory    = "memory"
	CommentResourceTypePrompt    = "prompt"
	CommentResourceTypeBlueprint = "blueprint"
)

// CommentContentMaxLen bounds a comment body, mirroring the feed-reply limit.
const CommentContentMaxLen = 10000

// IsValidCommentResourceType reports whether t is one of the four commentable
// resource types.
func IsValidCommentResourceType(t string) bool {
	switch t {
	case CommentResourceTypeArtifact, CommentResourceTypeMemory,
		CommentResourceTypePrompt, CommentResourceTypeBlueprint:
		return true
	default:
		return false
	}
}

// Comment is a team-visible annotation on a resource (artifact, memory, prompt,
// or blueprint). It is keyed by the polymorphic (ResourceType, ResourceID)
// pair; ResourceID has no foreign key (cf. attachments), so a resource's
// comments are cleaned up in app code when the resource is deleted. A comment
// whose UpdatedAt is later than its CreatedAt has been edited.
type Comment struct {
	ID           string    `json:"id"            db:"id"`
	TeamID       string    `json:"team_id"       db:"team_id"`
	ResourceType string    `json:"resource_type" db:"resource_type"`
	ResourceID   string    `json:"resource_id"   db:"resource_id"`
	UserID       string    `json:"user_id"       db:"user_id"`
	Content      string    `json:"content"       db:"content"`
	CreatedAt    time.Time `json:"created_at"    db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"    db:"updated_at"`
}

// CreateCommentRequest is the request body for creating a comment.
type CreateCommentRequest struct {
	ResourceType string `json:"resource_type" validate:"required,oneof=artifact memory prompt blueprint"`
	ResourceID   string `json:"resource_id"   validate:"required,uuid"`
	Content      string `json:"content"       validate:"required,min=1,max=10000"`
}

// UpdateCommentRequest is the request body for editing a comment's content.
type UpdateCommentRequest struct {
	Content string `json:"content" validate:"required,min=1,max=10000"`
}

// CommentListResponse is the paginated response for listing a resource's comments.
type CommentListResponse struct {
	Comments   JSONArray[Comment] `json:"comments"`
	TotalCount int                `json:"total_count"`
	Page       int                `json:"page"`
	PerPage    int                `json:"per_page"`
	TotalPages int                `json:"total_pages"`
}

// CommentActivity is a comment enriched, at query time, with the resolved
// title and link fields of its resource for the homepage "recent comments"
// card. Slug is nil for memories (they have no slug); ProjectID is present for
// every resource type. Rows whose resource has vanished are omitted by the
// query, so these fields are always populated for a returned row.
type CommentActivity struct {
	Comment
	ResourceTitle string  `json:"resource_title"`
	ProjectID     *string `json:"project_id,omitempty"`
	Slug          *string `json:"slug,omitempty"`
}
